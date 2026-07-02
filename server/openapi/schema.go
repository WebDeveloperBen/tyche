package openapi

type Schema struct {
	Default              any                `json:"default,omitempty"`
	AdditionalProperties any                `json:"additionalProperties,omitempty"`
	Example              any                `json:"example,omitempty"`
	Not                  *Schema            `json:"not,omitempty"`
	MaxItems             *int               `json:"maxItems,omitempty"`
	MaxProperties        *int               `json:"maxProperties,omitempty"`
	MinProperties        *int               `json:"minProperties,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	ExclusiveMinimum     *float64           `json:"exclusiveMinimum,omitempty"`
	MinItems             *int               `json:"minItems,omitempty"`
	Maximum              *float64           `json:"maximum,omitempty"`
	MinLength            *int               `json:"minLength,omitempty"`
	ExclusiveMaximum     *float64           `json:"exclusiveMaximum,omitempty"`
	Minimum              *float64           `json:"minimum,omitempty"`
	MaxLength            *int               `json:"maxLength,omitempty"`
	Description          string             `json:"description,omitempty"`
	Title                string             `json:"title,omitempty"`
	Type                 string             `json:"type,omitempty"`
	Format               string             `json:"format,omitempty"`
	Pattern              string             `json:"pattern,omitempty"`
	Ref                  string             `json:"$ref,omitempty"`
	AllOf                []*Schema          `json:"allOf,omitempty"`
	Enum                 []any              `json:"enum,omitempty"`
	OneOf                []*Schema          `json:"oneOf,omitempty"`
	AnyOf                []*Schema          `json:"anyOf,omitempty"`
	Required             []string           `json:"required,omitempty"`
	UniqueItems          bool               `json:"uniqueItems,omitempty"`
	Nullable             bool               `json:"nullable,omitempty"`
	ReadOnly             bool               `json:"readOnly,omitempty"`
	WriteOnly            bool               `json:"writeOnly,omitempty"`
	Deprecated           bool               `json:"deprecated,omitempty"`
}

type Parameter struct {
	Schema      *Schema `json:"schema,omitempty"`
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Description string  `json:"description,omitempty"`
	Style       string  `json:"style,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Explode     bool    `json:"explode,omitempty"`
}

type RequestBody struct {
	Content     map[string]*MediaType `json:"content,omitempty"`
	Description string                `json:"description,omitempty"`
	Required    bool                  `json:"required,omitempty"`
}

type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

type Response struct {
	Headers     map[string]*Parameter `json:"headers,omitempty"`
	Content     map[string]*MediaType `json:"content,omitempty"`
	Description string                `json:"description,omitempty"`
}

type Info struct {
	Contact     *Contact `json:"contact,omitempty"`
	License     *License `json:"license,omitempty"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version"`
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
	Variables   map[string]*ServerVariable `json:"variables,omitempty"`
	URL         string                     `json:"url"`
	Description string                     `json:"description,omitempty"`
}

type ServerVariable struct {
	Default     string   `json:"default"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type OpenAPI struct {
	Info       Info                 `json:"info"`
	Paths      map[string]*PathItem `json:"paths"`
	Components *Components          `json:"components,omitempty"`
	OpenAPI    string               `json:"openapi"`
	Servers    []*Server            `json:"servers,omitempty"`
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
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]*Response  `json:"responses,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	OperationID string                `json:"operationId,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	Parameters  []*Parameter          `json:"parameters,omitempty"`
	Security    []map[string][]string `json:"security,omitempty"`
	Deprecated  bool                  `json:"deprecated,omitempty"`
}

type Components struct {
	Schemas         map[string]*Schema         `json:"schemas,omitempty"`
	Parameters      map[string]*Parameter      `json:"parameters,omitempty"`
	Responses       map[string]*Response       `json:"responses,omitempty"`
	SecuritySchemes map[string]*SecurityScheme `json:"securitySchemes,omitempty"`
}

// SecurityScheme describes a single OpenAPI security scheme (an authentication
// method). See https://spec.openapis.org/oas/v3.1.0#security-scheme-object.
type SecurityScheme struct {
	// Type is one of "apiKey", "http", "mutualTLS", "oauth2", or
	// "openIdConnect".
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	// Name and In apply to type "apiKey" (In is "query", "header", or
	// "cookie").
	Name string `json:"name,omitempty"`
	In   string `json:"in,omitempty"`
	// Scheme and BearerFormat apply to type "http" (e.g. "bearer", "basic").
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	// Flows applies to type "oauth2".
	Flows *OAuthFlows `json:"flows,omitempty"`
	// OpenIDConnectURL applies to type "openIdConnect".
	OpenIDConnectURL string `json:"openIdConnectUrl,omitempty"`
}

// OAuthFlows describes the OAuth2 flows for an oauth2 [SecurityScheme].
type OAuthFlows struct {
	Implicit          *OAuthFlow `json:"implicit,omitempty"`
	Password          *OAuthFlow `json:"password,omitempty"`
	ClientCredentials *OAuthFlow `json:"clientCredentials,omitempty"`
	AuthorizationCode *OAuthFlow `json:"authorizationCode,omitempty"`
}

// OAuthFlow describes a single OAuth2 flow.
type OAuthFlow struct {
	Scopes           map[string]string `json:"scopes,omitempty"`
	AuthorizationURL string            `json:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty"`
	RefreshURL       string            `json:"refreshUrl,omitempty"`
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

// AddSecurityScheme registers a named security scheme under
// components.securitySchemes. The name is what operations reference in their
// security requirements.
func (o *OpenAPI) AddSecurityScheme(name string, scheme *SecurityScheme) {
	if o.Components == nil {
		o.Components = &Components{}
	}
	if o.Components.SecuritySchemes == nil {
		o.Components.SecuritySchemes = make(map[string]*SecurityScheme)
	}
	o.Components.SecuritySchemes[name] = scheme
}
