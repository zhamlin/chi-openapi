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

	err := fmt.Errorf("unsupported combo error for query param and location: %v %v",
		p.Style, typKind)
	return reflect.Value{}, err
}

func createTypeFromPathParam(typ reflect.Type, p openapi3.Parameter, info RouteInfo) (reflect.Value, error) {
	urlParam, has := info.URLParams[p.Name]
	if has {
		urlValue, err := stringToValue(urlParam, typ)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("stringToValue: %w", err)
		}
		return urlValue, nil
	}
	return reflect.Value{}, fmt.Errorf("url param not found: %s (%s)", p.Name, typ.String())
}

func createTypeFromParam(typ reflect.Type, p openapi3.Parameter, info RouteInfo) (reflect.Value, error) {
	if p.Schema.Ref != nil {
		// TODO: look up ref in RouteInfo
		return reflect.Value{}, fmt.Errorf("schema ref not supported")
	}
	schema := openapi3.Schema{Schema: p.Schema.Spec}

	switch openapi3.ParameterLocation(p.In) {
	case openapi3.ParameterLocationQuery:

		return createTypeFromQueryParam(typ, p, info, schema)
	case openapi3.ParameterLocationPath:
		return createTypeFromPathParam(typ, p, info)
	default:
		return reflect.Value{}, fmt.Errorf("param location not supported: %s", p.In)
	}
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

	retErr := func(err error) error {
		if c.HasType(typ) {
			// if the container already knows how to create the type
			// don't return any errors, only the inputTypes are needed
			return nil
		}
		return err
	}

	err := reflectUtil.WalkStructWithIndex(typ, func(idx int, field reflect.StructField) error {
		if v := field.Tag.Get("request"); v == "body" {
			inputTypes.Add(structField{
				Type:          field.Type,
				fieldIndex:    idx,
				needProvider:  true,
				isRequestBody: true,
			})
			return nil
		}

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

		if field.Type.Kind() == reflect.Struct {
			// TODO: Check to verify all fields are loadable
			n := field.Type.NumField()
			for i := 0; i < n; i++ {
				f := field.Type.Field(i)
				if !f.IsExported() {
					return retErr(fmt.Errorf(
						"can not create the type: %s: field %s: unexported field: %s",
						field.Type.String(), f.Type.String(), f.Name))
				}
			}

			inputTypes.Add(structField{Type: field.Type, needProvider: true})
			return nil
		}
		return retErr(fmt.Errorf("cannot create field `%s (%s)` for struct: %s",
			field.Name, field.Type.String(), typ.String()))
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
		// if this type check fails panic
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
	fnInfo *fnInfo,
) error {
	fields, err := getStructFields(typ, c)
	if err != nil {
		return err
	}
	// ensure a *http.Request is available
	fields.Add(structField{Type: reqType})

	inputTypes := []reflect.Type{}
	for field := range fields {
		if field.needProvider && field.isRequestBody {
			structField := typ.Field(field.fieldIndex)
			fnInfo.requestBody = structField
			if !c.HasType(field.Type) {
				fn := createProviderForJsonRequestBody(field.Type, loader)
				c.Provide(fn)
			}
		} else if field.needProvider {
			err := createProviderForType(field.Type, c, loader, fnInfo)
			if err != nil {
				return err
			}
		}
		inputTypes = append(inputTypes, field.Type)
	}

	if !c.HasType(typ) {
		fn, err := newStructGenerator(typ, inputTypes, fnInfo)
		if err != nil {
			return err
		}
		c.Provide(fn)
	}
	return nil
}

// newStructGenerator takes a struct type and a set of the inputs it expects
// from a container to set its fields. A new function `fn(inputs...) (typ, error)` is returned
// which will create a new struct and set each field via:
// - createTypeFromParam if the field has a parameter tag
// - from the inputs of the function
func newStructGenerator(typ reflect.Type, inputs []reflect.Type, fnInfo *fnInfo) (any, error) {
	// create a map of types to the index in the input array
	typesToIndex := make(map[reflect.Type]int, len(inputs))
	for i, item := range inputs {
		typesToIndex[item] = i
	}

	type Value struct {
		fieldIdx int
		inputIdx int
		typ      reflect.Type
	}
	values := []Value{}

	type Param struct {
		fieldIdx int
		typ      reflect.Type
		param    openapi3.Parameter
	}
	params := []Param{}

	// check to see if every field on the struct can be created
	err := reflectUtil.WalkStructWithIndex(typ,
		func(idx int, field reflect.StructField) error {
			if location, name := openapi3.GetParameterLocationTag(field); location != "" {
				for _, param := range fnInfo.params {
					if param.Name == name && param.In == string(location) {
						params = append(params, Param{
							fieldIdx: idx,
							typ:      field.Type,
							param:    param,
						})
						return nil
					}
				}
				return fmt.Errorf("could not load param: %s: in: %s", name, location)
			}

			inputIdx, has := typesToIndex[field.Type]
			if !has {
				// TODO: improve error message
				return fmt.Errorf("non param input not in the input array")
			}
			values = append(values, Value{
				fieldIdx: idx,
				inputIdx: inputIdx,
				typ:      field.Type,
			})
			return nil
		})
	if err != nil {
		return nil, err
	}

	fnType := reflect.FuncOf(inputs, []reflect.Type{typ, reflectUtil.ErrType}, false)
	dynamicFunc := func(in []reflect.Value) []reflect.Value {
		// if this function does not have a *http.Request panic
		req := in[typesToIndex[reqType]].Interface().(*http.Request)
		routeInfo, has := GetRouteInfo(req.Context())
		if !has && len(params) > 0 {
			err := fmt.Errorf("missing required openapi info in context")
			return []reflect.Value{reflect.Zero(typ), reflect.ValueOf(err)}
		}

		typObj := reflect.New(typ).Elem()
		for _, p := range params {
			paramValue, err := createTypeFromParam(p.typ, p.param, routeInfo)
			if err != nil {
				return []reflect.Value{reflect.Zero(typ), reflect.ValueOf(err)}
			}
			typObj.Field(p.fieldIdx).Set(paramValue)
		}

		for _, v := range values {
			typObj.Field(v.fieldIdx).Set(in[v.inputIdx])
		}
		return []reflect.Value{typObj, reflect.Zero(reflectUtil.ErrType)}
	}
	fn := reflect.MakeFunc(fnType, dynamicFunc)
	return fn.Interface(), nil
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
	hasReturns  bool
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
	fnInfo.hasReturns = fnType.NumOut() > 0

	fnParams := reflectUtil.GetFuncParams(fnType)
	for _, p := range fnParams {
		if isHttpType(p) {
			// *http.Request and http.ResponseWriter will be passed in
			// later so skip them
			continue
		}
		if p.Kind() != reflect.Struct {
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

		err = createProviderForType(p, router.Container, router.RequestBodyLoader, &fnInfo)
		if err != nil {
			return nil, fnInfo, err
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

	plan, err := router.Container.CreatePlan(fn, reqType, respWriterType)
	if err != nil {
		return nil, fnInfo, err
	}
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := router.Container.RunPlan(plan, &w, r)
		if h := router.ResponseHandler; h != nil {
			if err != nil {
				// TODO:clean up error
				err = fmt.Errorf("%s:%d %s: %w", file, line, name, err)
			}
			h(w, r, resp, err)
		}
	}, fnInfo, nil
}
