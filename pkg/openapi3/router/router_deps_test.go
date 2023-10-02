package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	. "github.com/zhamlin/chi-openapi/internal/testing"
	"github.com/zhamlin/chi-openapi/pkg/container"
	. "github.com/zhamlin/chi-openapi/pkg/openapi3/operations"
	"github.com/zhamlin/chi-openapi/pkg/openapi3/router"
)

func newDepRouter(t Tester) *router.DepRouter {
	router := router.NewDepRouter("", "").
		WithRequestHandler(func(_ http.ResponseWriter, _ *http.Request, _ any, err error) {
			MustMatch(t, err, nil, "request handler returned an error")
		})
	router.PanicOnError(true)
	return router
}

func do(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	return resp
}

type CustomParam struct {
	Name string
}

func (p *CustomParam) UnmarshalText(text []byte) error {
	p.Name = string(text)
	return nil
}

func TestDepRouterLoadParams(t *testing.T) {
	type ParamObj struct {
		String string `query:"string"`
		Number int    `query:"number"`

		// TextUnmarshaler
		Custom      CustomParam  `query:"custom"`
		OtherCustom *CustomParam `query:"other"`

		Default string `query:"default" default:"default-value"`
	}

	h := func(w http.ResponseWriter, r *http.Request, p ParamObj, otherParams struct {
		Name string `query:"name"`
	}) {
		MustMatch(t, "foo", p.String)
		MustMatch(t, "bar", p.Custom.Name)
		MustMatch(t, "foobar", p.OtherCustom.Name)
		MustMatch(t, "default-value", p.Default)
		MustMatch(t, "name", otherParams.Name)
		MustMatch(t, 2, p.Number)
		w.WriteHeader(http.StatusCreated)
	}

	r := newDepRouter(t)
	r.Get("/", h,
		ResponseAs[struct{ A string }]("A", http.StatusOK, ""),
	)

	req := httptest.NewRequest(http.MethodGet, "/?string=foo&custom=bar&other=foobar&number=2&name=name", nil)
	resp := do(r, req)
	MustMatch(t, resp.Code, http.StatusCreated)
}

func TestDepRouterLoadBody(t *testing.T) {
	type RequestBody struct {
		String string `json:"string"`
		Number int    `json:"number"`
	}
	reqBody := RequestBody{String: "str", Number: -1}

	h := func(w http.ResponseWriter, r *http.Request, params struct {
		Body RequestBody `request:"body" doc:"input"`
	}) {
		MustMatch(t, reqBody.Number, params.Body.Number)
		MustMatch(t, reqBody.String, params.Body.String)
	}

	r := newDepRouter(t)
	r.Get("/", h, ResponseAs[struct{ A string }]("A", http.StatusOK, ""))

	arrayHandler := func(w http.ResponseWriter, r *http.Request, params struct {
		Body []RequestBody `request:"body" doc:"input"`
	}) {
		MustMatch(t, reqBody.Number, params.Body[0].Number)
		MustMatch(t, reqBody.String, params.Body[0].String)
	}
	r.Get("/array", arrayHandler, ResponseAs[struct{ A string }]("A", http.StatusOK, ""))

	data := MustMarshal(t, reqBody)
	req := httptest.NewRequest(http.MethodGet, "/", bytes.NewBufferString(data))
	resp := do(r, req)
	MustMatch(t, resp.Code, http.StatusOK)

	body := []RequestBody{reqBody}
	data = MustMarshal(t, body)
	req = httptest.NewRequest(http.MethodGet, "/array", bytes.NewBufferString(data))
	resp = do(r, req)
	MustMatch(t, resp.Code, http.StatusOK)

	MustMatchAsJson(t, r.OpenAPI(), `
    {
        "components": {
            "schemas": {
                "A": {
                    "properties": {
                        "A": {
                            "type": "string"
                        }
                    },
                    "type": "object"
                },
                "RequestBody": {
                    "type": "object",
                    "properties": {
                        "number": {
                            "type": "integer"
                        },
                        "string": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "info": {
            "version": "",
            "title": ""
        },
        "openapi": "3.1.0",
        "paths": {
            "/": {
                "get": {
                    "requestBody": {
                        "content": {
                            "application/json": {
                                "schema": {
                                    "$ref": "#/components/schemas/RequestBody"
                                }
                            }
                        },
                        "description": "input",
                        "required": true
                    },
                    "responses": {
                        "200": {
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "$ref": "#/components/schemas/A"
                                    }
                                }
                            }
                        }
                    }
                }
            },
            "/array": {
                "get": {
                    "requestBody": {
                        "content": {
                            "application/json": {
                                "schema": {
                                    "items": {
                                        "$ref": "#/components/schemas/RequestBody"
                                    },
                                    "type": "array"
                                }
                            }
                        },
                        "description": "input",
                        "required": true
                    },
                    "responses": {
                        "200": {
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "$ref": "#/components/schemas/A"
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    `)
}

