package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidate_RequiresVersionOne(t *testing.T) {
	tests := map[string]*File{
		"missing version": {Client: &ClientBlock{Module: "github.com/x/y"}},
		"version 0":       {Version: 0, Client: &ClientBlock{Module: "github.com/x/y"}},
		"version 2":       {Version: 2, Client: &ClientBlock{Module: "github.com/x/y"}},
		"version 99":      {Version: 99, Client: &ClientBlock{Module: "github.com/x/y"}},
	}
	for name, f := range tests {
		t.Run(name, func(t *testing.T) {
			err := f.Validate()
			if err == nil || !strings.Contains(err.Error(), "unsupported version") {
				t.Fatalf("expected unsupported-version error, got %v", err)
			}
		})
	}
}

func TestValidate_ClientModuleRequired(t *testing.T) {
	f := &File{Version: 1, Client: &ClientBlock{}}
	err := f.Validate()
	if err == nil || !strings.Contains(err.Error(), "client.module is required") {
		t.Fatalf("expected client.module required error, got %v", err)
	}
}

func TestValidate_ClientModuleFormat(t *testing.T) {
	bad := []string{
		"noSlash",
		"Has/Caps",
		"has space/path",
		"/leading-slash/path",
		"trailing/",
		"./relative/path",
	}
	for _, mod := range bad {
		t.Run(mod, func(t *testing.T) {
			f := &File{Version: 1, Client: &ClientBlock{Module: mod}}
			err := f.Validate()
			if err == nil || !strings.Contains(err.Error(), "not a valid Go module path") {
				t.Fatalf("expected module-path error for %q, got %v", mod, err)
			}
		})
	}

	good := []string{
		"github.com/x/y",
		"gitlab.com/group/subgroup/project",
		"example.com/v2",
		"a.b-c.io/module_name",
	}
	for _, mod := range good {
		t.Run(mod, func(t *testing.T) {
			f := &File{Version: 1, Client: &ClientBlock{Module: mod}}
			if err := f.Validate(); err != nil {
				t.Fatalf("expected %q to be valid, got %v", mod, err)
			}
		})
	}
}

func TestValidate_TypeNamingEnum(t *testing.T) {
	tests := map[string]bool{
		"":                 true,
		"structural":       true,
		"operation-scoped": true,
		"OPERATION-SCOPED": false,
		"nope":             false,
	}
	for value, ok := range tests {
		f := &File{Version: 1, Client: &ClientBlock{Module: "github.com/x/y", TypeNaming: value}}
		err := f.Validate()
		if ok && err != nil {
			t.Errorf("expected %q valid, got %v", value, err)
		}
		if !ok && err == nil {
			t.Errorf("expected %q invalid, got nil", value)
		}
	}
}

func TestValidate_SpecFormatOnly(t *testing.T) {
	// Validate only checks the structure of the config, not whether the
	// spec file actually exists. Existence is a runtime concern: the spec
	// path is checked at the point of use, not at config-load time.
	f := &File{Version: 1, Spec: "./does/not/exist/openapi.json", Client: &ClientBlock{Module: "github.com/x/y"}}
	if err := f.Validate(); err != nil {
		t.Fatalf("expected non-existent spec to pass structural validation, got %v", err)
	}
}

func TestValidate_ServerIgnorePaths(t *testing.T) {
	tests := map[string]bool{
		"./tmp":      true,
		"./bin":      true,
		"..":         false,
		"../outside": false,
		".":          false,
		"":           false,
		"sub/dir":    true,
	}
	for p, ok := range tests {
		f := &File{Version: 1, Server: &ServerBlock{Ignore: []string{p}}}
		err := f.Validate()
		if ok && err != nil {
			t.Errorf("expected %q valid, got %v", p, err)
		}
		if !ok && err == nil {
			t.Errorf("expected %q invalid, got nil", p)
		}
	}
}

func TestParse_BadJSON_LineCol(t *testing.T) {
	bad := []byte("{\n  \"version\": 1,\n  \"client\": {\n    \"module\":\n}")
	_, err := parse(bad)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "line") || !strings.Contains(err.Error(), "col") {
		t.Fatalf("expected line:col in error, got %v", err)
	}
}

