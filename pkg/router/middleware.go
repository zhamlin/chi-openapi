package router

import (
	"bytes"
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

func JSONHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func VerifyRequest(r *Router) func(http.Handler) http.Handler {
	router := openapi3filter.NewRouter()
	if err := router.AddSwagger(r.swagger); err != nil {
		panic(err)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: cache data for already hit routes
			rCtx := chi.RouteContext(r.Context())

			pathParams := map[string]string{}
			for i := 0; i < len(rCtx.URLParams.Values); i++ {
				name := rCtx.URLParams.Keys[i]
				pathParams[name] = rCtx.URLParams.Values[i]
			}

			route, _, err := router.FindRoute(r.Method, r.URL)
			if err != nil {
				fmt.Printf("%+v\n", err)
				return
			}

			err = openapi3filter.ValidateRequest(r.Context(), &openapi3filter.RequestValidationInput{
				Request:     r,
				Route:       route,
				QueryParams: r.URL.Query(),
				PathParams:  pathParams,
				Options: &openapi3filter.Options{
					IncludeResponseStatus: true,
				},
			})
			if err != nil {
				if re, ok := err.(*openapi3filter.RequestError); ok {
					if se, ok := re.Err.(*openapi3.SchemaError); ok {
						// fmt.Printf("%+v\n", se.JSONPointer()[0])
						// fmt.Printf("%+v\n", se.Reason)
						// fmt.Printf("%+v\n", re.Reason)
						// fmt.Printf("%+v\n", re.RequestBody.Content["application/json"].Schema.Value)

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

					// fmt.Printf("%+v\n", err)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func wrapResponse(w http.ResponseWriter) wrapperWritter {
	return wrapperWritter{
		ResponseWriter: w,
		body:           &bytes.Buffer{},
	}
}

type wrapperWritter struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (w *wrapperWritter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	// w.ResponseWriter.WriteHeader(statusCode)
}

func (w *wrapperWritter) Write(b []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	// writer := io.MultiWriter(w.ResponseWriter, w.body)
	return w.body.Write(b)
}

func (w *wrapperWritter) Done() {
	w.ResponseWriter.WriteHeader(w.statusCode)
	w.ResponseWriter.Write(w.body.Bytes())
}

func VerifyResponse(r *Router) func(http.Handler) http.Handler {
	router := openapi3filter.NewRouter()
	if err := router.AddSwagger(r.swagger); err != nil {
		panic(err)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: cache data for already hit routes
			rCtx := chi.RouteContext(r.Context())

			pathParams := map[string]string{}
			for i := 0; i < len(rCtx.URLParams.Values); i++ {
				name := rCtx.URLParams.Keys[i]
				pathParams[name] = rCtx.URLParams.Values[i]
			}

			route, _, err := router.FindRoute(r.Method, r.URL)
			if err != nil {
				fmt.Printf("%+v\n", err)
				return
			}

			wrapped := wrapResponse(w)
			next.ServeHTTP(&wrapped, r)

			input := &openapi3filter.ResponseValidationInput{
				Header: wrapped.Header(),
				Status: wrapped.statusCode,
				RequestValidationInput: &openapi3filter.RequestValidationInput{
					Request:     r,
					Route:       route,
					QueryParams: r.URL.Query(),
					PathParams:  pathParams,
					Options: &openapi3filter.Options{
						IncludeResponseStatus: true,
					},
				},
			}
			input.SetBodyBytes(wrapped.body.Bytes())
			err = openapi3filter.ValidateResponse(r.Context(), input)
			if err != nil {
				if re, ok := err.(*openapi3filter.ResponseError); ok {
					if se, ok := re.Err.(*openapi3.SchemaError); ok {
						field := "unknown"
						if p := se.JSONPointer(); len(p) > 0 {
							field = p[0]
						}
						schemaError := ErrorResponse{Errors: Errors{Error{
							Field:  field,
							Reason: se.Reason,
						}}}
						_, err := json.Marshal(&schemaError)
						if err != nil {
							// TODO: log this?
							panic(err)
						}

						// TODO: log this
						// fmt.Printf("%+v\n", string(b))
						w.WriteHeader(http.StatusInternalServerError)
					}

					// fmt.Printf("%+v\n", err)
				}
				return
			}
			wrapped.Done()
		})
	}
}
