package openapi

type Schema struct {
	Type                 string             `json:"type,omitempty"`
	Format               string             `json:"format,omitempty"`
	Description          string             `json:"description,omitempty"`
	Title                string             `json:"title,omitempty"`
	Default              any                `json:"default,omitempty"`
	Example              any                `json:"example,omitempty"`
	Nullable             bool               `json:"nullable,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	Required             []string           `json:"required,omitempty"`
	AdditionalProperties any                `json:"additionalProperties,omitempty"`
	Enum                 []any              `json:"enum,omitempty"`
	Ref                  string             `json:"$ref,omitempty"`
	AllOf                []*Schema          `json:"allOf,omitempty"`
	OneOf                []*Schema          `json:"oneOf,omitempty"`
	AnyOf                []*Schema          `json:"anyOf,omitempty"`
	Not                  *Schema            `json:"not,omitempty"`
	Minimum              *float64           `json:"minimum,omitempty"`
	Maximum              *float64           `json:"maximum,omitempty"`
	ExclusiveMinimum     *float64           `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum     *float64           `json:"exclusiveMaximum,omitempty"`
	MinLength            *int               `json:"minLength,omitempty"`
	MaxLength            *int               `json:"maxLength,omitempty"`
	Pattern              string             `json:"pattern,omitempty"`
	MinItems             *int               `json:"minItems,omitempty"`
	MaxItems             *int               `json:"maxItems,omitempty"`
	UniqueItems          bool               `json:"uniqueItems,omitempty"`
	MinProperties        *int               `json:"minProperties,omitempty"`
	MaxProperties        *int               `json:"maxProperties,omitempty"`
	ReadOnly             bool               `json:"readOnly,omitempty"`
	WriteOnly            bool               `json:"writeOnly,omitempty"`
	Deprecated           bool               `json:"deprecated,omitempty"`
}

type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Schema      *Schema `json:"schema,omitempty"`
	Style       string  `json:"style,omitempty"`
	Explode     bool    `json:"explode,omitempty"`
}

type RequestBody struct {
	Description string                `json:"description,omitempty"`
	Required    bool                  `json:"required,omitempty"`
	Content     map[string]*MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

type Response struct {
	Description string                `json:"description,omitempty"`
	Headers     map[string]*Parameter `json:"headers,omitempty"`
	Content     map[string]*MediaType `json:"content,omitempty"`
}

type Info struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version"`
	Contact     *Contact `json:"contact,omitempty"`
	License     *License `json:"license,omitempty"`
}

type Contact struct {
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

type License struct {
	Name       string `json:"name,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	URL        string `json:"url,omitempty"`
}

type Server struct {
	URL         string                     `json:"url"`
	Description string                     `json:"description,omitempty"`
	Variables   map[string]*ServerVariable `json:"variables,omitempty"`
}

type ServerVariable struct {
	Default     string   `json:"default"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type OpenAPI struct {
	OpenAPI    string               `json:"openapi"`
	Info       Info                 `json:"info"`
	Servers    []*Server            `json:"servers,omitempty"`
	Paths      map[string]*PathItem `json:"paths"`
	Components *Components          `json:"components,omitempty"`
}

type PathItem struct {
	Ref        string       `json:"$ref,omitempty"`
	GET        *Operation   `json:"get,omitempty"`
	PUT        *Operation   `json:"put,omitempty"`
	POST       *Operation   `json:"post,omitempty"`
	DELETE     *Operation   `json:"delete,omitempty"`
	OPTIONS    *Operation   `json:"options,omitempty"`
	HEAD       *Operation   `json:"head,omitempty"`
	PATCH      *Operation   `json:"patch,omitempty"`
	TRACE      *Operation   `json:"trace,omitempty"`
	CONNECT    *Operation   `json:"connect,omitempty"`
	Parameters []*Parameter `json:"parameters,omitempty"`
}

type Operation struct {
	Tags        []string              `json:"tags,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	OperationID string                `json:"operationId,omitempty"`
	Parameters  []*Parameter          `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]*Response  `json:"responses,omitempty"`
	Deprecated  bool                  `json:"deprecated,omitempty"`
	Security    []map[string][]string `json:"security,omitempty"`
}

type Components struct {
	Schemas    map[string]*Schema    `json:"schemas,omitempty"`
	Parameters map[string]*Parameter `json:"parameters,omitempty"`
	Responses  map[string]*Response  `json:"responses,omitempty"`
}

func NewOpenAPI(title, version string) *OpenAPI {
	return &OpenAPI{
		OpenAPI: "3.1.0",
		Info: Info{
			Title:   title,
			Version: version,
		},
		Paths: make(map[string]*PathItem),
		Components: &Components{
			Schemas: make(map[string]*Schema),
		},
	}
}

func (o *OpenAPI) AddOperation(method, path string, op *Operation) {
	if o.Paths[path] == nil {
		o.Paths[path] = &PathItem{}
	}

	switch method {
	case "GET":
		o.Paths[path].GET = op
	case "POST":
		o.Paths[path].POST = op
	case "PUT":
		o.Paths[path].PUT = op
	case "DELETE":
		o.Paths[path].DELETE = op
	case "PATCH":
		o.Paths[path].PATCH = op
	case "OPTIONS":
		o.Paths[path].OPTIONS = op
	case "HEAD":
		o.Paths[path].HEAD = op
	case "TRACE":
		o.Paths[path].TRACE = op
	case "CONNECT":
		o.Paths[path].CONNECT = op
	}
}

func (o *OpenAPI) AddSchema(name string, schema *Schema) {
	o.Components.Schemas[name] = schema
}
