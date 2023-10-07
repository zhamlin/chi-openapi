package jsonschema

import (
	"fmt"
	"reflect"
	"strings"

	reflectUtil "github.com/zhamlin/chi-openapi/internal/reflect"

	"github.com/sv-tools/openapi/spec"
)

func GetTypeName(typ reflect.Type) string {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.Name()
}

func GetFieldName(f reflect.StructField) string {
	jsonTag := f.Tag.Get("json")
	if jsonTag == "-" {
		return ""
	}

	name := strings.Split(jsonTag, ",")[0]
	if name == "" {
		name = f.Name
	}
	return name
}

type Schema struct {
	spec.JsonSchema

	noRef bool
	name  string
}

func (s Schema) WithDescription(desc string) Schema {
	s.Description = desc
	return s
}

func (s *Schema) SetEnum(values []string) {
	for _, v := range values {
		s.Enum = append(s.Enum, v)
	}
}

func (s Schema) Name() string {
	return s.name
}

func (s Schema) NoRef() bool {
	return s.noRef
}

func NewSchema() Schema {
	return Schema{}
}

func NewObjectSchema() Schema {
	schema := NewSchema()
	schema.Type = spec.NewSingleOrArray(spec.ObjectType)
	return schema
}

func NewIntSchema() Schema {
	schema := NewSchema()
	schema.Type = spec.NewSingleOrArray(spec.NumberType)
	schema.Format = spec.Int64Format
	return schema
}

func NewFloatSchema() Schema {
	schema := NewSchema()
	schema.Type = spec.NewSingleOrArray(spec.NumberType)
	schema.Format = spec.FloatFormat
	return schema
}

func NewDateTimeSchema() Schema {
	schema := NewSchema()
	schema.Type = spec.NewSingleOrArray(spec.StringType)
	schema.Format = spec.DateTimeFormat
	return schema
}

func NewStringSchema() Schema {
	schema := NewSchema()
	schema.Type = spec.NewSingleOrArray(spec.StringType)
	return schema
}

func NewSchemer() Schemer {
	return Schemer{
		types: map[reflect.Type]Schema{},

		UseRefs:     true,
		RefPath:     "/schemas/",
		GetTypeName: GetTypeName,
	}
}

type Schemer struct {
	types map[reflect.Type]Schema

	UseRefs     bool
	RefPath     string
	GetTypeName func(reflect.Type) string
}

func (s Schemer) Types() map[reflect.Type]Schema {
	return s.types
}

func (s Schemer) Get(obj any) (Schema, error) {
	if t, ok := obj.(reflect.Type); ok {
		return s.schemaFromType(t)
	}
	return s.schemaFromType(reflect.TypeOf(obj))
}

func (s Schemer) Set(obj any, schema Schema, options ...Option) Schema {
	var typ reflect.Type
	if t, ok := obj.(reflect.Type); ok {
		typ = t
	} else {
		typ = reflect.TypeOf(obj)
	}

	for _, option := range options {
		schema = option(schema)
	}

	if schema.name == "" {
		schema.name = s.GetTypeName(typ)
	}
	s.types[typ] = schema
	return schema
}

func (s Schemer) NewRef(name string) string {
	if name == "" {
		return ""
	}
	return s.RefPath + name
}

type Option func(Schema) Schema

// NoRef will cause this schema to always be used
// directly instead of a reference to it
func NoRef() Option {
	return func(s Schema) Schema {
		s.noRef = true
		return s
	}
}

func Name(name string) Option {
	return func(s Schema) Schema {
		s.name = name
		return s
	}
}

type noRefer interface {
	NoRef()
}

var noReferType = reflectUtil.MakeType[noRefer]()

