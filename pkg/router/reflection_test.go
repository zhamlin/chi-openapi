package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"chi-openapi/pkg/openapi"
	. "chi-openapi/pkg/openapi/operations"
)

type reflectInput struct {
	Value string `json:"name"`
}
type reflectParmas struct {
	Int int `query:"int" required:"true"`
}

func reflectionHandlerBody(w http.ResponseWriter, body reflectInput) {
	response := Response{
		Int:    3,
		String: body.Value,
	}
	b, _ := json.Marshal(&response)
	w.Write(b)
}

func reflectionHandlerReturnErr(body reflectInput) error {
	return fmt.Errorf("error " + body.Value)
}

func errHandler(t tester) RequestHandleFns {
	return RequestHandleFns{
		ErrFn: func(_ http.ResponseWriter, _ *http.Request, err error) {
			t.Log("error", err)
		},
		SuccessFn: func(_ http.ResponseWriter, resp interface{}) {
			t.Log("response", resp)
		},
	}
}

func routerWithMiddleware(handler interface{}) *ReflectRouter {
	dummyR := NewReflectRouter(RequestHandleFns{})
	dummyR.Get("/", handler, []Option{
		Params(reflectParmas{}),
		JSONResponse(http.StatusOK, "OK", Response{}),
	})
	return dummyR
}

func TestReflectionPathParams(t *testing.T) {
	type pathParam struct {
		ID string `path:"some_id"`
	}
	fns := RequestHandleFns{
		ErrFn: func(_ http.ResponseWriter, _ *http.Request, err error) {
			t.Errorf("expected: 'error %s', got: %v", "test", err)
		},
		SuccessFn: func(_ http.ResponseWriter, obj interface{}) {
			t.Logf("%+v\n", obj)
		},
	}
	dummyR := NewReflectRouter(fns)
	dummyR.Get("/path_param/{some_id}", func(ctx context.Context, param pathParam) (Response, error) {
		t.Logf("%+v\n", param)
		return Response{}, nil
	}, []Option{
		Params(pathParam{}),
		JSONResponse(http.StatusOK, "OK", Response{}),
	})

	filterRouter, err := dummyR.FilterRouter()
	if err != nil {
		t.Error(err)
	}

	r := NewReflectRouter(RequestHandleFns{})
	r.Use(jsonHeader)
	r.Use(SetOpenAPIInput(filterRouter, errorHandler(t)))
	r.UseRouter(dummyR)
	components := r.Components()

	t.Run("simple path", func(t *testing.T) {
		handler, err := HandlerFromFnDefault(r.ServeHTTP, fns, components)
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest("GET", "/path_param/20", nil)
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()

		ctx := context.Background()
		handler(w, req.WithContext(ctx))
	})
}

func TestReflectionFuncReturns(t *testing.T) {
	dummyR := NewReflectRouter(RequestHandleFns{})
	dummyR.Get("/multi_return", func(ctx context.Context, params reflectParmas) (Response, error) {
		t.Logf("%+v\n", params)
		if params.Int < 0 {
			return Response{}, fmt.Errorf("")
		}
		return Response{}, nil
	}, []Option{
		Params(reflectParmas{}),
		JSONResponse(http.StatusOK, "OK", Response{}),
	})

	filterRouter, err := dummyR.FilterRouter()
	if err != nil {
		t.Error(err)
	}

	r := NewReflectRouter(RequestHandleFns{})
	r.Use(jsonHeader)
	r.Use(SetOpenAPIInput(filterRouter, errorHandler(t)))
	r.UseRouter(dummyR)

	components := r.Components()
	openapi.SchemaFromObj(reflectInput{}, components.Schemas)

	t.Run("error only return", func(t *testing.T) {
		input := reflectInput{Value: "name"}
		handler, err := HandlerFromFnDefault(reflectionHandlerReturnErr, RequestHandleFns{
			ErrFn: func(_ http.ResponseWriter, _ *http.Request, err error) {
				if err.Error() != "error "+input.Value {
					t.Errorf("expected: 'error %s', got: %v", input.Value, err)
				}
			},
		}, components)
		if err != nil {
			t.Fatal(err)
		}

		data, err := json.Marshal(&input)
		if err != nil {
			t.Error(err)
		}

		req := httptest.NewRequest("GET", "/", bytes.NewReader(data))
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler(w, req)
	})

	t.Run("multiple return", func(t *testing.T) {
		input := reflectInput{Value: "name"}
		handler, err := HandlerFromFnDefault(r.ServeHTTP, RequestHandleFns{
			ErrFn: func(_ http.ResponseWriter, _ *http.Request, err error) {
				if err.Error() != "error "+input.Value {
					t.Errorf("expected: 'error %s', got: %v", input.Value, err)
				}
			},
			SuccessFn: func(_ http.ResponseWriter, obj interface{}) {
			},
		}, components)
		if err != nil {
			t.Fatal(err)
		}

		data, err := json.Marshal(&input)
		if err != nil {
			t.Error(err)
		}

		req := httptest.NewRequest("GET", "/multi_return?int=2", bytes.NewReader(data))
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()

		ctx := context.Background()
		handler(w, req.WithContext(ctx))
	})
}

