package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"

	"chi-openapi/pkg/openapi"
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

type HandleFns struct {
	ErrFn     func(http.ResponseWriter, error)
	SuccessFn func(w http.ResponseWriter, response interface{})
}

func (h HandleFns) Error(w http.ResponseWriter, err error) {
	if h.ErrFn != nil {
		h.ErrFn(w, err)
	}
}

func (h HandleFns) Success(w http.ResponseWriter, obj interface{}) {
	if h.SuccessFn != nil {
		h.SuccessFn(w, obj)
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
func HandlerFromFn(fnPtr interface{}, fns HandleFns, components openapi.Components, creators ArgCreators) (http.HandlerFunc, error) {
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
					fns.Error(w, err)
					return
				}
			}
		case 2:
			returnHandlerFn = func(w http.ResponseWriter, r *http.Request, values []reflect.Value) {
				if err := getErr(values, 1); err != nil {
					fns.Error(w, err)
					return
				} else {
					fns.Success(w, values[0].Interface())
				}
			}

		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		localArgs := []reflect.Value{}
		for _, arg := range args {
			switch arg.Type {
			case argTypeOther:
				if argCreator, has := creators[arg.ReflectType]; has {
					value, err := argCreator(w, r)
					if err != nil {
						fns.Error(w, err)
						return
					}
					localArgs = append(localArgs, value)
				}
			case argTypeParam:
				localArgs = append(localArgs, reflect.New(arg.ReflectType).Elem())
			case argTypeJSONBody:
				if _, has := components.Schemas[arg.ReflectType.Name()]; has {
					// TODO: support data wrapper types
					// probably need to look for any refs, and load appropriately

					// only support loading json bodies
					argObj := reflect.New(arg.ReflectType)
					b, err := ioutil.ReadAll(r.Body)
					if err != nil {
						fns.Error(w, err)
						return
					}
					if err := json.Unmarshal(b, argObj.Interface()); err != nil {
						fns.Error(w, err)
						return
					}
					localArgs = append(localArgs, argObj.Elem())
				}
			}

		}
		returns := val.Call(localArgs)
		returnHandlerFn(w, r, returns)
	}, nil
}

func HandlerFromFnDefault(fnPtr interface{}, fns HandleFns, components openapi.Components) (http.HandlerFunc, error) {
	return HandlerFromFn(fnPtr, fns, components, DefaultArgCreators)
}
