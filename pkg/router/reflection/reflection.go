package reflection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"unicode"

	"github.com/zhamlin/chi-openapi/pkg/container"
	"github.com/zhamlin/chi-openapi/pkg/openapi"
	"github.com/zhamlin/chi-openapi/pkg/router"

	"github.com/getkin/kin-openapi/openapi3"
)

var (
	requestPtrType = reflect.TypeOf(&http.Request{})
	ctxType        = reflect.TypeOf((*context.Context)(nil)).Elem()
	errType        = reflect.TypeOf((*error)(nil)).Elem()
)

type RequestHandler func(w http.ResponseWriter, r *http.Request, response interface{}, err error)

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
func HandlerFromFn(fptr interface{}, fn RequestHandler, components openapi.Components, c *container.Container) (http.HandlerFunc, error) {
	switch handler := fptr.(type) {
	case nil:
		return nil, fmt.Errorf("received a nil value for the fnPtr to HandlerFromFn")
		// TODO: remove
	case http.HandlerFunc:
		return handler, nil
	case http.Handler:
		return handler.ServeHTTP, nil
	}
	typ := reflect.TypeOf(fptr)
	if k := typ.Kind(); k != reflect.Func {
		return nil, fmt.Errorf("expected a function to HandlerFromFn, got: %+v", k)
	}

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
	if err := loadArgsIntoContainer(c, typ, components); err != nil {
		return nil, err
	}
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := c.Execute(fptr, w, r, r.Context())
		fn(w, r, result, err)
	}, nil
}

func HandlerFromFnDefault(fnPtr interface{}, fn RequestHandler, components openapi.Components) (http.HandlerFunc, error) {
	return HandlerFromFn(fnPtr, fn, components, container.NewContainer())
}

// loadArgsIntoContainer checks that it knows how to create what the handler function expects
// returns a list of the arguments with the location
func loadArgsIntoContainer(container *container.Container, typ reflect.Type, components openapi.Components) error {
	var err error
	e := func(e error) {
		if e != nil && err == nil {
			err = e
		}
	}
	// dummy providers, these will be overridden when the container
	// is executed
	e(container.Provide(func() (http.ResponseWriter, error) {
		return nil, fmt.Errorf("http.ResponseWriter not provided")
	}))
	e(container.Provide(func() (*http.Request, error) {
		return nil, fmt.Errorf("*http.Request not provided")
	}))
	e(container.Provide(func() (context.Context, error) {
		return nil, fmt.Errorf("context.Context not provided")
	}))
	if err != nil {
		return err
	}

	hasJSONBody := false
	for i := 0; i < typ.NumIn(); i++ {
		arg := typ.In(i)

		// we already know how to create this type, so
		// go ahead and skip it
		if container.HasType(arg) {
			continue
		}
		if components.Schemas != nil {
			// TODO FIXME: this might have to change if the type uses a custom name
			schema, has := components.Schemas[openapi.GetTypeName(arg)]
			if has && hasJSONBody {
				return fmt.Errorf("multiple json body values per handler not allowed")
			}

			if has {
				hasJSONBody = true
				fn := createJSONBodyLoadFunc(arg, schema)
				if !fn.IsValid() || fn.IsZero() {
					return fmt.Errorf("failed to create the load func for: %v", arg)
				}
				if err := container.Provide(fn.Interface()); err != nil {
					return err
				}
				continue
			}
		}

		if arg.Kind() != reflect.Struct {
			return fmt.Errorf("no way of creating type: %+v", arg)
		}

		fn, err := createLoadStructFunc(arg, components, container)
		if err != nil {
			return err
		}

		if err := container.Provide(fn.Interface()); err != nil {
			return err
		}
	}

	// sanity check, make sure there aren't any cyclic dependencies
	_, err = container.Graph.Sort()
	return err
}

type QueryParamError struct {
	Name     string
	Location string
	Reason   string
	Input    string
}

func (e QueryParamError) Error() string {
	return fmt.Sprintf("%s@'%s' error: %s", e.Name, e.Location, e.Reason)
}

