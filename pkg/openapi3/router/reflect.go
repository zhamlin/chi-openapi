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
	getFunctionName := func(temp any) string {
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
	val reflect.Value,
	p openapi3.Parameter,
	info RouteInfo,
	schema openapi3.Schema,
) (reflect.Value, error) {
	// TODO: validate p.Style with typ?
	// At this point validateStyleWithType should have already been called

	typ := val.Type()
	typKind := val.Kind()
	// https://spec.openapis.org/oas/v3.1.0#styleValues
	// https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.1.0.md#parameterStyle
	explode := p.Explode
	switch openapi3.ParameterStyle(p.Style) {
	case openapi3.ParameterStyleNone, openapi3.ParameterStyleForm:
		values, has := getParamAsString(info, p.Name, openapi3.ParameterLocation(p.In))

		if !has && schema.Default != nil {
			strDefault, ok := schema.Default.(string)
			if !ok {
				return val, nil
			}
			// treat defaults as a single query param
			explode = false
			values = []string{strDefault}
		}

		if len(values) == 0 {
			// TODO: required should be handled by validator, not this func
			return val, nil
		}

		switch typKind {
		case reflect.Slice, reflect.Array:
			if !explode {
				// explode=   ?color=blue&color=black&color=brown
				// no-explode=?color=blue,black,brown

				// if explode is false then all of the values should be inside
				// a single query param separated by commas.
				values = strings.Split(values[0], ",")
			}
			arrayItemType := typ.Elem()
			for _, value := range values {
				// create new elem of array
				v := reflect.New(arrayItemType).Elem()
				if err := loadValueFromString(v, value); err != nil {
					return val, err
				}
				val = reflect.Append(val, v)
			}
			return val, nil
		default:
			// try to load the type as a single string
			err := loadValueFromString(val, values[0])
			return val, err
		}
	case openapi3.ParameterStyleDeepObject:
		if typKind != reflect.Struct {
			// TODO: Fill in reasons
			// this _should_ never be reachable for the following reasons:
			// -
			return val, fmt.Errorf("non struct type with DeepObject style: this should have been caught before this func")
		}
		return val, reflectUtil.WalkStructWithIndex(typ, func(i int, field reflect.StructField) error {
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
				fieldValue := val.Field(i)
				return loadValueFromString(fieldValue, paramStr[0])
			}
			return nil
		})
	}

	err := fmt.Errorf("unsupported combo error for query param and location: %v %v",
		p.Style, typKind.String())
	return val, err
}

func createTypeFromPathParam(val reflect.Value, p openapi3.Parameter, info RouteInfo) (reflect.Value, error) {
	urlParam, has := info.URLParams[p.Name]
	if has {
		err := loadValueFromString(val, urlParam)
		if err != nil {
			return val, fmt.Errorf("stringToValue: %w", err)
		}
		return val, nil
	}
	return val, fmt.Errorf("url param not found: %s (%s)", p.Name, val.Type().String())
}

func loadTypeFromParam(val reflect.Value, p openapi3.Parameter, info RouteInfo) (reflect.Value, error) {
	if p.Schema.Ref != nil {
		// TODO: look up ref in RouteInfo
		return val, fmt.Errorf("schema ref not supported")
	}
	schema := openapi3.Schema{Schema: p.Schema.Spec}

	switch openapi3.ParameterLocation(p.In) {
	case openapi3.ParameterLocationQuery:
		return createTypeFromQueryParam(val, p, info, schema)
	case openapi3.ParameterLocationPath:
		return createTypeFromPathParam(val, p, info)
	default:
		return val, fmt.Errorf("param location not supported: %s", p.In)
	}
}

