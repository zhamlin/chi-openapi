package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"reflect"
	"runtime"

	"github.com/zhamlin/chi-openapi/pkg/openapi"
	"github.com/zhamlin/chi-openapi/pkg/openapi/operations"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers"
	gorillaRouter "github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/go-chi/chi/v5"
)

// NewRouter returns a wrapped chi router
func NewRouter() *Router {
	return &Router{
		Mux: chi.NewRouter(),
		OpenAPI: openapi.OpenAPI{
			RegisteredTypes: openapi.RegisteredTypes{},
			T: &openapi3.T{
				Info: &openapi3.Info{
					Version: "0.0.1",
					Title:   "Title",
				},
				Servers: openapi3.Servers{},
				OpenAPI: "3.0.0",
				Paths:   openapi3.Paths{},
				Components: openapi3.Components{
					Schemas:         openapi3.Schemas{},
					Parameters:      openapi3.ParametersMap{},
					Responses:       map[string]*openapi3.ResponseRef{},
					SecuritySchemes: map[string]*openapi3.SecuritySchemeRef{},
				},
			},
		},
		defaultResponses: map[string]*openapi3.ResponseRef{},
	}
}

func (r *Router) WithInfo(info openapi.Info) *Router {
	apiInfo := openapi3.Info(info)
	r.OpenAPI.Info = &apiInfo
	return r
}

// SecuritySchema represents an openapi3 security scheme
type SecuritySchema struct {
	Name                         string
	SchemeName, Type, Scheme, In string
}

func (r *Router) WithSecurity(security SecuritySchema) *Router {
	schema := openapi3.NewSecurityScheme()
	if security.SchemeName != "" {
		schema = schema.WithName(security.SchemeName)
	}
	if security.Type != "" {
		schema = schema.WithType(security.Type)
	}
	if security.In != "" {
		schema = schema.WithIn(security.In)
	}
	if security.Scheme != "" {
		schema = schema.WithScheme(security.Scheme)
	}
	r.OpenAPI.Components.SecuritySchemes[security.Name] = &openapi3.SecuritySchemeRef{Value: schema}
	return r
}

func (r *Router) SetGlobalSecurity(name string) *Router {
	r.OpenAPI.Security.With(openapi3.
		NewSecurityRequirement().
		Authenticate(name))
	return r
}

// Router is a small wrapper over a chi.Router to help generate an openapi spec
type Router struct {
	Mux     chi.Router
	OpenAPI openapi.OpenAPI

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
	newRouter.OpenAPI = r.OpenAPI
	newRouter.Mux = r.Mux.With(middlewares...)
	return newRouter
}

// TODO: implement group function, regarding middlewares
// Group adds a new inline-Router along the current routing
// path, with a fresh middleware stack for the inline-Router.
// func (r *Router) Group(path, name, description string) {
// }

// Route mounts a sub-Router along a `pattern` string.
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
func (r *Router) Mount(pattern string, handler http.Handler) {
	switch obj := handler.(type) {
	case *Router:
		for name, item := range obj.OpenAPI.Paths {
			for _, op := range item.Operations() {
				r.setDefaultResp(op)
			}

			r.OpenAPI.Paths[path.Join(pattern, name)] = item
		}
		for name, item := range obj.OpenAPI.Components.Schemas {
			if _, has := r.OpenAPI.Components.Schemas[name]; !has {
				r.OpenAPI.Components.Schemas[name] = item
			}
		}
		for name, item := range obj.OpenAPI.Components.Responses {
			if _, has := r.OpenAPI.Components.Responses[name]; !has {
				r.OpenAPI.Components.Responses[name] = item
			}
		}
	}
	r.Mux.Mount(pattern, handler)
}

func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	router.Mux.ServeHTTP(w, r)
}

// Method adds routes for `pattern` that matches the `method` HTTP method.
func (r *Router) Method(method, pattern string, handler http.Handler, options []operations.Option) {
	r.MethodFunc(method, pattern, handler.ServeHTTP, options)
}

func getFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

