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

const clientCmdNamingSpec = `{
  "openapi":"3.1.0","info":{"title":"Names","version":"1.0.0"},
  "paths":{
    "/alpha":{"get":{"operationId":"alpha","responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"type":"object","properties":{"data":{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}}}}}}}}},
    "/beta":{"get":{"operationId":"beta","responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"type":"object","properties":{"data":{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}}}}}}}}}
  },
  "components":{"schemas":{}}
}`

func TestStandaloneCommand_GeneratesFiles(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "openapi.json")
	if err := os.WriteFile(specPath, []byte(clientCmdSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "client")

	cmd := NewCommand("clientgen")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--spec", specPath, "--out", outDir, "--module", "example.com/x/client"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("clientgen command failed: %v\n%s", err, out.String())
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

func TestStandaloneCommand_RemovesStaleFiles(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "client")

	streamSpec := `{"openapi":"3.1.0","info":{"title":"T","version":"1.0.0"},"paths":{"/s":{"get":{"operationId":"s","responses":{"200":{"description":"x","content":{"text/event-stream":{"schema":{"type":"object","properties":{"m":{"type":"string"}}}}}}}}}},"components":{"schemas":{}}}`
	plainSpec := clientCmdSpec // a non-streaming spec

	run := func(spec string) {
		t.Helper()
		specPath := filepath.Join(dir, "openapi.json")
		if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := NewCommand("clientgen")
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"--spec", specPath, "--out", outDir, "--module", "example.com/x/client"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("clientgen command failed: %v\n%s", err, out.String())
		}
	}

	run(streamSpec)
	if _, err := os.Stat(filepath.Join(outDir, "stream.go")); err != nil {
		t.Fatalf("expected stream.go after streaming spec: %v", err)
	}

	// Regenerating a non-streaming spec must remove the now-stale stream.go.
	run(plainSpec)
	if _, err := os.Stat(filepath.Join(outDir, "stream.go")); !os.IsNotExist(err) {
		t.Errorf("stale stream.go was not removed on regeneration (err=%v)", err)
	}
}

func TestStandaloneCommand_TypeNamingFlag(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "openapi.json")
	if err := os.WriteFile(specPath, []byte(clientCmdNamingSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "client")

	cmd := NewCommand("clientgen")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"--spec", specPath,
		"--out", outDir,
		"--module", "example.com/x/client",
		"--type-naming", "operation-scoped",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("clientgen command failed: %v\n%s", err, out.String())
	}

	types, err := os.ReadFile(filepath.Join(outDir, "types.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(types), "type BetaOutput struct") {
		t.Errorf("operation-scoped type naming did not emit BetaOutput:\n%s", types)
	}
}

func TestStandaloneCommand_RequiresFlags(t *testing.T) {
	cmd := NewCommand("clientgen")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when required flags are missing")
	}
}
