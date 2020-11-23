package reflection

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
	"time"

	"chi-openapi/pkg/openapi"
	. "chi-openapi/pkg/openapi/operations"
	"chi-openapi/pkg/router"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
)

// jsonHeader sets the content type to application/json
func jsonHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

type tester interface {
	Error(args ...interface{})
	Log(args ...interface{})
	Logf(msg string, args ...interface{})
	Fatal(args ...interface{})
}

func errorHandler(t tester) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, _ *http.Request, err error) {
		if re, ok := err.(*openapi3filter.RequestError); ok {
			if _, ok := re.Err.(*openapi3.SchemaError); ok {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		t.Logf("%t\n", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func dummyHandler(_ http.ResponseWriter, _ *http.Request) {}

type Response struct {
	String string    `json:"string" nullable:"true"`
	Int    int       `json:"int" min:"3"`
	Date   time.Time `json:"date"`
}

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
		SuccessFn: func(_ http.ResponseWriter, _ *http.Request, resp interface{}) {
			t.Log("response", resp)
		},
	}
}

func routerWithMiddleware(handler interface{}) *ReflectRouter {
	dummyR := NewRouter()
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
		SuccessFn: func(_ http.ResponseWriter, _ *http.Request, obj interface{}) {
			t.Logf("%+v\n", obj)
		},
	}
	dummyR := NewRouter().WithHandlers(fns)
	dummyR.Get("/path_param/{some_id}", func(ctx context.Context, param pathParam) (Response, error) {
		if param.ID != "20" {
			t.Fatalf("expected an id of 20, got: %v", param.ID)
		}
		return Response{}, nil
	}, []Option{
		Params(pathParam{}),
		JSONResponse(http.StatusOK, "OK", Response{}),
	})

	filterRouter, err := dummyR.FilterRouter()
	if err != nil {
		t.Error(err)
	}

	r := NewRouter()
	r.Use(jsonHeader)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatal(r)
				}
			}()
			next.ServeHTTP(w, r)
		})
	})
	r.Use(router.SetOpenAPIInput(filterRouter, errorHandler(t)))

	r.UseRouter(dummyR)
	components := r.Components()
	t.Log(components)

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
	dummyR := NewRouter()
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

	r := NewRouter()
	r.Use(jsonHeader)
	r.Use(router.SetOpenAPIInput(filterRouter, errorHandler(t)))
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
			SuccessFn: func(_ http.ResponseWriter, _ *http.Request, obj interface{}) {
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
		input := Response{Int: 3}
		data, err := json.Marshal(&input)
		if err != nil {
			t.Error(err)
		}
		w.Write([]byte(data))
	}

	dummyR := router.NewRouter()
	dummyR.Get("/", reflectionHandler, []Option{
		Params(reflectParmas{}),
		JSONResponse(http.StatusOK, "OK", Response{}),
	})
	filterRouter, err := dummyR.FilterRouter()
	if err != nil {
		t.Error(err)
	}

	r := router.NewRouter()
	r.Use(jsonHeader)
	r.Use(router.SetOpenAPIInput(filterRouter, errorHandler(t)))
	r.Use(router.VerifyRequest(errorHandler(t)))
	r.Use(router.VerifyResponse(errorHandler(t)))
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
	dummyR := router.NewRouter()
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
		r := router.NewRouter().
			With(jsonHeader).
			With(router.SetOpenAPIInput(filterRouter, errorHandler(b))).
			With(router.VerifyRequest(errorHandler(b))).
			With(router.VerifyResponse(errorHandler(b)))
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
	dummyR := router.NewRouter()
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
		r := router.NewRouter().
			With(jsonHeader).
			With(router.SetOpenAPIInput(filterRouter, errorHandler(b))).
			With(router.VerifyRequest(errorHandler(b))).
			With(router.VerifyResponse(errorHandler(b)))
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

func TestRouteMiddleware(t *testing.T) {
	fns := RequestHandleFns{
		ErrFn: func(_ http.ResponseWriter, _ *http.Request, err error) {
			t.Fatal(err)
		},
		SuccessFn: func(_ http.ResponseWriter, _ *http.Request, obj interface{}) {
			t.Logf("%+v\n", obj)
		},
	}
	dummyR := NewRouter().WithHandlers(fns)
	dummyR.Get("/", func(r *http.Request, ctx context.Context) error {
		if v, ok := r.Context().Value("test").(string); !ok || v != "test" {
			return fmt.Errorf("expected test, got: '%v'", v)
		}
		return nil
	}, []Option{}, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			newCtx := context.WithValue(ctx, "test", "test")
			next.ServeHTTP(w, r.WithContext(newCtx))
		})
	},
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := r.Context()
				// test and make sure this middleware gets called second
				if _, ok := ctx.Value("test").(string); !ok {
					newCtx := context.WithValue(ctx, "test", "second-func")
					r = r.WithContext(newCtx)
				}
				next.ServeHTTP(w, r)
			})
		},
	)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Add("Content-Type", "application/json")
	w := httptest.NewRecorder()

	dummyR.ServeHTTP(w, req)
}

func TestContainerProvider(t *testing.T) {
	fns := RequestHandleFns{
		ErrFn: func(_ http.ResponseWriter, _ *http.Request, err error) {
			t.Fatal(err)
		},
	}

	counter := func() func() {
		state := 0
		return func() {
			state++
			if state > 1 {
				t.Fatalf("this function should only be called once, but was called: %v times", state)
			}
			t.Log(state)
		}
	}()
	dummyR := NewRouter().WithHandlers(fns)
	err := dummyR.Provide(func() (string, int, error) {
		// make sure this function is cached
		// will error if called more than once
		counter()
		return "hello world", 1, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	dummyR.Get("/", func(r *http.Request, ctx context.Context, someStr, another string, number int) error {
		if someStr != "hello world" || another != "hello world" || number != 1 {
			return fmt.Errorf("expected 'hello world', got: %v", someStr)
		}
		return nil
	}, []Option{})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Add("Content-Type", "application/json")
	w := httptest.NewRecorder()

	dummyR.ServeHTTP(w, req)
}
