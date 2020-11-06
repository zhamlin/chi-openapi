package openapi_test

import (
	"testing"
	"time"

	. "chi-openapi/internal/testing"
	"chi-openapi/pkg/openapi"
)

type ref1 struct {
	Random int32 `json:"random" min:"3" max:"4"`
}

type ref2 struct {
	Randoms []int64 `json:"randoms"`
}

type Wrapper struct {
	Data interface{} `json:"data"`
}

func (Wrapper) SchemaInline() bool {
	return true
}

func TestSchema(t *testing.T) {
	tests := []struct {
		name     string
		schemas  bool
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
			name:    "inline struct struct",
			schemas: true,
			obj: Wrapper{
				Data: ref1{
					Random: 10,
				},
			},
			expected: `
            {
              "properties": {
                "data": {
                  "$ref": "#/components/schemas/ref1"
                }
              },
              "type": "object"
            }
        `},
		{
			name:    "empty inline struct struct",
			schemas: true,
			obj:     Wrapper{},
			expected: `
            {
              "properties": {
                "data": {
                  "type": "object"
                }
              },
              "type": "object"
            }
        `},
		{
			name: "required struct fields",
			obj: struct {
				Required bool `json:"required" required:"true"`
			}{},
			expected: `
            {
              "properties": {
                "required": {
                  "type": "boolean"
                }
              },
              "required": [
                "required"
              ],
              "type": "object"
            }
        `},
		{
			name: "minmax",
			obj:  ref1{},
			expected: `
            {
              "properties": {
                "random": {
                  "format": "int32",
                  "maximum": 4,
                  "minimum": 3,
                  "type": "integer"
                }
              },
              "type": "object"
            }
        `},
		{
			name: "bool",
			obj: struct {
				Truthy bool `json:"truthy"`
			}{},
			expected: `
            {
              "properties": {
                "truthy": {
                  "type": "boolean"
                }
              },
              "type": "object"
            }
        `},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var schemas openapi.Schemas = nil
			if test.schemas {
				schemas = openapi.Schemas{}
			}
			schema := openapi.SchemaFromObj(test.obj, schemas)
			if err := JSONDiff(t, JSONT(t, schema), test.expected); err != nil {
				t.Error(err)
				if test.schemas {
					for name, schema := range schemas {
						t.Logf("%v: %+v\n", name, JSONT(t, schema))
					}
				}
			}
		})
	}
}

func TestSchemaNumberFormats(t *testing.T) {
	tests := []struct {
		name     string
		schemas  bool
		expected string
		obj      interface{}
	}{
		{
			name: "int32",
			obj: struct {
				Number int32 `json:"number"`
			}{},
			expected: `
            {
              "properties": {
                "number": {
                  "type": "integer",
                  "format": "int32"
                }
              },
              "type": "object"
            }
        `},
		{
			name: "int64",
			obj: struct {
				Number int64 `json:"number"`
			}{},
			expected: `
            {
              "properties": {
                "number": {
                  "type": "integer",
                  "format": "int64"
                }
              },
              "type": "object"
            }
        `},
		{
			name: "int",
			obj: struct {
				Number int `json:"number"`
			}{},
			expected: `
            {
              "properties": {
                "number": {
                  "type": "integer"
                }
              },
              "type": "object"
            }
        `},
		{
			name: "float32",
			obj: struct {
				Number float32 `json:"number"`
			}{},
			expected: `
            {
              "properties": {
                "number": {
                  "type": "number",
                  "format": "float"
                }
              },
              "type": "object"
            }
        `},
		{
			name: "float64",
			obj: struct {
				Number float64 `json:"number"`
			}{},
			expected: `
            {
              "properties": {
                "number": {
                  "type": "number",
                  "format": "float"
                }
              },
              "type": "object"
            }
        `},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var schemas openapi.Schemas = nil
			if test.schemas {
				schemas = openapi.Schemas{}
			}
			schema := openapi.SchemaFromObj(test.obj, schemas)
			if err := JSONDiff(t, JSONT(t, schema), test.expected); err != nil {
				t.Error(err)
				if test.schemas {
					for name, schema := range schemas {
						t.Logf("%v: %+v\n", name, JSONT(t, schema))
					}
				}
			}
		})
	}
}

