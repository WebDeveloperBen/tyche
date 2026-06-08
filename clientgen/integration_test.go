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
)

type intgThing struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type intgCreateInput struct {
	Tenant string `path:"tenant"`
	Body   struct {
		Name string `json:"name" validate:"required"`
	} `body:"true"`
}

type intgCreateOutput struct {
	Body intgThing `body:"true"`
}

type intgStreamInput struct {
	Topic string `query:"topic" required:"true"`
}

type intgStreamEvent struct {
	Message string `json:"message"`
}

// Register a reflection-based codec inline so this test does not depend on
// servergen's generated codec file (which is gitignored and absent on a fresh
// CI clone). The codec is never invoked — it only needs to exist so
// server.Register succeeds and produces a real OpenAPI document.
func init() {
	server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
		OperationID:       "intg-create",
		Method:            http.MethodPost,
		Path:              "/tenants/:tenant/things",
		HasGeneratedCodec: true,
	}, server.GeneratedRouteCodec{
		Parse: func(req *http.Request) (any, error) { return server.ParseRequest[intgCreateInput](req) },
		Write: func(w http.ResponseWriter, req *http.Request, out any) error {
			return server.WriteTypedResponse(w, out.(*intgCreateOutput))
		},
	})
}

// TestGenerate_FromRealServerSpec builds a real tyche router (a typed
// data-envelope operation plus a Server-Sent Events operation), marshals the
// OpenAPI document it produces, generates a client from that exact spec, and
// compiles it — validating clientgen against actual server output (data
// envelope, inlined schemas, component naming, text/event-stream).
func TestGenerate_FromRealServerSpec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile-based integration test in -short mode")
	}

	router := server.NewRouter()
	api := router.Group("/v1")

	server.Register(api, server.Operation{
		OperationID: "intg-create",
		Method:      http.MethodPost,
		Path:        "/tenants/:tenant/things",
	}, func(ctx context.Context, in *intgCreateInput) (*intgCreateOutput, error) {
		return &intgCreateOutput{}, nil
	})

	server.RegisterStream(api, server.Operation{
		OperationID: "intg-stream",
		Method:      http.MethodGet,
		Path:        "/messages/stream",
	}, func(ctx context.Context, in *intgStreamInput, s *server.Stream[intgStreamEvent]) error {
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

	if len(res.Files) < 4 {
		t.Errorf("expected go.mod, client.go, operations.go, types.go, stream.go; got %d files", len(res.Files))
	}
}
