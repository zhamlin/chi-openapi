package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/zhamlin/chi-openapi/internal"
	"github.com/zhamlin/chi-openapi/pkg/container"
	"github.com/zhamlin/chi-openapi/pkg/jsonschema"
	"github.com/zhamlin/chi-openapi/pkg/openapi3"
	"github.com/zhamlin/chi-openapi/pkg/openapi3/operations"
)

type ResponseHandler func(w http.ResponseWriter, r *http.Request, resp any, err error)

type RequestBodyLoader func(r *http.Request, obj any) error

type DepConfig struct {
	Config
	Container         *container.Container
	RequestBodyLoader RequestBodyLoader
	ResponseHandler   ResponseHandler
}

func (c DepConfig) WithSchemer(schemer jsonschema.Schemer) DepConfig {
	c.Schemer = &schemer
	return c
}

func (c DepConfig) WithContainer(container container.Container) DepConfig {
	c.Container = &container
	return c
}

type DepRouter struct {
	// keep this as a private field vs embedding to
	// prevent the *Router from being used directly
	router            *Router
	Container         container.Container
	ResponseHandler   ResponseHandler
	RequestBodyLoader RequestBodyLoader

	mounted []*DepRouter
}

func defaultRequestBodyLoader(r *http.Request, obj any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read the request body: %w", err)
	}
	err = json.Unmarshal(body, obj)
	if err != nil {
		return fmt.Errorf("failed to unmarshal request body (%T): %w", obj, err)
	}
	return nil
}

func NewDepRouter(cfg DepConfig) *DepRouter {
	if cfg.RequestBodyLoader == nil {
		cfg.RequestBodyLoader = defaultRequestBodyLoader
	}

	if cfg.Container == nil {
		cfg = cfg.WithContainer(container.New())
	}

	router := NewRouter(cfg.Config)
	return &DepRouter{
		router:            router,
		mounted:           []*DepRouter{},
		Container:         *cfg.Container,
		RequestBodyLoader: cfg.RequestBodyLoader,
		ResponseHandler:   cfg.ResponseHandler,
	}
}

func (r *DepRouter) Schemer() jsonschema.Schemer {
	return r.router.schemer
}

func (r DepRouter) clone() DepRouter {
	router := r.router.clone()
	return DepRouter{
		router:            &router,
		ResponseHandler:   r.ResponseHandler,
		RequestBodyLoader: r.RequestBodyLoader,
		Container:         r.Container,
		mounted:           []*DepRouter{},
	}
}

func (r *DepRouter) createHandler(handler any) (http.HandlerFunc, fnInfo) {
	h, fnInfo, err := httpHandlerFromFn(handler, r)
	if err != nil {
		r.handleError(fmt.Errorf("could not create an http.HandlerFunc from handler: %w", err))
	}
	return h, fnInfo
}

func (r *DepRouter) handleError(err error) {
	r.router.handleErr(err)
}

// Methods from Router
func (r DepRouter) RegisterComponent(obj any, schema jsonschema.Schema, options ...jsonschema.Option) {
	r.router.RegisterComponent(obj, schema, options...)
}

func (r DepRouter) RegisterComponentAs(name string, obj any, schema jsonschema.Schema, options ...jsonschema.Option) {
	r.router.RegisterComponentAs(name, obj, schema, options...)
}

func (r DepRouter) OpenAPI() openapi3.OpenAPI {
	return r.router.OpenAPI()
}

// DefaultStatusResponse sets the default response for the specified status code on all operations.
// Can be overriden at the route level.
func (r *DepRouter) DefaultStatusResponse(code int, desc string, obj any, contentType ...string) {
	r.router.DefaultStatusResponse(code, desc, obj, contentType...)
}

// DefaultResponse sets the default response on all operations. Can be overriden
// at the route level.
func (r *DepRouter) DefaultResponse(desc string, obj any, contentType ...string) {
	r.router.DefaultResponse(desc, obj, contentType...)
}

// chi router methods

func (r DepRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.router.ServeHTTP(w, req)
}

// Route mounts a sub-Router along a `pattern` string.
func (r *DepRouter) Route(pattern string, fn func(r *DepRouter), args ...string) {
	subRouter := r.clone()
	subRouter.router.spec = openapi3.NewOpenAPI("subrouter")
	subRouter.router.mux = newChiRouter()
	setRouterGroupName(subRouter.router, args...)
	fn(&subRouter)
	r.Mount(pattern, &subRouter, args...)
}

