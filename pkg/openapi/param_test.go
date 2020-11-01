package openapi_test

import (
	"testing"

	. "chi-openapi/internal/testing"
	"chi-openapi/pkg/openapi"
)

type definedStruct struct {
	Random int
}

func TestParams(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		obj      interface{}
	}{
		{
			name: "basic path",
			obj: struct {
				Name string `path:"name"`
			}{},
			expected: `
            [
              {
                "in": "path",
                "name": "name",
                "required": true,
                "schema": {
                  "type": "string"
                }
              }
            ]
        `},
		{
			name: "basic no tag",
			obj: struct {
				Name string
			}{},
			expected: `[]`,
		},
		{
			name: "array query param",
			obj: struct {
				IDs []string `query:"ids" required:"true" minItems:"1" maxItems:"3" doc:"test doc" explode:"true"`
			}{},
			expected: `
            [
              {
                "in": "query",
                "name": "ids",
                "required": true,
                "explode": true,
                "description": "test doc",
                "schema": {
                  "items": {
                    "type": "string"
                  },
                  "type": "array",
                  "minItems": 1,
                  "maxItems": 3
                }
              }
            ]
        `},
		{
			name: "min int query param",
			obj: struct {
				Int int `query:"int" min:"1"`
			}{},
			expected: `
            [
              {
                "in": "query",
                "name": "int",
                "schema": {
                  "type": "integer"
                }
              }
            ]
        `},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := openapi.ParamsFromObj(test.obj)
			if err := JSONDiff(t, JSONT(t, params), test.expected); err != nil {
				t.Error(err)
			}
		})

	}

}
