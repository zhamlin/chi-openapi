package reflection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"chi-openapi/pkg/openapi"
	"chi-openapi/pkg/router"

	"github.com/getkin/kin-openapi/openapi3"
)

var (
	responseWriterType = reflect.TypeOf((*http.ResponseWriter)(nil)).Elem()
	requestPtrType     = reflect.TypeOf(&http.Request{})
	ctxType            = reflect.TypeOf((*context.Context)(nil)).Elem()
	errType            = reflect.TypeOf((*error)(nil)).Elem()
)

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
func HandlerFromFn(fnPtr interface{}, fns RequestHandler, components openapi.Components) (http.HandlerFunc, error) {
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

	// make sure func has the right amount of return values
	returnCount := typ.NumOut()
	if returnCount > 0 {
		if returnCount > 2 {
			return nil, fmt.Errorf("expected at most 2 returns, got: %v", returnCount)
		}
		// make sure the last return type is an error
		if lastError := typ.Out(returnCount - 1); lastError != errType {
			return nil, fmt.Errorf("expected the last return type to be an error, got: %+v", lastError)
		}
	}

	if k := typ.Kind(); k != reflect.Func {
		return nil, fmt.Errorf("expected a function to HandlerFromFn, got: %+v", k)
	}

	container, err := getArgs(typ, components)
	if err != nil {
		return nil, err
	}
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := container.Execute(fnPtr, w, r, r.Context())
		if err != nil {
			fns.Error(w, r, err)
			return
		}

		// return the success if we have one or two returns
		switch typ.NumOut() {
		case 1, 2:
			fns.Success(w, r, result)
		}
	}, nil
}

func HandlerFromFnDefault(fnPtr interface{}, fns RequestHandleFns, components openapi.Components) (http.HandlerFunc, error) {
	return HandlerFromFn(fnPtr, fns, components)
}

// getArgs checks that it knows how to create what the handler function expects
// returns a list of the arguments with the location
func getArgs(typ reflect.Type, components openapi.Components) (*container, error) {
	container := NewContainer()

	// dummy providers, these will be overridden when the container
	// is Executed
	container.Provide(func() (http.ResponseWriter, error) {
		return nil, fmt.Errorf("http.ResponseWriter not provided")
	})
	container.Provide(func() (*http.Request, error) {
		return nil, fmt.Errorf("*http.Request not provided")
	})
	container.Provide(func() (context.Context, error) {
		return nil, fmt.Errorf("context.Context not provided")
	})

	hasJSONBody := false
	for i := 0; i < typ.NumIn(); i++ {
		arg := typ.In(i)

		// we already know how to create this type, so
		// go ahead and skip it
		if container.HasType(arg) {
			continue
		}

		if components.Schemas != nil {
			schema, has := components.Schemas[arg.Name()]
			if has && hasJSONBody {
				return nil, fmt.Errorf("multiple json body values per handler not allowed")
			}
			if has {
				hasJSONBody = true
				fn := createJSONBodyLoadFunc(arg, schema)
				if !fn.IsValid() || fn.IsZero() {
					return nil, fmt.Errorf("failed to create the load func for: %v", arg)
				}
				if err := container.Provide(fn.Interface()); err != nil {
					return nil, err
				}
				continue
			}
		}

		if arg.Kind() != reflect.Struct {
			return nil, fmt.Errorf("no way of creating type: %+v", arg)
		}

		// it must be a parameter
		fn, err := createParamLoadFunc(arg, components)
		if err != nil {
			return nil, err
		}
		if err := container.Provide(fn.Interface()); err != nil {
			return nil, err
		}
	}

	// sanity check, make sure there aren't any cyclic dependencies
	_, err := container.Graph.Sort()
	return container, err
}

// createParamLoadFunc creates a function that can create the type passed in
func createParamLoadFunc(arg reflect.Type, components openapi.Components) (reflect.Value, error) {
	params, has := components.Parameters[arg]
	if !has {
		var err error
		params, err = openapi.ParamsFromType(arg, reflect.Value{})
		if err != nil {
			return reflect.Value{}, err
		}
		components.Parameters[arg] = params
	}

	// func to create this body
	dynamicFuncType := reflect.FuncOf([]reflect.Type{ctxType}, []reflect.Type{arg, errType}, false)
	dynamicFunc := func(in []reflect.Value) []reflect.Value {
		argObj := reflect.New(arg).Elem()
		ctx, ok := in[0].Interface().(context.Context)
		if !ok {
			err := fmt.Errorf("expected the first arg to be context.Context, got %v", in[0].Type())
			return []reflect.Value{argObj, reflect.ValueOf(err)}
		}
		input, err := router.InputFromCTX(ctx)
		if err != nil {
			return []reflect.Value{argObj, reflect.ValueOf(err)}
		}
		v, err := openapi.LoadParamStruct(argObj.Interface(), openapi.LoadParamInput{
			RequestValidationInput: input,
			Params:                 params,
		})
		if err != nil {
			return []reflect.Value{argObj, reflect.ValueOf(err)}
		}
		return []reflect.Value{v, reflect.Zero(errType)}
	}
	return reflect.MakeFunc(dynamicFuncType, dynamicFunc), nil
}

// createJSONBodyLoadFunc creates a function that can create the type passed in
func createJSONBodyLoadFunc(arg reflect.Type, schema *openapi3.SchemaRef) reflect.Value {
	dynamicFuncType := reflect.FuncOf([]reflect.Type{requestPtrType}, []reflect.Type{arg, errType}, false)
	dynamicFunc := func(in []reflect.Value) []reflect.Value {
		argObj := reflect.New(arg)
		r, ok := in[0].Interface().(*http.Request)
		if !ok {
			err := fmt.Errorf("expected the first arg to be *http.Request, got %v", in[0].Type())
			return []reflect.Value{argObj, reflect.ValueOf(err)}
		}
		if err := json.NewDecoder(r.Body).Decode(argObj.Interface()); err != nil {
			var jsonErr *json.SyntaxError
			if errors.As(err, &jsonErr) {
				input, err := router.InputFromCTX(r.Context())
				if err != nil {
					return []reflect.Value{argObj, reflect.ValueOf(err)}
				}

				body := input.Route.Operation.RequestBody
				if body != nil {
					return []reflect.Value{argObj, reflect.ValueOf(err)}
				}

				if body.Value.Required {
					err := fmt.Errorf("Required json body")
					return []reflect.Value{argObj, reflect.ValueOf(err)}
				}
				// because is is not required, return an empty result
				return []reflect.Value{argObj.Elem(), {}}
			} else {
				return []reflect.Value{argObj, reflect.ValueOf(err)}
			}
		}
		v, err := openapi.VarToInterface(argObj.Elem().Interface())
		if err != nil {
			return []reflect.Value{argObj, reflect.ValueOf(err)}
		}
		if err := schema.Value.VisitJSON(v); err != nil {
			return []reflect.Value{argObj, reflect.ValueOf(err)}
		}
		return []reflect.Value{argObj.Elem(), reflect.Zero(errType)}
	}
	return reflect.MakeFunc(dynamicFuncType, dynamicFunc)
}
