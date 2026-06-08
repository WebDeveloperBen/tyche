package clientgen_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/webdeveloperben/tyche/clientgen"
	"github.com/webdeveloperben/tyche/server"
	samplepkg "github.com/webdeveloperben/tyche/servergen/testdata/samplepkg"
)

type streamInput struct {
	Topic string `query:"topic" required:"true"`
}

type streamEvent struct {
	Message string `json:"message"`
}

// TestGenerate_FromRealServerSpec builds a real tyche router, marshals the
// OpenAPI document it produces, generates a client from that exact spec, and
// compiles it — validating clientgen against actual server output (data
// envelope, inlined schemas, component naming, body/array shapes).
func TestGenerate_FromRealServerSpec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile-based integration test in -short mode")
	}

	router := server.NewRouter()
	grp := router.Group("/api")
	samplepkg.RegisterTypedRoutes(grp)

	// A real Server-Sent Events operation, so the generated client exercises
	// the streaming path against genuine text/event-stream OpenAPI output.
	server.RegisterStream(grp, server.Operation{
		OperationID: "stream-messages",
		Method:      http.MethodGet,
		Path:        "/messages/stream",
	}, func(ctx context.Context, in *streamInput, s *server.Stream[streamEvent]) error {
		return nil
	})

	specJSON, err := json.Marshal(router.OpenAPI())
	if err != nil {
		t.Fatalf("marshal OpenAPI: %v", err)
	}

	doc, err := clientgen.ParseDocument(specJSON)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	res, err := clientgen.Generate(doc, clientgen.Options{Module: "example.com/sample/client"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	dir := t.TempDir()
	for _, f := range res.Files {
		if err := os.WriteFile(filepath.Join(dir, f.Name), f.Content, 0o644); err != nil {
			t.Fatalf("write %s: %v", f.Name, err)
		}
	}

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated client from real spec failed to compile: %v\n%s\n--- operations.go ---\n%s", err, out, mustFile(res, "operations.go"))
	}

	if len(res.Files) < 3 {
		t.Errorf("expected at least go.mod, client.go, types.go; got %d files", len(res.Files))
	}
}
