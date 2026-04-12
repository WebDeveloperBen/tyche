package server_test

import (
	"go/ast"
	"go/token"
	"reflect"
	"testing"

	"github.com/webdeveloperben/tyche/server"
)

func TestSchemaComponentName_UsesPackagePath(t *testing.T) {
	astName := server.SchemaComponentName(reflect.TypeFor[ast.File]())
	tokenName := server.SchemaComponentName(reflect.TypeFor[token.File]())

	if astName == tokenName {
		t.Fatalf("expected distinct schema component names, got %q", astName)
	}
}

func TestServerPathToOpenAPIPath_ConvertsWildcard(t *testing.T) {
	got := server.ServerPathToOpenAPIPath("/files/*path")
	if got != "/files/{path}" {
		t.Fatalf("expected wildcard path conversion, got %q", got)
	}
}
