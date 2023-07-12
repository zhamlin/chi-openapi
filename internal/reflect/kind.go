package reflect

import (
	"encoding"
	"reflect"

	"github.com/zhamlin/chi-openapi/internal"
)

var TextUnmarshallerType = MakeType[encoding.TextUnmarshaler]()

func TypeImplementsTextUnmarshal(typ reflect.Type) bool {
	if typ.Implements(TextUnmarshallerType) {
		return true
	}
	if reflect.PtrTo(typ).Implements(TextUnmarshallerType) {
		return true
	}
	return false
}

var PrimitiveKind = internal.Set[reflect.Kind]{
	reflect.Bool:    struct{}{},
	reflect.String:  struct{}{},
	reflect.Int:     struct{}{},
	reflect.Int8:    struct{}{},
	reflect.Int16:   struct{}{},
	reflect.Int32:   struct{}{},
	reflect.Uint:    struct{}{},
	reflect.Uint8:   struct{}{},
	reflect.Uint16:  struct{}{},
	reflect.Uint32:  struct{}{},
	reflect.Uint64:  struct{}{},
	reflect.Float32: struct{}{},
	reflect.Float64: struct{}{},
}

var ArrayKind = internal.Set[reflect.Kind]{
	reflect.Array: struct{}{},
	reflect.Slice: struct{}{},
}

var ObjectKind = internal.Set[reflect.Kind]{
	reflect.Struct: struct{}{},
}
