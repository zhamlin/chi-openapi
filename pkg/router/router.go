package router

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"chi-openapi/pkg/openapi"
	"chi-openapi/pkg/openapi/operations"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi"
)

// NewRouter returns a wrapped chi router
func NewRouter() *Router {
	return &Router{
		mux: chi.NewRouter(),
		swagger: &openapi3.Swagger{
			Info: &openapi3.Info{
				Version: "0.0.1",
				Title:   "Title",
			},
			OpenAPI: "3.0.0",
			Paths:   openapi3.Paths{},
			Components: openapi3.Components{
				Schemas:    openapi.Schemas{},
				Parameters: openapi.Parameters{},
			},
		},
	}
}

func NewRouterWithInfo(info openapi.Info) *Router {
	r := NewRouter()
	apiInfo := openapi3.Info(info)
	r.swagger.Info = &apiInfo
	return r
}

type Router struct {
	mux        chi.Router
	swagger    *openapi3.Swagger
	prefixPath string
}

// Use appends one or more middlewares onto the Router stack.
func (r *Router) Use(middlewares ...func(http.Handler) http.Handler) {
	r.mux.Use(middlewares...)
}

// With adds inline middlewares for an endpoint handler.
func (r *Router) With(middlewares ...func(http.Handler) http.Handler) *Router {
	newRouter := NewRouter()
	newRouter.swagger = r.swagger
	newRouter.mux = r.mux.With(middlewares...)
	return newRouter
}

// TODO: implement group function, regarding middlewares
// Group adds a new inline-Router along the current routing
// path, with a fresh middleware stack for the inline-Router.
// func (r *Router) Group(path, name, description string) {
// }

// Route mounts a sub-Router along a `pattern`` string.
func (router *Router) Route(pattern string, fn func(*Router)) {
	subRouter := NewRouter()
	if fn != nil {
		fn(subRouter)
	}
	router.Mount(pattern, subRouter)
}

// Mount attaches another http.Handler along ./pattern/*
func (router *Router) Mount(path string, handler http.Handler) {
	switch obj := handler.(type) {
	case *Router:
		for name, item := range obj.swagger.Paths {
			router.swagger.Paths[path+strings.TrimRight(name, "/")] = item
		}
		for name, item := range obj.swagger.Components.Schemas {
			router.swagger.Components.Schemas[name] = item
		}
	}
	router.mux.Mount(path, handler)
}

func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	router.mux.ServeHTTP(w, r)
}

// Method adds routes for `pattern` that matches the `method` HTTP method.
func (r *Router) Method(method, path string, handler http.Handler, options []operations.Option) {
	r.MethodFunc(method, path, handler.ServeHTTP, options)
}

// MethodFunc adds routes for `pattern` that matches the `method` HTTP method.
func (r *Router) MethodFunc(method, path string, handler http.HandlerFunc, options []operations.Option) {
	o := operations.Operation{}
	for _, option := range options {
		o = option(r.swagger, o)
	}

	path = r.prefixPath + path
	pathItem, exists := r.swagger.Paths[path]
	if !exists {
		pathItem = &openapi3.PathItem{}
	}
	switch method {
	case http.MethodGet:
		pathItem.Get = &o.Operation
	case http.MethodPut:
		pathItem.Put = &o.Operation
	case http.MethodPost:
		pathItem.Post = &o.Operation
	case http.MethodDelete:
		pathItem.Delete = &o.Operation
	case http.MethodPatch:
		pathItem.Patch = &o.Operation
	case http.MethodHead:
		pathItem.Head = &o.Operation
	case http.MethodTrace:
		pathItem.Trace = &o.Operation
	case http.MethodConnect:
		pathItem.Connect = &o.Operation
	case http.MethodOptions:
		pathItem.Options = &o.Operation
	}
	r.mux.MethodFunc(method, path, handler)
	r.swagger.Paths[path] = pathItem
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
	b, err := json.MarshalIndent(&r.swagger, "", " ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *Router) ValidateSpec() error {
	return r.swagger.Validate(context.Background())
}

// ServeSpec generates and validates the routers openapi spec.
// A route with the path supplied will return the spec on GET requests.
func (r *Router) ServeSpec(path string) error {
	if err := r.ValidateSpec(); err != nil {
		return err
	}

	spec, err := r.GenerateSpec()
	if err != nil {
		return err
	}

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(spec))
	}, operations.Options{
		operations.JSONResponse(200, "openapi defintion", nil),
	})
	return nil
}

// FilterRouter returns a router used for verifying middlewares
func (r *Router) FilterRouter() (*openapi3filter.Router, error) {
	router := openapi3filter.NewRouter()
	if err := router.AddSwagger(r.swagger); err != nil {
		return router, err
	}
	return router, nil
}

// UseRouter copies over the routes and swagger info from the other router.
func (r *Router) UseRouter(other *Router) *Router {
	r.swagger.Info = other.swagger.Info
	r.Mount("/", other)
	return r
}
