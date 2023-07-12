package openapi3

import (
	"testing"

	. "github.com/zhamlin/chi-openapi/internal/testing"
	"github.com/zhamlin/chi-openapi/pkg/jsonschema"
)

func TestParamsFromStruct(t *testing.T) {
	tests := []struct {
		name      string
		obj       any
		wantErr   bool
		wantCount int
	}{
		{
			obj: struct {
				Name    string `query:"name"`
				ID      int    `path:"id"`
				Header  string `header:"x-header-value"`
				Cookie  string `cookie:"cookie"`
				Ignored string
			}{},
			wantCount: 4,
		},
		{
			name: "deepObject is only valid on structs",
			obj: struct {
				Name string `query:"name" style:"deepObject"`
			}{},
			wantErr: true,
		},
	}

	schemer := jsonschema.NewSchemer()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params, err := ParamsFromStruct(schemer, test.obj)
			if test.wantErr {
				MustNotMatch(t, err, nil, "expected an error got none")
			} else {
				MustMatch(t, err, nil, "did not expect an error")
			}
			MustMatch(t, len(params), test.wantCount)
		})
	}

}

// func TestParamsFromStructError(t *testing.T) {
// 	// schemer := jsonschema.NewSchemer()
// 	// params, err := ParamsFromStructObj(schemer, struct {
// 	// 	Name    string `query:"name"`
// 	// 	ID      int    `path:"id"`
// 	// Header  string `header:"x-header-value" style:"fancy"`
// 	// 	Cookie  string `cookie:"cookie"`
// 	// 	Ignored string
// 	// }{})
// 	// MustMatch(t, err, nil)
// 	// MustMatch(t, len(params), 4)
// }
