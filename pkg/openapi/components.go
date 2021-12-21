package openapi

import (
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"
)

func NewComponents() Components {
	return Components{
		Schemas:         Schemas{},
		Parameters:      map[reflect.Type]openapi3.Parameters{},
		RegisteredTypes: RegisteredTypes{},
	}
}

// Components is used to store shared data between
// various parts of the openapi doc
type Components struct {
	Parameters      map[reflect.Type]openapi3.Parameters
	Schemas         Schemas
	RegisteredTypes RegisteredTypes
}
