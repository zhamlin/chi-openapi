package openapi_test

import (
	"testing"

	. "chi-openapi/internal/testing"
	"chi-openapi/pkg/openapi"
)

type definedStruct struct {
	Random int
}

func Test_Params(t *testing.T) {
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
	}
	for _, test := range tests {
		params := openapi.SchemaFromParams(test.obj)
		if err := JSONDiff(t, params, test.expected); err != nil {
			t.Errorf("test '%v': %v", test.name, err)
		}
	}

}
