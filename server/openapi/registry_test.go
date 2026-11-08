package openapi_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/webdeveloperben/tyche/server/openapi"
)

func TestRegistry_TimeFieldsUseDateTimeFormat(t *testing.T) {
	t.Parallel()

	type model struct {
		CreatedAt time.Time   `json:"created_at"`
		UpdatedAt *time.Time  `json:"updated_at"`
		History   []time.Time `json:"history"`
	}

	schema := openapi.NewRegistry("#/components/schemas").Schema(reflect.TypeFor[model]())
	for _, name := range []string{"created_at", "updated_at"} {
		field := schema.Properties[name]
		if field.Type != "string" || field.Format != "date-time" {
			t.Errorf("%s: got type %q, format %q", name, field.Type, field.Format)
		}
	}
	history := schema.Properties["history"]
	if history.Type != "array" || history.Items.Type != "string" || history.Items.Format != "date-time" {
		t.Errorf("history: got type %q, items %#v", history.Type, history.Items)
	}
}

func TestRegistry_TimeFieldFormatTagOverridesDateTime(t *testing.T) {
	t.Parallel()

	type model struct {
		Date time.Time `json:"date" format:"date"`
	}

	schema := openapi.NewRegistry("#/components/schemas").Schema(reflect.TypeFor[model]())
	field := schema.Properties["date"]
	if field.Type != "string" || field.Format != "date" {
		t.Fatalf("got type %q, format %q", field.Type, field.Format)
	}
}

type recursiveLeft struct {
	Right *recursiveRight `json:"right,omitempty"`
	Name  string          `json:"name"`
}

type recursiveRight struct {
	Left *recursiveLeft `json:"left,omitempty"`
	Name string         `json:"name"`
}

func TestRegistry_DoesNotLeakFieldMetadataAcrossSharedTypes(t *testing.T) {
	type child struct {
		Value string `json:"value"`
	}
	type parent struct {
		Alpha child `json:"alpha" doc:"Alpha child"`
		Beta  child `json:"beta" doc:"Beta child"`
	}

	registry := openapi.NewRegistry("#/components/schemas")
	schema := registry.Schema(reflect.TypeFor[parent]())

	alpha := schema.Properties["alpha"]
	beta := schema.Properties["beta"]

	if alpha.Description != "Alpha child" {
		t.Fatalf("expected alpha description, got %q", alpha.Description)
	}
	if beta.Description != "Beta child" {
		t.Fatalf("expected beta description, got %q", beta.Description)
	}
	if alpha == beta {
		t.Fatal("expected distinct field schema instances for shared child types")
	}
}

func TestRegistry_SupportsRecursiveTypes(t *testing.T) {
	t.Parallel()

	type node struct {
		Name     string `json:"name"`
		Parent   *node  `json:"parent,omitempty"`
		Children []node `json:"children,omitempty"`
	}

	registry := openapi.NewRegistry("#/components/schemas")
	schema := registry.Schema(reflect.TypeFor[node]())
	components := registry.RecursiveSchemas()
	name := ""
	for componentName, component := range components {
		if component == schema {
			name = componentName
			break
		}
	}
	if name == "" {
		t.Fatal("expected recursive node component")
	}
	ref := "#/components/schemas/" + name
	if got := schema.Properties["parent"]; got == nil || got.Ref != ref {
		t.Fatalf("expected parent reference %q, got %#v", ref, got)
	}
	children := schema.Properties["children"]
	if children == nil || children.Items == nil || children.Items.Ref != ref {
		t.Fatalf("expected children items reference %q, got %#v", ref, children)
	}
}

func TestRegistry_SupportsMutuallyRecursiveTypes(t *testing.T) {
	t.Parallel()

	registry := openapi.NewRegistry("#/components/schemas")
	schema := registry.Schema(reflect.TypeFor[recursiveLeft]())
	rightSchema := schema.Properties["right"]
	if rightSchema == nil {
		t.Fatal("expected right schema")
	}
	leftSchema := rightSchema.Properties["left"]
	if leftSchema == nil || leftSchema.Ref == "" {
		t.Fatalf("expected left to reference its component, got %#v", leftSchema)
	}
	if len(registry.RecursiveSchemas()) != 2 {
		t.Fatalf("expected both mutually recursive components, got %d", len(registry.RecursiveSchemas()))
	}
}

func TestCloneSchema_StopsAtRef(t *testing.T) {
	t.Parallel()

	schema := &openapi.Schema{Ref: "#/components/schemas/node"}
	clone := openapi.CloneSchema(schema)
	if clone == schema || clone.Ref != schema.Ref {
		t.Fatalf("expected copied reference schema, got %#v", clone)
	}
}
