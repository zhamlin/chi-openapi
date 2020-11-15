package router

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi"
)

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
		Options: &openapi3filter.Options{
			IncludeResponseStatus: true,
		},
	}
}

type ErrorHandler func(http.ResponseWriter, *http.Request, error)

type inputKey struct{}

// InputKey is used to get the *openapi3filter.RequestValidationInput{}
// from a ctx
var InputKey = inputKey{}

func InputFromCTX(ctx context.Context) (*openapi3filter.RequestValidationInput, error) {
	input, ok := ctx.Value(InputKey).(*openapi3filter.RequestValidationInput)
	if !ok {
		return input, fmt.Errorf("input not found in context")
	}
	return input, nil
}

func SetOpenAPIInput(router *openapi3filter.Router, errFn ErrorHandler) func(http.Handler) http.Handler {
	if router == nil {
		panic("SetOpenAPIInput got a nil router")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, pathParams, err := router.FindRoute(r.Method, r.URL)
			if err != nil {
				var rError *openapi3filter.RouteError
				if errors.As(err, &rError) {
					switch rError.Reason {
					case "Path was not found":
						w.WriteHeader(http.StatusNotFound)
					case "Path doesn't support the HTTP method":
						w.WriteHeader(http.StatusMethodNotAllowed)
					}
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			input := requestValidationInput(r)
			input.PathParams = pathParams
			input.Route = route

			ctx := r.Context()
			newCTX := context.WithValue(ctx, InputKey, input)
			next.ServeHTTP(w, r.WithContext(newCTX))
		})
	}
}

// VerifyRequest validates requests against matching openapi routes;
// Requires SetOpenAPIInput middleware to have been called
func VerifyRequest(errFn ErrorHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			input, err := InputFromCTX(ctx)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			// The body _could_ be read more than once so go ahead and create a copy
			newBody := &bytes.Buffer{}
			input.Request.Body = ioutil.NopCloser(io.TeeReader(r.Body, newBody))
			err = openapi3filter.ValidateRequest(ctx, input)
			if err != nil {
				errFn(w, r, err)
				return
			}
			r.Body = ioutil.NopCloser(newBody)
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

// VerifyResponse validates response against matching openapi routes
// Requires SetOpenAPIInput middleware to have been called
func VerifyResponse(errFn ErrorHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &responseWriter{
				header: w.Header(),
			}
			next.ServeHTTP(rw, r)

			input, err := InputFromCTX(r.Context())
			if err != nil {
				// next.ServeHTTP(w, r)
				errFn(w, r, err)
				return
			}

			responseInput := &openapi3filter.ResponseValidationInput{
				Header: rw.header,
				Status: rw.statusCode,
				Options: &openapi3filter.Options{
					IncludeResponseStatus: true,
				},
				RequestValidationInput: input,
			}
			responseInput.SetBodyBytes(rw.body)

			err = openapi3filter.ValidateResponse(r.Context(), responseInput)
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
