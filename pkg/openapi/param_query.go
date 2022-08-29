package openapi

import (
	"encoding"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/zhamlin/chi-openapi/pkg/container"

	"github.com/getkin/kin-openapi/openapi3"
)

type queryFormat struct {
	Explode bool
	Style   string
}

func jsonTagName(tag reflect.StructTag) (string, bool) {
	value, ok := tag.Lookup("json")
	if !ok {
		return "", ok
	}
	if value == "-" {
		return "", false
	}
	results := strings.Split(value, ",")
	return results[0], true
}

var (
	textUnmarshaller = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

func valueFromString(str string, typ reflect.Type) (reflect.Value, error) {
	typPtr := reflect.PtrTo(typ)
	if typPtr.Implements((textUnmarshaller)) {
		typPtrObj := reflect.New(typ)
		unmarhsaller := typPtrObj.Interface().(encoding.TextUnmarshaler)
		err := unmarhsaller.UnmarshalText([]byte(str))
		return reflect.ValueOf(typPtrObj.Elem().Interface()), err
	}
	return reflect.Value{}, nil
}

func strToValue(str string, typ reflect.Type, c *container.Container, schema *openapi3.Schema) (reflect.Value, error) {
	if c != nil && c.HasType(typ) {
		value, err := c.CreateType(typ, str)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(value), nil
	}

	strValue, err := valueFromString(str, typ)
	if err != nil {
		return reflect.Value{}, err
	}
	if strValue.IsValid() {
		return strValue, nil
	}

	switch typ.Kind() {
	case reflect.String:
		return reflect.ValueOf(str), nil
	case reflect.Bool:
		return reflect.ValueOf(tagBoolValue(str)), nil
	case reflect.Int:
		i, err := strconv.ParseInt(str, 10, 32)
		return reflect.ValueOf(int(i)), err
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
	return reflect.Value{}, nil
}

func queryValueFn(value string, typ reflect.Type, c *container.Container, schema *openapi3.Schema) (reflect.Value, error) {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	v, err := strToValue(value, typ, c, schema)
	if err != nil {
		return v, err
	}
	if v.IsValid() {
		return v, nil
	}

	const delim = ","
	switch typ.Kind() {
	case reflect.Slice, reflect.Array:
		results := strings.Split(value, delim)
		obj := reflect.New(typ).Elem()
		for _, r := range results {
			if r == "" {
				continue
			}

			// TODO: text marhsaller check
			v, err := queryValueFn(r, typ.Elem(), c, schema)
			if err != nil {
				return reflect.Value{}, err
			}
			obj = reflect.Append(obj, v)
		}
		return obj, nil
	}

	return reflect.Value{}, fmt.Errorf("unknown type: %v", typ)
}

// https://swagger.io/docs/specification/serialization/

func LoadQueryParam(r *http.Request, typ reflect.Type, param *openapi3.Parameter, c *container.Container) (result reflect.Value, err error) {
	if param == nil {
		return result, nil
	}

	format := queryFormat{Explode: param.Explode != nil && *param.Explode, Style: param.Style}
	q := r.URL.Query()
	switch format {
	case queryFormat{false, "form"}:
		if typ.Kind() == reflect.Struct {
			return result, fmt.Errorf("structs are not supported for this format")
		}
		value := q.Get(param.Name)
		return queryValueFn(value, typ, c, param.Schema.Value)
	case queryFormat{true, "form"}:
		objPtr := reflect.New(typ)
		obj := objPtr.Elem()
		isTextUnmarshaller := objPtr.Type().Implements((textUnmarshaller))

		// handle structs differently than the rest
		// all of the structs field are going to be inlined
		if typ.Kind() == reflect.Struct && !isTextUnmarshaller {

			// check for textUnmarshaller

			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				jsonTag, ok := jsonTagName(field.Tag)
				if !ok {
					continue
				}
				v, has := q[jsonTag]
				if has && len(v) == 1 {
					value, err := queryValueFn(v[0], field.Type, c, param.Schema.Value)
					if err != nil {
						return value, err
					}
					obj.Field(i).Set(value)
					continue
				}
				if !has {
					defaultTag, ok := field.Tag.Lookup("default")
					if ok {
						value := reflect.Value{}
						switch field.Type.Kind() {
						case reflect.Int:
							n, err := strconv.Atoi(defaultTag)
							if err != nil {
								return value, err
							}
							value = reflect.ValueOf(n)
						case reflect.String:
							value = reflect.ValueOf(defaultTag)
						}
						obj.Field(i).Set(value)
					}
				}
			}
			return obj, nil
		}

		values, has := q[param.Name]
		if !has {
			if param.Required {
				return result, fmt.Errorf("query param '%v' is required", param.Name)
			}
			if defValue := param.Schema.Value.Default; defValue != nil {
				// if this type implements TextUnmarshaler and the default value is a string,
                // attempt to unmarhsal the default value into the type
				if isTextUnmarshaller {
					switch v := defValue.(type) {
					case string:
						newTyp := reflect.New(typ).Interface().(encoding.TextUnmarshaler)
						err := newTyp.UnmarshalText([]byte(v))
						return reflect.ValueOf(newTyp).Elem(), err
					}
				}
				return reflect.ValueOf(defValue), nil
			}
			return reflect.New(typ).Elem(), nil
		}

		if len(values) == 1 && param.Schema.Value.Type != "array" {
			return strToValue(values[0], typ, c, param.Schema.Value)
		}

		for _, r := range values {
			v, err := strToValue(r, typ.Elem(), c, param.Schema.Value)
			if err != nil {
				return reflect.Value{}, err
			}
			obj = reflect.Append(obj, v)
		}
		return obj, nil
	case queryFormat{false, "deepObject"}, queryFormat{true, "deepObject"}:
		if typ.Kind() != reflect.Struct {
			return result, fmt.Errorf("deepObject only supports structs")
		}

		obj := reflect.New(typ).Elem()
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			jsonTag, ok := jsonTagName(field.Tag)
			if !ok {
				continue
			}
			queryName := fmt.Sprintf("%s[%s]", param.Name, jsonTag)
			v, has := q[queryName]
			if has && len(v) == 1 {
				value, err := queryValueFn(v[0], field.Type, c, param.Schema.Value)
				if err != nil {
					return value, err
				}
				obj.Field(i).Set(value)
				continue
			}

			if !has {
				defaultTag, ok := field.Tag.Lookup("default")
				if ok {
					value := reflect.Value{}
					strValue, err := valueFromString(defaultTag, field.Type)
					if err != nil {
						return reflect.Value{}, err
					}
					if strValue.IsValid() {
						obj.Field(i).Set(strValue)
						continue
					}

					switch field.Type.Kind() {
					case reflect.Int:
						n, err := strconv.Atoi(defaultTag)
						if err != nil {
							return value, err
						}
						value = reflect.ValueOf(n)
					case reflect.String:
						value = reflect.ValueOf(defaultTag)
					}
					obj.Field(i).Set(value)
				}
			}
		}
		return obj, nil

	}

	return result, nil
}
