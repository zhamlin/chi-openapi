package openapi

import (
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
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
		if has {
			val := tagBoolValue(value)
			p.Explode = &val
		}
		return p, nil
	},
	"style": func(value string, has bool, p Parameter) (Parameter, error) {
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

func ParamsFromObj(obj interface{}) openapi3.Parameters {
	params := openapi3.Parameters{}
	typ := reflect.TypeOf(obj)
	objValue := reflect.Value{}
	if obj != nil {
		objValue = reflect.ValueOf(obj)
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// get param tags
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

		// get schema
		var fieldValue reflect.Value
		if objValue.IsValid() {
			fieldValue = objValue.Field(i)
		}
		if fieldValue.IsValid() {
			param.Schema = schemaFromType(nil, field.Type, fieldValue)
		} else {
			param.Schema = schemaFromType(nil, field.Type, nil)
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

		paramRef := &openapi3.ParameterRef{
			Value: &param.Parameter,
		}
		params = append(params, paramRef)
	}
	return params
}
