package openapi

import (
	"encoding/json"
	"fmt"
	"maps"
	"mime/multipart"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
)

type Registry struct {
	schemas   map[reflect.Type]*Schema
	building  map[reflect.Type]bool
	recursive map[reflect.Type]bool
	stack     []reflect.Type
	prefix    string
	mu        sync.RWMutex
}

func indirectType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func NewRegistry(prefix string) *Registry {
	return &Registry{
		schemas:   make(map[reflect.Type]*Schema),
		building:  make(map[reflect.Type]bool),
		recursive: make(map[reflect.Type]bool),
		prefix:    prefix,
	}
}

func (r *Registry) Schema(t reflect.Type) *Schema {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.SchemaUnlocked(t)
}

func (r *Registry) SchemaUnlocked(t reflect.Type) *Schema {
	t = indirectType(t)
	if r.building[t] {
		// Re-entering a named struct that is still being built: reference its
		// component instead of recursing, and remember every type on the
		// cycle so its component is published.
		for _, v := range slices.Backward(r.stack) {
			r.recursive[v] = true
			if v == t {
				break
			}
		}
		return &Schema{Ref: r.componentRef(t)}
	}
	if s, ok := r.schemas[t]; ok {
		return s
	}

	// Only named structs can close a cycle; anonymous composites (slices,
	// maps, pointers) build through so the reference lands on the struct.
	if !cycleCandidate(t) {
		return r.schemaFromType(t)
	}

	s := &Schema{}
	r.schemas[t] = s
	r.building[t] = true
	r.stack = append(r.stack, t)
	*s = *r.schemaFromType(t)
	r.stack = r.stack[:len(r.stack)-1]
	delete(r.building, t)
	return s
}

func cycleCandidate(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t.Name() != ""
}

func (r *Registry) componentRef(t reflect.Type) string {
	return r.prefix + "/" + schemaComponentName(t)
}

func schemaComponentName(t reflect.Type) string {
	t = indirectType(t)
	name := t.Name()
	if name == "" {
		name = t.String()
	}
	if t.PkgPath() != "" {
		name = strings.NewReplacer("/", "_", ".", "_", "-", "_").Replace(t.PkgPath()) + "_" + name
	}
	return name
}

func (r *Registry) RecursiveSchemas() map[string]*Schema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]*Schema, len(r.recursive))
	for t := range r.recursive {
		result[schemaComponentName(t)] = r.schemas[t]
	}
	return result
}

func (r *Registry) schemaFromType(t reflect.Type) *Schema {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	s := &Schema{}

	if t == reflect.TypeFor[time.Time]() {
		s.Type = "string"
		s.Format = "date-time"
		return s
	}

	if t == reflect.TypeFor[multipart.FileHeader]() {
		s.Type = "string"
		s.Format = "binary"
		return s
	}

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

			fieldSchema := cloneSchema(r.SchemaUnlocked(f.Type))
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
