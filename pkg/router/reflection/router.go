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
	// The returned operations will be used in place of the original
	BeforeOptions func(method, path string, handler interface{}, options []operations.Option) (operations.Options, error)

	// BeforeMiddleware is called before the handler is wrapped with the middleware.
	// The returned middlewares will be used in place of the original
	BeforeMiddleware func(method, path string, handler interface{}, c *ReflectRouter, middleware []Middleware) ([]Middleware, error)
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
	r.Swagger.Components = parent.Swagger.Components
	r.hooks = parent.hooks
	r.Swagger.Info = parent.Swagger.Info
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
	r.Swagger.Info = other.Swagger.Info
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
func (r *ReflectRouter) Mount(path string, handler http.Handler) {
	switch obj := handler.(type) {
	case *ReflectRouter:
		r.Router.Mount(path, obj.Router)
	default:
		r.Router.Mount(path, handler)
	}
}

// MethodFunc adds routes for `pattern` that matches the `method` HTTP method.
// Middleware are executed from first to last
func (r *ReflectRouter) MethodFunc(method, path string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	p := func(err error) {
		panic(fmt.Sprintf("router [%s %s]: cannot create automatic handler: %v", method, path, err))
	}
	if h := r.hooks.BeforeOptions; h != nil {
		opts, err := h(method, path, handler, options)
		if err != nil {
			p(err)
		}
		options = opts
	}

	o := operations.Operation{}
	for _, option := range options {
		// don't modify the operation here, just check for errors and update schemas
		_, err := option(r.Swagger, o)
		if err != nil {
			p(err)
		}
	}

	fn, err := HandlerFromFn(handler, r.handleFn, r.Components(), r.c)
	if err != nil {
		p(err)
	}

	if h := r.hooks.BeforeMiddleware; h != nil {
		middle, err := h(method, path, handler, r, middleware)
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

	r.Router.MethodFunc(method, path, fn, options)
}

// FIXME: remove this?
func (r *ReflectRouter) Container() *container.Container {
	return r.c
}

func (r *ReflectRouter) Get(path string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodGet, path, handler, options, middleware...)
}

func (r *ReflectRouter) Options(path string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodOptions, path, handler, options, middleware...)
}

func (r *ReflectRouter) Connect(path string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodConnect, path, handler, options, middleware...)
}

func (r *ReflectRouter) Trace(path string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodTrace, path, handler, options, middleware...)
}

func (r *ReflectRouter) Post(path string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodPost, path, handler, options, middleware...)
}

func (r *ReflectRouter) Put(path string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodPut, path, handler, options, middleware...)
}

func (r *ReflectRouter) Patch(path string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodPatch, path, handler, options, middleware...)
}

func (r *ReflectRouter) Delete(path string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodDelete, path, handler, options, middleware...)
}

func (r *ReflectRouter) Head(path string, handler interface{}, options []operations.Option, middleware ...Middleware) {
	r.MethodFunc(http.MethodHead, path, handler, options, middleware...)
}