func TestParse_UnknownField(t *testing.T) {
	// Unknown top-level fields are rejected today for predictable failure;
	// when forward compat is wanted this test should switch to "ignored".
	data := []byte(`{"version": 1, "client": {"module": "github.com/x/y"}, "extra": true}`)
	_, err := parse(data)
	if err == nil {
		t.Fatal("expected unknown-field error")
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Fatalf("expected error mentioning extra field, got %v", err)
	}
}

func TestLoad_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tyche.json")
	writeFile(t, path, `{"version": 1, "client": {"module": "github.com/x/y"}}`)

	res, err := Load(LoadOptions{ExplicitPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path == "" || res.File == nil {
		t.Fatalf("expected file and path, got %+v", res)
	}
	if res.File.Client.Module != "github.com/x/y" {
		t.Errorf("unexpected module: %q", res.File.Client.Module)
	}
}

func TestLoad_ExplicitPath_Missing(t *testing.T) {
	_, err := Load(LoadOptions{ExplicitPath: "/no/such/file.json"})
	if err == nil {
		t.Fatal("expected error for missing explicit path")
	}
}

func TestLoad_EnvConfigPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tyche.json")
	writeFile(t, path, `{"version": 1, "client": {"module": "github.com/x/y"}}`)

	res, err := Load(LoadOptions{CWD: dir, EnvConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path == "" {
		t.Fatal("expected file to be loaded from env path")
	}
}

func TestLoad_DiscoveryCwd(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tyche.json"), `{"version": 1, "client": {"module": "github.com/x/y"}}`)

	res, err := Load(LoadOptions{CWD: dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path == "" {
		t.Fatal("expected discovery to find tyche.json in cwd")
	}
}

func TestLoad_DiscoveryWalksUp(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "tyche.json"), `{"version": 1, "client": {"module": "github.com/x/y"}}`)
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Load(LoadOptions{CWD: sub})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path == "" {
		t.Fatal("expected discovery to walk up and find tyche.json")
	}
	if filepath.Dir(res.Path) != root {
		t.Errorf("expected root, got %s", filepath.Dir(res.Path))
	}
}

func TestLoad_StopsAtGoMod(t *testing.T) {
	// Build a workspace:
	//   workspace/                    (no go.mod, no typhi.json)
	//   workspace/project/go.mod      (boundary)
	//   workspace/tyche.json          (must not be discovered)
	parent := t.TempDir()
	project := filepath.Join(parent, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(parent, "tyche.json"), `{"version": 1, "client": {"module": "github.com/x/y"}}`)
	writeFile(t, filepath.Join(project, "go.mod"), "module example.com/foo\n")

	res, err := Load(LoadOptions{CWD: project})
	if err != nil {
		t.Fatal(err)
	}
	if res != nil && res.File != nil {
		t.Errorf("expected no file beyond go.mod boundary, got %+v", res)
	}
}

func TestLoad_BothFilesInSameDir_Error(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tyche.json"), `{"version": 1, "client": {"module": "github.com/x/y"}}`)
	writeFile(t, filepath.Join(dir, "tyche.config.json"), `{"version": 1, "client": {"module": "github.com/x/y"}}`)

	_, err := Load(LoadOptions{CWD: dir})
	if err == nil || !strings.Contains(err.Error(), "found both") {
		t.Fatalf("expected both-files error, got %v", err)
	}
}

func TestLoad_NoFile_NoError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/foo\n")

	res, err := Load(LoadOptions{CWD: dir})
	if err != nil {
		t.Fatalf("expected nil error when no config exists, got %v", err)
	}
	if res != nil && res.File != nil {
		t.Fatalf("expected nil file, got %+v", res)
	}
}

func TestLoad_ConfigAlias(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tyche.config.json"), `{"version": 1, "client": {"module": "github.com/x/y"}}`)

	res, err := Load(LoadOptions{CWD: dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path == "" {
		t.Fatal("expected tyche.config.json to be discovered")
	}
}

func TestLoad_READMEPreserved(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tyche.json"), `{"_README": "hello", "version": 1, "client": {"module": "github.com/x/y"}}`)

	res, err := Load(LoadOptions{CWD: dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.README != "hello" {
		t.Errorf("expected README 'hello', got %q", res.README)
	}
}

func TestApplyClient_MergesZero(t *testing.T) {
	// Empty block leaves opts untouched.
	opts := defaultClientOpts()
	(&ClientBlock{}).ApplyClient(&opts)
	if !equalClientOpts(opts, defaultClientOpts()) {
		t.Errorf("empty block should leave opts untouched, got %+v", opts)
	}
}

func TestApplyClient_OverridesFields(t *testing.T) {
	opts := defaultClientOpts()
	(&ClientBlock{
		Module:     "github.com/x/y",
		Package:    "clientpkg",
		Go:         "1.21",
		ClientName: "APIClient",
		TypeNaming: "operation-scoped",
	}).ApplyClient(&opts)
	if opts.Module != "github.com/x/y" {
		t.Errorf("module: %q", opts.Module)
	}
	if opts.Package != "clientpkg" {
		t.Errorf("package: %q", opts.Package)
	}
	if opts.GoVersion != "1.21" {
		t.Errorf("go: %q", opts.GoVersion)
	}
	if opts.ClientName != "APIClient" {
		t.Errorf("client_name: %q", opts.ClientName)
	}
	if opts.TypeNamingStrategy != 1 {
		t.Errorf("type_naming strategy: %d", opts.TypeNamingStrategy)
	}
}

func TestApplyServer_PreservesDefaults(t *testing.T) {
	def := []string{"./..."}
	patterns, ignore := (*ServerBlock)(nil).ApplyServer(def)
	if len(patterns) != 1 || patterns[0] != "./..." {
		t.Errorf("default patterns lost: %v", patterns)
	}
	if len(ignore) != 0 {
		t.Errorf("expected no ignore, got %v", ignore)
	}
}

func TestApplyServer_OverridesPatterns(t *testing.T) {
	patterns, _ := (&ServerBlock{Patterns: []string{"./api/..."}}).ApplyServer([]string{"./..."})
	if len(patterns) != 1 || patterns[0] != "./api/..." {
		t.Errorf("file patterns not honoured: %v", patterns)
	}
}

func TestApplyServer_AppendsIgnore(t *testing.T) {
	_, ignore := (&ServerBlock{Ignore: []string{"./tmp", "./bin"}}).ApplyServer(nil)
	if len(ignore) != 2 || ignore[0] != "./tmp" || ignore[1] != "./bin" {
		t.Errorf("ignore lost: %v", ignore)
	}
}

func TestOffsetToLineCol(t *testing.T) {
	// Data layout (0-indexed bytes, 1-indexed line/col):
	//   offset  0: '{'  -> line 1 col 1
	//   offset  1: '\n' -> line 1 col 2 (end of line 1)
	//   offset  2: ' '  -> line 2 col 1
	//   offset  3: ' '  -> line 2 col 2
	//   offset  4: '"'  -> line 2 col 3
	//   offset 12: ' '  -> line 3 col 1
	//   offset 14: '"'  -> line 3 col 3
	data := []byte("{\n  \"a\": 1,\n  \"b\":")
	tests := []struct {
		off       int64
		line, col int
	}{
		{0, 1, 1},
		{1, 1, 2},
		{2, 2, 1},
		{3, 2, 2},
		{4, 2, 3},
		{12, 3, 1},
		{14, 3, 3},
	}
	for _, tt := range tests {
		line, col := offsetToLineCol(data, tt.off)
		if line != tt.line || col != tt.col {
			t.Errorf("offset %d: got (%d,%d), want (%d,%d)", tt.off, line, col, tt.line, tt.col)
		}
	}
}

func TestWrapSyntaxError_OtherErrorUnchanged(t *testing.T) {
	other := errors.New("nope")
	got := wrapSyntaxError([]byte("{}"), other)
	if got != other {
		t.Errorf("expected unchanged error, got %v", got)
	}
}
