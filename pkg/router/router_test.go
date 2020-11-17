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

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi"
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
}

func errorHandler(t tester) ErrorHandler {
	return func(w http.ResponseWriter, _ *http.Request, err error) {
		if re, ok := err.(*openapi3filter.RequestError); ok {
			if _, ok := re.Err.(*openapi3.SchemaError); ok {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		w.WriteHeader(http.StatusInternalServerError)
	}
}

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
            "type": "object",
            "required": [
                "string",
                "int",
                "date"
            ]
          }
        }
      },
      "info": {
        "title": "Title",
        "version": "0.0.1"
      },
      "openapi": "3.0.0",
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

type TestParams struct {
	Filter int `query:"filter" min:"3"`
}

func TestRouterVerifyRequestMiddleware(t *testing.T) {
	dummyR := NewRouter()
	dummyR.Get("/", dummyHandler, []Option{
		Params(TestParams{}),
		JSONBody("required data", InputBody{}),
		JSONResponse(http.StatusOK, "OK", Response{}),
	})
	filterRouter, err := dummyR.FilterRouter()
	if err != nil {
		t.Error(err)
		t.Fail()
	}

	r := NewRouter().
		With(SetOpenAPIInput(filterRouter, errorHandler(t))).
		With(VerifyRequest(errorHandler(t)))
	r.UseRouter(dummyR)

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
		{
			name: "invalid query param",
			body: InputBody{
				Amount: 3,
				SSN:    "123-45-6789",
			},
			method: "GET",
			route:  "/?filter=1",
			status: http.StatusBadRequest,
		},
		{
			name: "valid query param",
			body: InputBody{
				Amount: 3,
				SSN:    "123-45-6789",
			},
			method: "GET",
			route:  "/?filter=3",
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
	router := NewRouter()
	router.Get("/", responseHandler, []Option{
		JSONBody("required data", InputBody{}),
		JSONResponse(http.StatusOK, "OK", Response{}),
	})

	filterRouter, err := router.FilterRouter()
	if err != nil {
		t.Error(err)
	}
	r := NewRouter().
		With(jsonHeader).
		With(SetOpenAPIInput(filterRouter, errorHandler(t))).
		With(VerifyResponse(errorHandler(t)))
	r.Mount("/", router)

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

func TestRouterDefaultResponse(t *testing.T) {
	type Error struct {
		Description string `json:"description"`
	}

	router := NewRouter()
	router.Get("/", responseHandler, []Option{
		JSONResponse(http.StatusOK, "OK", nil),
	})
	router.Get("/defaultResponse", responseHandler, []Option{
		JSONResponse(http.StatusOK, "OK", nil),
		DefaultJSONResponse("default response", nil),
	})

	r := NewRouter().With(jsonHeader)
	r.SetDefaultJSON("unexpected error", Error{})
	r.SetStatusDefault(http.StatusNotFound, "NotFound", nil)
	r.Mount("/", router)

	spec, err := r.GenerateSpec()
	if err != nil {
		t.Fatal(err)
	}
	err = JSONDiff(t, spec, `
    {
      "components": {
        "responses": {
          "404": {
            "description": "NotFound"
          },
          "default": {
            "content": {
              "application/json": {
                "schema": {
                  "$ref": "#/components/schemas/Error"
                }
              }
            },
            "description": "unexpected error"
          }
        },
        "schemas": {
          "Error": {
            "properties": {
              "description": {
                "type": "string"
              }
            },
            "type": "object",
              "required": [
                  "description"
              ]
          }
        }
      },
      "info": {
        "title": "Title",
        "version": "0.0.1"
      },
      "openapi": "3.0.0",
      "paths": {
        "/": {
          "get": {
            "responses": {
              "200": {
                "description": "OK"
              },
              "404": {
                "$ref": "#/components/responses/404"
              },
              "default": {
                "$ref": "#/components/responses/default"
              }
            }
          }
        },
        "/defaultResponse": {
          "get": {
            "responses": {
              "200": {
                "description": "OK"
              },
              "404": {
                "$ref": "#/components/responses/404"
              },
              "default": {
                "description": "default response"
              }
            }
          }
        }
      }
    }
    `)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRouterMapComponents(t *testing.T) {
	type Other struct {
		String string `json:"string"`
	}
	type mapper struct {
		Map map[string]Other `json:"map"`
	}

	router := NewRouter()
	router.Get("/", responseHandler, []Option{
		JSONResponse(http.StatusOK, "OK", mapper{}),
	})

	r := NewRouter().With(jsonHeader)
	r.Mount("/", router)

	spec, err := r.GenerateSpec()
	if err != nil {
		t.Fatal(err)
	}
	err = JSONDiff(t, spec, `
    {
      "components": {
        "schemas": {
          "Other": {
            "properties": {
              "string": {
                "type": "string"
              }
            },
            "type": "object",
            "required": [
              "string"
            ]
          },
          "mapper": {
            "properties": {
              "map": {
                "additionalProperties": {
                  "$ref": "#/components/schemas/Other"
                },
                "type": "object"
              }
            },
            "type": "object",
            "required": [
              "map"
            ]
          }
        }
      },
      "info": {
        "title": "Title",
        "version": "0.0.1"
      },
      "openapi": "3.0.0",
      "paths": {
        "/": {
          "get": {
            "responses": {
              "200": {
                "content": {
                  "application/json": {
                    "schema": {
                      "$ref": "#/components/schemas/mapper"
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
		t.Fatal(err)
	}
}

func BenchmarkRouter(b *testing.B) {
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

	b.Run("chi router", func(b *testing.B) {
		r := chi.NewRouter().
			With(jsonHeader)
		r.Get("/", responseHandler)
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			r.ServeHTTP(w, req)
		}
	})

	b.Run("no verify middleware", func(b *testing.B) {
		r := NewRouter().
			With(jsonHeader)
		r.Get("/", responseHandler, []Option{
			JSONBody("required data", InputBody{}),
			JSONResponse(200, "OK", Response{}),
		})
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			r.ServeHTTP(w, req)
		}
	})

	b.Run("verify response", func(b *testing.B) {
		r := NewRouter().
			With(jsonHeader).
			With(SetOpenAPIInput(filterRouter, errorHandler(b))).
			With(VerifyResponse(errorHandler(b)))
		r.Get("/", responseHandler, []Option{
			JSONBody("required data", InputBody{}),
			JSONResponse(200, "OK", Response{}),
		})
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
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
			With(jsonHeader).
			With(SetOpenAPIInput(filterRouter, errorHandler(b))).
			With(VerifyRequest(errorHandler(b)))
		r.Get("/", responseHandler, []Option{
			JSONBody("required data", InputBody{}),
			JSONResponse(200, "OK", Response{}),
		})

		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			r.ServeHTTP(w, req)
		}
	})

	b.Run("verify request and response", func(b *testing.B) {
		r := NewRouter().
			With(jsonHeader).
			With(SetOpenAPIInput(filterRouter, errorHandler(b))).
			With(VerifyRequest(errorHandler(b))).
			With(VerifyResponse(errorHandler(b)))
		r.Get("/", responseHandler, []Option{
			JSONBody("required data", InputBody{}),
			JSONResponse(200, "OK", Response{}),
		})

		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			r.ServeHTTP(w, req)
		}
	})

}
