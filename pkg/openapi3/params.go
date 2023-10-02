package openapi3

import (
	"fmt"
	"reflect"

	"github.com/sv-tools/openapi/spec"
	"github.com/zhamlin/chi-openapi/internal"
	reflectUtil "github.com/zhamlin/chi-openapi/internal/reflect"
	"github.com/zhamlin/chi-openapi/pkg/jsonschema"
)

func ParamsFromStruct(schemer jsonschema.Schemer, obj any) ([]Parameter, error) {
	if t, ok := obj.(reflect.Type); ok {
		return paramsFromStruct(schemer, t)
	}
	return paramsFromStruct(schemer, reflect.TypeOf(obj))
}

// https://spec.openapis.org/oas/v3.1.0#styleValues
type ParameterStyle string

const (
	ParameterStyleNone               ParameterStyle = ""
	ParameterStyleMatrix             ParameterStyle = "matrix"
	ParameterStyleLabel              ParameterStyle = "label"
	ParameterStyleForm               ParameterStyle = "form"
	ParameterStyleSimple             ParameterStyle = "simple"
	ParameterStyleSpaceDelimited     ParameterStyle = "spaceDelimited"
	ParameterStyleSpacePipeDelimited ParameterStyle = "pipeDelimited"
	ParameterStyleDeepObject         ParameterStyle = "deepObject"
)

func ParameterStyleFromString(str string) (ParameterStyle, error) {
	switch ParameterStyle(str) {
	case ParameterStyleNone:
		return ParameterStyleNone, nil
	case ParameterStyleMatrix:
		return ParameterStyleMatrix, nil
	case ParameterStyleLabel:
		return ParameterStyleLabel, nil
	case ParameterStyleForm:
		return ParameterStyleForm, nil
	case ParameterStyleSimple:
		return ParameterStyleSimple, nil
	case ParameterStyleSpaceDelimited:
		return ParameterStyleSpaceDelimited, nil
	case ParameterStyleSpacePipeDelimited:
		return ParameterStyleSpacePipeDelimited, nil
	case ParameterStyleDeepObject:
		return ParameterStyleDeepObject, nil
	}
	return ParameterStyleNone, fmt.Errorf("invalid ParameterStyle: %s", str)
}

type ParameterLocation string

const (
	ParameterLocationNone   ParameterLocation = ""
	ParameterLocationPath   ParameterLocation = spec.InPath
	ParameterLocationQuery  ParameterLocation = spec.InQuery
	ParameterLocationHeader ParameterLocation = spec.InHeader
	ParameterLocationCookie ParameterLocation = spec.InCookie
)

func paramDefaultStyle(loc ParameterLocation) ParameterStyle {
	switch loc {
	case ParameterLocationPath:
		return ParameterStyleSimple
	case ParameterLocationQuery:
		return ParameterStyleForm
	case ParameterLocationHeader:
		return ParameterStyleSimple
	case ParameterLocationCookie:
		return ParameterStyleForm
	}
	return ParameterStyleNone
}

func ParameterLocationFromString(str string) (ParameterLocation, error) {
	switch ParameterLocation(str) {
	case ParameterLocationPath:
		return ParameterLocationPath, nil
	case ParameterLocationQuery:
		return ParameterLocationQuery, nil
	case ParameterLocationHeader:
		return ParameterLocationHeader, nil
	case ParameterLocationCookie:
		return ParameterLocationCookie, nil
	}
	return ParameterLocationNone, fmt.Errorf("invalid ParameterNone: %s", str)
}

var paramLocationTags = []ParameterLocation{
	ParameterLocationPath,
	ParameterLocationQuery,
	ParameterLocationHeader,
	ParameterLocationCookie,
}

func GetParameterLocationTag(field reflect.StructField) (ParameterLocation, string) {
	for _, typ := range paramLocationTags {
		tagValue := field.Tag.Get(string(typ))
		if tagValue != "" {
			return typ, tagValue
		}
	}
	return "", ""
}

// TODO: check for duplicate names of params?
// paramsFromStruct recursively goes through a struct and its fields
// looking for any field with the correct tag.
func paramsFromStruct(schemer jsonschema.Schemer, typ reflect.Type) ([]Parameter, error) {
	params := []Parameter{}
	err := reflectUtil.WalkStruct(typ, func(field reflect.StructField) error {
		paramLoc, paramName := GetParameterLocationTag(field)

		fieldKind := field.Type.Kind()
		if paramName == "" && fieldKind == reflect.Struct {
			// this is a struct without a query tag
			// so get all of its fields as well
			newParams, err := paramsFromStruct(schemer, field.Type)
			if err != nil {
				return err
			}
			params = append(params, newParams...)
			return nil
		} else if paramName == "" {
			return nil
		}

		schema, err := schemer.Get(field.Type)
		if err != nil {
			return err
		}

		schema, err = jsonschema.LoadSchemaOptions(field, schema)
		if err != nil {
			return err
		}

		p := Parameter{&spec.Parameter{
			In:     string(paramLoc),
			Name:   paramName,
			Schema: spec.NewRefOrSpec(nil, &spec.Schema{JsonSchema: schema.JsonSchema}),
		}}
		if err := updateParamFromTags(field, p); err != nil {
			return err
		}
		style := ParameterStyle(p.Style)
		if style == "" {
			style = paramDefaultStyle(paramLoc)
		}

		if err := validateStyleWithType(field.Type, style, paramLoc); err != nil {
			return err
		}

		canLoadParam := false
		switch paramLoc {
		case ParameterLocationPath:
			canLoadParam = canLoadPathParam(field.Type, style)
		case ParameterLocationQuery:
			canLoadParam = canLoadQueryParam(field.Type, style)
		default:
			// TODO: verify ParameterLocationHeader
			// TODO: verify ParameterLocationCookie
			canLoadParam = true
		}

		if !canLoadParam {
			return fmt.Errorf("can not load %s param: `%s` (%s) for %s",
				paramLoc, paramName, field.Type, typ)
		}

		params = append(params, p)
		return nil
	})
	return params, err
}