func TestDepRouterParamStyles(t *testing.T) {
	// taken from: https://spec.openapis.org/oas/v3.1.0#styleValues
	// string -> "blue"
	// array -> ["blue","black","brown"]
	// object -> { "R": 100, "G": 200, "B": 150 }
	type ParamObj struct {
		String string   `query:"string"`
		Color  string   `path:"color"`
		Array  []string `query:"array"`
		Object struct {
			R, G, B int
		} `query:"object" style:"deepObject"`
	}

	h := func(w http.ResponseWriter, r *http.Request, p ParamObj) {
		MustMatch(t, "red", p.Color)
		MustMatch(t, "string", p.String)
		MustMatch(t, []string{"a", "b", "c"}, p.Array)
		MustMatch(t, 100, p.Object.R)
		w.WriteHeader(http.StatusCreated)
	}

	r := newDepRouter(t)
	r.Get("/", h,
		Params(ParamObj{}),
		Response[None](http.StatusOK, ""),
	)
	subRouter := newDepRouter(t)
	subRouter.Get("/color/{color}", h,
		Params(ParamObj{}),
		Response[struct{ A string }](http.StatusOK, ""),
	)
	r.Mount("/v1/", subRouter)

	queryParams := "object[R]=100" + "&string=string" + "&array=a,b,c"
	req := httptest.NewRequest(http.MethodGet, "/v1/color/red?"+queryParams, nil)
	resp := do(r, req)
	MustMatch(t, resp.Code, http.StatusCreated)
}

func TestDepRouterGroup(t *testing.T) {
	r := newDepRouter(t)
	type A struct {
		Foo string
	}
	type B struct {
		Bar string
	}
	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}

	r.Group(func(r *router.DepRouter) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
				w.Header().Set("content-type", "application/json")
			})
		})
		r.Get("/group", h, Response[A](http.StatusOK, ""))
	})
	r.Get("/no-middle", h, Response[B](http.StatusOK, ""))

	req := httptest.NewRequest(http.MethodGet, "/group", nil)
	resp := do(r, req)

	MustMatch(t, resp.Header().Get("content-type"), "application/json",
		"expected router group Use() middleware to run")

	req = httptest.NewRequest(http.MethodGet, "/no-middle", nil)
	resp = do(r, req)

	MustMatch(t, resp.Header().Get("content-type"), "",
		"expected no middleware to run")
	MustMatch(t, resp.Code, http.StatusCreated, "wanted 201")

	MustMatchAsJson(t, r.OpenAPI(), `{
        "components": {
            "schemas": {
                "A": {
                    "properties": {
                        "Foo": {
                            "type": "string"
                        }
                    },
                    "type": "object"
                },
                "B": {
                    "type": "object",
                    "properties": {
                        "Bar": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "info": {
            "title": "",
            "version": ""
        },
        "openapi": "3.1.0",
        "paths": {
            "/group": {
                "get": {
                    "responses": {
                        "200": {
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "$ref": "#/components/schemas/A"
                                    }
                                }
                            }
                        }
                    }
                }
            },
            "/no-middle": {
                "get": {
                    "responses": {
                        "200": {
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "$ref": "#/components/schemas/B"
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }`)
}

func TestDepRouterMount(t *testing.T) {
	r := newDepRouter(t)
	h := func(w http.ResponseWriter, r *http.Request) {}
	type FooResp struct {
		Field string
	}

	r.Get("/foo", h, Response[FooResp](http.StatusOK, ""))

	rootRouter := newRouter()
	rootRouter.Mount("/v1", r)
	rootRouter.Get("/bar", h, ResponseAs[struct {
		Foo FooResp
	}]("Bar", http.StatusOK, ""))

	MustMatchAsJson(t, rootRouter.OpenAPI(), `
        {
            "components": {
                "schemas": {
                    "Bar": {
                        "properties": {
                            "Foo": {
                                "$ref": "#/components/schemas/FooResp"
                            }
                        },
                        "type": "object"
                    },
                    "FooResp": {
                        "properties": {
                            "Field": {
                                "type": "string"
                            }
                        },
                        "type": "object"
                    }
                }
            },
            "info": {
                "title": "",
                "version": ""
            },
            "openapi": "3.1.0",
            "paths": {
                "/bar": {
                    "get": {
                        "responses": {
                            "200": {
                                "content": {
                                    "application/json": {
                                        "schema": {
                                            "$ref": "#/components/schemas/Bar"
                                        }
                                    }
                                }
                            }
                        }
                    }
                },
                "/v1/foo": {
                    "get": {
                        "responses": {
                            "200": {
                                "content": {
                                    "application/json": {
                                        "schema": {
                                            "$ref": "#/components/schemas/FooResp"
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }`)
}

