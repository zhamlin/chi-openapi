package router

import (
	"encoding"
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"path"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"unicode"

	"github.com/zhamlin/chi-openapi/internal"
	reflectUtil "github.com/zhamlin/chi-openapi/internal/reflect"
	"github.com/zhamlin/chi-openapi/pkg/container"
	"github.com/zhamlin/chi-openapi/pkg/jsonschema"
	"github.com/zhamlin/chi-openapi/pkg/openapi3"
)

func getPublicFunctionName(fn any) string {
	getFunctionName := func(temp interface{}) string {
		strs := strings.Split((runtime.FuncForPC(reflect.ValueOf(temp).Pointer()).Name()), ".")
		return strs[len(strs)-1]
	}
	funcName := getFunctionName(fn)
	for _, r := range funcName {
		if unicode.IsLower(r) {
			funcName = ""
		}
		// stop after the first rune
		//nolint
		break
	}
	return funcName
}

var errNil = errors.New("received a nil obj")

func createTypeFromQueryParam(
	typ reflect.Type,
	p openapi3.Parameter,
	info RouteInfo,
	schema openapi3.Schema,
) (reflect.Value, error) {
	// TODO: validate p.Style with typ?
	// At this point validateStyleWithType should have already been called

	typKind := typ.Kind()
	// https://spec.openapis.org/oas/v3.1.0#styleValues
	// https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.1.0.md#parameterStyle
	explode := p.Explode
	switch openapi3.ParameterStyle(p.Style) {
	case openapi3.ParameterStyleNone, openapi3.ParameterStyleForm:
		values, has := getParamAsString(info, p.Name, openapi3.ParameterLocation(p.In))

		if !has && schema.Default != nil {
			strDefault, ok := schema.Default.(string)
			if !ok {
				return reflect.New(typ).Elem(), nil
			}
			// treat defaults as a single query param
			explode = false
			values = []string{strDefault}
		}

		if len(values) == 0 {
			// TODO: required should be handled by validator, not this func
			return reflect.New(typ).Elem(), nil
		}

		switch typKind {
		case reflect.Slice, reflect.Array:
			typValue := reflect.New(typ).Elem()
			if !explode {
				// explode=   ?color=blue&color=black&color=brown
				// no-explode=?color=blue,black,brown

				// if explode is false then all of the values should be inside
				// a single query param separated by commas.
				values = strings.Split(values[0], ",")
			}
			for _, value := range values {
				v, err := stringToValue(value, typ.Elem())
				if err != nil {
					return reflect.Value{}, err
				}
				typValue = reflect.Append(typValue, v)
			}
			return typValue, nil
		default:
			// try to load the type as a single string
			val, err := stringToValue(values[0], typ)
			if err != nil {
				return reflect.Value{}, err
			}
			if val.IsValid() {
				return val, nil
			}
		}
	case openapi3.ParameterStyleDeepObject:
		if typKind != reflect.Struct {
			// TODO: Fill in reasons
			// this _should_ never be reachable for the following reasons:
			// -
			panic("non struct type with DeepObject style: this should have been caught before this func")
		}
		typValue := reflect.New(typ).Elem()
		err := reflectUtil.WalkStructWithIndex(typ, func(i int, field reflect.StructField) error {
			name := jsonschema.GetFieldName(field)
			if name == "" {
				return nil
			}
			queryParamName := fmt.Sprintf("%s[%s]", p.Name, name)
			paramStr, has := getParamAsString(info, queryParamName, openapi3.ParameterLocationQuery)
			if !has {
				// TODO: handle default?
			}
			if has {
				val, err := stringToValue(paramStr[0], field.Type)
				if err != nil {
					return err
				}
				typValue.Field(i).Set(val)
				return nil
			}
			return nil
		})
		if err != nil {
			return reflect.Value{}, err
		}
		return typValue, nil
	}
	// TODO: unsupported combo error
	panic(fmt.Sprintf("TODO:unsupported combo error for query param and location: %v %v", openapi3.ParameterStyle(p.Style) == openapi3.ParameterStyleNone, typKind))
}

func createTypeFromPathParam(typ reflect.Type, p openapi3.Parameter, info RouteInfo) (reflect.Value, error) {
	urlParam, has := info.URLParams[p.Name]
	if has {
		urlValue, err := stringToValue(urlParam, typ)
		if err != nil {
			return reflect.Value{}, nil
		}
		return urlValue, nil
	}
	return reflect.Value{}, nil
}

