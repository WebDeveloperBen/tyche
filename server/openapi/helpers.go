package openapi

import (
	"reflect"

	"github.com/webdeveloperben/tyche/server/validation"
)

func CloneSchema(s *Schema) *Schema {
	if s == nil {
		return nil
	}
	clone := *s
	if s.Properties != nil {
		clone.Properties = make(map[string]*Schema, len(s.Properties))
		for k, v := range s.Properties {
			clone.Properties[k] = CloneSchema(v)
		}
	}
	if s.Items != nil {
		clone.Items = CloneSchema(s.Items)
	}
	if s.Required != nil {
		clone.Required = append([]string(nil), s.Required...)
	}
	if s.Enum != nil {
		clone.Enum = append([]any(nil), s.Enum...)
	}
	if s.AllOf != nil {
		clone.AllOf = cloneSchemaSlice(s.AllOf)
	}
	if s.OneOf != nil {
		clone.OneOf = cloneSchemaSlice(s.OneOf)
	}
	if s.AnyOf != nil {
		clone.AnyOf = cloneSchemaSlice(s.AnyOf)
	}
	if s.Not != nil {
		clone.Not = CloneSchema(s.Not)
	}
	return &clone
}

func cloneSchemaSlice(src []*Schema) []*Schema {
	dst := make([]*Schema, len(src))
	for i := range src {
		dst[i] = CloneSchema(src[i])
	}
	return dst
}

func ApplyFieldSchemaMetadata(schema *Schema, f reflect.StructField) {
	if schema == nil {
		return
	}
	if desc := f.Tag.Get("doc"); desc != "" {
		schema.Description = desc
	}
	if desc := f.Tag.Get("description"); desc != "" {
		schema.Description = desc
	}
	constraints, err := validation.ConstraintsForField(f, schema.Type)
	if err != nil {
		return
	}
	schema.Format = constraints.Format
	schema.MinLength = constraints.MinLength
	schema.MaxLength = constraints.MaxLength
	schema.Minimum = constraints.Minimum
	schema.Maximum = constraints.Maximum
	schema.Pattern = constraints.Pattern
	schema.MinItems = constraints.MinItems
	schema.MaxItems = constraints.MaxItems
	schema.Enum = constraints.Enum
}

func cloneSchema(s *Schema) *Schema {
	return CloneSchema(s)
}

func applyFieldSchemaMetadata(schema *Schema, f reflect.StructField) {
	ApplyFieldSchemaMetadata(schema, f)
}

func fieldRequired(f reflect.StructField, tagKey string) bool {
	return validation.FieldRequired(f, tagKey)
}