func TestDepRouterRoute(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {}
	type FooResp struct {
		Field string
	}

	rootRouter := newRouter()
	rootRouter.Route("/v1", func(r *router.Router) {
		r.Get("/foo", h, Response[FooResp](http.StatusOK, ""))
	})

	rootRouter.Get("/bar", h, ResponseAs[struct {
		Foo FooResp
	}]("Bar", http.StatusOK, ""))

	MustMatchAsJson(t, rootRouter.OpenAPI(), `
        {
            "components": {
                "schemas": {
                    "Bar": {
                        "properties": {
                            "Foo": {
                                "$ref": "#/components/schemas/FooResp"
                            }
                        },
                        "type": "object"
                    },
                    "FooResp": {
                        "properties": {
                            "Field": {
                                "type": "string"
                            }
                        },
                        "type": "object"
                    }
                }
            },
            "info": {
                "title": "",
                "version": ""
            },
            "openapi": "3.1.0",
            "paths": {
                "/bar": {
                    "get": {
                        "responses": {
                            "200": {
                                "content": {
                                    "application/json": {
                                        "schema": {
                                            "$ref": "#/components/schemas/Bar"
                                        }
                                    }
                                }
                            }
                        }
                    }
                },
                "/v1/foo": {
                    "get": {
                        "responses": {
                            "200": {
                                "content": {
                                    "application/json": {
                                        "schema": {
                                            "$ref": "#/components/schemas/FooResp"
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }`)
}

func TestDepRouterWithMiddleware(t *testing.T) {
	r := newDepRouter(t)

	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)

		ctx := r.Context()
		custom, has := ctx.Value("custom").(string)
		if has {
			w.Header().Set("custom", custom)
		}
	}
	middle := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//nolint
			ctx := context.WithValue(r.Context(), "custom", "hello world")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			w.Header().Set("content-type", "application/json")
		})
	})
	r.With(middle).Get("/middle", h)
	r.Get("/no-middle", h)

	req := httptest.NewRequest(http.MethodGet, "/middle", nil)
	resp := do(r, req)

	MustMatch(t, resp.Header().Get("custom"), "hello world",
		"expected router.With() middleware to run")

	MustMatch(t, resp.Header().Get("content-type"), "application/json",
		"expected router.Use() middleware to run")

	req = httptest.NewRequest(http.MethodGet, "/no-middle", nil)
	resp = do(r, req)

	MustMatch(t, resp.Code, http.StatusCreated)
	MustMatch(t, resp.Header().Get("custom"), "",
		"expected router.With() middleware to not run r")
}

func TestDepRouterRequestHandler(t *testing.T) {
	rootRouter := newDepRouter(t)
	rootRouter = rootRouter.WithRequestHandler(
		func(w http.ResponseWriter, _ *http.Request, _ any, _ error) {
			w.WriteHeader(http.StatusTeapot)
		},
	)
	subRouter := newDepRouter(t)
	subRouter = subRouter.WithRequestHandler(
		func(w http.ResponseWriter, _ *http.Request, _ any, _ error) {
			w.WriteHeader(http.StatusNotImplemented)
		},
	)
	subSubRouter := newDepRouter(t)
	subSubRouter = subSubRouter.WithRequestHandler(
		func(w http.ResponseWriter, _ *http.Request, _ any, _ error) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	)

	h := func(w http.ResponseWriter, r *http.Request) error {
		return errors.New("error")
	}
	subSubRouter.Get("/", h)
	subRouter.Mount("/", subSubRouter)
	rootRouter.Mount("/", subRouter)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := do(rootRouter, req)
	// the rootRouter RequestHandler takes precedence over all routers below it
	MustMatch(t, resp.Code, http.StatusTeapot)

}

func TestDepRouterNested(t *testing.T) {
	rootRouter := newDepRouter(t)
	rootRouter = rootRouter.WithRequestHandler(
		func(w http.ResponseWriter, _ *http.Request, _ any, _ error) {
			w.WriteHeader(http.StatusTeapot)
		},
	)
	subRouter := newDepRouter(t)
	subRouter = subRouter.WithRequestHandler(
		func(w http.ResponseWriter, _ *http.Request, _ any, _ error) {
			w.WriteHeader(http.StatusNotImplemented)
		},
	)
	subSubRouter := newDepRouter(t)
	subSubRouter = subSubRouter.WithRequestHandler(
		func(w http.ResponseWriter, _ *http.Request, _ any, _ error) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	)

	h := func(r *http.Request, params struct {
		ID string `path:"id"`
	}) error {
		info, has := router.GetRouteInfo(r.Context())
		MustMatch(t, has, true)
		MustMatch(t, info.URLParams["id"], "1")
		MustMatch(t, params.ID, "1")
		return errors.New("error")
	}
	subSubRouter.Route("/object", func(r *router.DepRouter) {
		r.Get("/{id}", h)
	})
	subRouter.Mount("/nested", subSubRouter)
	rootRouter.Mount("/", subRouter)

	req := httptest.NewRequest(http.MethodGet, "/nested/object/1", nil)
	resp := do(rootRouter, req)
	// the rootRouter RequestHandler takes precedence over all routers below it
	MustMatch(t, resp.Code, http.StatusTeapot)
}

