package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCommand_WritesConfig(t *testing.T) {
	dir := t.TempDir()
	// go.mod must exist for the validator path to be reachable; we just need
	// a valid version/module so the file the init command writes can be
	// re-loaded by config.Load.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"init", "--root", dir, "--module", "github.com/acme/api/client", "--spec", "./api/openapi.json", "--type-naming", "structural", "--yes"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out.String())
	}

	dest := filepath.Join(dir, "tyche.json")
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("expected tyche.json, got %v", err)
	}
	s := string(body)
	for _, want := range []string{`"version": 1`, `"github.com/acme/api/client"`, `"./api/openapi.json"`, `"structural"`} {
		if !strings.Contains(s, want) {
			t.Errorf("scaffold missing %q:\n%s", want, s)
		}
	}
}

func TestInitCommand_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tyche.json"), []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"init", "--root", dir, "--module", "github.com/acme/api/client", "--yes"})

	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected overwrite error, got %v", err)
	}
}

func TestInitCommand_ForceOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tyche.json"), []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"init", "--root", dir, "--module", "github.com/acme/api/client", "--type-naming", "structural", "--yes", "--force"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--force should overwrite, got %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "tyche.json"))
	if !strings.Contains(string(body), "github.com/acme/api/client") {
		t.Errorf("overwritten file missing module: %s", body)
	}
}

func TestInitCommand_RoundTripsValidation(t *testing.T) {
	// An invalid module must fail init before the file is left behind.
	dir := t.TempDir()
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"init", "--root", dir, "--module", "bad module with spaces", "--yes"})

	err := cmd.Execute()
	t.Logf("err=%v\noutput=%s", err, out.String())
	if err == nil {
		t.Fatal("expected validation error for bad module")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "tyche.json")); !os.IsNotExist(statErr) {
		t.Errorf("expected no tyche.json after failed init, got %v", statErr)
	}
}

func TestConfigShow_NoFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"config", "show", "--root", dir, "--quiet"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("config show: %v", err)
	}
	if !strings.Contains(out.String(), "no config file found") {
		t.Errorf("expected no-config message, got %q", out.String())
	}
}

func TestConfigShow_PrintsLoaded(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := `{"version": 1, "spec": "./api/openapi.json", "client": {"out": "./client", "module": "github.com/acme/api/client", "type_naming": "structural"}}`
	if err := os.WriteFile(filepath.Join(dir, "tyche.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"config", "show", "--root", dir, "--quiet"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("config show: %v", err)
	}
	text := out.String()
	for _, want := range []string{"tyche: config", "client.module", "github.com/acme/api/client"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in output, got:\n%s", want, text)
		}
	}
}

func TestConfigShow_JSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := `{"version": 1, "client": {"out": "./client", "module": "github.com/acme/api/client"}}`
	if err := os.WriteFile(filepath.Join(dir, "tyche.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"config", "show", "--root", dir, "--quiet", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("config show --json: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, `"version": 1`) || !strings.Contains(text, `"github.com/acme/api/client"`) {
		t.Errorf("expected JSON output, got:\n%s", text)
	}
}

func TestGenerate_ReadsConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\ngo 1.25.5\nrequire github.com/webdeveloperben/tyche v0.0.0\n\nreplace github.com/webdeveloperben/tyche => "+repoRoot(t)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal/routes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal/routes/routes.go"), []byte(`package routes

import (
	"context"
	"net/http"

	"github.com/webdeveloperben/tyche/server"
)

type input struct{}
type output struct {
	Body struct {
		OK bool `+"`json:\"ok\"`"+`
	} `+"`body:\"true\"`"+`
}

func Register(router *server.API) {
	api := router.Group("/api")
	server.Register(api, server.Operation{
		OperationID: "fixture-ok",
		Method:      http.MethodGet,
		Path:        "/ok",
	}, func(ctx context.Context, in *input) (*output, error) {
		out := &output{}
		out.Body.OK = true
		return out, nil
	})
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := `{"version": 1, "server": {"patterns": ["./internal/..."]}}`
	if err := os.WriteFile(filepath.Join(root, "tyche.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"generate", "--root", root, "--quiet"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("generate: %v\n%s", err, out.String())
	}

	// The server.patterns from the config should have driven generation, so
	// the route file should now have its codec sibling next to it.
	if _, err := os.Stat(filepath.Join(root, "internal", "routes", "zz_server_routes_gen.go")); err != nil {
		t.Errorf("expected generated codec next to routes.go, got %v", err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestGenerate_FailsOnBadVersion(t *testing.T) {
	// A tyche.json with the wrong version must fail loudly at the CLI level,
	// not at the servergen step that follows. The error must mention the
	// version so the user can fix the file without spelunking.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tyche.json"), []byte(`{"version": 99}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"generate", "--root", dir, "--quiet"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for bad version, got nil\noutput=%s", out.String())
	}
	if !strings.Contains(err.Error(), "unsupported version") && !strings.Contains(err.Error(), "version") {
		t.Errorf("expected error to mention version, got %v", err)
	}
}

func TestConfigShow_FailsOnBadVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tyche.json"), []byte(`{"version": 99}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"config", "show", "--root", dir, "--quiet"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for bad version")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("expected error to mention version, got %v", err)
	}
}
