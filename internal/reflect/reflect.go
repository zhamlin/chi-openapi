package reflect

import (
	"reflect"
)

// https://github.com/golang/go/issues/60088

// MakeType returns the reflection Type that represents the static type of T.
func MakeType[T any]() reflect.Type {
	return reflect.TypeOf((*T)(nil)).Elem()
}
