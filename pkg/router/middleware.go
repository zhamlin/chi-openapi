package router

import (
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi"
)

type Error struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

type Errors []Error

type ErrorResponse struct {
	Errors Errors `json:"errors"`
}

// pathParams returns all chi url params from the request
func pathParams(r *http.Request) map[string]string {
	rCtx := chi.RouteContext(r.Context())

	pathParams := map[string]string{}
	for i := 0; i < len(rCtx.URLParams.Values); i++ {
		name := rCtx.URLParams.Keys[i]
		pathParams[name] = rCtx.URLParams.Values[i]
	}
	return pathParams
}

// queryParamLoader handles loading booleans, basic strings and numbers
func queryParamLoader(param *openapi3.Parameter, values []string) (interface{}, *openapi3.Schema, error) {
	v := param.Schema.Value
	var err error
	var value interface{}
	return value, v, err
}

func requestValidationInput(r *http.Request) *openapi3filter.RequestValidationInput {
	return &openapi3filter.RequestValidationInput{
		Request:     r,
		QueryParams: r.URL.Query(),
		// ParamDecoder: queryParamLoader,
		PathParams: pathParams(r),
		Options: &openapi3filter.Options{
			IncludeResponseStatus: true,
		},
	}
}

type ErrorHandler func(http.ResponseWriter, *http.Request, error)

// VerifyRequest validates requests against matching openapi routes on the router
func VerifyRequest(router *openapi3filter.Router, errFn ErrorHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, _, err := router.FindRoute(r.Method, r.URL)
			if err != nil {
				errFn(w, r, err)
				return
			}

			input := requestValidationInput(r)
			input.Route = route
			err = openapi3filter.ValidateRequest(r.Context(), input)
			if err != nil {
				errFn(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type responseWriter struct {
	body       []byte
	statusCode int
	header     http.Header
}

func (w *responseWriter) Header() http.Header {
	return w.header
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	w.body = b
	return len(w.body), nil
}

// VerifyResponse validates response against matching openapi routes on the router
func VerifyResponse(router *openapi3filter.Router, errFn ErrorHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, _, err := router.FindRoute(r.Method, r.URL)
			if err != nil {
				errFn(w, r, err)
				return
			}

			rw := &responseWriter{
				header: w.Header(),
			}
			next.ServeHTTP(rw, r)

			validationInput := requestValidationInput(r)
			validationInput.Route = route
			input := &openapi3filter.ResponseValidationInput{
				Header: rw.header,
				Status: rw.statusCode,
				Options: &openapi3filter.Options{
					IncludeResponseStatus: true,
				},
				RequestValidationInput: validationInput,
			}
			input.SetBodyBytes(rw.body)

			err = openapi3filter.ValidateResponse(r.Context(), input)
			if err != nil {
				errFn(w, r, err)
				return
			}
			for name, value := range rw.Header() {
				for _, v := range value {
					w.Header().Add(name, v)
				}
			}
			w.WriteHeader(rw.statusCode)
			w.Write(rw.body)
		})
	}
}
