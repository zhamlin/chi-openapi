package openapi

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

type Schemas map[string]*openapi3.SchemaRef

func (s Schemas) Inject(schema *openapi3.SchemaRef) *openapi3.SchemaRef {
	if schema.Ref != "" {
		name := strings.TrimLeft(schema.Ref, componentSchemasPath)
		if obj, has := s[name]; has {
			return s.Inject(obj)
		}
	}
	if val := schema.Value; val != nil {
		if val.Items != nil {
			val.Items = s.Inject(val.Items)
		}
		for name, ref := range val.Properties {
			val.Properties[name] = s.Inject(ref)
		}
	}
	return schema
}

// SchemaFromObj returns an openapi3 schema for the object.
// For paramters, use ParamsFromObj.
func SchemaFromObj(schemas Schemas, obj interface{}) *openapi3.SchemaRef {
	typ := reflect.TypeOf(obj)
	return schemaFromType(schemas, typ, obj)
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

var timeType = reflect.TypeOf(time.Time{})
var timeKind = timeType.Kind()

func timeSchema() *openapi3.Schema {
	schema := openapi3.NewSchema()
	schema.Type = "string"
	// https://tools.ietf.org/html/rfc3339#section-5.6
	schema.Format = "date-time"
	return schema
}

func schemaFromType(schemas Schemas, typ reflect.Type, obj interface{}) *openapi3.SchemaRef {
	schema := openapi3.NewSchema()
	name := getSchemaTypeName(typ)

	if schemas != nil {
		// if we've already loaded this type, return a reference
		if obj, has := schemas[name]; has {
			return &openapi3.SchemaRef{
				Ref:   componentSchemasPath + name,
				Value: obj.Value,
			}
		}
	}

	switch typ {
	case timeType:
		return openapi3.NewSchemaRef("", timeSchema())
	}

	switch typ.Kind() {
	case reflect.Interface:
		if obj != nil {
			typ := reflect.ValueOf(obj).Type()
			return schemaFromType(schemas, typ, obj)
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
		// TODO: handle pointers correctly, they should be optional
	case reflect.Array:
		fallthrough
	case reflect.Slice:
		schema.Type = "array"
		schema.Items = schemaFromType(schemas, typ.Elem(), obj)
	case reflect.Struct:
		// handle special structs
		inline := false
		{
			// check to see if the we should inline this schema via the SchemaInline method
			schemaInterface := reflect.TypeOf((*SchemaInline)(nil)).Elem()
			if typ.Implements(schemaInterface) {
				objPtr := reflect.New(typ)

				b := objPtr.Elem().Interface().(SchemaInline)
				inline = b.SchemaInline()
			}
		}
		schema = getSchemaFromStruct(schemas, typ, obj)
		if schemas != nil && !inline {
			schemas[name] = openapi3.NewSchemaRef("", schema)
			return openapi3.NewSchemaRef(componentSchemasPath+name, schemas[name].Value)
		}
	default:
		fmt.Println("DEFAULT", typ.Kind())
	}
	return openapi3.NewSchemaRef("", schema)
}

func getSchemaFromStruct(schemas Schemas, t reflect.Type, obj interface{}) *openapi3.Schema {
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
		// fieldObj := objValue.Field(i).Elem().Type()

		// json name lookup, ignore -, default to field name
		name := field.Name
		if val, ok := field.Tag.Lookup("json"); ok {
			if val == "-" {
				continue
			}
			name = val
		}

		if val, ok := field.Tag.Lookup("required"); ok {
			if tagBoolValue(val) {
				requiredFields = append(requiredFields, name)
			}
		}

		s := &openapi3.SchemaRef{}
		// handle special structs here
		switch field.Type.Kind() {
		case reflect.Slice:
			newObj := obj
			if objValue.IsValid() {
				newObj = objValue.Field(i)
			}
			s = schemaFromType(schemas, field.Type, newObj)
		default:
			if objValue.IsValid() {
				switch objValue.Kind() {
				case reflect.Struct:
					newObj := obj
					if objValue.IsValid() {
						fieldObj := objValue.Field(i)
						if fieldObj.IsValid() {
							newObj = fieldObj.Interface()
						}
					}
					s = schemaFromType(schemas, field.Type, newObj)
				default:
					s = schemaFromType(schemas, field.Type, obj)
				}
			}
		}
		for name, fn := range schemaFuncTags {
			value, has := field.Tag.Lookup(name)
			if err := fn(value, has, s.Value); err != nil {
				// TODO: remove
				panic(err)
			}
		}
		schema.Properties[name] = s
	}

	schema.Required = requiredFields
	return schema
}

type schemaTagFunc func(string, bool, *openapi3.Schema) error

var schemaFuncTags = map[string]schemaTagFunc{
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