func createTypeFromParam(typ reflect.Type, p openapi3.Parameter, info RouteInfo) (reflect.Value, error) {
	schema, has := info.OpenAPI.GetParameterSchema(p)
	if !has {
		return reflect.Value{}, fmt.Errorf("could not find schema for parameter: %v", p.Name)
	}

	switch openapi3.ParameterLocation(p.In) {
	case openapi3.ParameterLocationQuery:
		return createTypeFromQueryParam(typ, p, info, schema)
	case openapi3.ParameterLocationPath:
		return createTypeFromPathParam(typ, p, info)
	}
	return reflect.Value{}, nil
}

func getParamAsString(info RouteInfo, name string, loc openapi3.ParameterLocation) ([]string, bool) {
	r := info.Request
	switch loc {
	case openapi3.ParameterLocationPath:
		value, has := info.URLParams[name]
		values := []string{}
		if has {
			values = []string{value}
		}
		return values, has
	case openapi3.ParameterLocationHeader:
		key := textproto.CanonicalMIMEHeaderKey(name)
		values, has := r.Header[key]
		return values, has
	case openapi3.ParameterLocationQuery:
		values, has := info.QueryValues[name]
		return values, has
	case openapi3.ParameterLocationCookie:
		c, err := r.Cookie(name)
		// r.Cookie _should_ only return one err if any: http.ErrNoCookie
		has := err == nil
		values := []string{}
		if has {
			values = []string{c.Value}
		}
		return values, has
	}
	return []string{}, false
}

func stringToValue(str string, typ reflect.Type) (reflect.Value, error) {
	kind := typ.Kind()
	noValue := reflect.Value{}

	if reflectUtil.TypeImplementsTextUnmarshal(typ) {
		isPtr := kind == reflect.Ptr
		if isPtr {
			nonPtrType := typ.Elem()
			typ = nonPtrType
		}

		// create a pointer to typ
		fieldValue := reflect.New(typ)
		unmarhsaller := fieldValue.Interface().(encoding.TextUnmarshaler)
		err := unmarhsaller.UnmarshalText([]byte(str))
		if err != nil {
			return noValue, err
		}

		if isPtr {
			// return pointer to typ
			return fieldValue, nil
		}
		// deref pointer to return typ
		return fieldValue.Elem(), nil
	}

	switch kind {
	case reflect.String:
		return reflect.ValueOf(str), nil
	case reflect.Bool:
		b, err := internal.BoolFromString(str)
		return reflect.ValueOf(b), err
	case reflect.Uint:
		i, err := strconv.ParseUint(str, 10, 32)
		return reflect.ValueOf(uint(i)), err
	case reflect.Uint8:
		i, err := strconv.ParseUint(str, 10, 32)
		return reflect.ValueOf(uint8(i)), err
	case reflect.Uint16:
		i, err := strconv.ParseUint(str, 10, 32)
		return reflect.ValueOf(uint16(i)), err
	case reflect.Uint32:
		i, err := strconv.ParseUint(str, 10, 32)
		return reflect.ValueOf(uint32(i)), err
	case reflect.Uint64:
		i, err := strconv.ParseUint(str, 10, 64)
		return reflect.ValueOf(i), err
	case reflect.Int:
		i, err := strconv.ParseInt(str, 10, 64)
		return reflect.ValueOf(int(i)), err
	case reflect.Int8:
		i, err := strconv.ParseInt(str, 10, 32)
		return reflect.ValueOf(int8(i)), err
	case reflect.Int16:
		i, err := strconv.ParseInt(str, 10, 32)
		return reflect.ValueOf(int16(i)), err
	case reflect.Int32:
		i, err := strconv.ParseInt(str, 10, 32)
		return reflect.ValueOf(int32(i)), err
	case reflect.Int64:
		i, err := strconv.ParseInt(str, 10, 64)
		return reflect.ValueOf(i), err
	case reflect.Float64:
		i, err := strconv.ParseFloat(str, 64)
		return reflect.ValueOf(i), err
	case reflect.Float32:
		i, err := strconv.ParseFloat(str, 32)
		return reflect.ValueOf(float32(i)), err
	}
	return noValue, nil
}

func tryLoadParam(info RouteInfo, field reflect.StructField) (reflect.Value, error) {
	if paramLocation, name := openapi3.GetParameterLocationTag(field); paramLocation != "" {
		param, has := info.OpenAPI.GetParameter(name, info.Operation)
		if !has {
			panic("does not have param")
		}
		value, err := createTypeFromParam(field.Type, param, info)
		if err != nil {
			return reflect.Value{}, err
		}
		if !value.IsValid() {
			return reflect.Value{}, fmt.Errorf("unsupported type for param: %s", field.Type.String())
		}
		return value, nil
	}
	return reflect.Value{}, nil
}

type structField struct {
	reflect.Type
	// track the structField via index vs reflect.StructField
	// so this type can be used in a map
	fieldIndex    int
	needProvider  bool
	isRequestBody bool
}