// MethodFunc adds routes for `pattern` that matches the `method` HTTP method.
func (r *Router) MethodFunc(method, pattern string, handler http.HandlerFunc, options []operations.Option) {
	pattern = r.prefixPath + pattern

	o := operations.Operation{}
	var err error
	for _, option := range options {
		o, err = option(&r.OpenAPI, o)
		if err != nil {
			panic(fmt.Sprintf("router [%s %s]: cannot create handler: %v", method, pattern, err))
		}
	}

	if o.Operation.Responses == nil {
		panic(fmt.Sprintf("router [%s %s]: route does not have any responses defined", method, pattern))
	}
	r.setDefaultResp(&o.Operation)

	r.Mux.MethodFunc(method, pattern, handler)
	r.OpenAPI.AddOperation(pattern, method, &o.Operation)
}

func (r *Router) Get(pattern string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodGet, pattern, handler, options)
}

func (r *Router) Options(pattern string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodOptions, pattern, handler, options)
}

func (r *Router) Connect(pattern string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodConnect, pattern, handler, options)
}

func (r *Router) Trace(pattern string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodTrace, pattern, handler, options)
}

func (r *Router) Post(pattern string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodPost, pattern, handler, options)
}

func (r *Router) Put(pattern string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodPut, pattern, handler, options)
}

func (r *Router) Patch(pattern string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodPatch, pattern, handler, options)
}

func (r *Router) Delete(pattern string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodDelete, pattern, handler, options)
}

func (r *Router) Head(pattern string, handler http.HandlerFunc, options []operations.Option) {
	r.MethodFunc(http.MethodHead, pattern, handler, options)
}

func (r *Router) GenerateSpec() (string, error) {
	b, err := json.MarshalIndent(r.OpenAPI.T, "", " ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *Router) ValidateSpec() error {
	return r.OpenAPI.Validate(context.Background())
}

// FilterRouter returns a router used for verifying middlewares
func (r *Router) FilterRouter() (routers.Router, error) {
	router, err := gorillaRouter.NewRouter(r.OpenAPI.T)
	if err != nil {
		return nil, err
	}
	return router, nil
}

// UseRouter copies over the routes and swagger info from the other router.
func (r *Router) UseRouter(other *Router) *Router {
	r.OpenAPI.Info = other.OpenAPI.Info
	r.Mount("/", other)
	return r
}

func (r *Router) Components() openapi.Components {
	return openapi.Components{
		Schemas:         openapi.Schemas(r.OpenAPI.Components.Schemas),
		RegisteredTypes: r.OpenAPI.RegisteredTypes,
		Parameters:      map[reflect.Type]openapi3.Parameters{},
	}
}

func (r *Router) setStatusDefault(status string, description string, obj interface{}) {
	resp := openapi3.NewResponse().WithDescription(description)
	if obj != nil {
		schema := openapi.SchemaFromObj(obj, openapi.Schemas(r.OpenAPI.Components.Schemas), r.OpenAPI.RegisteredTypes)
		resp = resp.WithContent(openapi3.NewContentWithJSONSchemaRef(schema))
	}

	r.defaultResponses[status] = &openapi3.ResponseRef{Value: resp}
	r.OpenAPI.Components.Responses[status] = r.defaultResponses[status]
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

// RegisterType registers the type as an inline schema
func (r *Router) RegisterType(obj interface{}, schema *openapi3.Schema) {
	typ := reflect.TypeOf(obj)
	r.OpenAPI.RegisteredTypes[typ] = openapi.TypeOption{Schema: schema}
}

// RegisterTypeAsComponent registers the objects type as a reference to
// the schema, which is inserted into the schema.components with the supplied name
func (r *Router) RegisterTypeAsComponent(obj interface{}, name string, schema *openapi3.Schema) {
	typ := reflect.TypeOf(obj)

	r.OpenAPI.RegisteredTypes[typ] = openapi.TypeOption{
		SchemaRef: openapi3.NewSchemaRef(openapi.ComponentSchemasPath+name, schema),
	}
	r.OpenAPI.Components.Schemas[name] = openapi3.NewSchemaRef("", schema)
}

// RegisterTypeAsRef registers the objects type as a reference to
// the schema located in schema.components with the supplied name
func (r *Router) RegisterTypeAsRef(obj interface{}, name string) error {
	schema, has := r.OpenAPI.Components.Schemas[name]
	if !has {
		return fmt.Errorf("%s not found in the component schemas", name)
	}

	typ := reflect.TypeOf(obj)
	r.OpenAPI.RegisteredTypes[typ] = openapi.TypeOption{
		SchemaRef: openapi3.NewSchemaRef(openapi.ComponentSchemasPath+name, schema.Value),
	}
	return nil
}
