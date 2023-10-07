package reflect_test

import (
	"reflect"
	"testing"

	reflectUtil "github.com/zhamlin/chi-openapi/internal/reflect"

	. "github.com/zhamlin/chi-openapi/internal/testing"
)

type unmarshaller struct {
}

func (u *unmarshaller) UnmarshalText(_ []byte) error {
	return nil
}

func TestUnmarshalType(t *testing.T) {
	tests := []struct {
		name            string
		typ             any
		isTextUnmarshal bool
	}{
		{
			typ:             unmarshaller{},
			isTextUnmarshal: true,
		},
		{
			typ:             &unmarshaller{},
			isTextUnmarshal: true,
		},
		{
			typ:             struct{}{},
			isTextUnmarshal: false,
		},
	}
	for _, test := range tests {
		isTextUnmarshal := reflectUtil.TypeImplementsTextUnmarshal(reflect.TypeOf(test.typ))
		MustMatch(t, isTextUnmarshal, test.isTextUnmarshal,
			"%T implements TextUnmarshal: %v", test.typ, isTextUnmarshal)
	}
}
