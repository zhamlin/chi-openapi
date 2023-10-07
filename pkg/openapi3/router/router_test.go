package router_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/zhamlin/chi-openapi/internal/testing"
	"github.com/zhamlin/chi-openapi/pkg/jsonschema"
	. "github.com/zhamlin/chi-openapi/pkg/openapi3/operations"
	"github.com/zhamlin/chi-openapi/pkg/openapi3/router"
)

func newRouter() *router.Router {
	r := router.NewRouter(router.Config{})
	return r
}

func TestRouterWithMiddleware(t *testing.T) {
	r := newRouter()

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

	req := httptest.NewRequest(http.MethodGet, "/middle", http.NoBody)
	resp := do(r, req)

	MustMatch(t, resp.Header().Get("custom"), "hello world",
		"expected router.With() middleware to run")

	MustMatch(t, resp.Header().Get("content-type"), "application/json",
		"expected router.Use() middleware to run")

	req = httptest.NewRequest(http.MethodGet, "/no-middle", http.NoBody)
	resp = do(r, req)

	MustMatch(t, resp.Code, http.StatusCreated)
	MustMatch(t, resp.Header().Get("custom"), "",
		"expected router.With() middleware to not run r")
}

func TestRouterGroup(t *testing.T) {
	r := newRouter()

	type A struct {
		Foo string
	}

	type B struct {
		Bar string
	}

	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}

	r.Group(func(r *router.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
				w.Header().Set("content-type", "application/json")
			})
		})
		r.Get("/group", h, Response[A](http.StatusOK, ""))
	})
	r.Get("/no-middle", h, Response[B](http.StatusOK, ""))

	req := httptest.NewRequest(http.MethodGet, "/group", http.NoBody)
	resp := do(r, req)

	MustMatch(t, resp.Header().Get("content-type"), "application/json",
		"expected router group Use() middleware to run")

	req = httptest.NewRequest(http.MethodGet, "/no-middle", http.NoBody)
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

func TestRouterMount(t *testing.T) {
	r := newRouter()
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

func TestRouterParameters(t *testing.T) {
	r := newRouter()
	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}
	r.Get("/foo/{id}", h,
		Params(struct {
			Name   string `query:"name" style:"form"`
			ID     int    `path:"id"`
			Header string `header:"x-header-value"`
			Cookie string `cookie:"cookie"`
			Object struct {
				Field string
			} `query:"object" style:"deepObject"`
			Ignored string
		}{}),
		Response[None](http.StatusOK, ""),
	)

	MustMatchAsJson(t, r.OpenAPI(), `{
        "components": {},
        "info": {
            "title": "",
            "version": ""
        },
        "openapi": "3.1.0",
        "paths": {
            "/foo/{id}": {
                "get": {
                    "responses": {
                        "200": {}
                    },
                    "parameters": [
                        {
                            "in": "query",
                            "name": "name",
                            "schema": {
                                "type": "string"
                            },
                            "style": "form"
                        },
                        {
                            "in": "path",
                            "name": "id",
                            "schema": {
                                "type": "integer"
                            }
                        },
                        {
                            "in": "header",
                            "name": "x-header-value",
                            "schema": {
                                "type": "string"
                            }
                        },
                        {
                            "in": "cookie",
                            "name": "cookie",
                            "schema": {
                                "type": "string"
                            }
                        },
                        {
                            "style": "deepObject",
                            "in": "query",
                            "name": "object",
                            "schema": {
                                "properties": {
                                    "Field": {
                                        "type": "string"
                                    }
                                },
                                "type": "object"
                            }
                        }
                    ]
                }
            }
        }
    }`)
}

func TestRouterJSONBody(t *testing.T) {
	r := newRouter()
	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}
	r.Get("/foo/{id}", h,
		BodyAs[struct {
			Field string
		}]("FooInput", "input", false),
		Response[None](http.StatusOK, ""),
	)

	MustMatchAsJson(t, r.OpenAPI(), `
    {
        "components": {
            "schemas": {
                "FooInput": {
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
            "/foo/{id}": {
                "get": {
                    "requestBody": {
                        "content": {
                            "application/json": {
                                "schema": {
                                    "$ref": "#/components/schemas/FooInput"
                                }
                            }
                        },
                        "description": "input"
                    },
                    "responses": {
                        "200": {}
                    }
                }
            }
        }
    }`)
}

