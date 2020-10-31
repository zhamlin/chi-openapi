package router

import (
	"encoding/json"
	"fmt"
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

// JSONHeader sets the content type to application/json
func JSONHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
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

func requestValidationInput(r *http.Request) *openapi3filter.RequestValidationInput {
	return &openapi3filter.RequestValidationInput{
		Request:     r,
		QueryParams: r.URL.Query(),
		PathParams:  pathParams(r),
		Options: &openapi3filter.Options{
			IncludeResponseStatus: true,
		},
	}
}

// VerifyRequest validates requests against matching openapi routes on the router
func VerifyRequest(router *openapi3filter.Router) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, _, err := router.FindRoute(r.Method, r.URL)
			if err != nil {
				fmt.Printf("%+v\n", err)
				return
			}

			input := requestValidationInput(r)
			input.Route = route
			err = openapi3filter.ValidateRequest(r.Context(), input)
			if err != nil {
				if re, ok := err.(*openapi3filter.RequestError); ok {
					if se, ok := re.Err.(*openapi3.SchemaError); ok {
						field := "unknown"
						if p := se.JSONPointer(); len(p) > 0 {
							field = p[0]
						}
						schemaError := ErrorResponse{Errors: Errors{Error{
							Field:  field,
							Reason: se.Reason,
						}}}
						b, err := json.Marshal(&schemaError)
						if err != nil {
							// TODO: log this?
							panic(err)
						}

						w.Header().Add("Content-Type", "application/json")
						w.WriteHeader(re.HTTPStatus())
						w.Write(b)
					}
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func newResponseWriter() *responseWriter {
	return &responseWriter{
		headers: http.Header{},
	}
}

type responseWriter struct {
	body       []byte
	statusCode int
	headers    http.Header
}

func (w *responseWriter) Header() http.Header {
	return w.headers
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
func VerifyResponse(router *openapi3filter.Router) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, _, err := router.FindRoute(r.Method, r.URL)
			if err != nil {
				fmt.Printf("%+v\n", err)
				return
			}

			rw := newResponseWriter()
			next.ServeHTTP(rw, r)

			validationInput := requestValidationInput(r)
			validationInput.Route = route
			input := &openapi3filter.ResponseValidationInput{
				Header: rw.Header(),
				Status: rw.statusCode,
				Options: &openapi3filter.Options{
					IncludeResponseStatus: true,
				},
				RequestValidationInput: validationInput,
			}
			input.SetBodyBytes(rw.body)

			err = openapi3filter.ValidateResponse(r.Context(), input)
			if err != nil {
				if re, ok := err.(*openapi3filter.ResponseError); ok {
					w.WriteHeader(http.StatusInternalServerError)
					// TODO: handle invalid content type
					if _, ok := re.Err.(*openapi3.SchemaError); ok {
						// TODO: log this
					}
				}
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
