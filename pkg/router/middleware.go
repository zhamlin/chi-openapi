package router

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/felixge/httpsnoop"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
)

func requestValidationInput(r *http.Request) *openapi3filter.RequestValidationInput {
	return &openapi3filter.RequestValidationInput{
		Request:     r,
		QueryParams: r.URL.Query(),
	}
}

type ErrorHandler func(http.ResponseWriter, *http.Request, error)

type ctxKey struct {
	name string
}

// InputKey is used to get the *openapi3filter.RequestValidationInput{}
// from a ctx
var InputKey = ctxKey{"input"}

func InputFromCTX(ctx context.Context) (*openapi3filter.RequestValidationInput, error) {
	input, ok := ctx.Value(InputKey).(*openapi3filter.RequestValidationInput)
	if !ok {
		return input, fmt.Errorf("*openapi3filter.RequestValidationInput not found in context")
	}
	return input, nil
}

func SetOpenAPIInput(router routers.Router, optionsFn func(r *http.Request, options *openapi3filter.Options)) func(http.Handler) http.Handler {
	if router == nil {
		panic("SetOpenAPIInput got a nil router")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, pathParams, err := router.FindRoute(r)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			input := requestValidationInput(r)
			options := openapi3filter.Options{}
			if optionsFn != nil {
				optionsFn(r, &options)
			}
			input.Options = &options
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

			err = openapi3filter.ValidateRequest(ctx, input)
			if err != nil {
				errFn(w, r, err)
				return
			}

			// ValidateRequest reads from the body and sets it back in the
			// input struct, so copy it back to the original request
			r.Body = input.Request.Body
			next.ServeHTTP(w, r)
		})
	}
}

// VerifyResponse validates response against matching openapi routes
// Requires SetOpenAPIInput middleware to have been called
func VerifyResponse(errFn ErrorHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			responseBody := &bytes.Buffer{}
			statusCode := 0

			wrapped := httpsnoop.Wrap(w, httpsnoop.Hooks{
				WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
					return func(code int) {
						if statusCode == 0 {
							statusCode = code
						}
					}
				},
				Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
					return func(p []byte) (int, error) {
						bytesWritten, err := responseBody.Write(p)
						if err != nil {
							return -1, err
						}
						if statusCode == 0 {
							statusCode = http.StatusOK
						}
						return bytesWritten, nil
					}
				},
			})
			next.ServeHTTP(wrapped, r)

			input, err := InputFromCTX(r.Context())
			if err != nil {
				errFn(w, r, err)
				return
			}

			responseInput := &openapi3filter.ResponseValidationInput{
				Status:                 statusCode,
				Header:                 wrapped.Header(),
				Options:                input.Options,
				RequestValidationInput: input,
				Body:                   io.NopCloser(responseBody),
			}
			if responseInput.Body == nil {
				responseInput.Options.ExcludeResponseBody = true
			}
			responseInput.Options.IncludeResponseStatus = false

			err = openapi3filter.ValidateResponse(r.Context(), responseInput)
			if err != nil {
				responseErr := err.(*openapi3filter.ResponseError)

				// ignore any attempt to parse the body on an optional return type
				var parseErr *openapi3filter.ParseError
				if errors.As(responseErr.Err, &parseErr) && errors.Is(parseErr.RootCause(), io.EOF) {
					response, has := responseInput.RequestValidationInput.Route.Operation.Responses[fmt.Sprintf("%d", statusCode)]
					if has {
						if mt := response.Value.Content.Get("application/json"); mt != nil {
							if len(mt.Schema.Value.Required) != 0 {
								errFn(w, r, err)
								return
							}
						}
					}
				} else {
					errFn(w, r, err)
					return
				}
			}

			// copy over data from wrapped ResponseWriter to actual one
			h := w.Header()
			for name, value := range wrapped.Header() {
				for _, v := range value {
					if _, has := h[name]; !has {
						h.Add(name, v)
					}
				}
			}

			w.WriteHeader(statusCode)
			io.Copy(w, responseInput.Body)
		})
	}
}