func TestSchemaStringFormats(t *testing.T) {
	tests := []struct {
		name     string
		schemas  bool
		expected string
		obj      interface{}
	}{
		{
			name: "string",
			obj: struct {
				String string `json:"string"`
			}{},
			expected: `
            {
              "properties": {
                "string": {
                  "type": "string"
                }
              },
              "type": "object"
            }
        `},
		{
			name: "string length",
			obj: struct {
				String string `json:"string" minLength:"1" maxLength:"2"`
			}{},
			expected: `
            {
              "properties": {
                "string": {
                  "type": "string",
                  "maxLength": 2,
                  "minLength": 1
                }
              },
              "type": "object"
            }
        `},
		{
			name: "string format",
			obj: struct {
				String string `json:"string" format:"email"`
			}{},
			expected: `
            {
              "properties": {
                "string": {
                  "type": "string",
                  "format": "email"
                }
              },
              "type": "object"
            }
        `},
		{
			name: "time.Time date-time support",
			obj: struct {
				Date time.Time `json:"date"`
			}{},
			expected: `
            {
              "properties": {
                "date": {
                  "type": "string",
                  "format": "date-time"
                }
              },
              "type": "object"
            }
        `},
		{
			name: "time.Time override format",
			obj: struct {
				Date time.Time `json:"date" format:"date"`
			}{},
			expected: `
            {
              "properties": {
                "date": {
                  "type": "string",
                  "format": "date"
                }
              },
              "type": "object"
            }
        `},
		{
			name: "string pattern",
			obj: struct {
				String string `json:"string" pattern:"^\\d{3}-\\d{2}-\\d{4}$"`
			}{},
			expected: `
            {
              "properties": {
                "string": {
                  "type": "string",
                  "pattern": "^\\d{3}-\\d{2}-\\d{4}$"
                }
              },
              "type": "object"
            }
        `},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var schemas openapi.Schemas = nil
			if test.schemas {
				schemas = openapi.Schemas{}
			}
			schema := openapi.SchemaFromObj(test.obj, schemas)
			if err := JSONDiff(t, JSONT(t, schema), test.expected); err != nil {
				t.Error(err)
				if test.schemas {
					for name, schema := range schemas {
						t.Logf("%v: %+v\n", name, JSONT(t, schema))
					}
				}
			}
		})
	}
}

type SSN string

func TestSchemaArrays(t *testing.T) {
	tests := []struct {
		name     string
		schemas  bool
		expected string
		obj      interface{}
	}{
		{
			name: "string array",
			obj: struct {
				Array []string `json:"array"`
			}{},
			expected: `
            {
              "properties": {
                "array": {
                  "items": {
                    "type": "string"
                  },
                  "type": "array"
                }
              },
              "type": "object"
            }
        `},
		{
			name: "time array",
			obj: struct {
				Array []time.Time `json:"array"`
			}{},
			expected: `
            {
              "properties": {
                "array": {
                  "items": {
                    "type": "string",
                    "format": "date-time"
                  },
                  "type": "array"
                }
              },
              "type": "object"
            }
        `},
		{
			name: "array min max",
			obj: struct {
				Array []time.Time `json:"array" minItems:"1" maxItems:"10"`
			}{},
			expected: `
            {
              "properties": {
                "array": {
                  "items": {
                    "type": "string",
                    "format": "date-time"
                  },
                  "type": "array",
                  "maxItems": 10,
                  "minItems": 1
                }
              },
              "type": "object"
            }
        `},
		{
			name: "array uniqueItems",
			obj: struct {
				Array []time.Time `json:"array" uniqueItems:"true"`
			}{},
			expected: `
            {
              "properties": {
                "array": {
                  "items": {
                    "type": "string",
                    "format": "date-time"
                  },
                  "type": "array",
                  "uniqueItems": true
                }
              },
              "type": "object"
            }
        `},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var schemas openapi.Schemas = nil
			if test.schemas {
				schemas = openapi.Schemas{}
			}
			schema := openapi.SchemaFromObj(test.obj, schemas)
			if err := JSONDiff(t, JSONT(t, schema), test.expected); err != nil {
				t.Error(err)
				if test.schemas {
					for name, schema := range schemas {
						t.Logf("%v: %+v\n", name, JSONT(t, schema))
					}
				}
			}
		})
	}
}
