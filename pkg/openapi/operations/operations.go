package operations

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zhamlin/chi-openapi/pkg/openapi"

	"github.com/getkin/kin-openapi/openapi3"
)

// trimString removes extra lines and new spaces from each
// line in the provided str
func trimString(str string) string {
	strLines := strings.Split(str, "\n")
	for i, line := range strLines {
		line = strings.Trim(line, "\n")
		line = strings.TrimSpace(line)
		strLines[i] = line
	}
	str = strings.Join(strLines, "\n")
	return strings.Trim(str, "\n")

}

type Operation struct {
	openapi3.Operation
}

type OpenAPI = *openapi.OpenAPI
type Option func(OpenAPI, Operation) (Operation, error)
type Options []Option

type ExtensionData map[string]interface{}

func Extensions(data ExtensionData) Option {
	return func(_ OpenAPI, o Operation) (Operation, error) {
		o.Extensions = data
		return o, nil
	}
}

// NoSecurity sets the security options to an empty array for this operation
func NoSecurity() Option {
	return func(_ OpenAPI, o Operation) (Operation, error) {
		o.Security = openapi3.NewSecurityRequirements()
		return o, nil
	}
}

// Security sets the security for the operation
func Security(name string, scopes ...string) Option {
	return func(_ OpenAPI, o Operation) (Operation, error) {
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
	return func(_ OpenAPI, o Operation) (Operation, error) {
		o.OperationID = id
		return o, nil
	}
}

func Tags(tags ...string) Option {
	return func(_ OpenAPI, o Operation) (Operation, error) {
		o.Tags = tags
		return o, nil
	}
}

func Deprecated() Option {
	return func(_ OpenAPI, o Operation) (Operation, error) {
		o.Deprecated = true
		return o, nil
	}
}

func Summary(summary string) Option {
	return func(_ OpenAPI, o Operation) (Operation, error) {
		o.Summary = trimString(summary)
		return o, nil
	}
}

func DefaultJSONResponse(description string, model interface{}) Option {
	return func(s OpenAPI, o Operation) (Operation, error) {
		resp := openapi3.NewResponse().WithDescription(trimString(description))
		if model == nil {
			o.Responses["default"] = &openapi3.ResponseRef{Value: resp}
			return o, nil
		}

		schema := openapi.SchemaFromObj(model, openapi.Schemas(s.Components.Schemas), s.RegisteredTypes)
		resp = resp.WithContent(openapi3.NewContentWithJSONSchemaRef(schema))
		o.Responses["default"] = &openapi3.ResponseRef{Value: resp}
		return o, nil
	}
}

func Params(model interface{}) Option {
	return func(s OpenAPI, o Operation) (Operation, error) {
		var err error
		o.Parameters, err = openapi.ParamsFromObj(model, openapi.Schemas(s.Components.Schemas), s.RegisteredTypes)
		if err != nil {
			return o, err
		}
		return o, nil
	}
}

func FileResponse(code int, description string) Option {
	return func(s OpenAPI, o Operation) (Operation, error) {
		if o.Responses == nil {
			o.Responses = openapi3.Responses{}

		}

		schema := openapi3.NewStringSchema().
			WithFormat("binary")

		response := &openapi3.ResponseRef{
			Value: openapi3.NewResponse().
				WithDescription(trimString(description)).
				WithContent(openapi3.Content{
					"application/pdf":  openapi3.NewMediaType().WithSchema(schema),
					"application/tiff": openapi3.NewMediaType().WithSchema(schema),
				}),
		}
		o.Responses[fmt.Sprintf("%d", code)] = response
		return o, nil
	}
}

func FormBody(description string, model interface{}) Option {
	return func(s OpenAPI, o Operation) (Operation, error) {
		if s.Components.Schemas == nil {
			s.Components.Schemas = openapi3.Schemas{}
		}
		schema := openapi.SchemaFromObj(model, openapi.Schemas(s.Components.Schemas), s.RegisteredTypes)
		// schema.Value.Extensions = map[string]interface{}{
		// 	"form": true,
		// }
		requestBody := openapi3.NewRequestBody().
			WithContent(openapi3.NewContentWithFormDataSchemaRef(schema)).
			WithDescription(trimString(description)).
			WithRequired(false)
		o.RequestBody = &openapi3.RequestBodyRef{Value: requestBody}
		return o, nil
	}
}

func JSONBodyRequired(description string, model interface{}) Option {
	return func(s OpenAPI, o Operation) (Operation, error) {
		if s.Components.Schemas == nil {
			s.Components.Schemas = openapi3.Schemas{}
		}
		schema := openapi.SchemaFromObj(model, openapi.Schemas(s.Components.Schemas), s.RegisteredTypes)
		requestBody := openapi3.NewRequestBody().
			WithContent(openapi3.NewContentWithJSONSchemaRef(schema)).
			WithDescription(trimString(description)).
			WithRequired(true)
		o.RequestBody = &openapi3.RequestBodyRef{Value: requestBody}
		return o, nil
	}
}

func JSONBody(description string, model interface{}) Option {
	return func(s OpenAPI, o Operation) (Operation, error) {
		if s.Components.Schemas == nil {
			s.Components.Schemas = openapi3.Schemas{}
		}
		schema := openapi.SchemaFromObj(model, openapi.Schemas(s.Components.Schemas), s.RegisteredTypes)
		requestBody := openapi3.NewRequestBody().
			WithContent(openapi3.NewContentWithJSONSchemaRef(schema)).
			WithDescription(trimString(description)).
			WithRequired(false)
		o.RequestBody = &openapi3.RequestBodyRef{Value: requestBody}
		return o, nil
	}
}

func JSONResponse(code int, description string, model interface{}) Option {
	return func(s OpenAPI, o Operation) (Operation, error) {
		if o.Responses == nil {
			o.Responses = openapi3.Responses{}

		}
		if model == nil {
			response := &openapi3.ResponseRef{
				Value: openapi3.NewResponse().WithDescription(trimString(description)),
			}
			o.Responses[fmt.Sprintf("%d", code)] = response
			return o, nil
		}
		schema := openapi.SchemaFromObj(model, openapi.Schemas(s.Components.Schemas), s.RegisteredTypes)
		// TODO: check for content first before just overwriting it
		// "application/json": NewMediaType().WithSchema(schema),
		response := &openapi3.ResponseRef{
			Value: openapi3.NewResponse().
				WithDescription(trimString(description)).
				WithContent(openapi3.NewContentWithJSONSchemaRef(schema)),
		}
		o.Responses[fmt.Sprintf("%d", code)] = response
		return o, nil
	}
}
