package router

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	. "chi-openapi/internal/testing"
	. "chi-openapi/pkg/openapi/operations"
)

func dummyHandler(_ http.ResponseWriter, _ *http.Request) {}

type Response struct {
	Name string `json:"name"`
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
              "name": {
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

func TestRouterVerifyMiddleware(t *testing.T) {
	dummyR := NewRouter()
	dummyR.Get("/", dummyHandler, []Option{
		JSONBody("required data", InputBody{}),
		JSONResponse(http.StatusOK, "OK", Response{}),
	})

	r := NewRouter().With(Test(dummyR))
	r.Get("/", dummyHandler, []Option{
		JSONBody("required data", InputBody{}),
		JSONResponse(200, "OK", Response{}),
	})

	t.Run("invalid", func(t *testing.T) {
		body, _ := json.Marshal(InputBody{Amount: 1})
		req := httptest.NewRequest("GET", "/", bytes.NewReader(body))
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		if expected := http.StatusBadRequest; resp.StatusCode != expected {
			respBody, _ := ioutil.ReadAll(resp.Body)
			t.Errorf("Expected %v, got %v:body:\n%v", expected, resp.StatusCode, string(respBody))
		}
	})

	t.Run("invalid ssn", func(t *testing.T) {
		body, _ := json.Marshal(InputBody{
			Amount: 3,
			SSN:    "123-45-689",
		})
		req := httptest.NewRequest("GET", "/", bytes.NewReader(body))
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		if expected := http.StatusBadRequest; resp.StatusCode != expected {
			respBody, _ := ioutil.ReadAll(resp.Body)
			t.Errorf("Expected %v, got %v:body:\n%v", expected, resp.StatusCode, string(respBody))
		}
	})

	t.Run("valid", func(t *testing.T) {
		body, _ := json.Marshal(InputBody{
			Amount: 3,
			SSN:    "123-45-6789",
		})
		req := httptest.NewRequest("GET", "/", bytes.NewReader(body))
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		resp := w.Result()
		if expected := http.StatusOK; resp.StatusCode != expected {
			respBody, _ := ioutil.ReadAll(resp.Body)
			t.Errorf("Expected %v, got %v:body:\n%v", expected, resp.StatusCode, string(respBody))
		}
	})
}
