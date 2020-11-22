package reflection

import (
	"chi-openapi/pkg/openapi"
	"chi-openapi/pkg/openapi/operations"
	"chi-openapi/pkg/router"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
)

type ReflectRouter struct {
	*router.Router
	handleFns RequestHandleFns
}

// NewRouter returns a wrapped chi router
func NewRouter(handleFns RequestHandleFns) *ReflectRouter {
	return &ReflectRouter{
		router.NewRouter(),
		handleFns,
	}
}

func NewRouterWithInfo(info openapi.Info, handleFns RequestHandleFns) *ReflectRouter {
	r := NewRouter(handleFns)
	apiInfo := openapi3.Info(info)
	r.Swagger.Info = &apiInfo
	return r
}

// Route mounts a sub-Router along a `pattern`` string.
func (r *ReflectRouter) Route(pattern string, fn func(*ReflectRouter)) {
	subRouter := NewRouter(r.handleFns)
	if fn != nil {
		fn(subRouter)
	}
	r.Mount(pattern, subRouter)
}

// Mount attaches another http.Handler along ./pattern/*
func (r *ReflectRouter) Mount(path string, handler http.Handler) {
	switch obj := handler.(type) {
	case *ReflectRouter:
		r.Router.Mount(path, obj.Router)
	default:
		r.Router.Mount(path, handler)
	}
}

// MethodFunc adds routes for `pattern` that matches the `method` HTTP method.
func (r *ReflectRouter) MethodFunc(method, path string, handler interface{}, options []operations.Option) {
	o := operations.Operation{}
	for _, option := range options {
		option(r.Swagger, o)
	}

	fn, err := HandlerFromFnDefault(handler, r.handleFns, r.Components())
	if err != nil {
		panic(err)
	}
	r.Router.MethodFunc(method, path, fn, options)
}

func (r *ReflectRouter) Get(path string, handler interface{}, options []operations.Option) {
	r.MethodFunc(http.MethodGet, path, handler, options)
}

func (r *ReflectRouter) Options(path string, handler interface{}, options []operations.Option) {
	r.MethodFunc(http.MethodOptions, path, handler, options)
}

func (r *ReflectRouter) Connect(path string, handler interface{}, options []operations.Option) {
	r.MethodFunc(http.MethodConnect, path, handler, options)
}

func (r *ReflectRouter) Trace(path string, handler interface{}, options []operations.Option) {
	r.MethodFunc(http.MethodTrace, path, handler, options)
}

func (r *ReflectRouter) Post(path string, handler interface{}, options []operations.Option) {
	r.MethodFunc(http.MethodPost, path, handler, options)
}

func (r *ReflectRouter) Put(path string, handler interface{}, options []operations.Option) {
	r.MethodFunc(http.MethodPut, path, handler, options)
}

func (r *ReflectRouter) Patch(path string, handler interface{}, options []operations.Option) {
	r.MethodFunc(http.MethodPatch, path, handler, options)
}

func (r *ReflectRouter) Delete(path string, handler interface{}, options []operations.Option) {
	r.MethodFunc(http.MethodDelete, path, handler, options)
}

func (r *ReflectRouter) Head(path string, handler interface{}, options []operations.Option) {
	r.MethodFunc(http.MethodHead, path, handler, options)
}

// UseRouter copies over the routes and swagger info from the other router.
func (r *ReflectRouter) UseRouter(other *ReflectRouter) *ReflectRouter {
	r.Swagger.Info = other.Swagger.Info
	r.Mount("/", other)
	return r
}