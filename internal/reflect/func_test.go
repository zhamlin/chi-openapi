package reflect

import (
	"reflect"
	"testing"
)

type A struct{}

func (A) Error() string {
	return ""
}

var _ error = A{}

func TestGetErrorLocation(t *testing.T) {
	tests := []struct {
		name   string
		typ    any
		hasErr bool
		errLoc int
	}{
		{
			typ:    func() {},
			hasErr: false,
		},
		{
			typ:    func() error { return nil },
			hasErr: true,
			errLoc: 0,
		},
		{
			typ:    func() (int, error) { return 0, nil },
			hasErr: true,
			errLoc: 1,
		},
		{
			typ:    func() (int, error, string) { return 0, nil, "" },
			hasErr: true,
			errLoc: 1,
		},
		{
			name:   "return type must be error vs type that implements error",
			typ:    func() (A, error) { return A{}, nil },
			hasErr: true,
			errLoc: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			errLoc, hasErr := GetErrorLocation(reflect.TypeOf(test.typ))
			if errLoc != test.errLoc {
				t.Fatalf("wanted errLoc=%d, got: %d", test.errLoc, errLoc)
			}

			if hasErr != test.hasErr {
				t.Fatalf("wanted hasErr=%v, got: %v", test.hasErr, hasErr)
			}
		})
	}
}
