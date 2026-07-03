package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestScaffold_WritesConfigAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	got, err := Scaffold(ScaffoldOptions{
		Root:       dir,
		Module:     "github.com/acme/api/client",
		Spec:       "./api/openapi.json",
		TypeNaming: "structural",
	})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if got != filepath.Join(dir, "tyche.json") {
		t.Fatalf("Scaffold returned %q, want %q", got, filepath.Join(dir, "tyche.json"))
	}
	body, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"version": 1`, `"github.com/acme/api/client"`, `"./api/openapi.json"`, `"structural"`} {
		if !contains(body, want) {
			t.Errorf("scaffold missing %q:\n%s", want, body)
		}
	}
}

func TestScaffold_AppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	if _, err := Scaffold(ScaffoldOptions{
		Root:   dir,
		Module: "github.com/acme/api/client",
	}); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "tyche.json"))
	if !contains(body, `"./api/openapi.json"`) {
		t.Errorf("expected default spec path, got:\n%s", body)
	}
	if !contains(body, `"structural"`) {
		t.Errorf("expected default type_naming, got:\n%s", body)
	}
}

func TestScaffold_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "tyche.json")
	if err := os.WriteFile(dest, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(ScaffoldOptions{Root: dir, Module: "github.com/acme/api/client"}); err == nil {
		t.Fatal("expected overwrite error, got nil")
	}
}

func TestScaffold_ForceOverwrite(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "tyche.json")
	if err := os.WriteFile(dest, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(ScaffoldOptions{
		Root:   dir,
		Module: "github.com/acme/api/client",
		Force:  true,
	}); err != nil {
		t.Fatalf("Scaffold with force: %v", err)
	}
	body, _ := os.ReadFile(dest)
	if !contains(body, "github.com/acme/api/client") {
		t.Errorf("overwritten file missing module: %s", body)
	}
}

func TestScaffold_RoundTripsValidation(t *testing.T) {
	dir := t.TempDir()
	_, err := Scaffold(ScaffoldOptions{
		Root:   dir,
		Module: "bad module with spaces",
	})
	if err == nil {
		t.Fatal("expected validation error for bad module")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "tyche.json")); !os.IsNotExist(statErr) {
		t.Errorf("expected no tyche.json after failed init, got %v", statErr)
	}
}

func TestScaffold_RequiresModule(t *testing.T) {
	dir := t.TempDir()
	_, err := Scaffold(ScaffoldOptions{Root: dir})
	if err == nil {
		t.Fatal("expected error for missing module")
	}
}

func TestLoadConfig_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"version": 1, "client": {"out": "./client", "module": "github.com/acme/api/client"}}`
	cfgPath := filepath.Join(dir, "tyche.json")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadConfig(LoadOptions{Root: dir, ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if res == nil {
		t.Fatal("LoadConfig returned nil")
	}
	if res.Path == "" {
		t.Errorf("LoadConfig returned empty path")
	}
}

func TestLoadConfig_DiscoveryFromCwd(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"version": 1, "client": {"out": "./client", "module": "github.com/acme/api/client"}}`
	if err := os.WriteFile(filepath.Join(dir, "tyche.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadConfig(LoadOptions{Root: dir})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if res == nil {
		t.Fatal("LoadConfig returned nil; expected to find tyche.json")
	}
}

func TestLoadConfig_NoConfig(t *testing.T) {
	dir := t.TempDir()
	res, err := LoadConfig(LoadOptions{Root: dir})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil result for missing config, got %+v", res)
	}
}

func TestShowConfig_ResolvesFields(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"version": 1, "spec": "./api/openapi.json", "client": {"out": "./client", "module": "github.com/acme/api/client", "type_naming": "structural"}, "server": {"patterns": ["./..."]}}`
	if err := os.WriteFile(filepath.Join(dir, "tyche.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ShowConfig(LoadOptions{Root: dir})
	if err != nil {
		t.Fatalf("ShowConfig: %v", err)
	}
	if res == nil {
		t.Fatal("ShowConfig returned nil")
	}
	if res.Version != 1 {
		t.Errorf("Version = %d, want 1", res.Version)
	}
	if res.Spec != "./api/openapi.json" {
		t.Errorf("Spec = %q", res.Spec)
	}
	if res.Client == nil || res.Client["module"] != "github.com/acme/api/client" {
		t.Errorf("Client.module missing: %+v", res.Client)
	}

	// JSON-encodable: round-trip the result through json.Marshal and
	// confirm the consumer sees the same shape.
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !contains(data, `"github.com/acme/api/client"`) {
		t.Errorf("JSON output missing module: %s", data)
	}
}

func TestShowConfig_NoConfig(t *testing.T) {
	dir := t.TempDir()
	res, err := ShowConfig(LoadOptions{Root: dir})
	if err != nil {
		t.Fatalf("ShowConfig: %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil for missing config, got %+v", res)
	}
}

func TestParseTypeNamingStrategy_RejectsUnknown(t *testing.T) {
	_, err := parseTypeNamingStrategy("nonsense")
	if err == nil {
		t.Fatal("expected error for unknown type-naming value")
	}
}

func TestParseTypeNamingStrategy_AcceptsAliases(t *testing.T) {
	cases := []string{"", "structural", "operation-scoped", "operation_scoped", "operation"}
	for _, c := range cases {
		if _, err := parseTypeNamingStrategy(c); err != nil {
			t.Errorf("parseTypeNamingStrategy(%q) = %v, want nil", c, err)
		}
	}
}

func TestResolveRoot_DefaultsToCwd(t *testing.T) {
	got, err := ResolveRoot("")
	if err != nil {
		t.Fatalf("ResolveRoot: %v", err)
	}
	cwd, _ := os.Getwd()
	if got != cwd {
		t.Errorf("ResolveRoot(\"\") = %q, want cwd %q", got, cwd)
	}
}

func TestResolvePath_AbsolutePassthrough(t *testing.T) {
	got := ResolvePath("/anywhere", "/abs/path")
	if got != "/abs/path" {
		t.Errorf("ResolvePath with abs input = %q, want /abs/path", got)
	}
}

func TestResolvePath_RelativeJoins(t *testing.T) {
	got := ResolvePath("/root", "rel/path")
	if got != "/root/rel/path" {
		t.Errorf("ResolvePath with rel input = %q, want /root/rel/path", got)
	}
}

// contains is a tiny helper because we want to use it on raw []byte
// without converting to string first. Saves a few allocations in
// tests that scan many fields.
func contains(haystack []byte, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack []byte, needle string) int {
	n := len(needle)
	for i := 0; i+n <= len(haystack); i++ {
		if string(haystack[i:i+n]) == needle {
			return i
		}
	}
	return -1
}