func createLoadStructFunc(arg reflect.Type, components openapi.Components, container *container.Container) (reflect.Value, error) {

	params, has := components.Parameters[arg]
	if !has {
		var err error
		params, err = openapi.ParamsFromType(arg, components.Schemas)
		if err != nil {
			return reflect.Value{}, err
		}
		components.Parameters[arg] = params
	}

	inputTypes := []reflect.Type{ctxType}
	// find anything that isn't a query param and try to load it
	for i := 0; i < arg.NumField(); i++ {
		field := arg.Field(i)
		fieldType := field.Type

		// throw an error on private fields
		if !unicode.IsUpper(rune(field.Name[0])) {
			err := fmt.Errorf("struct '%v' must only contain public fields: field '%v' not public", arg, field.Name)
			return reflect.Value{}, err
		}

		queryLocation := openapi.GetParameterType(field.Tag)
		if queryLocation.IsValid() {
			continue
		}

		if container.HasType(fieldType) {
			inputTypes = append(inputTypes, fieldType)
			continue
		}
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		inputTypes = append(inputTypes, fieldType)

		// check to see if there is a jsonBody
		schema, has := components.Schemas[openapi.GetTypeName(fieldType)]
		if !has {
			if fieldType.Kind() != reflect.Struct {
				return reflect.Value{}, fmt.Errorf("unknown type: %v", fieldType)
			}
			// not a recognized json body, so try to create it via
			fn, err := createLoadStructFunc(field.Type, components, container)
			if err != nil {
				return reflect.Value{}, err
			}
			if err := container.Provide(fn.Interface()); err != nil {
				return reflect.Value{}, err
			}
			continue
		}

		// create a provider for the json body
		fn := createJSONBodyLoadFunc(field.Type, schema)
		if !fn.IsValid() || fn.IsZero() {
			return reflect.Value{}, fmt.Errorf("failed to create the load func for: %v", arg)
		}
		if err := container.Provide(fn.Interface()); err != nil {
			return reflect.Value{}, err
		}
	}

	dynamicFuncType := reflect.FuncOf(inputTypes, []reflect.Type{arg, errType}, false)
	dynamicFunc := func(in []reflect.Value) []reflect.Value {
		argObj := reflect.New(arg).Elem()
		popIn := func() reflect.Value {
			val := in[0]
			in = in[1:]
			return val
		}
		ctx, ok := popIn().Interface().(context.Context)
		if !ok {
			err := fmt.Errorf("expected the first arg to be context.Context, got %v", in[0].Type())
			return []reflect.Value{argObj, reflect.ValueOf(err)}
		}

		input, err := router.InputFromCTX(ctx)
		if err != nil {
			return []reflect.Value{argObj, reflect.ValueOf(err)}
		}
		err = func() error {
			for i := 0; i < arg.NumField(); i++ {
				field := arg.Field(i)
				fieldType := field.Type
				queryLocation := openapi.GetParameterType(field.Tag)
				if queryLocation.IsValid() {
					p := params.GetByInAndName(queryLocation.In, queryLocation.Name)
					var fValue reflect.Value
					var err error
					switch p.In {
					case openapi3.ParameterInQuery:
						fValue, err = openapi.LoadQueryParam(input.Request, fieldType, p, container)
					case openapi3.ParameterInPath:
						fValue, err = openapi.LoadPathParam(input.PathParams, p, fieldType, container)
					}
					if err != nil {
						// if this param isn't required we don't care about the error
						// if p.Required {
						var numError *strconv.NumError
						if errors.As(err, &numError) {
							_ = numError.Num
							return QueryParamError{
								Location: p.In,
								Name:     p.Name,
								Reason:   numError.Error(),
								Input:    numError.Num,
							}
						}
						// }
						return fmt.Errorf("failed loading param '%+v': %w", p, err)
					}
					if !fValue.IsValid() {

						return fmt.Errorf("unknown type for type: (%v), %v",
							field.Type.PkgPath(),
							field.Type)
					}

					// _, err = openapi.VarToInterface(fValue.Interface())
					// if err != nil {
					// 	return err
					// }
					// if err := p.Schema.Value.VisitJSON(v); err != nil {
					// 	return err
					// }
					argObj.Field(i).Set(fValue)
					continue
				}

				if len(in) >= 1 {
					// grab the first item, this array is in order that
					// the struct fields were parsed in
					argObj.Field(i).Set(popIn())
				}

			}
			return nil
		}()
		if err != nil {
			return []reflect.Value{argObj, reflect.ValueOf(err)}
		}
		return []reflect.Value{argObj, reflect.Zero(errType)}
	}
	return reflect.MakeFunc(dynamicFuncType, dynamicFunc), nil
}

var ErrRequiredJSONBody = fmt.Errorf("expected a request body")

// createJSONBodyLoadFunc creates a function that can create the type passed in
func createJSONBodyLoadFunc(arg reflect.Type, schema *openapi3.SchemaRef) reflect.Value {
	dynamicFuncType := reflect.FuncOf([]reflect.Type{requestPtrType}, []reflect.Type{arg, errType}, false)
	dynamicFunc := func(in []reflect.Value) []reflect.Value {
		// deref the pointer to the new obj
		argObjPtr := reflect.New(arg)
		argObj := argObjPtr.Elem()

		r, ok := in[0].Interface().(*http.Request)
		if !ok {
			err := fmt.Errorf("expected the first arg to be *http.Request, got %v", in[0].Type())
			return []reflect.Value{argObj, reflect.ValueOf(err)}
		}

		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return []reflect.Value{argObj, reflect.ValueOf(err)}
		}

		if err := json.NewDecoder(bytes.NewBuffer(data)).Decode(argObjPtr.Interface()); err != nil {
			var jsonErr *json.SyntaxError
			if errors.As(err, &jsonErr) {
				input, err := router.InputFromCTX(r.Context())
				if err != nil {
					return []reflect.Value{argObj, reflect.ValueOf(err)}
				}

				body := input.Route.Operation.RequestBody
				if body != nil {
					return []reflect.Value{argObj, reflect.ValueOf(ErrRequiredJSONBody)}
				}

				if body.Value.Required {
					return []reflect.Value{argObj, reflect.ValueOf(ErrRequiredJSONBody)}
				}
				// because is is not required, return an empty result
				return []reflect.Value{argObj, reflect.Zero(errType)}
			}
			if errors.Is(err, io.EOF) {
				if len(schema.Value.Required) == 0 {
					return []reflect.Value{argObj, reflect.Zero(errType)}
				}
			}
			return []reflect.Value{argObj, reflect.ValueOf(ErrRequiredJSONBody)}
		}

		v, err := openapi.VarToInterface(argObj.Interface())
		if err != nil {
			return []reflect.Value{argObj, reflect.ValueOf(err)}
		}
		if err := schema.Value.VisitJSON(v); err != nil {
			return []reflect.Value{argObj, reflect.ValueOf(err)}
		}
		return []reflect.Value{argObj, reflect.Zero(errType)}
	}
	return reflect.MakeFunc(dynamicFuncType, dynamicFunc)
}