// Mount attaches another http.Handler along ./pattern/*
func (r *DepRouter) Mount(pattern string, h http.Handler, args ...string) {
	if depRouter, ok := h.(*DepRouter); ok {
		copyFns := func(router *DepRouter) {
			router.ResponseHandler = r.ResponseHandler
			router.RequestBodyLoader = r.RequestBodyLoader
		}
		// r.ResponseHandler takes precedence over the router being mounted
		for _, router := range depRouter.mounted {
			copyFns(router)
		}
		copyFns(depRouter)
		r.mounted = append(r.mounted, depRouter)
		r.mounted = append(r.mounted, depRouter.mounted...)
	}
	r.router.Mount(pattern, h, args...)
}

// With adds inline middlewares for an endpoint handler.
func (r *DepRouter) With(middlewares ...func(http.Handler) http.Handler) *DepRouter {
	newRouter := r.clone()
	newRouter.router = r.router.With(middlewares...)
	return &newRouter
}

// Use appends one or more middlewares onto the Router stack.
func (r *DepRouter) Use(middlewares ...func(http.Handler) http.Handler) {
	r.router.Use(middlewares...)
}

// Group adds a new inline-Router along the current routing
// path, with a fresh middleware stack for the inline-Router.
func (r *DepRouter) Group(fn func(r *DepRouter)) {
	depRouter := r.clone()
	r.router.Group(func(r *Router) {
		depRouter.router = r
		fn(&depRouter)
	})
}

// MethodNotAllowed defines a handler to respond whenever a method is
// not allowed.
func (r *DepRouter) MethodNotAllowed(handler any) {
	h, _ := r.createHandler(handler)
	r.router.MethodNotAllowed(h)
}

// NotFound defines a handler to respond whenever a route could
// not be found.
func (r *DepRouter) NotFound(handler any) {
	h, _ := r.createHandler(handler)
	r.router.NotFound(h)
}

func addParams(params []openapi3.Parameter) operations.Option {
	return func(_ operations.OptionCtx, o openapi3.Operation) (openapi3.Operation, error) {
		for _, param := range params {
			if !o.HasParameter(param) {
				o.AddParameter(param)
			}
		}
		return o, nil
	}
}

// Method adds routes for `pattern` that matches the `method` HTTP method.
func (r *DepRouter) Method(method, pattern string, handler any, options ...operations.Option) {
	h, fnInfo := r.createHandler(handler)
	if fnInfo.hasReturns && r.ResponseHandler == nil {
		r.handleError(fmt.Errorf("%T has return values, but router ResponseHandler is not set", handler))
	}

	if getID := r.router.operationIDGrabber; getID != nil {
		if id := getID(handler); id != "" {
			// this option needs to come first so it can be overriden
			// if Method was already supplied with an ID
			options = append([]operations.Option{operations.ID(id)}, options...)
		}
	}

	options = append(options, addParams(fnInfo.params))
	if body := fnInfo.requestBody; body.Type != nil {
		desc := body.Tag.Get("doc")
		required := true
		if s := body.Tag.Get("required"); s != "" {
			b, err := internal.BoolFromString(s)
			if err != nil {
				r.handleError(err)
			} else {
				required = b
			}
		}
		options = append(options, operations.BodyObj(desc, body.Type, required))
	}

	oldSetRouteInfo := r.router.addRouteInfo
	{
		needsRouteInfo := r.router.addRouteInfo || len(fnInfo.params) > 0
		r.router.addRouteInfo = needsRouteInfo
		r.router.Method(method, pattern, h, options...)
	}
	r.router.addRouteInfo = oldSetRouteInfo
}

func (r *DepRouter) Connect(pattern string, h any, options ...operations.Option) {
	r.Method(http.MethodConnect, pattern, h, options...)
}

func (r *DepRouter) Head(pattern string, h any, options ...operations.Option) {
	r.Method(http.MethodHead, pattern, h, options...)
}

func (r *DepRouter) Options(pattern string, h any, options ...operations.Option) {
	r.Method(http.MethodOptions, pattern, h, options...)
}

func (r *DepRouter) Get(pattern string, h any, options ...operations.Option) {
	r.Method(http.MethodGet, pattern, h, options...)
}

func (r *DepRouter) Post(pattern string, h any, options ...operations.Option) {
	r.Method(http.MethodPost, pattern, h, options...)
}

func (r *DepRouter) Put(pattern string, h any, options ...operations.Option) {
	r.Method(http.MethodPut, pattern, h, options...)
}

func (r *DepRouter) Patch(pattern string, h any, options ...operations.Option) {
	r.Method(http.MethodPatch, pattern, h, options...)
}

func (r *DepRouter) Delete(pattern string, h any, options ...operations.Option) {
	r.Method(http.MethodDelete, pattern, h, options...)
}

func (r *DepRouter) Trace(pattern string, h any, options ...operations.Option) {
	r.Method(http.MethodTrace, pattern, h, options...)
}