func getParamAsString(info RouteInfo, name string, loc openapi3.ParameterLocation) ([]string, bool) {
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
		values, has := info.Request.Header[key]
		return values, has
	case openapi3.ParameterLocationQuery:
		values, has := info.QueryValues[name]
		return values, has
	case openapi3.ParameterLocationCookie:
		c, err := info.Request.Cookie(name)
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

func loadValueFromString(val reflect.Value, str string) error {
	typ := val.Type()
	kind := val.Kind()

	if reflectUtil.TypeImplementsTextUnmarshal(typ) {
		isPtr := kind == reflect.Ptr
		if isPtr && val.IsNil() {
			val.Set(reflect.New(typ.Elem()))
		} else {
			val = val.Addr()
		}

		unmarhsaller := val.Interface().(encoding.TextUnmarshaler)
		err := unmarhsaller.UnmarshalText([]byte(str))
		if err != nil {
			return fmt.Errorf("failed unmarhsalling: %w", err)
		}
		return nil
	}

	switch kind {
	case reflect.String:
		val.Set(reflect.ValueOf(str))
		return nil
	case reflect.Bool:
		// TODO: use strconv.ParseBool
		b, err := internal.BoolFromString(str)
		val.Set(reflect.ValueOf(b))
		return err
	case reflect.Uint:
		i, err := strconv.ParseUint(str, 10, 32)
		val.Set(reflect.ValueOf(uint(i)))
		return err
	case reflect.Uint8:
		i, err := strconv.ParseUint(str, 10, 32)
		val.Set(reflect.ValueOf(uint8(i)))
		return err
	case reflect.Uint16:
		i, err := strconv.ParseUint(str, 10, 32)
		val.Set(reflect.ValueOf(uint16(i)))
		return err
	case reflect.Uint32:
		i, err := strconv.ParseUint(str, 10, 32)
		val.Set(reflect.ValueOf(uint32(i)))
		return err
	case reflect.Uint64:
		i, err := strconv.ParseUint(str, 10, 64)
		val.Set(reflect.ValueOf(i))
		return err
	case reflect.Int:
		i, err := strconv.ParseInt(str, 10, 64)
		val.Set(reflect.ValueOf(int(i)))
		return err
	case reflect.Int8:
		i, err := strconv.ParseInt(str, 10, 32)
		val.Set(reflect.ValueOf(int8(i)))
		return err
	case reflect.Int16:
		i, err := strconv.ParseInt(str, 10, 32)
		val.Set(reflect.ValueOf(int16(i)))
		return err
	case reflect.Int32:
		i, err := strconv.ParseInt(str, 10, 32)
		val.Set(reflect.ValueOf(int32(i)))
		return err
	case reflect.Int64:
		i, err := strconv.ParseInt(str, 10, 64)
		val.Set(reflect.ValueOf(i))
		return err
	case reflect.Float64:
		i, err := strconv.ParseFloat(str, 64)
		val.Set(reflect.ValueOf(i))
		return err
	case reflect.Float32:
		i, err := strconv.ParseFloat(str, 32)
		val.Set(reflect.ValueOf(float32(i)))
		return err
	}
	return fmt.Errorf("can not create type from string: %s", typ.String())
}

type structField struct {
	reflect.Type
	// track the structField via index vs reflect.StructField
	// so this type can be used in a map
	fieldIndex    int
	needProvider  bool
	isRequestBody bool
}

func isRequestBody(field reflect.StructField) bool {
	return field.Tag.Get("request") == "body"
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
		if isRequestBody(field) {
			inputTypes.Add(structField{
				Type:          field.Type,
				fieldIndex:    idx,
				isRequestBody: true,
			})
			return nil
		}

		if isHTTPType(field.Type) {
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
		return retErr(fmt.Errorf("cannot create field `%s: %s` for struct: %s",
			field.Name, field.Type.String(), typ.String()))
	})
	return inputTypes, err
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

	inputTypes := []reflect.Type{}
	for field := range fields {
		if field.isRequestBody && fnInfo.requestBody.Type != nil {
			return fmt.Errorf("found two request bodies %s: %s, %s: %s",
				fnInfo.requestBody.Name,
				fnInfo.requestBody.Type.String(),
				typ.Field(field.fieldIndex).Name,
				field.Type.String(),
			)
		} else if field.isRequestBody {
			structField := typ.Field(field.fieldIndex)
			fnInfo.requestBody = structField
			// skip adding the request body as a function param
			continue
		} else if field.needProvider {
			err := createProviderForType(field.Type, c, loader, fnInfo)
			if err != nil {
				return err
			}
		}
		inputTypes = append(inputTypes, field.Type)
	}

	if !c.HasType(typ) {
		fn, err := newStructGenerator(typ, inputTypes, fnInfo, loader)
		if err != nil {
			return err
		}
		c.Provide(fn)
	}
	return nil
}

var ErrMissingRouteInfo = errors.New("ctx missing RouteInfo")

