package openapi

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
	// . "github.com/zhamlin/chi-openapi/internal/testing"
)

func TestLoadQueryParam(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
		obj     interface{}
		queries url.Values
	}{
		{
			name: "int array",
			obj: struct {
				Int []int `query:"int"`
			}{},
			queries: url.Values{
				"int": []string{"1", "2"},
			},
		},
		{
			name: "int64 array",
			obj: struct {
				Int []int64 `query:"int"`
			}{},
			queries: url.Values{
				"int": []string{"1", "2"},
			},
		},
		{
			name: "bool",
			obj: struct {
				Value bool `query:"value"`
			}{},
			queries: url.Values{
				"value": []string{"true"},
			},
		},
		{
			name: "int no explode array",
			obj: struct {
				Int []int `query:"int" explode:"false"`
			}{},
			queries: url.Values{
				"int": []string{"1,2,3"},
			},
		},
		{
			name: "str array",
			obj: struct {
				Values []string `query:"values"`
			}{},
			queries: url.Values{
				"values": []string{"1", "2"},
			},
		},
		{
			name: "obj",
			obj: struct {
				Obj struct {
					IntValue float64 `json:"float"`
					StrValue string  `json:"str" minLength:"10"`
				} `query:"obj"`
			}{},
			queries: url.Values{
				"float": []string{"1.01"},
				"str":   []string{"0123456789"},
			},
		},
		{
			name:    "obj short string",
			wantErr: true,
			obj: struct {
				Obj struct {
					IntValue float64 `json:"float"`
					StrValue string  `json:"str" minLength:"10"`
				} `query:"obj"`
			}{},
			queries: url.Values{
				"float": []string{"1.01"},
				"str":   []string{"123456789"},
			},
		},
		{
			name:    "obj no explode",
			wantErr: true,
			obj: struct {
				Obj struct {
					IntValue float64 `json:"float"`
					StrValue string  `json:"str" minLength:"10"`
				} `query:"obj" explode:"false"`
			}{},
			queries: url.Values{
				"float": []string{"1.01"},
				"str":   []string{"0123456789"},
			},
		},
		{
			name: "deepObj",
			obj: struct {
				Obj struct {
					IntValue float64 `json:"float"`
					StrValue string  `json:"str,omitempty" minLength:"10"`
				} `query:"obj" style:"deepObject"`
			}{},
			queries: url.Values{
				"obj[float]": []string{"1.01"},
				"obj[str]":   []string{"0123456789"},
			},
		},
		{
			name: "deepObj test",
			obj: struct {
				Obj struct {
					Array []float64 `json:"array" explode:"true"`
				} `query:"obj" style:"deepObject"`
			}{},
			queries: url.Values{
				"obj[array]": []string{"1.01,12.20"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params, err := ParamsFromObj(test.obj, nil)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("GET", "/", nil)
			req.URL.RawQuery = test.queries.Encode()
			req.Header.Add("Content-Type", "application/json")
			v, err := LoadParamStruct(test.obj, LoadParamInput{
				RequestValidationInput: &openapi3filter.RequestValidationInput{
					Request: req,
				},
				Params: params,
			})
			if !test.wantErr && err != nil {
				t.Fatal(err)
			}
			if !v.IsValid() && !test.wantErr {
				t.Fatal("expected valid value")
			}
			t.Log(v)
		})

	}
}
