package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"

	"chi-openapi/pkg/openapi"
	"chi-openapi/pkg/openapi/operations"
	"chi-openapi/pkg/router"

	"github.com/getkin/kin-openapi/openapi3"
)

var (
	responseWriterType = reflect.TypeOf((*http.ResponseWriter)(nil)).Elem()
	requestPtrType     = reflect.TypeOf(&http.Request{})
	ctxType            = reflect.TypeOf((*context.Context)(nil)).Elem()
	errType            = reflect.TypeOf((*error)(nil)).Elem()
)

type ArgCreator func(http.ResponseWriter, *http.Request) (reflect.Value, error)
type ArgCreators map[reflect.Type]ArgCreator

var DefaultArgCreators = ArgCreators{
	ctxType: func(_ http.ResponseWriter, r *http.Request) (reflect.Value, error) {
		return reflect.ValueOf(r.Context()), nil
	},
	responseWriterType: func(w http.ResponseWriter, _ *http.Request) (reflect.Value, error) {
		return reflect.ValueOf(w), nil
	},
	requestPtrType: func(_ http.ResponseWriter, r *http.Request) (reflect.Value, error) {
		return reflect.ValueOf(r), nil
	},
}

type RequestHandler interface {
	Error(w http.ResponseWriter, r *http.Request, err error)
	Success(w http.ResponseWriter, r *http.Request, obj interface{})
}

type ErrorHandler func(http.ResponseWriter, *http.Request, error)

type RequestHandleFns struct {
	ErrFn     ErrorHandler
	SuccessFn func(w http.ResponseWriter, r *http.Request, response interface{})
}

func (h RequestHandleFns) Error(w http.ResponseWriter, r *http.Request, err error) {
	if h.ErrFn != nil {
		h.ErrFn(w, r, err)
	}
}

func (h RequestHandleFns) Success(w http.ResponseWriter, r *http.Request, obj interface{}) {
	if h.SuccessFn != nil {
		h.SuccessFn(w, r, obj)
	}
}

type argType string

var (
	argTypeParam    argType = "param"
	argTypeJSONBody argType = "json_body"
	argTypeOther    argType = "other"
)

type handlerArgType struct {
	ReflectType reflect.Type
	Type        argType
}

