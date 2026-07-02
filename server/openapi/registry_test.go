package openapi_test

import (
	"reflect"
	"testing"

	"github.com/webdeveloperben/tyche/server/openapi"
)

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
	type node struct {
		Name     string `json:"name"`
		Parent   *node  `json:"parent,omitempty"`
		Children []node `json:"children,omitempty"`
	}

	registry := openapi.NewRegistry("#/components/schemas")
	schema := registry.Schema(reflect.TypeFor[node]())

	if schema == nil {
		t.Fatal("expected schema")
	}
	if schema.Properties["parent"] == nil {
		t.Fatal("expected recursive parent schema")
	}
	if schema.Properties["children"] == nil || schema.Properties["children"].Items == nil {
		t.Fatal("expected recursive children schema")
	}
	if got := schema.Properties["parent"].Properties["name"]; got == nil {
		t.Fatal("expected recursive parent fields to be populated")
	}
}

func TestRegistry_SupportsMutuallyRecursiveTypes(t *testing.T) {
	registry := openapi.NewRegistry("#/components/schemas")
	schema := registry.Schema(reflect.TypeFor[recursiveLeft]())

	rightSchema := schema.Properties["right"]
	if rightSchema == nil {
		t.Fatal("expected right schema")
	}
	leftSchema := rightSchema.Properties["left"]
	if leftSchema == nil {
		t.Fatal("expected nested left schema")
	}
	if got := leftSchema.Properties["name"]; got == nil {
		t.Fatal("expected nested mutually recursive fields to be populated")
	}
}
