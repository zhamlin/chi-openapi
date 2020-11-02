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

var responseWriterType = reflect.TypeOf((*http.ResponseWriter)(nil)).Elem()
var requestType = reflect.TypeOf(&http.Request{})
var ctxType = reflect.TypeOf((*context.Context)(nil)).Elem()
var errType = reflect.TypeOf((*error)(nil)).Elem()

type ArgCreator func(http.ResponseWriter, *http.Request) (reflect.Value, error)
type ArgCreators map[reflect.Type]ArgCreator

var DefaultArgCreators = ArgCreators{
	ctxType: func(_ http.ResponseWriter, r *http.Request) (reflect.Value, error) {
		return reflect.ValueOf(r.Context()), nil
	},
	responseWriterType: func(w http.ResponseWriter, _ *http.Request) (reflect.Value, error) {
		return reflect.ValueOf(w), nil
	},
	requestType: func(_ http.ResponseWriter, r *http.Request) (reflect.Value, error) {
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

	args := []reflect.Type{}
	hasJSONBody := false

	// find all arguments
	for i := 0; i < typ.NumIn(); i++ {
		arg := typ.In(i)
		if _, has := creators[arg]; has {
			args = append(args, arg)
			continue
		}
		if components.Schemas != nil {
			_, has := components.Schemas[arg.Name()]
			if has && hasJSONBody {
				return nil, fmt.Errorf("multiple json body values per handler not allowed")
			}
			if has {
				args = append(args, arg)
				hasJSONBody = true
				continue
			}
			return nil, fmt.Errorf("no way of creating type: %+v", arg)
		}
		// TODO: support params
	}

	// verify correct return
	returnCount := typ.NumOut()
	returnTypes := []reflect.Type{}
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
	}

	return func(w http.ResponseWriter, r *http.Request) {
		localArgs := []reflect.Value{}
		for _, arg := range args {
			if handler, has := creators[arg]; has {
				value, err := handler(w, r)
				if err != nil {
					fns.Error(w, err)
					return
				}
				localArgs = append(localArgs, value)
				continue
			}
			if _, has := components.Schemas[arg.Name()]; has {
				// only support loading json bodies
				argObj := reflect.New(arg)
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
				continue
			}

		}
		returns := val.Call(localArgs)
		if l := len(returns); l > 0 {
			getErr := func(i int) error {
				e, ok := returns[i].Interface().(error)
				if ok {
					return e
				}
				return nil
			}
			switch l {
			case 1:
				if err := getErr(0); err != nil {
					fns.Error(w, err)
					return
				}
			case 2:
				if err := getErr(1); err != nil {
					fns.Error(w, err)
					return
				} else {
					fns.Success(w, returns[0].Interface())
				}
			}
		}
	}, nil
}

func HandlerFromFnDefault(fnPtr interface{}, fns HandleFns, components openapi.Components) (http.HandlerFunc, error) {
	return HandlerFromFn(fnPtr, fns, components, DefaultArgCreators)
}
