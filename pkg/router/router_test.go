package router

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	. "chi-openapi/internal/testing"
	. "chi-openapi/pkg/openapi/operations"
)

func dummyHandler(_ http.ResponseWriter, _ *http.Request) {}

type Response struct {
	String string    `json:"string"`
	Int    int       `json:"int" min:"3"`
	Date   time.Time `json:"date"`
}

func TestRouterSimpleRoutes(t *testing.T) {
	r := NewRouter()
	r.Get("/", dummyHandler, []Option{
		JSONResponse(http.StatusOK, "OK", Response{}),
	})
	str, err := r.GenerateSpec()
	if err != nil {
		t.Error(err)
	}

	err = JSONDiff(t, str, `
    {
      "components": {
        "schemas": {
          "Response": {
            "properties": {
              "date": {
                "format": "date-time",
                "type": "string"
              },
              "int": {
                "minimum": 3,
                "type": "integer"
              },
              "string": {
                "type": "string"
              }
            },
            "type": "object"
          }
        }
      },
      "info": {
        "title": "Title",
        "version": "0.0.1"
      },
      "openapi": "3.0.1",
      "paths": {
        "/": {
          "get": {
            "responses": {
              "200": {
                "content": {
                  "application/json": {
                    "schema": {
                      "$ref": "#/components/schemas/Response"
                    }
                  }
                },
                "description": "OK"
              }
            }
          }
        }
      }
    }
    `)
	if err != nil {
		t.Error(err)
	}
}

type InputBody struct {
	Amount int    `json:"amount" min:"3" max:"4"`
	SSN    string `json:"string" pattern:"^\\d{3}-\\d{2}-\\d{4}$"`
}

func TestRouterVerifyRequestMiddleware(t *testing.T) {
	dummyR := NewRouter()
	dummyR.Get("/", dummyHandler, []Option{
		JSONBody("required data", InputBody{}),
		JSONResponse(http.StatusOK, "OK", Response{}),
	})

	r := NewRouter().With(VerifyRequest(dummyR))
	r.Get("/", dummyHandler, []Option{
		JSONBody("required data", InputBody{}),
		JSONResponse(200, "OK", Response{}),
	})

	tests := []struct {
		name   string
		body   interface{}
		method string
		route  string
		status int
	}{
		{
			name:   "invalid",
			body:   InputBody{Amount: 1},
			method: "GET",
			route:  "/",
			status: http.StatusBadRequest,
		},
		{
			name: "invalid ssn",
			body: InputBody{
				Amount: 3,
				SSN:    "123-45-689",
			},
			method: "GET",
			route:  "/",
			status: http.StatusBadRequest,
		},
		{
			name: "valid",
			body: InputBody{
				Amount: 3,
				SSN:    "123-45-6789",
			},
			method: "GET",
			route:  "/",
			status: http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var req *http.Request
			if body := test.body; body != nil {
				b, _ := json.Marshal(body)
				req = httptest.NewRequest(test.method, test.route, bytes.NewReader(b))
			} else {
				req = httptest.NewRequest(test.method, test.route, nil)
			}
			req.Header.Add("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			resp := w.Result()
			if expected := test.status; resp.StatusCode != expected {
				respBody, _ := ioutil.ReadAll(resp.Body)
				t.Errorf("Expected %v, got %v:body:\n%v", expected, resp.StatusCode, string(respBody))
			}
		})
	}
}

func responseHandler(w http.ResponseWriter, r *http.Request) {
	intQuery := r.URL.Query().Get("int")
	if intQuery == "" {
		intQuery = "1"
	}
	intValue, err := strconv.ParseInt(intQuery, 10, 64)
	if err != nil {
		panic(err)
	}
	response := Response{
		Date: time.Now(),
		Int:  int(intValue),
	}
	b, _ := json.Marshal(&response)
	w.Write(b)
}

func TestRouterVerifyResponse(t *testing.T) {
	dummyR := NewRouter()
	dummyR.Get("/", dummyHandler, []Option{
		JSONResponse(http.StatusOK, "OK", Response{}),
	})

	r := NewRouter().
		With(JSONHeader).
		With(VerifyResponse(dummyR))
	r.Get("/", responseHandler, []Option{
		JSONBody("required data", InputBody{}),
		JSONResponse(200, "OK", Response{}),
	})

	tests := []struct {
		name   string
		method string
		route  string
		status int
		query  url.Values
	}{
		{
			name:   "invalid",
			method: "GET",
			route:  "/",
			status: http.StatusInternalServerError,
		},
		{
			name:   "valid int",
			method: "GET",
			route:  "/",
			status: http.StatusOK,
			query: url.Values{
				"int": []string{"3"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.route+"?"+test.query.Encode(), nil)
			req.Header.Add("Content-Type", "application/json")

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			resp := w.Result()
			if expected := test.status; resp.StatusCode != expected {
				respBody, _ := ioutil.ReadAll(resp.Body)
				t.Errorf("Expected %v, got %v\nbody:\n%v", expected, resp.StatusCode, string(respBody))
			}
		})
	}
}

func BenchmarkRouter(b *testing.B) {
	dummyR := NewRouter()
	dummyR.Get("/", dummyHandler, []Option{
		JSONResponse(http.StatusOK, "OK", Response{}),
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Add("Content-Type", "application/json")

	b.Run("no verify middleware", func(b *testing.B) {
		r := NewRouter().
			With(JSONHeader)
		r.Get("/", responseHandler, []Option{
			JSONBody("required data", InputBody{}),
			JSONResponse(200, "OK", Response{}),
		})
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
	})

	b.Run("verify response", func(b *testing.B) {
		r := NewRouter().
			With(JSONHeader).
			With(VerifyResponse(dummyR))
		r.Get("/", responseHandler, []Option{
			JSONBody("required data", InputBody{}),
			JSONResponse(200, "OK", Response{}),
		})
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
	})

	input := InputBody{
		Amount: 5,
	}
	data, err := json.Marshal(&input)
	if err != nil {
		b.Error(err)
	}
	req = httptest.NewRequest("GET", "/", bytes.NewReader(data))
	req.Header.Add("Content-Type", "application/json")

	b.Run("verify request", func(b *testing.B) {
		r := NewRouter().
			With(JSONHeader).
			With(VerifyRequest(dummyR))
		r.Get("/", responseHandler, []Option{
			JSONBody("required data", InputBody{}),
			JSONResponse(200, "OK", Response{}),
		})

		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
	})

	b.Run("verify request and response", func(b *testing.B) {
		r := NewRouter().
			With(JSONHeader).
			With(VerifyRequest(dummyR)).
			With(VerifyResponse(dummyR))
		r.Get("/", responseHandler, []Option{
			JSONBody("required data", InputBody{}),
			JSONResponse(200, "OK", Response{}),
		})

		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
	})

}