// getStructFields returns all of the fields on the struct and checks to
// see which ones can be created. The checks are the following:
//   - is the field type in the container?
//   - is the field type a struct not in the container?
//   - does the field have a parameter tag?
//   - else err
func getStructFields(typ reflect.Type, c container.Container) (internal.Set[structField], error) {
	inputTypes := internal.NewSet[structField]()
	err := reflectUtil.WalkStructWithIndex(typ, func(idx int, field reflect.StructField) error {
		if isHttpType(field.Type) {
			// these types will be passed to the container via the args
			// so always add them
			inputTypes.Add(structField{Type: field.Type})
			return nil
		}

		if c.HasType(field.Type) {
			// the container can provide this type so add it to the inputs
			inputTypes.Add(structField{Type: field.Type})
			return nil
		}

		if paramLocation, _ := openapi3.GetParameterLocationTag(field); paramLocation != "" {
			// We dont know what route this is for at this point, so there is no way to verify
			// if this query param is allowed in this handler. The function loading the
			// parameter will handle verifying it
			return nil
		}

		if v := field.Tag.Get("request"); v == "body" {
			inputTypes.Add(structField{
				Type:          field.Type,
				fieldIndex:    idx,
				needProvider:  true,
				isRequestBody: true,
			})
			return nil
		}

		if field.Type.Kind() == reflect.Struct {
			// TODO: Check to verify all fields are loadable
			n := field.Type.NumField()
			for i := 0; i < n; i++ {
				f := field.Type.Field(i)
				if !f.IsExported() {
					return fmt.Errorf(
						"can not create the type: %s: field %s: unexported field: %s",
						field.Type.String(), f.Type.String(), f.Name)
				}
			}

			inputTypes.Add(structField{Type: field.Type, needProvider: true})
			return nil
		}
		return fmt.Errorf("cannot create field `%s (%s)` for struct: %s",
			field.Name, field.Type.String(), typ.String())
	})
	return inputTypes, err
}

type RequestBodyLoader func(r *http.Request, obj any) error

func createProviderForJsonRequestBody(
	t reflect.Type,
	loader RequestBodyLoader,
) any {
	fnType := reflect.FuncOf([]reflect.Type{reqType}, []reflect.Type{t, reflectUtil.ErrType}, false)
	dynamicFunc := func(in []reflect.Value) []reflect.Value {
		// If this type check fails panic. If this function does not
		// have a *http.Request at this point something has gone wrong
		req := in[0].Interface().(*http.Request)
		obj := reflect.New(t)
		if err := loader(req, obj.Interface()); err != nil {
			return []reflect.Value{reflect.Zero(t), reflect.ValueOf(err)}
		}
		// reflect.New returns a pointer so return the object directly
		return []reflect.Value{reflect.Indirect(obj), reflect.Zero(reflectUtil.ErrType)}
	}
	fn := reflect.MakeFunc(fnType, dynamicFunc)
	return fn.Interface()
}

func createProviderForType(
	typ reflect.Type,
	c container.Container,
	loader RequestBodyLoader,
) (*reflect.StructField, error) {
	fields, err := getStructFields(typ, c)
	if err != nil {
		return nil, err
	}
	// ensure a *http.Request is available
	fields.Add(structField{Type: reqType})

	var requestBody *reflect.StructField
	inputTypes := []reflect.Type{}
	for field := range fields {
		if field.needProvider && field.isRequestBody {
			structField := typ.Field(field.fieldIndex)
			requestBody = &structField
			fn := createProviderForJsonRequestBody(field.Type, loader)
			c.Provide(fn)
		} else if field.needProvider {
			f, err := createProviderForType(field.Type, c, loader)
			if err != nil {
				return nil, err
			}
			requestBody = f
		}
		inputTypes = append(inputTypes, field.Type)
	}

	fn := newStructGenerator(typ, inputTypes)
	c.Provide(fn)
	return requestBody, nil
}

