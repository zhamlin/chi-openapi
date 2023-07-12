package jsonschema_test

import (
	"testing"
	"time"

	testutils "github.com/zhamlin/chi-openapi/internal/testing"
	"github.com/zhamlin/chi-openapi/pkg/jsonschema"
)

func TestSchema(t *testing.T) {
	tests := []struct {
		name string
		obj  any
		want string
	}{
		{
			obj: true,
			want: `{
                "type": "boolean"
            }`,
		},
		{
			obj: int(1),
			want: `{
                "type": "integer"
            }`,
		},
		{
			obj: int8(1),
			want: `{
                "type": "integer"
            }`,
		},
		{
			obj: int16(1),
			want: `{
                "type": "integer"
            }`,
		},
		{
			obj: int32(1),
			want: `{
                "type": "integer",
                "format": "int32"
            }`,
		},
		{
			obj: int64(1),
			want: `{
                "type": "integer",
                "format": "int64"
            }`,
		},
		{
			obj: float32(1.0),
			want: `{
                "type": "number", 
                "format": "float"
            }`,
		},
		{
			obj: float64(1.0),
			want: `{
                "type": "number", 
                "format": "float"
            }`,
		},
		{
			obj: uint(1),
			want: `{
                "type": "integer", 
                "minimum": 0
            }`,
		},
		{
			obj: uint8(1),
			want: `{
                "type": "integer", 
                "minimum": 0
            }`,
		},
		{
			obj: uint16(1),
			want: `{
                "type": "integer", 
                "minimum": 0
            }`,
		},
		{
			obj: uint32(1),
			want: `{
                "type": "integer", 
                "minimum": 0,
                "format": "int32"
            }`,
		},
		{
			obj: uint64(1),
			want: `{
                "type": "integer", 
                "minimum": 0,
                "format": "int64"
            }`,
		},
		{
			obj: "string",
			want: `{
                "type": "string"
            }`,
		},
		{
			obj: []string{"a", "b", "c"},
			want: `{
                "type": "array",
                "items": {
                    "type": "string"
                }
            }`,
		},
		{
			obj: map[string]string{
				"a": "1",
				"b": "2",
			},
			want: `{
                "type": "object",
                "additionalProperties": {
                    "type": "string"
                }
            }`,
		},
		{
			obj: map[string]any{
				"a": 1,
				"b": "2",
			},
			want: `{
                "type": "object"
            }`,
		},
		{
			obj: struct {
				F string
			}{},
			want: `{
                "type": "object",
                "properties": {
                    "F": {
                        "type": "string"
                    }
                }
            }`,
		},
		{
			name: "pointer is nullable",
			obj: struct {
				F *string
			}{},
			want: `{
                "type": "object",
                "properties": {
                    "F": {
                        "type": ["string", "null"]
                    }
                }
            }`,
		},
		{
			name: "field ignored",
			obj: struct {
				F string `json:"-"`
			}{},
			want: `{
                "type": "object"
            }`,
		},
		{
			name: "json tag used for name",
			obj: struct {
				F string `json:"field"`
			}{},
			want: `{
                "type": "object",
                "properties": {
                    "field": {
                        "type": "string"
                    }
                }
            }`,
		},
		{
			name: "nested structs",
			obj: struct {
				F      string `json:"field"`
				Nested struct {
					F int `json:"nested_field"`
				} `json:"nested"`
			}{},
			want: `{
                "type": "object",
                "properties": {
                    "field": {
                        "type": "string"
                    },
                    "nested": {
                        "properties": {
                            "nested_field": {
                                "type": "integer"
                            }
                        },
                        "type": "object"
                    }
                }
            }`,
		},
		{
			name: "omitempty ignored",
			obj: struct {
				F string `json:",omitempty"`
				A string `json:"a,omitempty"`
			}{},
			want: `{
                "type": "object",
                "properties": {
                    "F": {
                        "type": "string"
                    },
                    "a": {
                        "type": "string"
                    }
                }
            }`,
		},
	}

	schemer := jsonschema.NewSchemer()
	schemer.UseRefs = false
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := schemer.Get(test.obj)
			if err != nil {
				t.Fatal(err)
			}
			testutils.MustMatchAsJson(t, schema, test.want)
		})

	}
}

func TestSchemaRef(t *testing.T) {
	type A struct {
		Name string
	}
	tests := []struct {
		name string
		obj  any
		want string
	}{
		{
			name: "show reference is created when seeing struct for the first time",
			obj: struct {
				A A
			}{},
			want: `{
                "type": "object",
                "properties": {
                    "A": {
                        "$ref": "/schemas/A"
                    }
                }
            }`,
		},
		{
			name: "show the objects schema is returned vs a reference",
			obj:  A{},
			want: `{
                "type": "object",
                "properties": {
                    "Name": {
                        "type": "string"
                    }
                }
            }`,
		},
		{
			name: "array items use a ref",
			obj:  []A{},
			want: `{
                "type": "array",
                "items": {
                    "$ref": "/schemas/A"
                }
            }`,
		},
	}

	schemer := jsonschema.NewSchemer()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := schemer.Get(test.obj)
			if err != nil {
				t.Fatal(err)
			}
			testutils.MustMatchAsJson(t, schema, test.want)
		})
	}
}

func TestSchemaCustomTypes(t *testing.T) {
	tests := []struct {
		name string
		obj  any
		want string
	}{
		{
			name: "show reference is created when seeing struct for the first time",
			obj:  time.Time{},
			want: `{
                "type": "string",
                "format": "date-time"
            }`,
		},
		{
			name: "show inline option is respected",
			obj: struct {
				DateCreated time.Time `json:"date_created"`
			}{},
			want: `{
                "type": "object",
                "properties": {
                    "date_created": {
                        "type": "string",
                        "format": "date-time"
                    }
                }
            }`,
		},
	}

	schemer := jsonschema.NewSchemer()
	schemer.Set(time.Time{}, jsonschema.NewDateTimeSchema(), jsonschema.NoRef())

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := schemer.Get(test.obj)
			if err != nil {
				t.Fatal(err)
			}
			testutils.MustMatchAsJson(t, schema, test.want)
		})
	}
}

func TestSchemaModifiers(t *testing.T) {
	tests := []struct {
		name string
		obj  any
		want string
	}{
		{
			name: "show reference is created when seeing struct for the first time",
			obj: struct {
				Default string `json:"default" default:"hello world"`
			}{},
			want: `{
                "type": "object",
                "properties": {
                    "default": {
                        "type": "string",
                        "default": "hello world"
                    }
                }
            }`,
		},
	}

	schemer := jsonschema.NewSchemer()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := schemer.Get(test.obj)
			if err != nil {
				t.Fatal(err)
			}
			testutils.MustMatchAsJson(t, schema, test.want)
		})
	}
}
