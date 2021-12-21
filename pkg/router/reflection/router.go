package reflection

import (
	"fmt"
	"net/http"

	"github.com/zhamlin/chi-openapi/pkg/container"
	"github.com/zhamlin/chi-openapi/pkg/openapi"
	"github.com/zhamlin/chi-openapi/pkg/openapi/operations"
	"github.com/zhamlin/chi-openapi/pkg/router"
)

type Middleware func(next http.Handler) http.Handler

type Hooks struct {
	// BeforeOptions is called before the options are passed into the operation.
	// The returned operation will be used in place of the original
	BeforeOptions func(method, pattern string, handler interface{}, options []operations.Option) (operations.Options, error)

	// BeforeMiddleware is called before the handler is wrapped with the middleware.
	// The returned middleware will be used in place of the original
	BeforeMiddleware func(method, pattern string, handler interface{}, c *ReflectRouter, middleware []Middleware) ([]Middleware, error)
}

type ReflectRouter struct {
	*router.Router
	handleFn RequestHandler
	c        *container.Container
	hooks    Hooks
}

// NewRouter returns a wrapped chi router
func NewRouter() *ReflectRouter {
	return &ReflectRouter{
		Router: router.NewRouter(),
		c:      container.NewContainer(),
	}
}

func (r *ReflectRouter) SetParent(parent *ReflectRouter) *ReflectRouter {
	if parent == nil {
		return r
	}
	r.c = parent.c
	r.handleFn = parent.handleFn
	r.OpenAPI.Components = parent.OpenAPI.Components
	r.hooks = parent.hooks
	r.OpenAPI.Info = parent.OpenAPI.Info
	return r
}

func (r *ReflectRouter) WithHooks(h Hooks) *ReflectRouter {
	r.hooks = h
	return r
}

func (r *ReflectRouter) WithContainer(c *container.Container) *ReflectRouter {
	r.c = c
	return r
}

func (r *ReflectRouter) WithHandler(fn RequestHandler) *ReflectRouter {
	r.handleFn = fn
	return r
}

func (r *ReflectRouter) WithInfo(info openapi.Info) *ReflectRouter {
	r.Router = r.Router.WithInfo(info)
	return r
}

func (r *ReflectRouter) Provide(fptr interface{}) error {
	return r.c.Provide(fptr)
}

// UseRouter copies over the routes and swagger info from the other router.
func (r *ReflectRouter) UseRouter(other *ReflectRouter) *ReflectRouter {
	r.OpenAPI.Info = other.OpenAPI.Info
	r.Mount("/", other)
	return r
}

// Route mounts a sub-Router along a `pattern`` string.
func (r *ReflectRouter) Route(pattern string, fn func(*ReflectRouter)) {
	subRouter := NewRouter().SetParent(r).WithHandler(r.handleFn)
	if fn != nil {
		fn(subRouter)
	}
	r.Mount(pattern, subRouter)
}

// Mount attaches another http.Handler along ./pattern/*
func (r *ReflectRouter) Mount(pattern string, handler http.Handler) {
	switch obj := handler.(type) {
	case *ReflectRouter:
		r.Router.Mount(pattern, obj.Router)
	default:
		r.Router.Mount(pattern, handler)
	}
}

// MethodFunc adds routes for `pattern` that matches the `method` HTTP method.
// Middleware are executed from first to last
func (r *ReflectRouter) MethodFunc(method, pattern string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	p := func(err error) {
		panic(fmt.Sprintf("router [%s %s]: cannot create automatic handler: %v", method, pattern, err))
	}
	if h := r.hooks.BeforeOptions; h != nil {
		opts, err := h(method, pattern, handler, options)
		if err != nil {
			p(err)
		}
		options = opts
	}

	o := operations.Operation{}
	for _, option := range options {
		// don't modify the operation here, just check for errors and update schemas
		_, err := option(&r.OpenAPI, o)
		if err != nil {
			p(err)
		}
	}

	fn, err := HandlerFromFn(handler, r.handleFn, r.Components(), r.c)
	if err != nil {
		p(err)
	}

	if h := r.hooks.BeforeMiddleware; h != nil {
		middle, err := h(method, pattern, handler, r, middleware)
		if err != nil {
			p(err)
		}
		middleware = middle
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

	r.Router.MethodFunc(method, pattern, fn, options)
}

// FIXME: remove this?
func (r *ReflectRouter) Container() *container.Container {
	return r.c
}

func (r *ReflectRouter) Get(pattern string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodGet, pattern, handler, options, middleware...)
}

func (r *ReflectRouter) Options(pattern string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodOptions, pattern, handler, options, middleware...)
}

func (r *ReflectRouter) Connect(pattern string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodConnect, pattern, handler, options, middleware...)
}

func (r *ReflectRouter) Trace(pattern string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodTrace, pattern, handler, options, middleware...)
}

func (r *ReflectRouter) Post(pattern string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodPost, pattern, handler, options, middleware...)
}

func (r *ReflectRouter) Put(pattern string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodPut, pattern, handler, options, middleware...)
}

func (r *ReflectRouter) Patch(pattern string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodPatch, pattern, handler, options, middleware...)
}

func (r *ReflectRouter) Delete(pattern string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodDelete, pattern, handler, options, middleware...)
}

func (r *ReflectRouter) Head(pattern string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodHead, pattern, handler, options, middleware...)
}
