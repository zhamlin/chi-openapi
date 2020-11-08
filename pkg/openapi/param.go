package openapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/pkg/errors"
)

func tagBoolValue(tag string) bool {
	var validValues = []string{"1", "true"}
	for _, value := range validValues {
		if value == tag {
			return true
		}
	}
	return false
}

type Parameter struct {
	openapi3.Parameter
}

type Parameters map[string]*openapi3.ParameterRef

var paramTags = []string{"path", "query", "header", "cookie"}

// getParamaterType will set the correct "in" value from the tag
func getParamaterType(tag reflect.StructTag) Parameter {
	for _, name := range paramTags {
		if tagValue, ok := tag.Lookup(name); ok {
			return Parameter{openapi3.Parameter{
				In:   name,
				Name: tagValue,
			}}
		}
	}
	return Parameter{}
}

type paramTagFunc func(string, bool, Parameter) (Parameter, error)

var paramFuncTags = map[string]paramTagFunc{
	"required": func(value string, has bool, p Parameter) (Parameter, error) {
		if has {
			p.Required = tagBoolValue(value)
		}

		// https://github.com/OAI/OpenAPI-Specification/blob/master/versions/3.0.3.md#parameterObject
		if p.In == "path" {
			p.Required = true
		}
		return p, nil
	},
	"doc": func(value string, has bool, p Parameter) (Parameter, error) {
		if has {
			p.Description = value
		}
		return p, nil
	},
	// param only
	"explode": func(value string, has bool, p Parameter) (Parameter, error) {
		if p.In == "query" {
			t := true
			p.Explode = &t
		}
		if has {
			val := tagBoolValue(value)
			p.Explode = &val
		}
		return p, nil
	},
	"style": func(value string, has bool, p Parameter) (Parameter, error) {
		if p.In == "query" {
			p.Style = "form"
		}
		if has {
			p.Style = value
		}
		return p, nil
	},
}

func getParamaterOptions(tag reflect.StructTag) Parameter {
	for _, name := range paramTags {
		if tagValue, ok := tag.Lookup(name); ok {
			return Parameter{openapi3.Parameter{
				In:   name,
				Name: tagValue,
			}}
		}
	}
	return Parameter{}
}

const componentParamsPath = "#/components/parameters/"

func paramFromStructField(field reflect.StructField, obj reflect.Value, params Parameters) *openapi3.ParameterRef {
	name := getTypeName(field.Type)
	if params != nil {
		// if we've already loaded this type, return a reference
		if obj, has := params[name]; has {
			return &openapi3.ParameterRef{
				Ref:   componentParamsPath + name,
				Value: obj.Value,
			}
		}
	}

	param := getParamaterType(field.Tag)
	var err error
	for name, fn := range paramFuncTags {
		value, has := field.Tag.Lookup(name)
		param, err = fn(value, has, param)
		if err != nil {
			// TODO: remove
			panic(err)
		}
	}

	if obj.IsValid() {
		param.Schema = schemaFromType(field.Type, nil, nil)
	} else {
		param.Schema = schemaFromType(field.Type, nil, nil)
	}

	// load schema tags
	for name, fn := range schemaFuncTags {
		value, has := field.Tag.Lookup(name)
		if param.Schema == nil || param.Schema.Value == nil {
			continue
		}
		// keep document to parma level not schema
		if name == "doc" {
			continue
		}
		if err := fn(value, has, param.Schema.Value); err != nil {
			// TODO: remove
			panic(err)
		}
	}

	if params != nil {
		ref := &openapi3.ParameterRef{
			Value: &param.Parameter,
		}

		params[name] = ref
		return &openapi3.ParameterRef{
			Ref:   componentParamsPath + name,
			Value: ref.Value,
		}
	}

	return &openapi3.ParameterRef{
		Value: &param.Parameter,
	}
}

func ParamsFromType(typ reflect.Type, obj reflect.Value, params Parameters) openapi3.Parameters {
	if typ.Kind() != reflect.Struct {
		return openapi3.Parameters{}
	}

	objParams := openapi3.Parameters{}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		var paramRef *openapi3.ParameterRef
		if obj.IsValid() {
			fieldObj := obj.Field(i)
			paramRef = paramFromStructField(field, fieldObj, params)
		} else {
			paramRef = paramFromStructField(field, obj, params)
		}
		objParams = append(objParams, paramRef)
	}
	return objParams
}

func ParamsFromObj(obj interface{}, params Parameters) openapi3.Parameters {
	typ := reflect.TypeOf(obj)
	value := reflect.ValueOf(obj)
	return ParamsFromType(typ, value, params)
}

// interfaceSlice converts a slice to an slices of interfaces.
// Used for validating query params, converts ints to floats.
func interfaceSlice(slice interface{}) []interface{} {
	s := reflect.ValueOf(slice)
	if s.Kind() != reflect.Slice {
		panic("InterfaceSlice() given a non-slice type")
	}

	ret := make([]interface{}, s.Len())
	for i := 0; i < s.Len(); i++ {
		switch s.Index(i).Kind() {
		case reflect.String:
			ret[i] = s.Index(i).Interface()
		case reflect.Bool:
			ret[i] = s.Index(i).Interface()
		case reflect.Float64:
			ret[i] = s.Index(i).Interface()
		case reflect.Int64:
			ret[i] = float64(s.Index(i).Int())
		case reflect.Int:
			ret[i] = float64(s.Index(i).Int())
		}
	}
	return ret
}

func varToInterface(obj interface{}) (interface{}, error) {
	t := reflect.TypeOf(obj)
	switch t.Kind() {
	case reflect.Slice:
		return interfaceSlice(obj), nil
	case reflect.Struct:
		// TODO: benchmark this
		b, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		data := map[string]interface{}{}
		if err := json.Unmarshal(b, &data); err != nil {
			return nil, err
		}
		return data, nil
	default:
		return obj, nil
	}
}

// LoadParamStruct takes a request, the param struct to populate, and the query params.
// The obj will have the fields populated based on the openapi schema
func LoadParamStruct(r *http.Request, obj interface{}, params []*openapi3.ParameterRef) (reflect.Value, error) {
	value := reflect.New(reflect.TypeOf(obj)).Elem()
	if value.NumField() != len(params) {
		return value, fmt.Errorf("invalid param and struct field count")
	}

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		p := params[i]
		var value reflect.Value
		var err error

		switch p.Value.In {
		// TODO: other query params
		case openapi3.ParameterInQuery:
			value, err = LoadQueryParam(r, field.Type(), p.Value)
		}

		if err != nil || !value.IsValid() {
			return value, errors.Wrapf(err, "failed loading param '%v', style: %v, explode: %v",
				p.Value.Name, p.Value.Style, *p.Value.Explode)
		}

		if value.IsValid() {
			v, err := varToInterface(value.Interface())
			if err != nil {
				return value, err
			}
			if err := p.Value.Schema.Value.VisitJSON(v); err != nil {
				return value, err
			}
			field.Set(value)
		} else {
			return value, fmt.Errorf("invalid value for type: %v", field.Type())
		}

	}

	return value, nil
}