// newStructGenerator takes a struct type and a set of the inputs it expects
// from a container to set its fields. A new function `fn(inputs...) (typ, error)` is returned
// which will create a new struct and set each field via:
// - createTypeFromParam if the field has a parameter tag
// - from the inputs of the function
func newStructGenerator(
	typ reflect.Type,
	inputs []reflect.Type,
	fnInfo *fnInfo,
	loader RequestBodyLoader,
) (any, error) {
	type Value struct {
		fieldIdx int
		inputIdx int
	}
	values := []Value{}

	type Param struct {
		param    openapi3.Parameter
		fieldIdx int
	}
	params := []Param{}

	// ensure a *http.Request is available
	inputs = append([]reflect.Type{reqType}, inputs...)
	// create a map of types to the index in the input array
	typesToIndex := make(map[reflect.Type]int, len(inputs))
	for i, item := range inputs {
		typesToIndex[item] = i
	}

	hasBody := fnInfo.requestBody.Type != nil
	bodyFieldIndex := -1
	// check to see if every field on the struct can be created
	err := reflectUtil.WalkStructWithIndex(typ,
		func(idx int, field reflect.StructField) error {
			if location, name := openapi3.GetParameterLocationTag(field); location != "" {
				for _, param := range fnInfo.params {
					if param.Name == name && param.In == string(location) {
						params = append(params, Param{
							fieldIdx: idx,
							param:    param,
						})
						return nil
					}
				}
				return fmt.Errorf("could not load param: %s: in: %s", name, location)
			}

			if hasBody && isRequestBody(field) {
				bodyFieldIndex = idx
				return nil
			}

			inputIdx, has := typesToIndex[field.Type]
			if !has {
				return fmt.Errorf("%s is not in the input array", field.Type.String())
			}
			values = append(values, Value{
				fieldIdx: idx,
				inputIdx: inputIdx,
			})
			return nil
		})
	if err != nil {
		return nil, err
	}

	if hasBody && bodyFieldIndex < 0 {
		return nil, fmt.Errorf("%s requires a request body but none was found",
			fnInfo.requestBody.Type.String())
	}

	fnType := reflect.FuncOf(inputs, []reflect.Type{typ, reflectUtil.ErrType}, false)
	dynamicFunc := func(in []reflect.Value) []reflect.Value {
		req := in[0].Interface().(*http.Request)
		routeInfo, has := GetRouteInfo(req.Context())
		if !has && len(params) > 0 {
			return []reflect.Value{reflect.Zero(typ), reflect.ValueOf(ErrMissingRouteInfo)}
		}

		typObj := reflect.New(typ).Elem()
		if bodyFieldIndex >= 0 {
			field := typObj.Field(bodyFieldIndex)
			if err := loader(req, field.Addr().Interface()); err != nil {
				return []reflect.Value{reflect.Zero(typ), reflect.ValueOf(err)}
			}
		}

		for _, p := range params {
			field := typObj.Field(p.fieldIdx)
			value, err := loadTypeFromParam(field, p.param, routeInfo)
			if err != nil {
				return []reflect.Value{reflect.Zero(typ), reflect.ValueOf(err)}
			}
			field.Set(value)
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

func isHTTPType(typ reflect.Type) bool {
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
	info := fnInfo{params: []openapi3.Parameter{}}
	switch handler := fn.(type) {
	case nil:
		return nil, info, errNil
	case func(http.ResponseWriter, *http.Request):
		return handler, info, nil
	case http.HandlerFunc:
		return handler, info, nil
	case http.Handler:
		return handler.ServeHTTP, info, nil
	}

	fnType := reflect.TypeOf(fn)
	if err := container.IsValidRunFunc(fnType); err != nil {
		return nil, info, err
	}
	info.hasReturns = fnType.NumOut() > 0

	fnParams := reflectUtil.GetFuncParams(fnType)
	for _, p := range fnParams {
		if isHTTPType(p) {
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
			return nil, info, err
		}
		info.params = append(info.params, params...)

		err = createProviderForType(p, router.Container, router.RequestBodyLoader, &info)
		if err != nil {
			return nil, info, err
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

	ignore := []any{reqType, respWriterType}
	if body := info.requestBody.Type; body != nil {
		ignore = append(ignore, body)
		// TODO: explain
		if reflectUtil.ArrayKind.Has(body.Kind()) {
			ignore = append(ignore, body.Elem())
		}
	}

	plan, err := router.Container.CreatePlan(fn, ignore...)
	if err != nil {
		return nil, info, err
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
	}, info, nil
}
