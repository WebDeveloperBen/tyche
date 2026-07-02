package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootCommand_ShowsHelpByDefault(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected help, got error: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "Usage:") {
		t.Fatalf("expected usage text, got %q", text)
	}
	if !strings.Contains(text, "generate") || !strings.Contains(text, "build") {
		t.Fatalf("expected subcommands in help, got %q", text)
	}
}

func TestBuildCommand_RequiresPackageArgAndOutput(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"build"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected missing package arg to fail")
	}

	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"build", "./cmd/api"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "output path is required") {
		t.Fatalf("expected missing output error, got %v", err)
	}
}

func TestDefaultGenerationPatterns_DefaultsToWholeProject(t *testing.T) {
	patterns := defaultGenerationPatterns(nil, "./cmd/api")
	if len(patterns) != 1 || patterns[0] != "./..." {
		t.Fatalf("expected whole-project default, got %#v", patterns)
	}
}

func TestBuildCommand_LeavesNoGeneratedFilesInRealTree(t *testing.T) {
	root := t.TempDir()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("failed to resolve repo root: %v", err)
	}

	write := func(rel, body string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	write("go.mod", "module fixture\n\ngo 1.25.5\n\nrequire github.com/webdeveloperben/tyche v0.0.0\n\nreplace github.com/webdeveloperben/tyche => "+repoRoot+"\n")
	write("internal/routes/routes.go", `package routes

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
		Method: http.MethodGet,
		Path: "/ok",
	}, func(ctx context.Context, in *input) (*output, error) {
		out := &output{}
		out.Body.OK = true
		return out, nil
	})
}
`)
	write("cmd/app/main.go", `package main

import (
	"github.com/webdeveloperben/tyche/server"
	"fixture/internal/routes"
)

func main() {
	router := server.NewAPI(server.NewServeMuxAdapter())
	routes.Register(router)
}
`)

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"build", "--root", root, "-o", "./bin/app", "./cmd/app"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected build to succeed, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "bin", "app")); err != nil {
		t.Fatalf("expected built binary, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "internal", "routes", "zz_server_routes_gen.go")); !os.IsNotExist(err) {
		t.Fatalf("expected no persisted generated file in real tree, got %v", err)
	}
}

func TestInstalledBinary_BuildsFixtureProject(t *testing.T) {
	root := t.TempDir()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("failed to resolve repo root: %v", err)
	}

	write := func(rel, body string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	write("go.mod", "module fixturebin\n\ngo 1.25.5\n\nrequire github.com/webdeveloperben/tyche v0.0.0\n\nreplace github.com/webdeveloperben/tyche => "+repoRoot+"\n")
	write("internal/routes/routes.go", `package routes

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
		OperationID: "fixture-binary",
		Method: http.MethodGet,
		Path: "/ok",
	}, func(ctx context.Context, in *input) (*output, error) {
		out := &output{}
		out.Body.OK = true
		return out, nil
	})
}
`)
	write("cmd/app/main.go", `package main

import (
	"github.com/webdeveloperben/tyche/server"
	"fixturebin/internal/routes"
)

func main() {
	router := server.NewAPI(server.NewServeMuxAdapter())
	routes.Register(router)
}
`)

	binPath := filepath.Join(t.TempDir(), "servergen")
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/servergen")
	buildCmd.Dir = repoRoot
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build servergen binary: %v\n%s", err, string(buildOutput))
	}

	runCmd := exec.Command(binPath, "build", "--root", root, "-o", "./bin/app", "./cmd/app")
	runCmd.Dir = repoRoot
	runOutput, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected installed binary build to succeed: %v\n%s", err, string(runOutput))
	}

	if _, err := os.Stat(filepath.Join(root, "bin", "app")); err != nil {
		t.Fatalf("expected built binary, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "internal", "routes", "zz_server_routes_gen.go")); !os.IsNotExist(err) {
		t.Fatalf("expected no persisted generated file in real tree, got %v", err)
	}
}
