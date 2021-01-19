package openapi

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
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

func (p Parameter) IsValid() bool {
	return p.Name != "" && p.In != ""
}

type Parameters map[string]*openapi3.ParameterRef

var paramTags = []string{
	openapi3.ParameterInPath,
	openapi3.ParameterInQuery,
	openapi3.ParameterInHeader,
	openapi3.ParameterInCookie,
}

// getParameterType will set the correct "in" value from the tag
func GetParameterType(tag reflect.StructTag) Parameter {
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

var errNoLocation = fmt.Errorf("no parameter location")

func paramFromStructField(field reflect.StructField, schemas Schemas) (*openapi3.ParameterRef, error) {
	param := GetParameterType(field.Tag)
	if param.In == "" {
		return nil, fmt.Errorf("field '%v': %w", field.Name, errNoLocation)
	}
	var err error
	for name, fn := range paramFuncTags {
		value, has := field.Tag.Lookup(name)
		param, err = fn(value, has, param)
		if err != nil {
			return nil, err
		}
	}

	param.Schema = schemaFromType(field.Type, nil, schemas)

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
			return nil, err
		}
	}

	return &openapi3.ParameterRef{
		Value: &param.Parameter,
	}, nil
}

var ErrNotStruct = fmt.Errorf("expected a struct")

func ParamsFromType(typ reflect.Type, schemas Schemas) (openapi3.Parameters, error) {
	// TODO: Handle pointer?
	if typ.Kind() != reflect.Struct {
		return openapi3.Parameters{}, fmt.Errorf("got %v: %w", typ.Kind(), ErrNotStruct)
	}

	objParams := openapi3.Parameters{}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		var paramRef *openapi3.ParameterRef
		var err error
		paramRef, err = paramFromStructField(field, schemas)
		if err != nil {
			// ignore this field
			if errors.Is(err, errNoLocation) {
				continue
			}
			return objParams, errors.Wrap(err, typ.String())
		}
		objParams = append(objParams, paramRef)
	}
	return objParams, nil
}

func ParamsFromObj(obj interface{}, schemas Schemas) (openapi3.Parameters, error) {
	typ := reflect.TypeOf(obj)
	return ParamsFromType(typ, schemas)
}

// interfaceSlice converts a slice to an slices of interfaces.
// Used for validating query params, converts ints to floats.
func interfaceSlice(slice interface{}) []interface{} {
	s := reflect.ValueOf(slice)

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

func VarToInterface(obj interface{}) (interface{}, error) {
	o := reflect.ValueOf(obj)
	switch o.Kind() {
	case reflect.Int64:
		return o.Int(), nil
	case reflect.Int:
		return float64(o.Int()), nil
	case reflect.Slice, reflect.Array:
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

type LoadParamInput struct {
	*openapi3filter.RequestValidationInput
	Params []*openapi3.ParameterRef
}

// LoadParamStruct takes a request, the param struct to populate, and the query params.
// The obj will have the fields populated based on the openapi schema
func LoadParamStruct(obj interface{}, input LoadParamInput) (reflect.Value, error) {
	// value := reflect.ValueOf(obj)
	value := reflect.New(reflect.TypeOf(obj)).Elem()
	if value.NumField() != len(input.Params) {
		return value, fmt.Errorf("invalid param and struct field count; param=%d,struct=%d", len(input.Params), value.NumField())
	}

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		p := input.Params[i]
		var fValue reflect.Value
		var err error

		switch p.Value.In {
		// TODO: other query params
		case openapi3.ParameterInQuery:
			fValue, err = LoadQueryParam(input.Request, field.Type(), p.Value, nil)
		case openapi3.ParameterInPath:
			fValue, err = LoadPathParam(input.PathParams, p.Value, nil, nil)
		}

		if err != nil {
			return fValue, err
		}
		if !fValue.IsValid() {
			return fValue, fmt.Errorf("invalid value for type: %v", field.Type())
		}

		v, err := VarToInterface(fValue.Interface())
		if err != nil {
			return fValue, err
		}
		if err := p.Value.Schema.Value.VisitJSON(v); err != nil {
			return fValue, err
		}
		field.Set(fValue)

	}
	return value, nil
}
