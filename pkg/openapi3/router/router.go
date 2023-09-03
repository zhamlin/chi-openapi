package router

import (
	"fmt"
	"net/http"
	"path"

	"github.com/zhamlin/chi-openapi/internal"
	"github.com/zhamlin/chi-openapi/internal/runtime"
	"github.com/zhamlin/chi-openapi/pkg/jsonschema"
	"github.com/zhamlin/chi-openapi/pkg/openapi3"
	"github.com/zhamlin/chi-openapi/pkg/openapi3/operations"

	"github.com/go-chi/chi/v5"
)

func setRouterGroupName(r *Router, args ...string) {
	if l := len(args); l > 0 {
		t := openapi3.NewTag()
		t.Spec.Name = args[0]
		if l > 1 {
			t.Spec.Description = args[1]
		}
		r.spec.Tags = append(r.spec.Tags, t.Extendable)
		r.groupName = t.Spec.Name
	}
}

func newChiRouter() chi.Router {
	return chi.NewRouter()
}

func NewRouter(title, version string) *Router {
	spec := openapi3.NewOpenAPI(title)
	spec.Info.Spec.Version = version

	schemer := jsonschema.NewSchemer()
	schemer.RefPath = "#/components/schemas/"
	schemer.UseRefs = true

	return &Router{
		spec:    spec,
		schemer: schemer,
		mux:     newChiRouter(),

		defaultStatusRoutes: map[int]string{},
		defaultRouteRef:     "",
		defaultContentType:  openapi3.JsonContentType,

		panicOnError: false,
		errors:       []error{},
	}
}

type Router struct {
	spec    openapi3.OpenAPI
	schemer jsonschema.Schemer
	mux     chi.Router

	// name of the tag to apply on all operations
	groupName string

	// map[statusCode]ref location
	defaultStatusRoutes map[int]string
	// ref location of the default response
	defaultRouteRef string

	defaultContentType string

	// used to track errors if panicOnError is false
	errors []error

	// TODO: move to config
	panicOnError bool

	// TODO: move to config
	setOperationID bool

	setRouteInfo bool
}

func (r Router) Errors() []error {
	return r.errors
}

func (r Router) Schemer() jsonschema.Schemer {
	return r.schemer
}

func (r Router) RegisterComponent(obj any, schema jsonschema.Schema, options ...jsonschema.Option) {
	r.schemer.Set(obj, schema, options...)
}

func (r Router) RegisterComponentAs(name string, obj any, schema jsonschema.Schema, options ...jsonschema.Option) {
	r.RegisterComponent(obj, schema, append([]jsonschema.Option{jsonschema.Name(name)}, options...)...)
	if err := r.spec.GetComponents().AddSchema(name, schema); err != nil {
		r.handleErr(err)
	}
}

func (r Router) OpenAPI() openapi3.OpenAPI {
	return r.spec
}

func (r *Router) WithSchemer(schemer jsonschema.Schemer) *Router {
	r.schemer = schemer
	return r
}

func (r *Router) WithOperationID(b bool) *Router {
	r.setOperationID = b
	return r
}

// DefaultResponse sets the default response on all operations. Can be overriden
// at the route level.
func (r *Router) DefaultResponse(desc string, obj any, contentType ...string) {
	err := r.setDefaultStatusResponse("default", desc, obj, contentType...)
	if err != nil {
		r.handleErr(err)
		return
	}
	r.defaultRouteRef = "#/components/responses/default"
}

// DefaultStatusResponse sets the default response for the specified status code on all operations.
// Can be overriden at the route level.
func (r *Router) DefaultStatusResponse(code int, desc string, obj any, contentType ...string) {
	statusCode := fmt.Sprintf("%d", code)
	err := r.setDefaultStatusResponse(statusCode, desc, obj, contentType...)
	if err != nil {
		r.handleErr(err)
		return
	}
	r.defaultStatusRoutes[code] = "#/components/responses/" + statusCode
}

// PanicOnError enables how errors are handled. By default all errors
// are added to an error array accessable via Errors(). If this is set to true
// panic will instead be called on error.
func (r *Router) PanicOnError(b bool) {
	r.panicOnError = b
}

func (r *Router) setDefaultStatusResponse(code string, desc string, obj any, contentType ...string) error {
	if len(contentType) == 0 {
		contentType = []string{openapi3.JsonContentType}
	}

	resp := openapi3.Response{}
	resp.Description = internal.TrimString(desc)

	if obj != nil {
		ctx := operations.NewOptionCtx(r.schemer, r.OpenAPI(), r.defaultContentType)
		mediaType, err := ctx.NewMediaType(obj)
		if err != nil {
			return err
		}
		for _, typ := range contentType {
			resp.SetContent(typ, mediaType)
		}
	}
	return r.spec.GetComponents().AddResponse(code, resp)
}

