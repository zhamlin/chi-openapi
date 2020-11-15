package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"reflect"

	"chi-openapi/pkg/openapi"
	"chi-openapi/pkg/openapi/operations"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi"
)

// NewRouter returns a wrapped chi router
func NewRouter() *Router {
	return &Router{
		Mux: chi.NewRouter(),
		Swagger: &openapi3.Swagger{
			Info: &openapi3.Info{
				Version: "0.0.1",
				Title:   "Title",
			},
			Servers: openapi3.Servers{},
			OpenAPI: "3.0.0",
			Paths:   openapi3.Paths{},
			Components: openapi3.Components{
				Schemas:    openapi.Schemas{},
				Parameters: openapi.Parameters{},
				Responses:  map[string]*openapi3.ResponseRef{},
			},
		},
		defaultResponses: map[string]*openapi3.ResponseRef{},
	}
}

func NewRouterWithInfo(info openapi.Info) *Router {
	r := NewRouter()
	apiInfo := openapi3.Info(info)
	r.Swagger.Info = &apiInfo
	return r
}

// Router is a small wrapper over a chi.Router to help generate an openapi spec
type Router struct {
	Mux     chi.Router
	Swagger *openapi3.Swagger

	prefixPath       string
	defaultResponses map[string]*openapi3.ResponseRef
}

// Use appends one or more middlewares onto the Router stack.
func (r *Router) Use(middlewares ...func(http.Handler) http.Handler) {
	r.Mux.Use(middlewares...)
}

// With adds inline middlewares for an endpoint handler.
func (r *Router) With(middlewares ...func(http.Handler) http.Handler) *Router {
	newRouter := NewRouter()
	newRouter.Swagger = r.Swagger
	newRouter.Mux = r.Mux.With(middlewares...)
	return newRouter
}

// TODO: implement group function, regarding middlewares
// Group adds a new inline-Router along the current routing
// path, with a fresh middleware stack for the inline-Router.
// func (r *Router) Group(path, name, description string) {
// }

// Route mounts a sub-Router along a `pattern`` string.
func (r *Router) Route(pattern string, fn func(*Router)) {
	subRouter := NewRouter()
	if fn != nil {
		fn(subRouter)
	}
	r.Mount(pattern, subRouter)
}

func (r *Router) setDefaultResp(o *openapi3.Operation) {
	for name, resp := range r.defaultResponses {
		// don't override an already set default response
		if _, has := o.Responses[name]; has {
			continue
		}

		o.Responses[name] = &openapi3.ResponseRef{
			Ref:   "#/components/responses/" + name,
			Value: resp.Value,
		}
	}
}

// Mount attaches another http.Handler along ./pattern/*
func (r *Router) Mount(route string, handler http.Handler) {
	switch obj := handler.(type) {
	case *Router:
		for name, item := range obj.Swagger.Paths {
			for _, op := range item.Operations() {
				r.setDefaultResp(op)
			}

			r.Swagger.Paths[path.Join(route, name)] = item
		}
		for name, item := range obj.Swagger.Components.Schemas {
			r.Swagger.Components.Schemas[name] = item
		}
		for name, item := range obj.Swagger.Components.Responses {
			r.Swagger.Components.Responses[name] = item
		}
	}
	r.Mux.Mount(route, handler)
}

func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	router.Mux.ServeHTTP(w, r)
}

// Method adds routes for `pattern` that matches the `method` HTTP method.
func (r *Router) Method(method, path string, handler http.Handler, options []operations.Option) {
	r.MethodFunc(method, path, handler.ServeHTTP, options)
}

// MethodFunc adds routes for `pattern` that matches the `method` HTTP method.
func (r *Router) MethodFunc(method, path string, handler http.HandlerFunc, options []operations.Option) {
	o := operations.Operation{}
	for _, option := range options {
		o = option(r.Swagger, o)
	}

	r.setDefaultResp(&o.Operation)

	path = r.prefixPath + path
	r.Mux.MethodFunc(method, path, handler)
	r.Swagger.AddOperation(path, method, &o.Operation)
}

func (r *Router) Get(path string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodGet, path, handler, options)
}

func (r *Router) Options(path string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodOptions, path, handler, options)
}

func (r *Router) Connect(path string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodConnect, path, handler, options)
}

func (r *Router) Trace(path string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodTrace, path, handler, options)
}

func (r *Router) Post(path string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodPost, path, handler, options)
}

func (r *Router) Put(path string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodPut, path, handler, options)
}

func (r *Router) Patch(path string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodPatch, path, handler, options)
}

func (r *Router) Delete(path string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodDelete, path, handler, options)
}

func (r *Router) Head(path string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodHead, path, handler, options)
}

func (r *Router) GenerateSpec() (string, error) {
	b, err := json.MarshalIndent(&r.Swagger, "", " ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *Router) ValidateSpec() error {
	return r.Swagger.Validate(context.Background())
}

// FilterRouter returns a router used for verifying middlewares
func (r *Router) FilterRouter() (*openapi3filter.Router, error) {
	router := openapi3filter.NewRouter()
	if err := router.AddSwagger(r.Swagger); err != nil {
		return router, err
	}
	return router, nil
}

// UseRouter copies over the routes and swagger info from the other router.
func (r *Router) UseRouter(other *Router) *Router {
	r.Swagger.Info = other.Swagger.Info
	r.Mount("/", other)
	return r
}

func (r *Router) Components() openapi.Components {
	return openapi.Components{
		Schemas:    r.Swagger.Components.Schemas,
		Parameters: map[reflect.Type]openapi3.Parameters{},
	}
}

func (r *Router) setStatusDefault(status string, description string, obj interface{}) {
	resp := openapi3.NewResponse().WithDescription(description)
	if obj != nil {
		schema := openapi.SchemaFromObj(obj, r.Swagger.Components.Schemas)
		resp = resp.WithContent(openapi3.NewContentWithJSONSchemaRef(schema))
	}

	r.defaultResponses[status] = &openapi3.ResponseRef{Value: resp}
	r.Swagger.Components.Responses[status] = r.defaultResponses[status]
}

// SetStatusDefault will set the statusCode for all routes to the supplied object.
func (r *Router) SetStatusDefault(status int, description string, obj interface{}) {
	r.setStatusDefault(fmt.Sprintf("%d", status), description, obj)
}

// SetDefaultJSON will set the default response for all routes unless overridden
// at the operation level
func (r *Router) SetDefaultJSON(description string, obj interface{}) {
	r.setStatusDefault("default", description, obj)
}
