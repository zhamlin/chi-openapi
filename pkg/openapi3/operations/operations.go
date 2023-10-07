package operations

import (
	"errors"
	"fmt"

	"github.com/zhamlin/chi-openapi/internal"
	"github.com/zhamlin/chi-openapi/pkg/jsonschema"
	"github.com/zhamlin/chi-openapi/pkg/openapi3"
)

func NewOptionCtx(s jsonschema.Schemer, o openapi3.OpenAPI, contentType ...string) OptionCtx {
	return OptionCtx{
		openAPI:     o,
		schemer:     s,
		contentType: contentType,
	}
}

type OptionCtx struct {
	openAPI openapi3.OpenAPI
	schemer jsonschema.Schemer

	// set the content type on any option that has a mediaType
	contentType []string

	// inline any types seen vs trying to store them in schema and create ref
	noRef bool
}

func (ctx OptionCtx) getContentType(existingContentType []string) []string {
	if len(existingContentType) == 0 && len(ctx.contentType) > 0 {
		return ctx.contentType
	}
	return existingContentType
}

func (ctx OptionCtx) NewMediaType(obj any) (openapi3.MediaType, error) {
	schema, err := ctx.schemer.Get(obj)
	if err != nil {
		return openapi3.MediaType{}, err
	}

	mediaType := openapi3.NewMediaType()
	noRef := schema.Name() == "" || ctx.noRef || schema.NoRef()
	if !noRef {
		c := ctx.openAPI.GetComponents()
		if err := c.AddSchema(schema.Name(), schema); err != nil {
			return mediaType, fmt.Errorf("components: %w", err)
		}
		mediaType.SetSchemaRef(ctx.schemer.NewRef(schema.Name()))
		return mediaType, nil
	}
	mediaType.SetSchema(schema)
	return mediaType, nil
}

type Option func(OptionCtx, openapi3.Operation) (openapi3.Operation, error)

// ContentType sets the contentTypes for any supplied options.
func ContentType(contentTypes []string, options ...Option) Option {
	return func(ctx OptionCtx, o openapi3.Operation) (openapi3.Operation, error) {
		oldContentType := ctx.contentType
		ctx.contentType = contentTypes
		var err error
		for _, option := range options {
			o, err = option(ctx, o)
			if err != nil {
				return o, err
			}
		}
		ctx.contentType = oldContentType
		return o, nil
	}
}

func ID(id string) Option {
	return func(_ OptionCtx, o openapi3.Operation) (openapi3.Operation, error) {
		o.OperationID = id
		return o, nil
	}
}

func Tags(tags ...string) Option {
	return func(_ OptionCtx, o openapi3.Operation) (openapi3.Operation, error) {
		o.Tags = tags
		return o, nil
	}
}

func Deprecated() Option {
	return func(_ OptionCtx, o openapi3.Operation) (openapi3.Operation, error) {
		o.Deprecated = true
		return o, nil
	}
}

func Summary(summary string) Option {
	return func(_ OptionCtx, o openapi3.Operation) (openapi3.Operation, error) {
		o.Summary = internal.TrimString(summary)
		return o, nil
	}
}

// NoRef causes any supplied types to be inlined in the schema instead of
// creating them in the components schema and using a reference to that.
func NoRef(options ...Option) Option {
	return func(ctx OptionCtx, o openapi3.Operation) (openapi3.Operation, error) {
		oldNoRef := ctx.noRef
		ctx.noRef = true

		var err error
		for _, option := range options {
			o, err = option(ctx, o)
			if err != nil {
				break
			}
		}
		ctx.noRef = oldNoRef
		return o, err
	}
}

type None any

func Response[T any](code int, desc string, contentType ...string) Option {
	return func(ctx OptionCtx, o openapi3.Operation) (openapi3.Operation, error) {
		resp := openapi3.Response{}
		resp.Description = internal.TrimString(desc)

		var obj T
		switch any(&obj).(type) {
		case *any:
		case *None:
		default:
			mediaType, err := ctx.NewMediaType(obj)
			if err != nil {
				return o, err
			}
			for _, typ := range ctx.getContentType(contentType) {
				resp.SetContent(typ, mediaType)
			}
		}

		o.AddResponse(code, resp)
		return o, nil
	}
}

func BodyObj(desc string, obj any, required bool, contentType ...string) Option {
	return func(ctx OptionCtx, o openapi3.Operation) (openapi3.Operation, error) {
		if obj == nil {
			return o, errors.New("nil obj given to Body")
		}

		body := openapi3.RequestBody{}
		body.Description = internal.TrimString(desc)
		mediaType, err := ctx.NewMediaType(obj)
		if err != nil {
			return o, err
		}
		body.Required = required
		for _, typ := range ctx.getContentType(contentType) {
			body.SetContent(typ, mediaType)
		}
		o.SetRequestBody(body)
		return o, nil
	}
}

func Body[T any](desc string, required bool, contentType ...string) Option {
	var obj T
	return BodyObj(desc, obj, required, contentType...)
}

func DefaultResponse[T any](desc string, contentType ...string) Option {
	return func(ctx OptionCtx, o openapi3.Operation) (openapi3.Operation, error) {
		var obj T
		mediaType, err := ctx.NewMediaType(obj)
		if err != nil {
			return o, err
		}

		resp := openapi3.Response{}
		for _, typ := range ctx.getContentType(contentType) {
			resp.SetContent(typ, mediaType)
		}
		resp.Description = internal.TrimString(desc)
		o.AddDefaultResponse(resp)
		return o, nil
	}
}

func Params(obj any) Option {
	return func(ctx OptionCtx, o openapi3.Operation) (openapi3.Operation, error) {
		params, err := openapi3.ParamsFromStruct(ctx.schemer, obj)
		if err != nil {
			return o, err
		}
		for _, param := range params {
			o.AddParameter(param)
		}
		return o, nil
	}
}

func as[T any](name string, option Option) Option {
	var obj T
	return func(ctx OptionCtx, o openapi3.Operation) (openapi3.Operation, error) {
		schema, err := ctx.schemer.Get(obj)
		if err != nil {
			return o, err
		}
		ctx.schemer.Set(obj, schema, jsonschema.Name(name))
		return option(ctx, o)
	}
}

func BodyAs[T any](name, desc string, required bool, contentType ...string) Option {
	return as[T](name, Body[T](desc, required, contentType...))
}

func DefaultResponseAs[T any](name, desc string, contentType ...string) Option {
	return as[T](name, DefaultResponse[T](desc, contentType...))
}

func ResponseAs[T any](name string, code int, desc string, contentType ...string) Option {
	return as[T](name, Response[T](code, desc, contentType...))
}
