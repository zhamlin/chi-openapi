package openapi

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

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
	results := strings.Split(value, ",")
	return results[0], true
}

func strToValue(str string, typ reflect.Type) (reflect.Value, error) {
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

func queryValueFn(value string, typ reflect.Type) (reflect.Value, error) {
	const delim = ","
	v, err := strToValue(value, typ)
	if err != nil {
		return v, err
	}
	if v.IsValid() {
		return v, nil
	}
	switch typ.Kind() {
	case reflect.Array:
		fallthrough
	case reflect.Slice:
		results := strings.Split(value, delim)
		obj := reflect.New(typ).Elem()
		for _, r := range results {
			v, err := queryValueFn(r, typ.Elem())
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

func LoadQueryParam(r *http.Request, typ reflect.Type, param *openapi3.Parameter) (result reflect.Value, err error) {
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
		return queryValueFn(value, typ)
	case queryFormat{true, "form"}:
		// handle structs differently than the rest
		// all of the structs field are going to be inlined
		if typ.Kind() == reflect.Struct {
			obj := reflect.New(typ).Elem()
			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				jsonTag, ok := jsonTagName(field.Tag)
				if !ok {
					continue
				}
				if v, has := q[jsonTag]; has && len(v) == 1 {
					value, err := queryValueFn(v[0], field.Type)
					if err != nil {
						return value, err
					}
					obj.Field(i).Set(value)
				}
			}
			return obj, nil
		}

		values, has := q[param.Name]
		if !has {
			return
		}

		if len(values) == 1 {
			return strToValue(values[0], typ)
		}

		obj := reflect.New(typ).Elem()
		for _, r := range values {
			v, err := strToValue(r, typ.Elem())
			if err != nil {
				return reflect.Value{}, err
			}
			obj = reflect.Append(obj, v)
		}
		return obj, nil
	case queryFormat{true, "deepObject"}:
		fallthrough
	case queryFormat{false, "deepObject"}:
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
			if v, has := q[queryName]; has && len(v) == 1 {
				value, err := queryValueFn(v[0], field.Type)
				if err != nil {
					return value, err
				}
				obj.Field(i).Set(value)
			}
		}
		return obj, nil

	}

	return result, nil
}
