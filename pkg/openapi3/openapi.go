package openapi3

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/sv-tools/openapi/spec"
	"github.com/zhamlin/chi-openapi/pkg/jsonschema"
)

type Tag struct {
	*spec.Extendable[spec.Tag]
}

func NewTag() Tag {
	return Tag{spec.NewTag()}
}

const JsonContentType = "application/json"

type OpenAPI struct {
	*spec.OpenAPI
}

func (o OpenAPI) GetParameter(name string, op Operation) (Parameter, bool) {
	if op.Parameters == nil {
		return Parameter{}, false
	}
	for _, paramSchemaOrRef := range op.Parameters {
		param := paramSchemaOrRef.Spec
		if ref := paramSchemaOrRef.Ref; ref != nil {
			// TODO: look up item by ref
			panic("TODO look up by ref")
		}

		if param.Spec.Name == name {
			return Parameter{Parameter: param.Spec}, true
		}
	}
	return Parameter{}, false
}

type Schema struct {
	*spec.Schema
}

func (o OpenAPI) GetComponents() Components {
	if o.Components == nil {
		o.Components = spec.NewComponents()
	}
	return Components{
		Components: o.Components.Spec,
	}
}

func NewPathItem() PathItem {
	return PathItem{
		PathItem: &spec.PathItem{},
	}
}

type PathItem struct {
	*spec.PathItem
}

var PathItemMethods = []string{
	http.MethodGet,
	http.MethodPut,
	http.MethodPost,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodHead,
	http.MethodTrace,
	http.MethodOptions,
}

func (p PathItem) GetOperation(method string) (Operation, bool) {
	var o *spec.Extendable[spec.Operation]
	switch method {
	case http.MethodGet:
		o = p.Get
	case http.MethodPut:
		o = p.Put
	case http.MethodPost:
		o = p.Post
	case http.MethodPatch:
		o = p.Patch
	case http.MethodDelete:
		o = p.Delete
	case http.MethodHead:
		o = p.Head
	case http.MethodTrace:
		o = p.Trace
	case http.MethodOptions:
		o = p.Options
	}
	var op Operation
	if o != nil {
		op = Operation{Operation: o.Spec}
	}
	return op, op.Operation != nil
}

func (p PathItem) SetOperation(method string, operation Operation) {
	op := NewExtendable(operation.Operation)
	switch method {
	case http.MethodGet:
		p.Get = op
	case http.MethodPut:
		p.Put = op
	case http.MethodPatch:
		p.Patch = op
	case http.MethodPost:
		p.Post = op
	case http.MethodDelete:
		p.Delete = op
	case http.MethodTrace:
		p.Trace = op
	case http.MethodOptions:
		p.Options = op
	case http.MethodHead:
		p.Head = op
	}
}

func (o OpenAPI) GetPath(name string) (PathItem, bool) {
	if o.Paths == nil {
		return PathItem{}, false
	}
	p, has := o.Paths.Spec.Paths[name]
	if has {
		return PathItem{p.Spec.Spec}, true
	}
	return PathItem{}, false
}

// SetPath overrides any existing paths if they exist, if not
// it creates the pathItem.
func (o OpenAPI) SetPath(name string, pathItem PathItem) {
	if o.Paths == nil {
		o.Paths = spec.NewPaths()
		o.Paths.Spec.Paths = map[string]*spec.RefOrSpec[spec.Extendable[spec.PathItem]]{}
	}
	if path, has := o.Paths.Spec.Paths[name]; has {
		path.Spec.Spec = pathItem.PathItem
	}
	o.Paths.Spec.Paths[name] = spec.NewRefOrSpec(nil, NewExtendable(pathItem.PathItem))
}

// func (o OpenAPI) Validate() error {
// 	return validate.Spec(spec.NewExtendable(o.OpenAPI))
// }

func NewOpenAPI(title string) OpenAPI {
	s := &spec.OpenAPI{
		Info:       spec.NewInfo(),
		Components: spec.NewComponents(),
		// ExternalDocs: spec.NewExternalDocs(),
		Paths:    spec.NewPaths(),
		WebHooks: map[string]*spec.RefOrSpec[spec.Extendable[spec.PathItem]]{},
		OpenAPI:  "3.1.0",
		// JsonSchemaDialect: "https://json-schema.org/draft/2020-12/schema",
	}
	s.Info.Spec.Title = title
	s.Components.Spec.Schemas = map[string]*spec.RefOrSpec[spec.Schema]{}
	s.Components.Spec.Paths = map[string]*spec.RefOrSpec[spec.Extendable[spec.PathItem]]{}
	s.Components.Spec.RequestBodies = map[string]*spec.RefOrSpec[spec.Extendable[spec.RequestBody]]{}
	s.Components.Spec.Parameters = map[string]*spec.RefOrSpec[spec.Extendable[spec.Parameter]]{}
	return OpenAPI{
		OpenAPI: s,
	}
}

func NewMediaType() MediaType {
	return MediaType{MediaType: spec.MediaType{}}
}

type MediaType struct {
	spec.MediaType
}

func (m *MediaType) SetSchema(schema jsonschema.Schema) {
	m.Schema = spec.NewRefOrSpec(nil, &spec.Schema{JsonSchema: schema.JsonSchema})
}

func (m *MediaType) SetSchemaRef(ref string) {
	m.Schema = spec.NewSchemaRef(spec.NewRef(ref))
}

type RequestBody struct {
	spec.RequestBody
}

func (r *RequestBody) SetContent(typ string, mediaType MediaType) {
	if r.Content == nil {
		r.Content = map[string]*spec.Extendable[spec.MediaType]{}
	}
	r.Content[typ] = spec.NewExtendable(&mediaType.MediaType)
}

type Response struct {
	spec.Response
}

