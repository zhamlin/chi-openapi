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
              "type": "object",
              "required": [
                  "name"
              ]
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
              "type": "object",
              "required": [
                  "name"
              ]
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
              "type": "object",
              "required": [
                  "data"
              ]
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
              "type": "object",
              "required": [
                  "data"
              ]
            }
        `},
		{
			name:    "inline array",
			schemas: true,
			obj: Wrapper{
				Data: []string{},
			},
			expected: `
            {
              "properties": {
                "data": {
                  "items": {
                      "type": "string"
                  },
                  "type": "array"
                }
              },
              "type": "object",
              "required": [
                  "data"
              ]
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
              "type": "object",
              "required": [
                  "random"
              ]
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
              "type": "object",
              "required": [
                  "truthy"
              ]
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
              "type": "object",
              "required": [
                  "number"
              ]
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
              "type": "object",
              "required": [
                  "number"
              ]
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
              "type": "object",
              "required": [
                  "number"
              ]
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
              "type": "object",
              "required": [
                  "number"
              ]
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
              "type": "object",
              "required": [
                  "number"
              ]
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
              "type": "object",
              "required": [
                  "string"
              ]
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
              "type": "object",
              "required": [
                  "string"
              ]
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
              "type": "object",
              "required": [
                  "string"
              ]
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
              "type": "object",
              "required": [
                  "date"
              ]
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
              "type": "object",
              "required": [
                  "date"
              ]
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
              "type": "object",
              "required": [
                  "string"
              ]
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
              "type": "object",
              "required": [
                  "array"
              ]
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
              "type": "object",
              "required": [
                  "array"
              ]
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
              "type": "object",
              "required": [
                  "array"
              ]
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
              "type": "object",
              "required": [
                  "array"
              ]
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

func TestSchemaMaps(t *testing.T) {
	tests := []struct {
		name     string
		schemas  bool
		expected string
		obj      interface{}
	}{
		{
			name: "basic map",
			obj: struct {
				Map map[string]interface{} `json:"map"`
			}{},
			expected: `
            {
              "properties": {
                "map": {
                  "type": "object",
                  "additionalProperties": {
                    "type": "object"
                  }
                }
              },
              "type": "object",
              "required": [
                  "map"
              ]
            }
        `},
		{
			name: "string map",
			obj: struct {
				Map map[string]string `json:"map"`
			}{},
			expected: `
            {
              "properties": {
                "map": {
                  "type": "object",
                  "additionalProperties": {
                    "type": "string"
                  }
                }
              },
              "type": "object",
              "required": [
                  "map"
              ]
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

type Color int

const (
	Unknown Color = 0
	Blue    Color = 1
	Red     Color = 2
	Green   Color = 3
)

func (i Color) String() string {
	switch i {
	case 0:
		return "Unknown"
	case 1:
		return "Blue"
	case 2:
		return "Red"
	case 3:
		return "Green"
	default:
		return ""
	}
}

// EnumValues returns an array of the values of this type
func (i Color) EnumValues() []Color {
	return []Color{0, 1, 2, 3}
}

func TestSchemaEnums(t *testing.T) {
	tests := []struct {
		name     string
		schemas  bool
		expected string
		obj      interface{}
	}{
		{
			name: "custom enumer type",
			obj: struct {
				Color Color `json:"color"`
			}{},
			expected: `
            {
              "properties": {
                "color": {
                  "enum": [
                    "Unknown",
                    "Blue",
                    "Red",
                    "Green"
                  ],
                  "type": "string"
                }
              },
              "type": "object",
              "required": [
                "color"
              ]
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

type testResponse struct {
	Value int `json:"int"`
}

func (testResponse) OpenAPIDescription() string {
	return `
    Contains the value.
    `
}

func TestOpenAPIDescriptionFunc(t *testing.T) {
	tests := []struct {
		name     string
		schemas  bool
		expected string
		obj      interface{}
	}{
		{
			name: "custom description",
			obj:  testResponse{},
			expected: `
            {
              "description": "Contains the value.",
              "properties": {
                "int": {
                  "type": "integer"
                }
              },
              "type": "object",
              "required": [
                "int"
              ]
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
