package server

import (
	"go/ast"
	"go/token"
	"reflect"
	"testing"
)

func TestSchemaComponentName_UsesPackagePath(t *testing.T) {
	astName := schemaComponentName(reflect.TypeFor[ast.File]())
	tokenName := schemaComponentName(reflect.TypeFor[token.File]())

	if astName == tokenName {
		t.Fatalf("expected distinct schema component names, got %q", astName)
	}
}

func TestServerPathToOpenAPIPath_ConvertsWildcard(t *testing.T) {
	got := serverPathToOpenAPIPath("/files/*path")
	if got != "/files/{path}" {
		t.Fatalf("expected wildcard path conversion, got %q", got)
	}
}