func (r *Response) SetContent(typ string, mediaType MediaType) {
	if r.Content == nil {
		r.Content = map[string]*spec.Extendable[spec.MediaType]{}
	}
	r.Content[typ] = spec.NewExtendable(&mediaType.MediaType)
}

func NewOperation() Operation {
	return Operation{
		&spec.Operation{},
	}
}

type Operation struct {
	*spec.Operation
}

func (o *Operation) SetRequestBody(body RequestBody) {
	o.RequestBody = spec.NewRefOrSpec(nil, spec.NewExtendable(&body.RequestBody))
}

func (o *Operation) SetRequestRef(ref string) {
	o.RequestBody = spec.NewRefOrSpec[spec.Extendable[spec.RequestBody]](spec.NewRef(ref), nil)
}

func (o *Operation) AddResponse(code int, schema Response) {
	if o.Responses == nil {
		o.Responses = spec.NewResponses()
		o.Responses.Spec.Response = map[string]*spec.RefOrSpec[spec.Extendable[spec.Response]]{}
	}
	statusCode := fmt.Sprintf("%d", code)
	o.Responses.Spec.Response[statusCode] = spec.NewRefOrSpec(nil, spec.NewExtendable(&schema.Response))
}

func (o *Operation) AddResponseRef(code int, ref string) {
	if o.Responses == nil {
		o.Responses = spec.NewResponses()
		o.Responses.Spec.Response = map[string]*spec.RefOrSpec[spec.Extendable[spec.Response]]{}
	}
	statusCode := fmt.Sprintf("%d", code)
	o.Responses.Spec.Response[statusCode] = spec.NewRefOrSpec[spec.Extendable[spec.Response]](spec.NewRef(ref), nil)
}

func (o *Operation) AddDefaultResponseRef(ref string) {
	if o.Responses == nil {
		o.Responses = spec.NewResponses()
	}
	o.Responses.Spec.Default = spec.NewRefOrSpec[spec.Extendable[spec.Response]](spec.NewRef(ref), nil)
}

func (o *Operation) AddDefaultResponse(resp Response) {
	if o.Responses == nil {
		o.Responses = spec.NewResponses()
	}
	o.Responses.Spec.Default = spec.NewRefOrSpec(nil, spec.NewExtendable(&resp.Response))
}

func (o *Operation) HasParameter(param Parameter) bool {
	if o.Parameters == nil {
		return false
	}
	for _, p := range o.Parameters {
		if p.Ref != nil {
			panic("TODO: handle parma ref in operations")
		}
		if p.Spec.Spec.Name == param.Name && p.Spec.Spec.In == param.In {
			return true
		}
	}
	return false
}

func (o *Operation) AddParameter(param Parameter) {
	if o.Parameters == nil {
		o.Parameters = []*spec.RefOrSpec[spec.Extendable[spec.Parameter]]{}
	}
	p := spec.NewRefOrSpec(nil, spec.NewExtendable(param.Parameter))
	o.Parameters = append(o.Parameters, p)
}

func (o *Operation) AddParameterRef(ref string) {
	if o.Parameters == nil {
		o.Parameters = []*spec.RefOrSpec[spec.Extendable[spec.Parameter]]{}
	}
	p := spec.NewRefOrSpec[spec.Extendable[spec.Parameter]](spec.NewRef(ref), nil)
	o.Parameters = append(o.Parameters, p)
}

func NewParameter() Parameter {
	return Parameter{
		Parameter: &spec.Parameter{},
	}
}

type Parameter struct {
	*spec.Parameter
}

func (p *Parameter) SetSchema(schema jsonschema.Schema) {
	p.Schema = spec.NewRefOrSpec(nil, &spec.Schema{JsonSchema: schema.JsonSchema})
}

func (p Parameter) SetSchemaRef(ref string) {
	p.Schema = spec.NewRefOrSpec[spec.Schema](spec.NewRef(ref), nil)
}

func NewExtendable[T any](t *T) *spec.Extendable[T] {
	return spec.NewExtendable(t)
}

func NewPathItemSpec() *spec.RefOrSpec[spec.Extendable[spec.PathItem]] {
	return spec.NewPathItemSpec()
}

type Components struct {
	*spec.Components
}

type ComponentSchema struct {
	jsonschema.Schema
	Name string
}

func (c Components) GetSchemas() []ComponentSchema {
	schemas := []ComponentSchema{}
	for name, schema := range c.Schemas {
		schemas = append(schemas, ComponentSchema{
			Schema: jsonschema.Schema{
				JsonSchema: schema.Spec.JsonSchema,
			},
			Name: name,
		})
	}
	return schemas
}

func (c Components) GetSchemaByName(name string) (Schema, bool) {
	if schema, has := c.Schemas[name]; has {
		return Schema{Schema: schema.Spec}, true
	}
	return Schema{}, false
}

func (c Components) AddSchema(name string, schema jsonschema.Schema) error {
	if existingSchema, has := c.Schemas[name]; has {
		if reflect.DeepEqual(existingSchema.Spec.JsonSchema, schema.JsonSchema) {
			return nil
		}
		return fmt.Errorf("%s already exists in the schema", name)
	}
	c.Schemas[name] = spec.NewRefOrSpec(nil, &spec.Schema{JsonSchema: schema.JsonSchema})
	return nil
}

func (c Components) AddResponse(name string, resp Response) error {
	if c.Responses == nil {
		c.Responses = map[string]*spec.RefOrSpec[spec.Extendable[spec.Response]]{}
	}
	if _, has := c.Responses[name]; has {
		return fmt.Errorf("%s already exists in the responses schema", name)
	}
	c.Responses[name] = spec.NewRefOrSpec(nil, NewExtendable(&resp.Response))
	return nil
}
