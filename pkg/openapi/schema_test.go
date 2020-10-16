package openapi_test

import (
	"testing"

	. "chi-openapi/internal/testing"
	"chi-openapi/pkg/openapi"
)

type ref1 struct {
	Random int32 `json:"random"`
}

type ref2 struct {
	Randoms []int64 `json:"randoms"`
}

func Test_Schema(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		obj      interface{}
	}{
		{
			name: "basic struct",
			obj: struct {
				Name string `json:"name"`
			}{},
			expected: `
            {
              "properties": {
                "name": {
                  "type": "string"
                }
              },
              "type": "object"
            }
        `},
		{
			name: "nested struct",
			obj: struct {
				Name  string `json:"name"`
				Other struct {
					Description string `json:"description"`
				} `json:"other"`
			}{},
			expected: `
            {
              "properties": {
                "name": {
                  "type": "string"
                },
                "other": {
                  "items": {
                    "properties": {
                      "description": {
                        "type": "string"
                      }
                    },
                    "type": "object"
                  },
                  "type": "object"
                }
              },
              "type": "object"
            }
        `},
	}

	for _, test := range tests {
		params := openapi.SchemaFromObj(test.obj)
		if err := JSONDiff(t, params, test.expected); err != nil {
			t.Errorf("test '%v': %v", test.name, err)
		}
	}
}
