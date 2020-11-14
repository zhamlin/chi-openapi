package operations

import (
	"fmt"

	"chi-openapi/pkg/openapi"

	"github.com/getkin/kin-openapi/openapi3"
)

type Operation struct {
	openapi3.Operation
}

type Option func(*openapi3.Swagger, Operation) Operation
type Options []Option

func Deprecated() Option {
	return func(_ *openapi3.Swagger, o Operation) Operation {
		o.Deprecated = true
		return o
	}
}

func Summary(summary string) Option {
	return func(_ *openapi3.Swagger, o Operation) Operation {
		o.Summary = summary
		return o
	}
}

func JSONBody(description string, model interface{}) Option {
	return func(s *openapi3.Swagger, o Operation) Operation {
		if s.Components.Schemas == nil {
			s.Components.Schemas = openapi.Schemas{}
		}
		schema := openapi.SchemaFromObj(model, s.Components.Schemas)
		o.RequestBody = &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Content:     openapi3.NewContentWithJSONSchemaRef(schema),
				Description: description,
				Required:    true,
			},
		}
		return o
	}
}

func Params(model interface{}) Option {
	return func(s *openapi3.Swagger, o Operation) Operation {
		var err error
		o.Parameters, err = openapi.ParamsFromObj(model)
		if err != nil {
			panic(err)
		}
		return o
	}
}

func JSONResponse(code int, description string, model interface{}) Option {
	return func(s *openapi3.Swagger, o Operation) Operation {
		if o.Responses == nil {
			o.Responses = openapi3.Responses{}

		}
		if model == nil {
			response := &openapi3.ResponseRef{
				Value: &openapi3.Response{Description: &description},
			}
			o.Responses[fmt.Sprintf("%d", code)] = response
			return o
		}
		schema := openapi.SchemaFromObj(model, s.Components.Schemas)
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
