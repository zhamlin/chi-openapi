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

func Test(r *Router) func(http.Handler) http.Handler {
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
		})
	}
}
