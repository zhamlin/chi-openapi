package openapi_test

import (
	"testing"

	. "chi-openapi/internal/testing"
	"chi-openapi/pkg/openapi"
)

func TestParams(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		obj      interface{}
	}{
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
                "style": "form",
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
		t.Run(test.name, func(t *testing.T) {
			params, err := openapi.ParamsFromObj(test.obj, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := JSONDiff(t, JSONT(t, params), test.expected); err != nil {
				t.Error(err)
			}
		})

	}

}

func TestParamsLocation(t *testing.T) {
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
			name: "basic query",
			obj: struct {
				Name string `query:"name"`
			}{},
			expected: `
            [
              {
                "in": "query",
                "name": "name",
                "style": "form",
                "explode": true,
                "schema": {
                  "type": "string"
                }
              }
            ]
        `},
		{
			name: "basic header",
			obj: struct {
				Name string `header:"name"`
			}{},
			expected: `
            [
              {
                "in": "header",
                "name": "name",
                "schema": {
                  "type": "string"
                }
              }
            ]
        `},
		{
			name: "basic cookie",
			obj: struct {
				Name string `cookie:"name"`
			}{},
			expected: `
            [
              {
                "in": "cookie",
                "name": "name",
                "schema": {
                  "type": "string"
                }
              }
            ]
        `},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params, err := openapi.ParamsFromObj(test.obj, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := JSONDiff(t, JSONT(t, params), test.expected); err != nil {
				t.Error(err)
			}
		})

	}

}

func TestParamsSpecificSettings(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		obj      interface{}
	}{
		{
			name: "style",
			obj: struct {
				Name []string `query:"name" style:"matrix"`
			}{},
			expected: `
            [
                {
                  "in": "query",
                  "name": "name",
                  "style": "matrix",
                  "explode": true,
                  "schema": {
                    "items": {
                      "type": "string"
                    },
                    "type": "array"
                  }
                }
            ]
        `},
		{
			name: "required",
			obj: struct {
				Name []string `query:"name" style:"matrix" required:"true"`
			}{},
			expected: `
            [
                {
                  "in": "query",
                  "name": "name",
                  "style": "matrix",
                  "explode": true,
                  "required": true,
                  "schema": {
                    "items": {
                      "type": "string"
                    },
                    "type": "array"
                  }
                }
            ]
        `},
		{
			name: "explode",
			obj: struct {
				Name []string `query:"name" style:"matrix" explode:"true"`
			}{},
			expected: `
            [
                {
                  "in": "query",
                  "name": "name",
                  "explode": true,
                  "style": "matrix",
                  "schema": {
                    "items": {
                      "type": "string"
                    },
                    "type": "array"
                  }
                }
            ]
        `},
		{
			name: "explode",
			obj: struct {
				Name []string `query:"name" style:"matrix" explode:"true"`
			}{},
			expected: `
            [
                {
                  "in": "query",
                  "name": "name",
                  "explode": true,
                  "style": "matrix",
                  "schema": {
                    "items": {
                      "type": "string"
                    },
                    "type": "array"
                  }
                }
            ]
        `},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params, err := openapi.ParamsFromObj(test.obj, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := JSONDiff(t, JSONT(t, params), test.expected); err != nil {
				t.Error(err)
			}
		})

	}

}