// newStructGenerator takes a struct type and a set of the inputs it expects
// from a container to set its fields. A new function `fn(inputs...) (typ, error)` is returned
// which will create a new struct and set each field via:
// - tryLoadParam if the field has a parameter tag
// - from the inputs of the function
func newStructGenerator(typ reflect.Type, inputs []reflect.Type) any {
	// create a map of types to the index in the input array
	typesToIndex := make(map[reflect.Type]int, len(inputs))
	for i, item := range inputs {
		typesToIndex[item] = i
	}

	getType := func(in []reflect.Value, t reflect.Type) (reflect.Value, error) {
		idx, has := typesToIndex[t]
		if !has {
			return reflect.Value{}, fmt.Errorf("%s does not exist in the type map", t.String())
		}
		return in[idx], nil
	}

	fnType := reflect.FuncOf(inputs, []reflect.Type{typ, reflectUtil.ErrType}, false)
	dynamicFunc := func(in []reflect.Value) []reflect.Value {
		typObj := reflect.New(typ).Elem()
		err := func() error {
			reqVal, err := getType(in, reqType)
			if err != nil {
				return err
			}
			// If this type check fails panic. If this function does not
			// have a *http.Request at this point something has gone wrong as getType
			// should have checked for it
			req := reqVal.Interface().(*http.Request)
			routeInfo, has := GetRouteInfo(req.Context())
			if !has {
				return fmt.Errorf("missing required openapi info in context")
			}
			return reflectUtil.WalkStructWithIndex(typ,
				func(idx int, field reflect.StructField) error {
					paramValue, err := tryLoadParam(routeInfo, field)
					if err != nil {
						return err
					}

					if !paramValue.IsValid() {
						// not a param so get it from the inputs
						val, err := getType(in, field.Type)
						if err != nil {
							return err
						}
						paramValue = val
					}
					typObj.Field(idx).Set(paramValue)
					return nil
				})
		}()
		if err != nil {
			return []reflect.Value{reflect.Zero(typ), reflect.ValueOf(err)}
		}
		return []reflect.Value{typObj, reflect.Zero(reflectUtil.ErrType)}
	}
	fn := reflect.MakeFunc(fnType, dynamicFunc)
	return fn.Interface()
}

var (
	reqType        = reflectUtil.MakeType[*http.Request]()
	respWriterType = reflectUtil.MakeType[http.ResponseWriter]()
)

func isHttpType(typ reflect.Type) bool {
	return typ == reqType || typ == respWriterType
}

type fnInfo struct {
	params      []openapi3.Parameter
	requestBody reflect.StructField
}

// httpHandlerFromFn takes a fn and DepRouter, and returns
// an http.HandlerFunc along with any parameters found.
// Any input params of fn are created via the container.
// fn must be:
// - http.Handler
// - http.HandlerFunc
// - func(http.ResponseWriter, *http.Request)
// - func(...) _
// - func(...) (_, error)
func httpHandlerFromFn(fn any, router *DepRouter) (http.HandlerFunc, fnInfo, error) {
	fnInfo := fnInfo{params: []openapi3.Parameter{}}
	switch handler := fn.(type) {
	case nil:
		return nil, fnInfo, errNil
	case func(http.ResponseWriter, *http.Request):
		return handler, fnInfo, nil
	case http.HandlerFunc:
		return handler, fnInfo, nil
	case http.Handler:
		return handler.ServeHTTP, fnInfo, nil
	}

	fnType := reflect.TypeOf(fn)
	if err := container.IsValidRunFunc(fnType); err != nil {
		return nil, fnInfo, err
	}

	fnParams := reflectUtil.GetFuncParams(fnType)
	for _, p := range fnParams {
		if isHttpType(p) {
			// *http.Request and http.ResponseWriter will be passed in
			// to c.Run() later so skip them
			continue
		}
		if router.container.HasType(p) {
			// container already knows how to create this type
			continue
		}
		if p.Kind() != reflect.Struct {
			// this type is not in the container, nor is a struct.
			// ignore this and let container.CreatePlan give a better error
			continue
		}

		// must be a struct at this point
		// check for any parameters from the given fns input
		params, err := openapi3.ParamsFromStruct(router.router.schemer, p)
		if err != nil {
			return nil, fnInfo, err
		}
		fnInfo.params = append(fnInfo.params, params...)

		f, err := createProviderForType(p, router.container, router.requestBodyLoader)
		if err != nil {
			return nil, fnInfo, err
		}
		if f != nil {
			fnInfo.requestBody = *f
		}
	}

	// TODO: move to internal/runtime
	// get func information for any errors later
	pc := reflect.ValueOf(fn).Pointer()
	runtimeFn := runtime.FuncForPC(pc)
	file, line := runtimeFn.FileLine(pc)
	strs := strings.Split(runtimeFn.Name(), ".")
	name := strs[len(strs)-2] + ":" + strs[len(strs)-1]
	file = path.Base(file)

	plan, err := router.container.CreatePlan(fn, reqType, respWriterType)
	if err != nil {
		return nil, fnInfo, err
	}
	return func(w http.ResponseWriter, r *http.Request) {
		writer := container.MustCast[http.ResponseWriter](w)
		resp, err := router.container.RunPlan(plan, writer, r)
		if h := router.requestHandler; h != nil {
			if err != nil {
				// TODO:clean up error
				err = fmt.Errorf("%s:%d %s: %w", file, line, name, err)
			}
			h(w, r, resp, err)
		}
	}, fnInfo, nil
}
