package openapi

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	schemas  map[reflect.Type]*Schema
	building map[reflect.Type]bool
	prefix   string
}

func indirectType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func NewRegistry(prefix string) *Registry {
	return &Registry{
		schemas:  make(map[reflect.Type]*Schema),
		building: make(map[reflect.Type]bool),
		prefix:   prefix,
	}
}

func (r *Registry) Schema(t reflect.Type) *Schema {
	r.mu.Lock()
	defer r.mu.Unlock()

	t = indirectType(t)
	if s, ok := r.schemas[t]; ok {
		return s
	}

	s := &Schema{}
	r.schemas[t] = s
	r.building[t] = true
	*s = *r.schemaFromType(t)
	delete(r.building, t)
	return s
}

func (r *Registry) SchemaUnlocked(t reflect.Type) *Schema {
	t = indirectType(t)
	if s, ok := r.schemas[t]; ok {
		return s
	}

	s := &Schema{}
	r.schemas[t] = s
	r.building[t] = true
	*s = *r.schemaFromType(t)
	delete(r.building, t)
	return s
}

func (r *Registry) schemaFromType(t reflect.Type) *Schema {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	s := &Schema{}

	switch t.Kind() {
	case reflect.Bool:
		s.Type = "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		s.Type = "integer"
		s.Format = "int32"
	case reflect.Int64:
		s.Type = "integer"
		s.Format = "int64"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		s.Type = "integer"
		s.Format = "int32"
		min := float64(0)
		s.Minimum = &min
	case reflect.Uint64:
		s.Type = "integer"
		s.Format = "int64"
		min := float64(0)
		s.Minimum = &min
	case reflect.Float32:
		s.Type = "number"
		s.Format = "float"
	case reflect.Float64:
		s.Type = "number"
		s.Format = "double"
	case reflect.String:
		s.Type = "string"
	case reflect.Slice:
		s.Type = "array"
		if t.Elem().Kind() == reflect.Uint8 {
			s.Type = "string"
			s.Format = "binary"
		} else {
			s.Items = r.SchemaUnlocked(t.Elem())
		}
	case reflect.Map:
		s.Type = "object"
		s.AdditionalProperties = r.SchemaUnlocked(t.Elem())
	case reflect.Struct:
		s.Type = "object"
		s.Properties = make(map[string]*Schema)
		s.Required = []string{}

		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}

			jsonName := f.Name
			if tag := f.Tag.Get("json"); tag != "" {
				parts := strings.Split(tag, ",")
				if parts[0] != "" {
					jsonName = parts[0]
				}
			}

			if jsonName == "-" {
				continue
			}

			required := fieldRequired(f, "json")

			var fieldSchema *Schema
			if r.containsBuildingType(f.Type, map[reflect.Type]bool{}) {
				fieldSchema = r.SchemaUnlocked(f.Type)
			} else {
				fieldSchema = cloneSchema(r.SchemaUnlocked(f.Type))
			}
			if fieldSchema == nil {
				continue
			}
			applyFieldSchemaMetadata(fieldSchema, f)

			s.Properties[jsonName] = fieldSchema

			if required {
				s.Required = append(s.Required, jsonName)
			}
		}

		if len(s.Required) == 0 {
			s.Required = nil
		}
	}

	return s
}

func (r *Registry) containsBuildingType(t reflect.Type, seen map[reflect.Type]bool) bool {
	t = indirectType(t)
	if t == nil {
		return false
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	if r.building[t] {
		return true
	}

	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		return r.containsBuildingType(t.Elem(), seen)
	case reflect.Map:
		return r.containsBuildingType(t.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			if r.containsBuildingType(f.Type, seen) {
				return true
			}
		}
	}
	return false
}

type SchemaProvider interface {
	Schema() *Schema
}

type SchemaRegistry[T any] struct {
	registry *Registry
	schema   *Schema
}

func Register[T any](r *Registry) *SchemaRegistry[T] {
	t := reflect.TypeFor[T]()
	schema := r.Schema(t)
	return &SchemaRegistry[T]{
		registry: r,
		schema:   schema,
	}
}

func (s *SchemaRegistry[T]) Schema() *Schema {
	return s.schema
}

func (s *SchemaRegistry[T]) Ref() string {
	return fmt.Sprintf("%s/schemas/%T", s.registry.prefix, *new(T))
}

func (r *Registry) MarshalJSON() ([]byte, error) {
	type registryJSON struct {
		Schemas map[string]*Schema `json:"schemas,omitempty"`
	}
	schemas := make(map[string]*Schema)
	for t, s := range r.schemas {
		schemas[fmt.Sprintf("%s/schemas/%s", r.prefix, t.Name())] = s
	}
	return json.Marshal(registryJSON{
		Schemas: schemas,
	})
}

func (r *Registry) Schemas() map[reflect.Type]*Schema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[reflect.Type]*Schema, len(r.schemas))
	maps.Copy(result, r.schemas)
	return result
}