func TestDepRouterNoRouteInfoNeeded(t *testing.T) {
	c := container.New()
	type A struct{}
	c.Provide(A{})

	r := newDepRouter(t).WithContainer(c)
	h := func(w http.ResponseWriter, r *http.Request, _ A) {
		_, has := router.GetRouteInfo(r.Context())
		MustMatch(t, has, false)
		w.WriteHeader(http.StatusLocked)
	}
	r.Get("/", h)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := do(r, req)
	MustMatch(t, resp.Code, http.StatusLocked)
}

func BenchmarkDepRouter(b *testing.B) {
	type RequestBody struct {
		String string `json:"string"`
		Number int    `json:"number"`
	}

	type ParamObj struct {
		String string `query:"string"`
		Number int    `query:"number"`

		// TextUnmarshaler
		Custom      CustomParam  `query:"custom"`
		OtherCustom *CustomParam `query:"other"`

		Default string      `query:"default" default:"default-value"`
		Body    RequestBody `request:"body" doc:"input"`
	}

	validateParams := func(p ParamObj) {
		if p.String != "foo" {
			b.Fatal("incorrect String")
		}
		if p.Custom.Name != "bar" {
			b.Fatal("incorrect Custom.Name")
		}
		if p.OtherCustom.Name != "foobar" {
			b.Fatal("incorrect OtherCustom.Name")
		}
		if p.Default != "default-value" {
			b.Fatal("incorrect Default")
		}
		if p.Number != 2 {
			b.Fatal("incorrect Number")
		}
		if p.Body.Number != 2 {
			b.Fatal("incorrect resp body")
		}
		if p.Body.String != "str" {
			b.Fatal("incorrect resp body")
		}
	}

	depRouterHandler := func(w http.ResponseWriter, p ParamObj) {
		validateParams(p)
		w.WriteHeader(http.StatusCreated)
	}

	chiHandler := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		customStr := q.Get("custom")
		custom := &CustomParam{}
		err := custom.UnmarshalText([]byte(customStr))
		if err != nil {
			b.Fatal(err)
		}

		otherCustomStr := q.Get("other")
		otherCustom := &CustomParam{}
		err = otherCustom.UnmarshalText([]byte(otherCustomStr))
		if err != nil {
			b.Fatal(err)
		}

		numberStr := q.Get("number")
		number, err := strconv.Atoi(numberStr)
		if err != nil {
			b.Fatal(err)
		}

		def := q.Get("default")
		if def == "" {
			def = "default-value"
		}
		p := ParamObj{
			String:      q.Get("string"),
			Number:      number,
			Custom:      *custom,
			OtherCustom: otherCustom,
			Default:     def,
		}
		err = json.NewDecoder(r.Body).Decode(&p.Body)
		if err != nil {
			b.Fatal(err)
		}
		validateParams(p)
		w.WriteHeader(http.StatusCreated)
	}

	depRouter := newDepRouter(b).WithRequestHandler(nil)
	depRouter.Get("/", depRouterHandler)
	depRouter.Get("/test", func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusCreated)
	})

	chiRotuer := chi.NewRouter()
	chiRotuer.Get("/", chiHandler)
	chiRotuer.Get("/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	paramPath := "/?string=foo&custom=bar&other=foobar&number=2&name=name"
	tests := []struct {
		name   string
		path   string
		router http.Handler
	}{
		{
			name:   "depRouter with params",
			path:   paramPath,
			router: depRouter,
		},
		{
			name:   "chi with params",
			path:   paramPath,
			router: chiRotuer,
		},
		{
			name:   "depRouter no params",
			path:   "/test",
			router: depRouter,
		},
		{
			name:   "chi no params",
			path:   "/test",
			router: chiRotuer,
		},
	}

	data := MustMarshal(b, RequestBody{
		String: "str",
		Number: 2,
	})
	reader := strings.NewReader(data)
	for _, test := range tests {
		resp := httptest.NewRecorder()
		b.Run(test.name, func(b *testing.B) {
			req := httptest.NewRequest(http.MethodGet, test.path, io.NopCloser(reader))
			for i := 0; i < b.N; i++ {
				test.router.ServeHTTP(resp, req)
				if resp.Code != http.StatusCreated {
					b.Fatal("incocrect status code")
				}
				reader.Reset(data)
			}
		})
	}
}
