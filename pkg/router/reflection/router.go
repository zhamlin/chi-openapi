package reflection

import (
	"chi-openapi/pkg/openapi"
	"chi-openapi/pkg/openapi/operations"
	"chi-openapi/pkg/router"
	"fmt"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
)

type middleware func(next http.Handler) http.Handler

type ReflectRouter struct {
	*router.Router
	handleFns RequestHandleFns
	c         *container
}

// NewRouter returns a wrapped chi router
func NewRouter() *ReflectRouter {
	return &ReflectRouter{
		Router: router.NewRouter(),
		c:      NewContainer(),
	}
}

func (r *ReflectRouter) SetParent(parent *ReflectRouter) *ReflectRouter {
	if parent == nil {
		return r
	}
	r.c = parent.c
	r.handleFns = parent.handleFns
	r.Swagger.Components = parent.Swagger.Components
	return r
}

func (r *ReflectRouter) WithHandlers(handleFns RequestHandleFns) *ReflectRouter {
	r.handleFns = handleFns
	return r
}

func (r *ReflectRouter) WithInfo(info openapi.Info) *ReflectRouter {
	apiInfo := openapi3.Info(info)
	r.Swagger.Info = &apiInfo
	return r
}

func (r *ReflectRouter) Provide(fptr interface{}) error {
	return r.c.Provide(fptr)
}

// UseRouter copies over the routes and swagger info from the other router.
func (r *ReflectRouter) UseRouter(other *ReflectRouter) *ReflectRouter {
	r.Swagger.Info = other.Swagger.Info
	r.Mount("/", other)
	return r
}

// Route mounts a sub-Router along a `pattern`` string.
func (r *ReflectRouter) Route(pattern string, fn func(*ReflectRouter)) {
	subRouter := NewRouter().WithHandlers(r.handleFns)
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
func (r *ReflectRouter) MethodFunc(method, path string, handler interface{}, options []operations.Option, middleware ...middleware) {
	o := operations.Operation{}
	for _, option := range options {
		option(r.Swagger, o)
	}

	fn, err := HandlerFromFn(handler, r.handleFns, r.Components(), r.c)
	if err != nil {
		panic(fmt.Sprintf("router [%s %s]: cannot create automatic handler: %v", method, path, err))
	}

	if len(middleware) > 0 {
		middlewareFn := func(next http.Handler) http.Handler {
			// apply middleware backwards to apply in the correct order
			for i := len(middleware) - 1; i >= 0; i-- {
				next = middleware[i](next)
			}
			return next
		}
		fn = middlewareFn(fn).ServeHTTP
	}

	r.Router.MethodFunc(method, path, fn, options)
}

func (r *ReflectRouter) Get(path string, handler interface{}, options []operations.Option, middleware ...middleware) {
	r.MethodFunc(http.MethodGet, path, handler, options, middleware...)
}

func (r *ReflectRouter) Options(path string, handler interface{}, options []operations.Option, middleware ...middleware) {
	r.MethodFunc(http.MethodOptions, path, handler, options, middleware...)
}

func (r *ReflectRouter) Connect(path string, handler interface{}, options []operations.Option, middleware ...middleware) {
	r.MethodFunc(http.MethodConnect, path, handler, options, middleware...)
}

func (r *ReflectRouter) Trace(path string, handler interface{}, options []operations.Option, middleware ...middleware) {
	r.MethodFunc(http.MethodTrace, path, handler, options, middleware...)
}

func (r *ReflectRouter) Post(path string, handler interface{}, options []operations.Option, middleware ...middleware) {
	r.MethodFunc(http.MethodPost, path, handler, options, middleware...)
}

func (r *ReflectRouter) Put(path string, handler interface{}, options []operations.Option, middleware ...middleware) {
	r.MethodFunc(http.MethodPut, path, handler, options, middleware...)
}

func (r *ReflectRouter) Patch(path string, handler interface{}, options []operations.Option, middleware ...middleware) {
	r.MethodFunc(http.MethodPatch, path, handler, options, middleware...)
}

func (r *ReflectRouter) Delete(path string, handler interface{}, options []operations.Option, middleware ...middleware) {
	r.MethodFunc(http.MethodDelete, path, handler, options, middleware...)
}

func (r *ReflectRouter) Head(path string, handler interface{}, options []operations.Option, middleware ...middleware) {
	r.MethodFunc(http.MethodHead, path, handler, options, middleware...)
}
