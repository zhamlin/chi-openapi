package operations

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zhamlin/chi-openapi/pkg/openapi"

	"github.com/getkin/kin-openapi/openapi3"
)

type Operation struct {
	openapi3.Operation
}

type Option func(*openapi3.T, Operation) (Operation, error)
type Options []Option

type ExtensionData map[string]interface{}

func Extensions(data ExtensionData) Option {
	return func(_ *openapi3.T, o Operation) (Operation, error) {
		o.Extensions = data
		return o, nil
	}
}

// NoSecurity sets the security options to an empty array for this operation
func NoSecurity() Option {
	return func(_ *openapi3.T, o Operation) (Operation, error) {
		o.Security = openapi3.NewSecurityRequirements()
		return o, nil
	}
}

// Security sets the security for the operation
func Security(name string, scopes ...string) Option {
	return func(_ *openapi3.T, o Operation) (Operation, error) {
		if o.Security == nil {
			o.Security = openapi3.NewSecurityRequirements()
		}

		if name == "" {
			return o, errors.New("expected a name for the operations security, got an empty string")
		}

		o.Security = o.Security.With(openapi3.
			NewSecurityRequirement().
			Authenticate(name, scopes...))
		return o, nil
	}
}

func ID(id string) Option {
	return func(_ *openapi3.T, o Operation) (Operation, error) {
		o.OperationID = id
		return o, nil
	}
}

func Tags(tags ...string) Option {
	return func(_ *openapi3.T, o Operation) (Operation, error) {
		o.Tags = tags
		return o, nil
	}
}

func Deprecated() Option {
	return func(_ *openapi3.T, o Operation) (Operation, error) {
		o.Deprecated = true
		return o, nil
	}
}

func Summary(summary string) Option {
	return func(_ *openapi3.T, o Operation) (Operation, error) {
		summaryLines := strings.Split(summary, "\n")
		for i, line := range summaryLines {
			line = strings.Trim(line, "\n")
			line = strings.TrimSpace(line)
			summaryLines[i] = line
		}
		summary = strings.Join(summaryLines, "\n")
		o.Summary = strings.Trim(summary, "\n")
		return o, nil
	}
}

func DefaultJSONResponse(description string, model interface{}) Option {
	return func(s *openapi3.T, o Operation) (Operation, error) {
		resp := openapi3.NewResponse().WithDescription(description)
		if model == nil {
			o.Responses["default"] = &openapi3.ResponseRef{Value: resp}
			return o, nil
		}

		schema := openapi.SchemaFromObj(model, openapi.Schemas(s.Components.Schemas))
		resp = resp.WithContent(openapi3.NewContentWithJSONSchemaRef(schema))
		o.Responses["default"] = &openapi3.ResponseRef{Value: resp}
		return o, nil
	}
}

func Params(model interface{}) Option {
	return func(s *openapi3.T, o Operation) (Operation, error) {
		var err error
		o.Parameters, err = openapi.ParamsFromObj(model, openapi.Schemas(s.Components.Schemas))
		if err != nil {
			return o, err
		}
		return o, nil
	}
}

func FileResponse(code int, description string) Option {
	return func(s *openapi3.T, o Operation) (Operation, error) {
		if o.Responses == nil {
			o.Responses = openapi3.Responses{}

		}

		schema := openapi3.NewSchema()
		schema.Type = "string"
		schema.Format = "binary"

		response := &openapi3.ResponseRef{
			Value: &openapi3.Response{
				Description: &description,
				Content: openapi3.Content{
					"application/pdf":  openapi3.NewMediaType().WithSchema(schema),
					"application/tiff": openapi3.NewMediaType().WithSchema(schema),
				},
			},
		}
		o.Responses[fmt.Sprintf("%d", code)] = response
		return o, nil
	}
}

func FormBody(description string, model interface{}) Option {
	return func(s *openapi3.T, o Operation) (Operation, error) {
		if s.Components.Schemas == nil {
			s.Components.Schemas = openapi3.Schemas{}
		}
		schema := openapi.SchemaFromObj(model, openapi.Schemas(s.Components.Schemas))
		// schema.Value.Extensions = map[string]interface{}{
		// 	"form": true,
		// }
		requestBody := openapi3.NewRequestBody().
			WithContent(openapi3.NewContentWithFormDataSchemaRef(schema)).
			WithDescription(description).
			WithRequired(false)
		o.RequestBody = &openapi3.RequestBodyRef{Value: requestBody}
		return o, nil
	}
}

func JSONBodyRequired(description string, model interface{}) Option {
	return func(s *openapi3.T, o Operation) (Operation, error) {
		if s.Components.Schemas == nil {
			s.Components.Schemas = openapi3.Schemas{}
		}
		schema := openapi.SchemaFromObj(model, openapi.Schemas(s.Components.Schemas))
		requestBody := openapi3.NewRequestBody().
			WithContent(openapi3.NewContentWithJSONSchemaRef(schema)).
			WithDescription(description).
			WithRequired(true)
		o.RequestBody = &openapi3.RequestBodyRef{Value: requestBody}
		return o, nil
	}
}

func JSONBody(description string, model interface{}) Option {
	return func(s *openapi3.T, o Operation) (Operation, error) {
		if s.Components.Schemas == nil {
			s.Components.Schemas = openapi3.Schemas{}
		}
		schema := openapi.SchemaFromObj(model, openapi.Schemas(s.Components.Schemas))
		requestBody := openapi3.NewRequestBody().
			WithContent(openapi3.NewContentWithJSONSchemaRef(schema)).
			WithDescription(description).
			WithRequired(false)
		o.RequestBody = &openapi3.RequestBodyRef{Value: requestBody}
		return o, nil
	}
}

func JSONResponse(code int, description string, model interface{}) Option {
	return func(s *openapi3.T, o Operation) (Operation, error) {
		if o.Responses == nil {
			o.Responses = openapi3.Responses{}

		}
		if model == nil {
			response := &openapi3.ResponseRef{
				Value: &openapi3.Response{Description: &description},
			}
			o.Responses[fmt.Sprintf("%d", code)] = response
			return o, nil
		}
		schema := openapi.SchemaFromObj(model, openapi.Schemas(s.Components.Schemas))
		// TODO: check for content first before just overwriting it
		// "application/json": NewMediaType().WithSchema(schema),
		response := &openapi3.ResponseRef{
			Value: &openapi3.Response{
				Description: &description,
				Content:     openapi3.NewContentWithJSONSchemaRef(schema),
			},
		}
		o.Responses[fmt.Sprintf("%d", code)] = response
		return o, nil
	}
}
