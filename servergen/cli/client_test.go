package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const clientCmdSpec = `{
  "openapi": "3.1.0",
  "info": {"title": "T", "version": "1.0.0"},
  "paths": {
    "/ping": {
      "get": {
        "operationId": "ping",
        "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {"type": "object", "properties": {"data": {"type": "object", "properties": {"pong": {"type": "boolean"}}}}}}}}}
      }
    }
  },
  "components": {"schemas": {}}
}`

func TestClientCommand_GeneratesFiles(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "openapi.json")
	if err := os.WriteFile(specPath, []byte(clientCmdSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "client")

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"client", "--spec", specPath, "--out", outDir, "--module", "example.com/x/client"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("client command failed: %v\n%s", err, out.String())
	}

	for _, name := range []string{"go.mod", "client.go", "operations.go", "types.go"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Errorf("expected generated file %q: %v", name, err)
		}
	}
	gomod, _ := os.ReadFile(filepath.Join(outDir, "go.mod"))
	if !strings.Contains(string(gomod), "module example.com/x/client") {
		t.Errorf("unexpected go.mod:\n%s", gomod)
	}
}

func TestClientCommand_RequiresFlags(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"client"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when required flags are missing")
	}
}