// setDefaultResponses ensures any default responses are set on the operation.
func (r *Router) setDefaultResponses(op openapi3.Operation) {
	for statusCode, ref := range r.defaultStatusRoutes {
		code := fmt.Sprintf("%d", statusCode)
		_, has := op.Responses.Spec.Response[code]
		if !has {
			op.AddResponseRef(statusCode, ref)
		}
	}

	// only set the global default response if this operation does not have
	// it set alerady
	if ref := r.defaultRouteRef; ref != "" {
		if responses := op.Responses; responses != nil && responses.Spec.Default == nil {
			op.AddDefaultResponseRef(ref)
		}
	}
}

func (r *Router) handleErr(err error) {
	// this func was called from the router, which in turn was called by something else
	// so start with a skip of 2
	if caller := runtime.GetCaller(2); caller != "" {
		err = fmt.Errorf("%s: %w", caller, err)
	}

	if r.panicOnError {
		panic(err)
	} else {
		r.errors = append(r.errors, err)
	}
}

func (r Router) clone() Router {
	return Router{
		spec:                r.spec,
		schemer:             r.schemer,
		mux:                 r.mux,
		groupName:           r.groupName,
		defaultStatusRoutes: r.defaultStatusRoutes,
		defaultRouteRef:     r.defaultRouteRef,
		defaultContentType:  r.defaultContentType,
		errors:              []error{},
		panicOnError:        r.panicOnError,
		setOperationID:      r.setOperationID,
	}
}

// chi router functions

// Route mounts a sub-Router along a `pattern` string.
func (r *Router) Route(pattern string, fn func(r *Router), args ...string) {
	subRouter := r.clone()
	subRouter.spec = openapi3.NewOpenAPI("subrouter")
	subRouter.mux = newChiRouter()
	fn(&subRouter)
	setRouterGroupName(&subRouter, args...)
	r.Mount(pattern, &subRouter, args...)
}

func joinPaths(a, b string) string {
	newPath := path.Join(a, b)
	// path.Join removes any trailing / from b
	// so add it back if it exists
	if len(b) > 1 && b[len(b)-1] == '/' {
		newPath = newPath + "/"
	}
	return newPath
}

type errKeyExists struct {
	key string
}

func (e errKeyExists) Error() string {
	return "key already exists: " + e.key
}

// setOrErr sets the key in the map if the key does not exist,
// if not it returns an error
func setOrErr[V any](t map[string]V, key string, v V) error {
	_, has := t[key]
	if has {
		return errKeyExists{key: key}
	}
	t[key] = v
	return nil
}

func tryCopy[V any](from, to map[string]V) error {
	for name, item := range from {
		if _, has := to[name]; has {
			return errKeyExists{name}
		}
		to[name] = item
	}
	return nil
}

func (r *Router) copyComponents(from, to openapi3.Components) error {
	for _, schema := range from.GetSchemas() {
		err := to.AddSchema(schema.Name, schema.Schema)
		if err != nil {
			return fmt.Errorf("components: %w", err)
		}
	}

	err := tryCopy(from.Responses, to.Responses)
	if err != nil {
		return fmt.Errorf("responses: %w", err)
	}
	err = tryCopy(from.Parameters, to.Parameters)
	if err != nil {
		return fmt.Errorf("parameters: %w", err)
	}
	err = tryCopy(from.Callbacks, to.Callbacks)
	if err != nil {
		return fmt.Errorf("callbacks: %w", err)
	}

	return nil
}

// Mount attaches another http.Handler along ./pattern/*
func (r *Router) Mount(pattern string, h http.Handler, args ...string) {
	routers := []*DepRouter{}
	var router *Router
	switch r := h.(type) {
	case *Router:
		router = r
	case *DepRouter:
		router = r.router
		routers = r.mounted
	}

	if router != nil {
		newComponents := router.spec.GetComponents()
		currentComponents := r.spec.GetComponents()
		err := r.copyComponents(newComponents, currentComponents)
		if err != nil {
			r.handleErr(fmt.Errorf("Mount('%s', ...): %w", pattern, err))
		}

		r.spec.Tags = append(r.spec.Tags, router.spec.Tags...)

		for p, pathItem := range router.spec.Paths.Spec.Paths {
			err := setOrErr(r.spec.Paths.Spec.Paths, joinPaths(pattern, p), pathItem)
			if err != nil {
				r.handleErr(fmt.Errorf("Mount('%s', ...): paths: %w", pattern, err))
			}

			pathItem := openapi3.PathItem{PathItem: pathItem.Spec.Spec}
			for _, method := range openapi3.PathItemMethods {
				if op, has := pathItem.GetOperation(method); has {
					r.setDefaultResponses(op)
				}
			}
		}

		r.errors = append(r.errors, router.errors...)

		// update the sub routers API to match this one
		router.spec.OpenAPI = r.spec.OpenAPI
		for _, mounted := range routers {
			mounted.router.spec.OpenAPI = r.spec.OpenAPI
		}
	}
	r.mux.Mount(pattern, h)
}

