package openapi

import (
	"fmt"
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
)

func LoadPathParam(paths map[string]string, p *openapi3.Parameter) (reflect.Value, error) {
	value, has := paths[p.Name]
	if !has && p.Schema.Value.Nullable {
		return reflect.ValueOf([]string{}), nil
	}
	if !has {
		return reflect.Value{}, fmt.Errorf("no path found for the param: %v", p.Name)
	}
	return reflect.ValueOf(value), nil
}
