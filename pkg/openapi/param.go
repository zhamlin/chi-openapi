package openapi

import (
	"reflect"
	"strconv"

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

var paramTags = []string{"path", "query", "header"}

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
	"format": func(value string, has bool, p Parameter) (Parameter, error) {
		// TODO: allow this to be overriden by structs
		if has {
			// TODO(zhamlin): make sure format is a valid type
			p.Schema.Value.Format = value
		}
		return p, nil
	},
	"minItems": func(value string, has bool, p Parameter) (Parameter, error) {
		// TODO: allow this to be overriden by structs
		if has {
			minItems, err := strconv.ParseUint(value, 0, 64)
			if err != nil {
				return p, err
			}
			// TODO(zhamlin): make sure format is a valid type
			p.Schema.Value.MinItems = minItems
		}
		return p, nil
	},
	"maxItems": func(value string, has bool, p Parameter) (Parameter, error) {
		if has {
			max, err := strconv.ParseUint(value, 0, 64)
			if err != nil {
				return p, err
			}
			p.Schema.Value.MaxItems = &max
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

func getParamSchemaFromField(field reflect.StructField) *openapi3.Schema {
	schema := openapi3.NewSchema()
	switch field.Type.Kind() {
	case reflect.String:
		schema.Type = "string"
	case reflect.Bool:
		schema.Type = "boolean"
	case reflect.Array:
	case reflect.Slice:
		schema.Type = "array"
		schema.Items = &openapi3.SchemaRef{
			Value: openapi3.NewSchema(),
		}
		// TODO: replace with func to lookup this from a switch statement
		schema.Items.Value.Type = field.Type.Elem().Kind().String()
	case reflect.Struct:
		// check for content type tag
		schema.Type = "object"
		schema.Items = &openapi3.SchemaRef{
			Value: getSchemaFromStruct(Schemas{}, field.Type, nil),
		}
	default:
		// TODO: zhamlin handle embeded via allOf?
		// fmt.Printf("OTHER: %+v\n", field.Type.Kind())
	}
	return schema
}

func ParamsFromObj(obj interface{}) openapi3.Parameters {
	params := openapi3.Parameters{}
	typ := reflect.TypeOf(obj)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		param := getParamaterType(field.Tag)

		schema := openapi3.SchemaRef{
			Value: getParamSchemaFromField(field),
		}
		param.Schema = &schema

		var err error
		for name, fn := range paramFuncTags {
			value, has := field.Tag.Lookup(name)
			param, err = fn(value, has, param)
			if err != nil {
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
