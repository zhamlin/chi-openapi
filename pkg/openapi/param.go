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

// SchemaFromObj returns an openapi3 schema for the object.
// For paramters, use ParamsFromObj.
func SchemaFromObj(schemas Schemas, obj interface{}) *openapi3.SchemaRef {
	typ := reflect.TypeOf(obj)
	return schemaFromType(schemas, typ)
}

type SchemaID interface {
	SchemaID() string
}

const componentSchemasPath = "#/components/schemas/"

func getSchemaTypeName(typ reflect.Type) string {
	// check to see if the name is set via the SchemaID method
	name := typ.Name()
	schemaInterface := reflect.TypeOf((*SchemaID)(nil)).Elem()
	if typ.Implements(schemaInterface) {
		objPtr := reflect.New(typ)
		b := objPtr.Elem().Interface().(SchemaID)
		name = b.SchemaID()
	}
	return name
}

func schemaFromType(schemas Schemas, typ reflect.Type) *openapi3.SchemaRef {
	schema := openapi3.NewSchema()
	name := getSchemaTypeName(typ)

	// if we've already loaded this typ, return a reference
	if _, has := schemas[name]; has {
		return &openapi3.SchemaRef{
			Ref: componentSchemasPath + name,
		}
	}

	switch typ.Kind() {
	case reflect.Interface:
		schema.Type = "object"
	case reflect.String:
		schema.Type = "string"
	case reflect.Bool:
		schema.Type = "boolean"
	case reflect.Float32:
		fallthrough
	case reflect.Float64:
		schema.Type = "number"
	case reflect.Int:
		fallthrough
	case reflect.Int32:
		fallthrough
	case reflect.Int64:
		schema.Type = "integer"
	case reflect.Array:
		fallthrough
	case reflect.Slice:
		schema.Type = "array"
		schema.Items = schemaFromType(schemas, typ.Elem())
	case reflect.Struct:
		schema = getSchemaFromStruct(schemas, typ)
		schemas[name] = openapi3.NewSchemaRef("", schema)
		return openapi3.NewSchemaRef(componentSchemasPath+name, nil)
	}
	return openapi3.NewSchemaRef("", schema)
}

func getSchemaFromStruct(schemas Schemas, t reflect.Type) *openapi3.Schema {
	schema := &openapi3.Schema{
		Type: "object",
	}
	schema.Properties = map[string]*openapi3.SchemaRef{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// json name lookup, ignore -, default to field name
		name := field.Name
		if val, ok := field.Tag.Lookup("json"); ok {
			if val == "-" {
				continue
			}
			name = val
		}

		schema.Properties[name] = schemaFromType(schemas, field.Type)
	}
	return schema
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
			Value: getSchemaFromStruct(Schemas{}, field.Type),
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