// HandlerFromFn takes in any function matching the following criteria:
// 1. Takes the following as input:
//      - context.Context
//      - *http.Request
//      - http.ResponseWriter
//      - any other type that is passed in via the ArgCreators map
// 2. Returns up to two responses
//      - last return of this function must be an error
// The second argument is a function to ErrorHandler to handle
// any errors during the http.Handler
// All arguments will be automatically created and supplied to the function.
// Only loads params and one json body schema in the components.
func HandlerFromFn(fnPtr interface{}, fns RequestHandler, components openapi.Components, creators ArgCreators) (http.HandlerFunc, error) {
	if fnPtr == nil {
		return nil, fmt.Errorf("received a nil value for the fnPtr to HandlerFromFn")
	}
	if handler, ok := fnPtr.(http.HandlerFunc); ok {
		return handler, nil
	}
	if handler, ok := fnPtr.(http.Handler); ok {
		return handler.ServeHTTP, nil
	}

	val := reflect.ValueOf(fnPtr)
	typ := val.Type()

	if k := typ.Kind(); k != reflect.Func {
		return nil, fmt.Errorf("expected a function to HandlerFromFn, got: %+v", k)
	}

	args := []handlerArgType{}
	hasJSONBody := false

	// find all arguments
	for i := 0; i < typ.NumIn(); i++ {
		arg := typ.In(i)
		if _, has := creators[arg]; has {
			args = append(args, handlerArgType{arg, argTypeOther})
			continue
		}
		if components.Schemas != nil {
			_, has := components.Schemas[arg.Name()]
			if has && hasJSONBody {
				return nil, fmt.Errorf("multiple json body values per handler not allowed")
			}
			if has {
				args = append(args, handlerArgType{arg, argTypeJSONBody})
				hasJSONBody = true
				continue
			}
		}
		if arg.Kind() != reflect.Struct {
			return nil, fmt.Errorf("no way of creating type: %+v", arg)
		}
		paramList, has := components.Parameters[arg]
		if !has {
			var err error
			paramList, err = openapi.ParamsFromType(arg, reflect.Value{})
			if err != nil {
				return nil, err
			}
			components.Parameters[arg] = paramList
		}
		args = append(args, handlerArgType{arg, argTypeParam})
	}

	// verify correct return
	returnCount := typ.NumOut()
	returnTypes := []reflect.Type{}
	returnHandlerFn := func(http.ResponseWriter, *http.Request, []reflect.Value) {}
	if returnCount > 0 {
		if returnCount > 2 {
			return nil, fmt.Errorf("expected at most 2 returns, got: %v", returnCount)
		}

		for i := 0; i < returnCount; i++ {
			returnTypes = append(returnTypes, typ.Out(i))
		}
		// make sure the last return type is an error
		if lastError := returnTypes[len(returnTypes)-1]; lastError != errType {
			return nil, fmt.Errorf("expected the last return type to be an error, got: %+v", lastError)
		}

		getErr := func(values []reflect.Value, i int) error {
			e, ok := values[i].Interface().(error)
			if ok {
				return e
			}
			return nil
		}

		switch returnCount {
		case 1:
			returnHandlerFn = func(w http.ResponseWriter, r *http.Request, values []reflect.Value) {
				if err := getErr(values, 0); err != nil {
					fns.Error(w, r, err)
					return
				}
			}
		case 2:
			returnHandlerFn = func(w http.ResponseWriter, r *http.Request, values []reflect.Value) {
				if err := getErr(values, 1); err != nil {
					fns.Error(w, r, err)
					return
				} else {
					fns.Success(w, r, values[0].Interface())
				}
			}

		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		localArgs := []reflect.Value{}
		for _, arg := range args {
			switch arg.Type {
			case argTypeOther:
				argCreator, has := creators[arg.ReflectType]
				if !has {
					fns.Error(w, r, fmt.Errorf("unknown type: %v", arg.ReflectType))
					return

				}
				value, err := argCreator(w, r)
				if err != nil {
					fns.Error(w, r, err)
					return
				}
				localArgs = append(localArgs, value)
			case argTypeParam:
				params, has := components.Parameters[arg.ReflectType]
				if !has {
					fns.Error(w, r, fmt.Errorf("unknown paramater: %v", arg.ReflectType))
					return
				}
				obj := reflect.New(arg.ReflectType).Elem()
				input, err := router.InputFromCTX(r.Context())
				if err != nil {
					fns.Error(w, r, err)
					return
				}
				v, err := openapi.LoadParamStruct(obj.Interface(), openapi.LoadParamInput{
					// TODO: pull this from r.Context()
					RequestValidationInput: input,
					Params:                 params,
				})
				if err != nil {
					fns.Error(w, r, err)
					return
				}
				localArgs = append(localArgs, v)
			case argTypeJSONBody:
				schema, has := components.Schemas[arg.ReflectType.Name()]
				if !has {
					fns.Error(w, r, fmt.Errorf("unknown json body: %v", arg.ReflectType))
					return
				}
				// TODO: support data wrapper types
				// probably need to look for any refs, and load appropriately

				// only support loading json bodies
				argObj := reflect.New(arg.ReflectType)
				b, err := ioutil.ReadAll(r.Body)

				if err != nil {
					fns.Error(w, r, err)
					return
				}

				if err := json.Unmarshal(b, argObj.Interface()); err != nil {
					var jsonErr *json.SyntaxError
					if errors.As(err, &jsonErr) {
						input, err := router.InputFromCTX(r.Context())
						if err != nil {
							fns.Error(w, r, err)
							return
						}

						body := input.Route.Operation.RequestBody
						if body != nil {
							fns.Error(w, r, err)
							return
						}

						if body.Value.Required {
							fns.Error(w, r, fmt.Errorf("Required json body"))
							return
						}
						localArgs = append(localArgs, argObj.Elem())
						continue
					} else {
						fns.Error(w, r, err)
						return
					}

				}

				v, err := openapi.VarToInterface(argObj.Elem().Interface())
				if err != nil {
					fns.Error(w, r, err)
					return
				}
				if err := schema.Value.VisitJSON(v); err != nil {
					fns.Error(w, r, err)
					return
				}
				localArgs = append(localArgs, argObj.Elem())
			}

		}
		returns := val.Call(localArgs)
		returnHandlerFn(w, r, returns)
	}, nil
}

func HandlerFromFnDefault(fnPtr interface{}, fns RequestHandleFns, components openapi.Components) (http.HandlerFunc, error) {
	return HandlerFromFn(fnPtr, fns, components, DefaultArgCreators)
}

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
		for name, item := range obj.Swagger.Paths {
			r.Swagger.Paths[path+strings.TrimRight(name, "/")] = item
		}
		for name, item := range obj.Swagger.Components.Schemas {
			r.Swagger.Components.Schemas[name] = item
		}
	}
	r.Router.Mount(path, handler)
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
