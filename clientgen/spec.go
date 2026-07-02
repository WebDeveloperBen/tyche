package clientgen

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Document is the subset of an OpenAPI 3.x document that the client generator
// consumes. It is intentionally self-contained (rather than reusing the server
// openapi types) so parsing is robust — most importantly so additionalProperties
// can be a schema or a boolean.
type Document struct {
	Paths      map[string]*PathItem `json:"paths"`
	Components *Components          `json:"components"`
	Info       Info                 `json:"info"`
	OpenAPI    string               `json:"openapi"`
}

type Info struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type Components struct {
	Schemas map[string]*Schema `json:"schemas"`
}

type PathItem struct {
	Get     *Operation `json:"get"`
	Put     *Operation `json:"put"`
	Post    *Operation `json:"post"`
	Delete  *Operation `json:"delete"`
	Options *Operation `json:"options"`
	Head    *Operation `json:"head"`
	Patch   *Operation `json:"patch"`
	Trace   *Operation `json:"trace"`
}

// methods returns the operations on this path item paired with their HTTP
// method, in a deterministic order.
func (p *PathItem) methods() []methodOp {
	out := make([]methodOp, 0, 4)
	for _, m := range []methodOp{
		{method: "GET", op: p.Get},
		{method: "POST", op: p.Post},
		{method: "PUT", op: p.Put},
		{method: "PATCH", op: p.Patch},
		{method: "DELETE", op: p.Delete},
		{method: "HEAD", op: p.Head},
		{method: "OPTIONS", op: p.Options},
		{method: "TRACE", op: p.Trace},
	} {
		if m.op != nil {
			out = append(out, m)
		}
	}
	return out
}

type methodOp struct {
	op     *Operation
	method string
}

type Operation struct {
	RequestBody *RequestBody         `json:"requestBody"`
	Responses   map[string]*Response `json:"responses"`
	OperationID string               `json:"operationId"`
	Summary     string               `json:"summary"`
	Description string               `json:"description"`
	Tags        []string             `json:"tags"`
	Parameters  []*Parameter         `json:"parameters"`
	Deprecated  bool                 `json:"deprecated"`
}

type Parameter struct {
	Schema      *Schema `json:"schema"`
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Description string  `json:"description"`
	Required    bool    `json:"required"`
}

type RequestBody struct {
	Content     map[string]*MediaType `json:"content"`
	Description string                `json:"description"`
	Required    bool                  `json:"required"`
}

type MediaType struct {
	Schema *Schema `json:"schema"`
}

type Response struct {
	Headers     map[string]*Parameter `json:"headers"`
	Content     map[string]*MediaType `json:"content"`
	Description string                `json:"description"`
}

// Schema is the subset of JSON Schema / OpenAPI schema the generator understands.
type Schema struct {
	Properties           map[string]*Schema    `json:"properties"`
	AdditionalProperties *AdditionalProperties `json:"additionalProperties"`
	Items                *Schema               `json:"items"`
	Description          string                `json:"description"`
	Ref                  string                `json:"$ref"`
	Format               string                `json:"format"`
	Type                 string                `json:"type"`
	Required             []string              `json:"required"`
	Enum                 []any                 `json:"enum"`
	AllOf                []*Schema             `json:"allOf"`
	OneOf                []*Schema             `json:"oneOf"`
	AnyOf                []*Schema             `json:"anyOf"`
	Nullable             bool                  `json:"nullable"`
}

// AdditionalProperties models the JSON Schema additionalProperties keyword,
// which may be a boolean or a schema.
type AdditionalProperties struct {
	Bool   *bool
	Schema *Schema
}

func (a *AdditionalProperties) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == 't' || b[0] == 'f' {
		var v bool
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		a.Bool = &v
		return nil
	}
	var s Schema
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	a.Schema = &s
	return nil
}

// ParseDocument decodes an OpenAPI document from JSON.
func ParseDocument(data []byte) (*Document, error) {
	var doc Document
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("clientgen: parse OpenAPI document: %w", err)
	}
	if doc.Paths == nil {
		doc.Paths = map[string]*PathItem{}
	}
	return &doc, nil
}

// resolve follows a local $ref ("#/components/schemas/Name") to its target
// schema, returning the schema unchanged if it is not a reference.
func (d *Document) resolve(s *Schema) *Schema {
	const prefix = "#/components/schemas/"
	seen := map[string]bool{}
	for s != nil && s.Ref != "" {
		name, ok := trimPrefix(s.Ref, prefix)
		if !ok || d.Components == nil || seen[name] {
			return s
		}
		seen[name] = true
		s = d.Components.Schemas[name]
	}
	return s
}

func trimPrefix(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):], true
	}
	return "", false
}