func TestRouterKeepsTrailingPath(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}

	r := newRouter()
	r.Get("/foo/", h, Response[None](http.StatusOK, ""))

	rootRouter := newRouter()
	rootRouter.Mount("/v1/", r)
	rootRouter.Get("/foo/", h, Response[None](http.StatusCreated, ""))

	MustMatchAsJson(t, rootRouter.OpenAPI(), `
    {
        "components": {},
        "info": {
            "version": "",
            "title": ""
        },
        "openapi": "3.1.0",
        "paths": {
            "/foo/": {
                "get": {
                    "responses": {
                        "201": {}
                    }
                }
            },
            "/v1/foo/": {
                "get": {
                    "responses": {
                        "200": {}
                    }
                }
            }
        }
    }`)
}

func TestRouterErrors(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}

	errs := []error{}
	r := router.NewRouter(router.Config{
		ErrorSink: func(err error) {
			errs = append(errs, err)
		},
	})
	r.Get("/foo", h, ResponseAs[struct{ A string }]("B", http.StatusOK, ""))
	r.Get("/bar", h, ResponseAs[struct{ B string }]("B", http.StatusOK, ""))

	MustMatch(t, len(errs), 1,
		"expected one error; /foo and /bar try to use the same component name")
}

func TestSubRouterErrors(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}

	rootRouterErrs := []error{}
	rootRouter := router.NewRouter(router.Config{
		ErrorSink: func(err error) {
			rootRouterErrs = append(rootRouterErrs, err)
		},
	})

	routerErrs := []error{}
	r := router.NewRouter(router.Config{
		ErrorSink: func(err error) {
			routerErrs = append(routerErrs, err)
		},
	})
	r.Get("/foo", h, ResponseAs[struct{ A string }]("B", http.StatusOK, ""))
	r.Get("/bar", h, ResponseAs[struct{ B string }]("B", http.StatusOK, ""))
	MustMatch(t, len(routerErrs), 1, "expected one error")

	MustMatch(t, len(rootRouterErrs), 0,
		"expected no errors on the root router before moutn")
	rootRouter.Mount("/v1", r)

	err := routerErrs[0].Error()
	if !strings.Contains(err, "components: B already exists in the schema") {
		t.Fatalf("expected components error, got: %s", err)
	}
}

func TestRouterMountComponentErrors(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}

	rootRouterErrs := []error{}
	rootRouter := router.NewRouter(router.Config{
		ErrorSink: func(err error) {
			rootRouterErrs = append(rootRouterErrs, err)
		},
	})

	routerErrs := []error{}
	r := router.NewRouter(router.Config{
		ErrorSink: func(err error) {
			routerErrs = append(routerErrs, err)
		},
	})

	r.Get("/foo", h, ResponseAs[struct{ A string }]("B", http.StatusOK, ""))
	MustMatch(t, len(routerErrs), 0, "expected no errors")

	r.Get("/bar", h, ResponseAs[struct{ B string }]("B", http.StatusOK, ""))
	MustMatch(t, len(rootRouterErrs), 0, "expected no errors")

	rootRouter.Mount("/v1", r)
	MustMatch(t, len(routerErrs), 1,
		"expected one error; both routers have a component named `B`")
}

func TestRouterMountPathErrors(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}

	rootRouterErrs := []error{}
	rootRouter := router.NewRouter(router.Config{
		ErrorSink: func(err error) {
			rootRouterErrs = append(rootRouterErrs, err)
		},
	})

	routerErrs := []error{}
	r := router.NewRouter(router.Config{
		ErrorSink: func(err error) {
			routerErrs = append(routerErrs, err)
		},
	})
	r.Get("/bar", h, ResponseAs[struct{ A string }]("A", http.StatusOK, ""))
	MustMatch(t, len(routerErrs), 0, "expected no errors")

	rootRouter.Get("/bar", h, ResponseAs[struct{ B string }]("B", http.StatusOK, ""))
	MustMatch(t, len(rootRouterErrs), 0, "expected no errors")

	rootRouter.Mount("/", r)
	MustMatch(t, len(rootRouterErrs), 1,
		"expected one error; both routers have /bar path",
	)

	err := rootRouterErrs[0].Error()
	if !strings.Contains(err, "paths: key already exists") {
		t.Fatalf("expected path error, got: %s", err)
	}
}

