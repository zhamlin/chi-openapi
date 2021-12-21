package openapi

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type Schemas map[string]*openapi3.SchemaRef
type RegisteredTypes map[reflect.Type]TypeOption

type TypeOption struct {
	SchemaRef *openapi3.SchemaRef
	// optional schema, if set ignore schema name and inline this type
	Schema *openapi3.Schema
}

// SchemaFromObj returns an openapi3 schema for the object.
// For paramters, use ParamsFromObj.
func SchemaFromObj(obj interface{}, schemas Schemas, typs RegisteredTypes) *openapi3.SchemaRef {
	typ := reflect.TypeOf(obj)
	return schemaFromType(typ, obj, schemas, typs)
}

// SchemaID is used to override the name of the schema type
type SchemaID interface {
	SchemaID() string
}

// SchemaInline is used to determine whether or not to pull this schema
// out to the schemas collection
type SchemaInline interface {
	SchemaInline() bool
}

const ComponentSchemasPath = "#/components/schemas/"

func GetTypeName(typ reflect.Type) string {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
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

type OpenAPIDescriptor interface {
	OpenAPIDescription() string
}

var (
	stringerType          = reflect.TypeOf((*fmt.Stringer)(nil)).Elem()
	openAPIDescriptorType = reflect.TypeOf((*OpenAPIDescriptor)(nil)).Elem()
	schemaInlineType      = reflect.TypeOf((*SchemaInline)(nil)).Elem()
)

func schemaFromType(typ reflect.Type, obj interface{}, schemas Schemas, typs RegisteredTypes) *openapi3.SchemaRef {
	if typs != nil {
		// handle registered types
		if info, has := typs[typ]; has {
			if info.SchemaRef != nil {
				return info.SchemaRef
			} else if info.Schema != nil {
				return openapi3.NewSchemaRef("", info.Schema)
			} else {
				panic("expected schema or schema ref, got neither")
			}
		}
	}

	name := GetTypeName(typ)
	if schemas != nil {
		// if we've already loaded this type, return a reference
		if obj, has := schemas[name]; has {
			return openapi3.NewSchemaRef(ComponentSchemasPath+name, obj.Value)
		}
	}

	schema := openapi3.NewSchema()
	if typ.Implements(openAPIDescriptorType) {
		descriptor := reflect.ValueOf(obj).Interface().(OpenAPIDescriptor)
		description := descriptor.OpenAPIDescription()
		description = strings.TrimSpace(description)
		description = strings.Trim(description, "\n")
		schema.Description = description
	}

	// custom enumer function, returns an array of its enum types
	if m, has := typ.MethodByName("EnumValues"); has {
		var value reflect.Value
		if obj != nil {
			value = reflect.ValueOf(obj)
		} else {
			value = reflect.New(typ).Elem()
		}
		types := m.Func.Call([]reflect.Value{value})
		if len(types) == 1 && types[0].Kind() == reflect.Slice {
			val := types[0]
			for i := 0; i < val.Len(); i++ {
				v := val.Index(i)
				if v.Type().Implements(stringerType) {
					stringer := v.Interface().(fmt.Stringer)
					schema.Enum = append(schema.Enum, stringer.String())
				}
			}
		}
		// only support one type of enum
		schema.Type = "string"

		if schemas != nil {
			schemas[name] = openapi3.NewSchemaRef("", schema)
			return openapi3.NewSchemaRef(ComponentSchemasPath+name, schemas[name].Value)
		}
		return openapi3.NewSchemaRef("", schema)
	}

	switch typ.Kind() {
	case reflect.Interface:
		if obj != nil {
			v := reflect.TypeOf(obj)
			return schemaFromType(v, obj, schemas, typs)
		}
		schema.Type = "object"
	case reflect.String:
		schema.Type = "string"
	case reflect.Bool:
		schema.Type = "boolean"
	case reflect.Float32:
		schema.Format = "float"
		schema.Type = "number"
	case reflect.Float64:
		schema.Format = "float"
		schema.Type = "number"
	case reflect.Int:
		schema.Type = "integer"
	case reflect.Int32:
		schema.Format = "int32"
		schema.Type = "integer"
	case reflect.Int64:
		schema.Format = "int64"
		schema.Type = "integer"
	case reflect.Ptr:
		if obj != nil {
			newObj := reflect.New(typ.Elem()).Elem().Interface()
			return schemaFromType(typ.Elem(), newObj, schemas, typs)
		}
	case reflect.Slice, reflect.Array:
		schema.Type = "array"
		if obj != nil {
			newObj := reflect.New(typ.Elem()).Elem().Interface()
			schema.Items = schemaFromType(typ.Elem(), newObj, schemas, typs)
		} else {
			schema.Items = schemaFromType(typ.Elem(), nil, schemas, typs)
		}
	case reflect.Map:
		// only support maps with string keys
		if typ.Key().Kind() == reflect.String {
			schema.Type = "object"

			if obj != nil {
				newObj := reflect.New(typ.Elem()).Elem().Interface()
				schema.AdditionalProperties = schemaFromType(typ.Elem(), newObj, schemas, typs)
			}
		}

	case reflect.Struct:
		// handle special structs
		inline := false
		// check to see if the we should inline this schema via the SchemaInline method
		if typ.Implements(schemaInlineType) {
			objPtr := reflect.New(typ)
			b := objPtr.Elem().Interface().(SchemaInline)
			inline = b.SchemaInline()
		}
		newSchema := getSchemaFromStruct(schemas, typs, typ, obj)
		newSchema.Description = schema.Description
		schema = newSchema
		if schemas != nil && !inline {
			schemas[name] = openapi3.NewSchemaRef("", schema)
			return openapi3.NewSchemaRef(ComponentSchemasPath+name, schemas[name].Value)
		}
	}
	return openapi3.NewSchemaRef("", schema)
}

func getSchemaFromStruct(schemas Schemas, typs RegisteredTypes, t reflect.Type, obj interface{}) *openapi3.Schema {
	schema := &openapi3.Schema{
		Type: "object",
	}
	schema.Properties = map[string]*openapi3.SchemaRef{}
	objValue := reflect.Value{}
	if obj != nil {
		objValue = reflect.ValueOf(obj)
	}
	requiredFields := []string{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// json name lookup, ignore -, default to field name
		val, ok := jsonTagName(field.Tag)
		if !ok {
			continue
		}
		name := val

		// allow required to be explicitly set
		if val, ok := field.Tag.Lookup("required"); ok && tagBoolValue(val) {
			requiredFields = append(requiredFields, name)
		} else if field.Type.Kind() != reflect.Ptr {
			// by default everything except pointer types will be required
			// check for required tag
			notRequired := field.Tag.Get("required") == "false"
			if !notRequired {
				requiredFields = append(requiredFields, name)
			}
		}

		s := &openapi3.SchemaRef{}
		// handle special structs here
		switch field.Type.Kind() {
		case reflect.Slice, reflect.Array:
			newObj := obj
			if objValue.IsValid() {
				newObj = objValue.Field(i)
			}
			s = schemaFromType(field.Type, newObj, schemas, typs)
		default:
			if objValue.IsValid() {
				newObj := obj
				fieldObj := objValue.Field(i)
				if fieldObj.IsValid() {
					newObj = fieldObj.Interface()
				}
				s = schemaFromType(field.Type, newObj, schemas, typs)
			} else {
				s = schemaFromType(field.Type, obj, schemas, typs)
			}
		}
		for name, fn := range schemaFuncTags {
			value, has := field.Tag.Lookup(name)
			if s.Value == nil {
				continue
			}
			if err := fn(value, has, s.Value); err != nil {
				// TODO: remove
				panic(err)
			}
		}

		if !s.Value.IsEmpty() {
			schema.Properties[name] = s
		}
	}

	schema.Required = requiredFields
	return schema
}

type schemaTagFunc func(string, bool, *openapi3.Schema) error

var schemaFuncTags = map[string]schemaTagFunc{
	// all
	"nullable": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			s.Nullable = true
		}
		return nil
	},
	// all
	"default": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			switch s.Type {
			case "string":
				s.Default = value
			case "boolean":
				s.Default = tagBoolValue(value)
			case "integer":
				n, err := strconv.Atoi(value)
				if err != nil {
					return err
				}
				s.Default = n
			}
		}
		return nil
	},
	// all
	"readOnly": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			s.ReadOnly = tagBoolValue(value)
		}
		return nil
	},
	// all
	"writeOnly": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			s.WriteOnly = tagBoolValue(value)
		}
		return nil
	},
	// all
	"doc": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			s.Description = value
		}
		return nil
	},
	// all
	"format": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			s.WithFormat(value)
		}
		return nil
	},
	// number
	"min": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			float, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return err
			}
			s.WithMin(float)
		}
		return nil
	},
	// number
	"exclusiveMin": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			s.WithExclusiveMin(tagBoolValue(value))
		}
		return nil
	},
	// number
	"max": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			float, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return err
			}
			s.WithMax(float)
		}
		return nil
	},
	// number
	"exclusiveMax": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			s.WithExclusiveMax(tagBoolValue(value))
		}
		return nil
	},
	// string
	"minLength": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			val, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return err
			}
			s.WithMinLength(val)
		}
		return nil
	},
	// string
	"maxLength": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			val, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return err
			}
			s.WithMaxLength(val)
		}
		return nil
	},
	// string
	"pattern": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			s.WithPattern(value)
		}
		return nil
	},
	// array
	"minItems": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			val, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return err
			}
			s.WithMinItems(val)
		}
		return nil
	},
	// array
	"maxItems": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			val, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return err
			}
			s.WithMaxItems(val)
		}
		return nil
	},
	// array
	"uniqueItems": func(value string, has bool, s *openapi3.Schema) error {
		if has {
			s.WithUniqueItems(tagBoolValue(value))
		}
		return nil
	},
}