func canLoadPathParam(typ reflect.Type, style ParameterStyle) bool {
	if reflectUtil.TypeImplementsTextUnmarshal(typ) {
		return true
	}

	if reflectUtil.PrimitiveKind.Has(typ.Kind()) {
		return true
	}
	return false
}

func canLoadQueryParam(typ reflect.Type, style ParameterStyle) bool {
	if reflectUtil.TypeImplementsTextUnmarshal(typ) {
		return true
	}

	kind := typ.Kind()
	if reflectUtil.PrimitiveKind.Has(kind) {
		return true
	}

	if reflectUtil.ArrayKind.Has(kind) {
		elemType := typ.Elem()
		return canLoadQueryParam(elemType, style)
	}

	if kind == reflect.Struct {
		return style == ParameterStyleDeepObject
	}

	return false
}

type paramValidion struct {
	in    []ParameterLocation
	kinds internal.Set[reflect.Kind]
}

func kinds(sets ...internal.Set[reflect.Kind]) internal.Set[reflect.Kind] {
	result := internal.NewSet[reflect.Kind]()
	for _, set := range sets {
		for item := range set {
			result.Add(item)
		}
	}
	return result
}

// https://spec.openapis.org/oas/v3.1.0#styleValues
var paramValidations = map[ParameterStyle]paramValidion{
	ParameterStyleDeepObject: {
		in: []ParameterLocation{
			ParameterLocationQuery,
		},
		kinds: reflectUtil.ObjectKind,
	},
	ParameterStyleSpaceDelimited: {
		in: []ParameterLocation{
			ParameterLocationQuery,
		},
		kinds: kinds(reflectUtil.ObjectKind, reflectUtil.ArrayKind),
	},
	ParameterStyleSpacePipeDelimited: {
		in: []ParameterLocation{
			ParameterLocationQuery,
		},
		kinds: kinds(reflectUtil.ObjectKind, reflectUtil.ArrayKind),
	},
	ParameterStyleSimple: {
		in: []ParameterLocation{
			ParameterLocationPath,
			ParameterLocationHeader,
		},
		// TODO: spec mentions only array kind allowed
		kinds: kinds(reflectUtil.PrimitiveKind, reflectUtil.ArrayKind),
	},
	ParameterStyleForm: {
		in: []ParameterLocation{
			ParameterLocationQuery,
			ParameterLocationCookie,
		},
		kinds: kinds(
			reflectUtil.PrimitiveKind,
			reflectUtil.ObjectKind,
			reflectUtil.ArrayKind,
		),
	},
	ParameterStyleLabel: {
		in: []ParameterLocation{
			ParameterLocationPath,
		},
		kinds: kinds(
			reflectUtil.PrimitiveKind,
			reflectUtil.ObjectKind,
			reflectUtil.ArrayKind,
		),
	},
	ParameterStyleMatrix: {
		in: []ParameterLocation{
			ParameterLocationPath,
		},
		kinds: kinds(
			reflectUtil.PrimitiveKind,
			reflectUtil.ObjectKind,
			reflectUtil.ArrayKind,
		),
	},
}

func validateStyleWithType(typ reflect.Type, style ParameterStyle, loc ParameterLocation) error {
	validation, has := paramValidations[style]
	if !has {
		return fmt.Errorf("invalid style: %q", style)
	}

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	correctKind := validation.kinds.Has(typ.Kind())
	correctLocation := false
	for _, location := range validation.in {
		if location == loc {
			correctLocation = true
			break
		}
	}
	if !correctKind || !correctLocation {
		return fmt.Errorf(
			"incorrect style (%s), kind (%s) or location (%s) for %s\nAllowed kinds: %s\nAllowed Locations: %v",
			style, typ.Kind(), loc, typ.String(), validation.kinds, validation.in,
		)
	}
	return nil
}

func updateParamFromTags(field reflect.StructField, p Parameter) error {
	if v := field.Tag.Get("explode"); v != "" {
		b, err := internal.BoolFromString(v)
		if err != nil {
			return err
		}
		p.Explode = b
	}
	if v := field.Tag.Get("deprecated"); v != "" {
		b, err := internal.BoolFromString(v)
		if err != nil {
			return err
		}
		p.Deprecated = b
	}
	if v := field.Tag.Get("required"); v != "" {
		b, err := internal.BoolFromString(v)
		if err != nil {
			return err
		}
		p.Required = b
	}
	if v := field.Tag.Get("style"); v != "" {
		style, err := ParameterStyleFromString(v)
		if err != nil {
			return err
		}
		p.Style = string(style)
	}
	return nil
}