// TODO: test request body
func TestReflectionFuncBody(t *testing.T) {
	components := openapi.NewComponents()
	openapi.SchemaFromObj(reflectInput{}, components.Schemas)
	handler, err := HandlerFromFnDefault(reflectionHandlerBody, errHandler(t), components)
	if err != nil {
		t.Error(err)
	}

	input := reflectInput{Value: "name"}
	// input := struct {
	// 	Int    int    `json:"int"`
	// 	String string `json:"string"`
	// }{String: "test", Int: 3}
	data, err := json.Marshal(&input)
	if err != nil {
		t.Error(err)
	}

	req := httptest.NewRequest("GET", "/?int=1", bytes.NewReader(data))
	req.Header.Add("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)
	b, err := ioutil.ReadAll(w.Body)

	t.Log("body", string(b))
}

func TestReflectionHandler(t *testing.T) {
	reflectionHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("{}"))
	}

	dummyR := NewRouter()
	dummyR.Get("/", reflectionHandler, []Option{
		Params(reflectParmas{}),
		JSONResponse(http.StatusOK, "OK", Response{}),
	})
	filterRouter, err := dummyR.FilterRouter()
	if err != nil {
		t.Error(err)
	}

	r := NewRouter()
	r.Use(jsonHeader)
	r.Use(SetOpenAPIInput(filterRouter, errorHandler(t)))
	r.Use(VerifyRequest(errorHandler(t)))
	r.Use(VerifyResponse(errorHandler(t)))
	r.UseRouter(dummyR)

	t.Run("normal", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?int=1", nil)
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Got %v, expected 200", w.Code)
		}
	})

	t.Run("normal", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?int=1", nil)
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Got %v, expected 200", w.Code)
		}
	})

}

func BenchmarkReflection(b *testing.B) {
	dummyR := NewRouter()
	dummyR.Use(jsonHeader)
	dummyR.Get("/", dummyHandler, []Option{
		JSONResponse(http.StatusOK, "OK", Response{}),
	})
	filterRouter, err := dummyR.FilterRouter()
	if err != nil {
		b.Error(err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Add("Content-Type", "application/json")
	w := httptest.NewRecorder()

	input := reflectInput{Value: "o"}
	data, err := json.Marshal(&input)
	if err != nil {
		b.Error(err)
	}
	req = httptest.NewRequest("GET", "/", bytes.NewReader(data))
	req.Header.Add("Content-Type", "application/json")

	components := openapi.NewComponents()
	openapi.SchemaFromObj(reflectInput{}, components.Schemas)
	handler, err := HandlerFromFnDefault(reflectionHandlerBody, RequestHandleFns{}, components)
	if err != nil {
		b.Error(err)
	}

	b.Run("verify request and response", func(b *testing.B) {
		r := NewRouter().
			With(jsonHeader).
			With(SetOpenAPIInput(filterRouter, errorHandler(b))).
			With(VerifyRequest(errorHandler(b))).
			With(VerifyResponse(errorHandler(b)))
		r.Get("/", handler, []Option{
			JSONBody("required data", reflectInput{}),
			JSONResponse(200, "OK", Response{}),
		})

		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			r.ServeHTTP(w, req)
		}
	})
}

func BenchmarkReflectionQueryParams(b *testing.B) {
	dummyR := NewRouter()
	dummyR.Use(jsonHeader)
	dummyR.Get("/", dummyHandler, []Option{
		Params(reflectParmas{}),
		JSONResponse(http.StatusOK, "OK", Response{}),
	})
	filterRouter, err := dummyR.FilterRouter()
	if err != nil {
		b.Error(err)
	}

	reflectionHandlerReturnMultiple := func(ctx context.Context, params reflectParmas) (Response, error) {
		if params.Int < 0 {
			return Response{}, fmt.Errorf("")
		}
		return Response{}, nil
	}

	components := openapi.NewComponents()
	openapi.SchemaFromObj(reflectInput{}, components.Schemas)
	handler, err := HandlerFromFnDefault(reflectionHandlerReturnMultiple, RequestHandleFns{}, components)
	if err != nil {
		b.Fatal(err)
	}

	v := url.Values{
		"int": []string{"10"},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.URL.RawQuery = v.Encode()
	req.Header.Add("Content-Type", "application/json")

	b.Run("load simple query param", func(b *testing.B) {
		r := NewRouter().
			With(jsonHeader).
			With(SetOpenAPIInput(filterRouter, errorHandler(b))).
			With(VerifyRequest(errorHandler(b))).
			With(VerifyResponse(errorHandler(b)))
		r.Get("/", handler, []Option{
			Params(reflectParmas{}),
			JSONBody("required data", reflectInput{}),
			JSONResponse(200, "OK", Response{}),
		})

		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			r.ServeHTTP(w, req)
		}
	})
}
