package router

import (
	"encoding/json"
	"fmt"
	"net/http"

	"chi-openapi/pkg/openapi"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi"
)

// NewRouter returns a wrapped chi router
func NewRouter(info openapi.Info) Router {
	apiInfo := openapi3.Info(info)
	return Router{
		mux: chi.NewRouter(),
		swagger: &openapi3.Swagger{
			OpenAPI: "3.0.1",
			Info:    &apiInfo,
			Paths:   openapi3.Paths{},
		},
	}
}

type Router struct {
	mux        chi.Router
	swagger    *openapi3.Swagger
	prefixPath string
}

func (r *Router) Group(path, name, description string) {
}

type Operation struct {
	openapi3.Operation
}

type OperationOption func(*openapi3.Swagger, Operation) Operation
type OperationOptions []OperationOption

func Summary(summary string) OperationOption {
	return func(_ *openapi3.Swagger, o Operation) Operation {
		o.Summary = summary
		return o
	}
}

func Body(description string, model interface{}) OperationOption {
	return func(_ *openapi3.Swagger, o Operation) Operation {
		o.RequestBody = &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Description: description,
			},
		}
		return o
	}
}

func Params(model interface{}) OperationOption {
	// check for required, min, max, etc
	return func(s *openapi3.Swagger, o Operation) Operation {
		o.Parameters = openapi.ParamsFromObj(model)
		return o
	}
}

func JSONResponse(code int, description string, model interface{}) OperationOption {
	return func(s *openapi3.Swagger, o Operation) Operation {
		if model == nil {
			return o
		}

		if o.Responses == nil {
			o.Responses = openapi3.Responses{}
		}

		if s.Components.Schemas == nil {
			s.Components.Schemas = openapi.Schemas{}
		}
		schema := openapi.SchemaFromObj(s.Components.Schemas, model)
		// TODO: check for content first before just overwriting it
		// "application/json": NewMediaType().WithSchema(schema),
		response := &openapi3.ResponseRef{
			Value: &openapi3.Response{
				Description: &description,
				Content:     openapi3.NewContentWithJSONSchemaRef(schema),
			},
		}
		o.Responses[fmt.Sprintf("%d", code)] = response
		return o
	}
}

func (router *Router) Route(path string, fn func(*Router)) {
	router.mux.Route(path, func(r chi.Router) {
		newRouter := Router{
			mux:        r,
			swagger:    router.swagger,
			prefixPath: router.prefixPath + path,
		}
		fn(&newRouter)
	})
}

func (router *Router) Mount(path string, handler http.Handler) {
}

func (r *Router) Handle(method, path string, options []OperationOption, handler http.HandlerFunc) {
	o := Operation{}
	for _, option := range options {
		o = option(r.swagger, o)
	}

	path = r.prefixPath + path
	pathItem, exists := r.swagger.Paths[path]
	if !exists {
		pathItem = &openapi3.PathItem{}
	}
	switch method {
	case http.MethodGet:
		pathItem.Get = &o.Operation
	case http.MethodPut:
		pathItem.Put = &o.Operation
	case http.MethodPost:
		pathItem.Post = &o.Operation
	case http.MethodDelete:
		pathItem.Delete = &o.Operation
	case http.MethodPatch:
		pathItem.Patch = &o.Operation
	case http.MethodHead:
		pathItem.Head = &o.Operation
	}
	r.swagger.Paths[path] = pathItem

}

func (r *Router) Get(path string, options []OperationOption, handler http.HandlerFunc) {
	r.Handle(http.MethodGet, path, options, handler)
}

func (r *Router) Post(path string, options []OperationOption, handler http.HandlerFunc) {
	r.Handle(http.MethodPost, path, options, handler)
}

func (r *Router) Put(path string, options []OperationOption, handler http.HandlerFunc) {
	r.Handle(http.MethodPut, path, options, handler)
}

func (r *Router) Patch(path string, options []OperationOption, handler http.HandlerFunc) {
	r.Handle(http.MethodPatch, path, options, handler)
}

func (r *Router) Delete(path string, options []OperationOption, handler http.HandlerFunc) {
	r.Handle(http.MethodDelete, path, options, handler)
}

func (r *Router) Head(path string, options []OperationOption, handler http.HandlerFunc) {
	r.Handle(http.MethodHead, path, options, handler)
}

func (r *Router) GenerateSpec() string {
	b, err := json.MarshalIndent(&r.swagger, "", " ")
	if err != nil {
		panic(err)
	}
	return string(b)
}
