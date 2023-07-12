package reflect

import (
	"fmt"
	"reflect"
)

func WalkStructWithIndex(typ reflect.Type, fn func(idx int, field reflect.StructField) error) error {
	if typ.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct, got: %v", typ.Name())
	}

	n := typ.NumField()
	for i := 0; i < n; i++ {
		field := typ.Field(i)
		if err := fn(i, field); err != nil {
			return err
		}
	}
	return nil
}

func WalkStruct(typ reflect.Type, fn func(field reflect.StructField) error) error {
	return WalkStructWithIndex(typ, func(_ int, field reflect.StructField) error {
		return fn(field)
	})
}