func (s Schemer) schemaFromType(typ reflect.Type) (Schema, error) {
	if typ == nil {
		return Schema{}, nil
	}

	if schema, has := s.types[typ]; has {
		return schema, nil
	}

	// used for the schema as it requires a reference to an int vs an int directly
	var zeroInt = 0

	typeName := s.GetTypeName(typ)
	schema := NewSchema()
	kind := typ.Kind()
	switch kind {
	case reflect.Bool:
		schema.Type = spec.NewSingleOrArray(spec.BooleanType)
	case reflect.String:
		schema.Type = spec.NewSingleOrArray(spec.StringType)
	case reflect.Int:
		schema.Type = spec.NewSingleOrArray(spec.IntegerType)
	case reflect.Int8:
		schema.Type = spec.NewSingleOrArray(spec.IntegerType)
	case reflect.Int16:
		schema.Type = spec.NewSingleOrArray(spec.IntegerType)
	case reflect.Int32:
		schema.Type = spec.NewSingleOrArray(spec.IntegerType)
		schema.Format = spec.Int32Format
	case reflect.Int64:
		schema.Type = spec.NewSingleOrArray(spec.IntegerType)
		schema.Format = spec.Int64Format
	case reflect.Uint:
		schema.Type = spec.NewSingleOrArray(spec.IntegerType)
		schema.Minimum = &zeroInt
	case reflect.Uint8:
		schema.Type = spec.NewSingleOrArray(spec.IntegerType)
		schema.Minimum = &zeroInt
	case reflect.Uint16:
		schema.Type = spec.NewSingleOrArray(spec.IntegerType)
		schema.Minimum = &zeroInt
	case reflect.Uint32:
		schema.Type = spec.NewSingleOrArray(spec.IntegerType)
		schema.Minimum = &zeroInt
		schema.Format = spec.Int32Format
	case reflect.Uint64:
		schema.Type = spec.NewSingleOrArray(spec.IntegerType)
		schema.Minimum = &zeroInt
		schema.Format = spec.Int64Format
	case reflect.Float32:
		schema.Type = spec.NewSingleOrArray(spec.NumberType)
		schema.Format = spec.FloatFormat
	case reflect.Float64:
		schema.Type = spec.NewSingleOrArray(spec.NumberType)
		schema.Format = spec.FloatFormat
	case reflect.Slice, reflect.Array:
		schema.Type = spec.NewSingleOrArray(spec.ArrayType)
		arrayItemSchema, err := s.schemaFromType(typ.Elem())
		if err != nil {
			return schema, err
		}
		if len(arrayItemSchema.Type) > 0 {
			specOrRef := s.refOrSpec(typ.Elem(), arrayItemSchema, s.UseRefs && !arrayItemSchema.noRef)
			schema.Items = spec.NewBoolOrSchema(true, specOrRef)
		}
	case reflect.Ptr:
		typeSchema, err := s.schemaFromType(typ.Elem())
		if err != nil {
			return schema, err
		}
		// pointers can be null
		typeSchema.Type = append(typeSchema.Type, spec.NullType)
		schema = typeSchema
	case reflect.Map:
		// only support maps with string keys
		if typ.Key().Kind() != reflect.String {
			// TODO: err
		}

		schema.Type = spec.NewSingleOrArray(spec.ObjectType)
		mapItemSchema, err := s.schemaFromType(typ.Elem())
		if err != nil {
			return schema, err
		}
		if len(mapItemSchema.Type) > 0 {
			s := spec.NewRefOrSpec(nil, &spec.Schema{
				JsonSchema: mapItemSchema.JsonSchema,
			})
			// TODO: allow changing this
			schema.AdditionalProperties = spec.NewBoolOrSchema(true, s)
		}
	case reflect.Struct:
		structSchema, err := s.schemaFromStruct(typ)
		if err != nil {
			return schema, err
		}
		schema = structSchema
		schema.name = typeName
		if typ.Implements(noReferType) {
			schema.noRef = true
		}
		s.types[typ] = schema
	case reflect.Interface:
	default:
		return schema, fmt.Errorf("unable to create jsonschema for type: %s: reflect.kind: %s", typ.String(), kind.String())
	}
	return schema, nil
}

func (s Schemer) refOrSpec(t reflect.Type, schema Schema, useRef bool) *spec.RefOrSpec[spec.Schema] {
	specOrRef := spec.NewRefOrSpec(nil, &spec.Schema{
		JsonSchema: schema.JsonSchema,
	})
	// if this type already exists maybe create a ref
	if fieldSchema, has := s.types[t]; has && useRef {
		ref := s.NewRef(fieldSchema.name)
		specOrRef = spec.NewRefOrSpec[spec.Schema](spec.NewRef(ref), nil)
	}
	return specOrRef
}

func (s Schemer) schemaFromStruct(t reflect.Type) (Schema, error) {
	schema := NewSchema()
	schema.Type = spec.NewSingleOrArray(spec.ObjectType)
	schema.Properties = map[string]*spec.RefOrSpec[spec.Schema]{}

	err := reflectUtil.WalkStruct(t, func(field reflect.StructField) error {
		fieldSchema, err := s.schemaFromType(field.Type)
		if err != nil {
			return err
		}

		fieldSchema, err = LoadSchemaOptions(field, fieldSchema)
		if err != nil {
			return err
		}

		fieldName := GetFieldName(field)
		if fieldName != "" {
			shouldUseRef := s.UseRefs && !fieldSchema.noRef
			specOrRef := s.refOrSpec(field.Type, fieldSchema, shouldUseRef)
			schema.Properties[fieldName] = specOrRef
		}
		return nil
	})
	return schema, err
}

func LoadSchemaOptions(field reflect.StructField, schema Schema) (Schema, error) {
	if v := field.Tag.Get("default"); v != "" {
		schema.Default = v
	}
	if v := field.Tag.Get("doc"); v != "" {
		schema.Description = v
	}
	return schema, nil
}
