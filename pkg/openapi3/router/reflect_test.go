package router

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/zhamlin/chi-openapi/internal"
	"github.com/zhamlin/chi-openapi/pkg/container"
	"github.com/zhamlin/chi-openapi/pkg/jsonschema"
	"github.com/zhamlin/chi-openapi/pkg/openapi3"

	. "github.com/zhamlin/chi-openapi/internal/testing"
	. "github.com/zhamlin/chi-openapi/pkg/openapi3/operations"
)

type unmarshaller struct {
	Name string
}

func (u *unmarshaller) UnmarshalText(text []byte) error {
	if string(text) == "error" {
		return errors.New("error")
	}
	u.Name = string(text)
	return nil
}

func TestStringToValue(t *testing.T) {
	tests := []struct {
		name    string
		typ     any
		have    string
		want    any
		wantErr bool
	}{
		{
			typ:  uint8(0),
			have: "1",
			want: uint8(1),
		},
		{
			typ:  uint16(0),
			have: "1",
			want: uint16(1),
		},
		{
			typ:  uint32(0),
			have: "1",
			want: uint32(1),
		},
		{
			typ:  uint64(0),
			have: strconv.FormatUint(math.MaxUint64, 10),
			want: uint64(math.MaxUint64),
		},
		{
			typ:  uint(0),
			have: "1",
			want: uint(1),
		},
		{
			typ:  int8(0),
			have: "1",
			want: int8(1),
		},
		{
			typ:  int16(0),
			have: "1",
			want: int16(1),
		},
		{
			typ:  int32(0),
			have: "1",
			want: int32(1),
		},
		{
			typ:  int64(0),
			have: strconv.FormatInt(math.MaxInt64, 10),
			want: int64(math.MaxInt64),
		},
		{
			typ:  int(0),
			have: "1",
			want: int(1),
		},
		{
			typ:  float32(0),
			have: "1.01",
			want: float32(1.01),
		},
		{
			typ:  float64(0),
			have: "1.01",
			want: float64(1.01),
		},
		{
			name:    "parsing error from out of range uint32",
			typ:     uint32(0),
			have:    strconv.FormatUint(math.MaxUint64, 10),
			want:    uint32(math.MaxUint32),
			wantErr: true,
		},
		{
			typ:  string(""),
			have: "hello world",
			want: "hello world",
		},
		{
			typ:  bool(false),
			have: "true",
			want: true,
		},
		{
			typ:     bool(false),
			have:    "some other value",
			wantErr: true,
		},
		{
			name: "non pointer type, with pointer impl of TextUnmarshaler",
			typ:  unmarshaller{},
			have: "the name",
			want: unmarshaller{Name: "the name"},
		},
		{
			typ:  &unmarshaller{},
			have: "the name",
			want: &unmarshaller{Name: "the name"},
		},
		{
			name:    "unmarshal error is shown",
			typ:     &unmarshaller{},
			have:    "error",
			wantErr: true,
		},
	}
	for _, test := range tests {
		value, err := stringToValue(test.have, reflect.TypeOf(test.typ))
		if test.wantErr {
			MustNotMatch(t, err, nil, "expected an error got none")
		} else {
			MustMatch(t, err, nil, "did not expect an error")
		}

		if test.want != nil {
			MustMatch(t, true, value.IsValid(), "value is not valid")
			MustMatch(t, test.want, value.Interface())
		}
	}
}

func TestGetParamAsStr(t *testing.T) {
	tests := []struct {
		obj  any
		name string

		paramName string
		paramLoc  openapi3.ParameterLocation

		url       string
		headers   map[string]string
		cookies   map[string]string
		urlParams map[string]string

		want []string
		has  bool
	}{
		{
			name:      "param exists but is empty",
			url:       "/?name=",
			paramName: "name",
			paramLoc:  openapi3.ParameterLocationQuery,
			want:      []string{""},
			has:       true,
		},
		{

			name:      "param does not exists",
			url:       "/",
			paramName: "name",
			paramLoc:  openapi3.ParameterLocationPath,
			want:      []string{},
			has:       false,
		},
		{
			url:       "/?name=foo",
			paramName: "name",
			paramLoc:  openapi3.ParameterLocationQuery,
			want:      []string{"foo"},
		},
		{
			url:       "/",
			paramName: "name",
			paramLoc:  openapi3.ParameterLocationHeader,
			want:      []string{"foo"},
			headers:   map[string]string{"name": "foo"},
		},
		{
			url:       "/",
			paramName: "name",
			paramLoc:  openapi3.ParameterLocationCookie,
			want:      []string{"foo"},
			cookies:   map[string]string{"name": "foo"},
		},
		{
			url:       "/",
			paramName: "name",
			paramLoc:  openapi3.ParameterLocationPath,
			want:      []string{"foo"},
			// these params are set via newRouteInfo()
			urlParams: map[string]string{"name": "foo"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, test.url, nil)
			if test.headers != nil {
				for header, value := range test.headers {
					req.Header.Set(header, value)
				}
			}
			if test.cookies != nil {
				for cookie, value := range test.cookies {
					cookie := http.Cookie{Name: cookie, Value: value}
					req.AddCookie(&cookie)
				}
			}
			info := RouteInfo{Request: req, QueryValues: req.URL.Query()}
			if test.urlParams != nil {
				info.URLParams = test.urlParams
			}
			values, has := getParamAsString(info, test.paramName, test.paramLoc)
			MustMatch(t, values, test.want)
			if len(test.want) > 0 {
				test.has = true
			}
			MustMatch(t, has, test.has)
		})
	}
}