// With adds inline middlewares for an endpoint handler.
func (r *Router) With(middlewares ...func(http.Handler) http.Handler) *Router {
	router := r.clone()
	router.mux = r.mux.With(middlewares...)
	return &router
}

// Group adds a new inline-Router along the current routing
// path, with a fresh middleware stack for the inline-Router.
func (r *Router) Group(fn func(r *Router)) {
	groupRouter := r.clone()
	groupRouter.mux = r.mux.Group(nil)
	fn(&groupRouter)
}

// Use appends one or more middlewares onto the Router stack.
func (r *Router) Use(middlewares ...func(http.Handler) http.Handler) {
	r.mux.Use(middlewares...)
}

func (r Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

// MethodNotAllowed defines a handler to respond whenever a method is
// not allowed.
func (r *Router) MethodNotAllowed(h http.HandlerFunc) {
	r.mux.MethodNotAllowed(h)
}

// NotFound defines a handler to respond whenever a route could
// not be found.
func (r *Router) NotFound(h http.HandlerFunc) {
	r.mux.NotFound(h)
}

// Method adds routes for `pattern` that matches the `method` HTTP method.
func (r *Router) Method(method, pattern string, h http.HandlerFunc, options ...operations.Option) {
	pathItem, has := r.spec.GetPath(pattern)
	if !has {
		pathItem = openapi3.NewPathItem()
	}

	ctx := operations.NewOptionCtx(r.schemer, r.OpenAPI(), r.defaultContentType)
	operation := openapi3.NewOperation()
	var err error
	for i, option := range options {
		operation, err = option(ctx, operation)
		if err != nil {
			r.handleErr(fmt.Errorf("%s %s : option %d returned an error: %w", method, pattern, i, err))
		}
	}

	if r.setOperationID && operation.OperationID == "" {
		// if an operation id isn't already set, try to use the handler name for one
		if name := getPublicFunctionName(h); name != "" {
			operation.OperationID = name
		}
	}

	if r.groupName != "" {
		operation.Tags = append(operation.Tags, r.groupName)
	}

	r.setDefaultResponses(operation)
	pathItem.SetOperation(method, operation)
	r.spec.SetPath(pattern, pathItem)

	// Copy any schemas from the jsonschema that might have been created
	// when adding another type. For example an array of objects was supplied as a response,
	// it would have the $ref correctly generated for the object, but the object its self would
	// not exist in the components, just the schemer.
	components := r.spec.GetComponents()
	for t, schema := range r.schemer.Types() {
		if schema.NoRef() {
			continue
		}

		name := schema.Name()
		if _, has := components.GetSchemaByName(name); !has && name != "" {
			r.RegisterComponentAs(name, t, schema)
		}
	}

	if r.setRouteInfo {
		// Because chi.Context works, to get the full path needed to match to an openapi path the
		// middleware needs to run at the _last_ router (if there are any mounted routers). The
		// easiest place to do so is right before the handler
		r.mux.With(addRouteInfo(r)).Method(method, pattern, h)
	} else {
		r.mux.Method(method, pattern, h)
	}
}

func (r *Router) Connect(pattern string, h http.HandlerFunc, options ...operations.Option) {
	r.Method(http.MethodConnect, pattern, h, options...)
}

func (r *Router) Head(pattern string, h http.HandlerFunc, options ...operations.Option) {
	r.Method(http.MethodHead, pattern, h, options...)
}

func (r *Router) Options(pattern string, h http.HandlerFunc, options ...operations.Option) {
	r.Method(http.MethodOptions, pattern, h, options...)
}

func (r *Router) Get(pattern string, h http.HandlerFunc, options ...operations.Option) {
	r.Method(http.MethodGet, pattern, h, options...)
}

func (r *Router) Post(pattern string, h http.HandlerFunc, options ...operations.Option) {
	r.Method(http.MethodPost, pattern, h, options...)
}

func (r *Router) Put(pattern string, h http.HandlerFunc, options ...operations.Option) {
	r.Method(http.MethodPut, pattern, h, options...)
}

func (r *Router) Patch(pattern string, h http.HandlerFunc, options ...operations.Option) {
	r.Method(http.MethodPatch, pattern, h, options...)
}

func (r *Router) Delete(pattern string, h http.HandlerFunc, options ...operations.Option) {
	r.Method(http.MethodDelete, pattern, h, options...)
}

func (r *Router) Trace(pattern string, h http.HandlerFunc, options ...operations.Option) {
	r.Method(http.MethodTrace, pattern, h, options...)
}