func TestRouterRouteDefaultResponses(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}

	r := newRouter()

	type ErrorResp struct {
		Type        string
		Description string
	}
	r.DefaultResponse("generic error", ErrorResp{})
	r.DefaultStatusResponse(http.StatusNotFound, "NotFound", nil)

	r.Get("/", h,
		Response[[]string](http.StatusOK, ""),
		DefaultResponse[struct{ B string }](""),
	)
	r.Get("/other", h, Response[None](http.StatusOK, ""))
	r.Get("/noresponse", h)

	MustMatchAsJson(t, r.OpenAPI(), `
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
                                "$ref": "#/components/schemas/ErrorResp"
                            }
                        }
                    },
                    "description": "generic error"
                }
            },
            "schemas": {
                "ErrorResp": {
                    "properties": {
                        "Description": {
                            "type": "string"
                        },
                        "Type": {
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
            "/other": {
                "get": {
                    "responses": {
                        "200": {},
                        "404": {
                            "$ref": "#/components/responses/404"
                        },
                        "default": {
                            "$ref": "#/components/responses/default"
                        }
                    }
                }
            },
            "/noresponse": {
                "get": {
                    "responses": {
                        "404": {
                            "$ref": "#/components/responses/404"
                        },
                        "default": {
                            "$ref": "#/components/responses/default"
                        }
                    }
                }
            },
            "/": {
                "get": {
                    "responses": {
                        "200": {
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "items": {
                                            "type": "string"
                                        },
                                        "type": "array"
                                    }
                                }
                            }
                        },
                        "404": {
                            "$ref": "#/components/responses/404"
                        },
                        "default": {
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "properties": {
                                            "B": {
                                                "type": "string"
                                            }
                                        },
                                        "type": "object"
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

func TestRouterNoRefResponse(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {}
	r := newRouter()

	type InlinedTime time.Time
	dateTime := jsonschema.NewDateTimeSchema()
	r.RegisterComponent(InlinedTime{}, dateTime, jsonschema.NoRef())
	r.RegisterComponentAs("TimeRef", time.Time{}, dateTime)

	r.Get("/inline", h, Response[InlinedTime](http.StatusOK, ""))
	r.Get("/no-ref", h, NoRef(Response[time.Time](http.StatusOK, "")))
	r.Get("/ref", h, Response[time.Time](http.StatusOK, ""))

	MustMatchAsJson(t, r.OpenAPI(), `
    {
        "components": {
            "schemas": {
                "TimeRef": {
                    "format": "date-time",
                    "type": "string"
                }
            }
        },
        "info": {
            "title": "",
            "version": ""
        },
        "openapi": "3.1.0",
        "paths": {
            "/inline": {
                "get": {
                    "responses": {
                        "200": {
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "format": "date-time",
                                        "type": "string"
                                    }
                                }
                            }
                        }
                    }
                }
            },
            "/no-ref": {
                "get": {
                    "responses": {
                        "200": {
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "format": "date-time",
                                        "type": "string"
                                    }
                                }
                            }
                        }
                    }
                }
            },
            "/ref": {
                "get": {
                    "responses": {
                        "200": {
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "$ref": "#/components/schemas/TimeRef"
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

func TestRouterArrayItems(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {}

	type Obj struct {
		Name string
	}
	type GetResp struct {
		Objs []Obj `json:"objs"`
	}

	r := newRouter()
	r.Get("/", h,
		Response[GetResp](http.StatusOK, ""),
	)

	MustMatchAsJson(t, r.OpenAPI(), `
    {
        "components": {
            "schemas": {
                "GetResp": {
                    "type": "object",
                    "properties": {
                        "objs": {
                            "items": {
                                "$ref": "#/components/schemas/Obj"
                            },
                            "type": "array"
                        }
                    }
                },
                "Obj": {
                    "properties": {
                        "Name": {
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
            "/": {
                "get": {
                    "responses": {
                        "200": {
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "$ref": "#/components/schemas/GetResp"
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

// TODO: Route