func TestCreateTypeFromParam(t *testing.T) {
	type RGB struct {
		R, G, B int
	}
	// values taken from:
	// - https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.1.0.md#parameterStyle
	tests := []struct {
		name    string
		wantErr bool
		want    any

		style    openapi3.ParameterStyle
		location openapi3.ParameterLocation
		explode  bool

		url       string
		headers   map[string]string
		cookies   map[string]string
		urlParams map[string]string
	}{
		{
			name:     "form: string array",
			style:    openapi3.ParameterStyleForm,
			location: openapi3.ParameterLocationQuery,
			url:      "/?color=blue,black,brown",
			want:     []string{"blue", "black", "brown"},
		},
		{
			name:     "form: explode: string array",
			style:    openapi3.ParameterStyleForm,
			location: openapi3.ParameterLocationQuery,
			explode:  true,
			url:      "/?color=blue&color=black&color=brown",
			want:     []string{"blue", "black", "brown"},
		},
		{
			name:     "form: string",
			style:    openapi3.ParameterStyleForm,
			location: openapi3.ParameterLocationQuery,
			url:      "/?color=blue",
			want:     "blue",
		},
		{
			name:     "deepObject struct for query param",
			style:    openapi3.ParameterStyleDeepObject,
			location: openapi3.ParameterLocationQuery,
			url:      "/?color[R]=100&color[G]=200&color[B]=150",
			want: RGB{
				R: 100,
				G: 200,
				B: 150,
			},
		},
		{
			name:      "url param",
			location:  openapi3.ParameterLocationPath,
			url:       "/color/red",
			urlParams: map[string]string{"color": "red"},
			want:      "red",
		},
	}

	r := NewRouter("", "")
	r.Get("/", nil)
	r.Get("/color/{this-value-is-not-used}", nil, Params(struct {
		// because the router isn't being used, the path route doesn't matter above.
		// only the path name on the field does. Outside of testing, the two should
		// always match
		Color string `path:"color"`
	}{}))

	schemer := jsonschema.NewSchemer()
	p := openapi3.NewParameter()
	p.Name = "color"

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p.Explode = test.explode
			p.Style = string(test.style)
			p.In = string(test.location)

			wantType := reflect.TypeOf(test.want)
			schema, err := schemer.Get(wantType)
			MustMatch(t, err, nil, "did not expect an error")
			p.SetSchema(schema)

			req := httptest.NewRequest(http.MethodGet, test.url, nil)
			if test.headers != nil {
				for header, value := range test.headers {
					req.Header.Set(header, value)
				}
			}
			if test.cookies != nil {
				for cookie, value := range test.cookies {
					cookie := http.Cookie{Name: cookie, Value: value}
					req.AddCookie(&cookie)
				}
			}

			chiCtx := chi.NewRouteContext()
			if matched := r.mux.Match(chiCtx, req.Method, req.URL.Path); matched {
				ctx := req.Context()
				req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, chiCtx))
			}

			info, has := newRouteInfo(r.OpenAPI(), req)
			MustMatch(t, has, true, "must get a valid RouteInfo")
			if test.urlParams != nil {
				info.URLParams = test.urlParams
			}
			value, err := createTypeFromParam(wantType, p, info)
			if test.wantErr {
				MustNotMatch(t, err, nil, "expected an error got none")
			} else {
				MustMatch(t, err, nil, "did not expect an error")
				MustMatch(t, value.IsValid(), true, "valid value expected")
				MustMatch(t, test.want, value.Interface())
			}
		})
	}
}

func TestGetStructFields(t *testing.T) {
	type S struct{}
	type ContainerStruct struct{}
	tests := []struct {
		obj       any
		name      string
		wantErr   bool
		wantTypes func(s internal.Set[structField])
	}{
		{
			name:    "must provide a struct",
			obj:     1,
			wantErr: true,
		},
		{
			name: "field type not in container nor a parameter",
			obj: struct {
				A string
			}{},
			wantErr: true,
		},
		{
			name: "valid parameters",
			obj: struct {
				Header    string `header:"name"`
				Cookie    string `cookie:"name"`
				PathParam string `path:"name"`
				Query     string `query:"name"`
			}{},
		},
		{
			name: "structs not in a container show up as needed",
			obj: struct {
				S S
			}{},
			wantTypes: func(s internal.Set[structField]) {
				s.Add(structField{Type: reflect.TypeOf(S{}), needProvider: true})
			},
		},
		{
			name: "struct in container is not marked as needed, but in required inputs",
			obj: struct {
				C ContainerStruct
			}{},
			wantTypes: func(s internal.Set[structField]) {
				s.Add(structField{Type: reflect.TypeOf(ContainerStruct{}), needProvider: false})
			},
		},
		{
			name: "http types are allowed even if not in container",
			obj: struct {
				r *http.Request
				w http.ResponseWriter
			}{},
			wantTypes: func(s internal.Set[structField]) {
				s.Add(structField{Type: respWriterType})
				s.Add(structField{Type: reqType})
			},
		},
	}

	c := container.New()
	c.Provide(func() ContainerStruct { return ContainerStruct{} })

	for _, test := range tests {
		fields, err := getStructFields(reflect.TypeOf(test.obj), c)
		if test.wantErr {
			MustNotMatch(t, err, nil, "expected an error got none")
		} else {
			MustMatch(t, err, nil, "did not expect an error")
		}
		fieldSet := internal.NewSet[structField]()
		if test.wantTypes != nil {
			test.wantTypes(fieldSet)
		}
		MustMatch(t, fieldSet, fields)
	}
}
