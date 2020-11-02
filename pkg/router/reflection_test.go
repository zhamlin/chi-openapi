package router

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"chi-openapi/pkg/openapi"
	. "chi-openapi/pkg/openapi/operations"
)

type reflectInput struct {
	Name string `json:"name"`
}
type reflectParmas struct {
	Int int `query:"int" required:"true"`
}

func reflectionHandler(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("{}"))
}

func reflectionHandlerAuto(w http.ResponseWriter, params reflectParmas, input reflectInput) {
	w.Write([]byte("{}"))
}

func reflectionHandlerParams(w http.ResponseWriter, params reflectParmas) {
	w.Write([]byte("{}"))
}

func reflectionHandlerBody(w http.ResponseWriter, body reflectInput) {
	response := Response{
		Int:    3,
		String: body.Name,
	}
	b, _ := json.Marshal(&response)
	w.Write(b)
}

func reflectionHandlerBackward(ctx context.Context, w http.ResponseWriter) {
	w.Write([]byte("{}"))
}

func reflectionHandlerNew(ctx context.Context) (Response, error) {
	return Response{}, nil
}

func errHandler(t tester) HandleFns {
	return HandleFns{
		ErrFn: func(_ http.ResponseWriter, err error) {
			t.Log("error", err)
		},
		SuccessFn: func(_ http.ResponseWriter, resp interface{}) {
			t.Log("response", resp)
		},
	}
}

func TestReflectionFuncSimple(t *testing.T) {
	handler, err := HandlerFromFnDefault(reflectionHandlerBackward, errHandler(t), openapi.NewComponents())
	if err != nil {
		t.Error(err)
	}
	req := httptest.NewRequest("GET", "/?int=1", nil)
	req.Header.Add("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)
	b, _ := ioutil.ReadAll(w.Body)

	t.Log("body", string(b))
}

func TestReflectionFuncBody(t *testing.T) {
	components := openapi.NewComponents()
	openapi.SchemaFromObj(components.Schemas, reflectInput{})
	handler, err := HandlerFromFnDefault(reflectionHandlerBody, errHandler(t), components)
	if err != nil {
		t.Error(err)
	}

	input := reflectInput{Name: "o"}
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
	r.Use(VerifyRequest(filterRouter, errorHandler(t)))
	r.Use(VerifyResponse(filterRouter, errorHandler(t)))
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

	input := reflectInput{Name: "o"}
	data, err := json.Marshal(&input)
	if err != nil {
		b.Error(err)
	}
	req = httptest.NewRequest("GET", "/", bytes.NewReader(data))
	req.Header.Add("Content-Type", "application/json")

	components := openapi.NewComponents()
	openapi.SchemaFromObj(components.Schemas, reflectInput{})
	handler, err := HandlerFromFnDefault(reflectionHandlerBody, errHandler(b), components)
	if err != nil {
		b.Error(err)
	}

	b.Run("verify request and response", func(b *testing.B) {
		r := NewRouter().
			With(jsonHeader).
			With(VerifyRequest(filterRouter, errorHandler(b))).
			With(VerifyResponse(filterRouter, errorHandler(b)))
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
