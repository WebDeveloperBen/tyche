package servergen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webdeveloperben/tyche/servergen"
)

func TestLoadRoutes(t *testing.T) {
	routes, err := servergen.LoadRoutes([]string{"./testdata/samplepkg"})
	if err != nil {
		t.Fatalf("LoadRoutes failed: %v", err)
	}

	if len(routes) != 12 {
		t.Fatalf("expected 12 routes, got %d", len(routes))
	}

	var route, bodyRoute, bulkRoute, unsupportedRoute, flatRoute, uploadRoute servergen.RouteSpec
	for _, candidate := range routes {
		switch candidate.OperationID {
		case "get-thing":
			route = candidate
		case "create-thing":
			bodyRoute = candidate
		case "bulk-create-thing":
			bulkRoute = candidate
		case "unsupported-thing":
			unsupportedRoute = candidate
		case "flat-thing":
			flatRoute = candidate
		case "upload-thing":
			uploadRoute = candidate
		}
	}
	if route.OperationID != "get-thing" {
		t.Fatalf("expected get-thing route, got %#v", routes)
	}
	if route.Method != "GET" {
		t.Fatalf("expected GET, got %q", route.Method)
	}
	if route.Path != "/things/:id" {
		t.Fatalf("expected path /things/:id, got %q", route.Path)
	}
	if route.InputType != "GetThingRequest" {
		t.Fatalf("expected input type GetThingRequest, got %q", route.InputType)
	}
	if route.OutputType != "GetThingResponse" {
		t.Fatalf("expected output type GetThingResponse, got %q", route.OutputType)
	}
	if route.ServerImportPath != "github.com/webdeveloperben/tyche/server" {
		t.Fatalf("expected server import path to be detected, got %q", route.ServerImportPath)
	}
	if bodyRoute.OperationID != "create-thing" {
		t.Fatalf("expected create-thing route, got %#v", routes)
	}
	if bodyRoute.InputBind.Body == nil {
		t.Fatal("expected generated body binding for create-thing")
	}
	if len(bodyRoute.InputBind.Body.Fields) != 5 {
		t.Fatalf("expected 5 generated body fields, got %d", len(bodyRoute.InputBind.Body.Fields))
	}
	var foundNested bool
	var foundAliases bool
	var foundChildren bool
	for _, field := range bodyRoute.InputBind.Body.Fields {
		switch field.FieldName {
		case "Meta":
			foundNested = field.Nested != nil && len(field.Nested.Fields) == 1
		case "Aliases":
			foundAliases = field.Slice && field.ElemKind == "string"
		case "Children":
			foundChildren = field.Slice && field.ElemNested != nil && len(field.ElemNested.Fields) == 1
		}
	}
	if !foundNested {
		t.Fatalf("expected nested generated body binding for meta, got %#v", bodyRoute.InputBind.Body.Fields)
	}
	if !foundAliases {
		t.Fatalf("expected generated slice body binding for aliases, got %#v", bodyRoute.InputBind.Body.Fields)
	}
	if !foundChildren {
		t.Fatalf("expected generated nested slice body binding for children, got %#v", bodyRoute.InputBind.Body.Fields)
	}
	if bulkRoute.OperationID != "bulk-create-thing" {
		t.Fatalf("expected bulk-create-thing route, got %#v", routes)
	}
	if bulkRoute.InputBind.Body == nil || bulkRoute.InputBind.Body.Direct == nil || !bulkRoute.InputBind.Body.Direct.Slice || bulkRoute.InputBind.Body.Direct.ElemNested == nil {
		t.Fatalf("expected direct generated slice body binding for bulk route, got %#v", bulkRoute.InputBind.Body)
	}
	if unsupportedRoute.OperationID != "unsupported-thing" {
		t.Fatalf("expected unsupported-thing route, got %#v", routes)
	}
	if unsupportedRoute.InputBind.Manual {
		t.Fatalf("expected unsupported route input binding to require runtime fallback, got %#v", unsupportedRoute.InputBind)
	}
	if flatRoute.OperationID != "flat-thing" {
		t.Fatalf("expected flat-thing route, got %#v", routes)
	}
	if flatRoute.InputBind.Body == nil || len(flatRoute.InputBind.Body.Fields) != 4 {
		t.Fatalf("expected flat generated body binding, got %#v", flatRoute.InputBind.Body)
	}
	if uploadRoute.OperationID != "upload-thing" {
		t.Fatalf("expected upload-thing route, got %#v", routes)
	}
	if uploadRoute.InputBind.Manual {
		t.Fatalf("expected multipart route input binding to require runtime fallback, got %#v", uploadRoute.InputBind)
	}
}

func TestGeneratePackageManifest_MainPackageKeys(t *testing.T) {
	// Typed routes in package main are supported: the generated codec keys must
	// use "main" (what reflect reports at runtime), not the import path, so they
	// match at runtime. This is what makes single-file main.go apps work.
	routes, err := servergen.LoadRoutes([]string{"./testdata/mainpkg"})
	if err != nil {
		t.Fatalf("LoadRoutes(mainpkg) should succeed, got: %v", err)
	}
	if len(routes) == 0 {
		t.Fatal("expected routes from mainpkg")
	}

	content, err := servergen.GeneratePackageManifest(routes[0].PackagePath, routes)
	if err != nil {
		t.Fatalf("GeneratePackageManifest failed: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		`InputTypeKey: "main.Input"`,
		`OutputTypeKey: "main.Output"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected main-package key %q in generated output:\n%s", want, text)
		}
	}
}

func TestCleanupGeneratedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	keepPath := filepath.Join(tmpDir, "keep", servergen.GeneratedFilename)
	removePath := filepath.Join(tmpDir, "remove", servergen.GeneratedFilename)
	goModPath := filepath.Join(tmpDir, "go.mod")

	if err := os.WriteFile(goModPath, []byte("module cleanupfixture\n\ngo 1.25.5\n"), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}
	for _, path := range []string{keepPath, removePath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		packageFile := filepath.Join(filepath.Dir(path), "package.go")
		if err := os.WriteFile(packageFile, []byte("package fixture\n"), 0o644); err != nil {
			t.Fatalf("failed to write package file: %v", err)
		}
		if err := os.WriteFile(path, []byte("package fixture\n"), 0o644); err != nil {
			t.Fatalf("failed to write generated file: %v", err)
		}
	}

	if err := servergen.CleanupGeneratedFiles(tmpDir, []string{"./..."}, map[string]struct{}{keepPath: {}}); err != nil {
		t.Fatalf("CleanupGeneratedFiles failed: %v", err)
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("expected keep file to remain, got %v", err)
	}
	if _, err := os.Stat(removePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale file to be removed, got %v", err)
	}
}

func TestGeneratePackageManifest(t *testing.T) {
	routes, err := servergen.LoadRoutes([]string{"./testdata/samplepkg"})
	if err != nil {
		t.Fatalf("LoadRoutes failed: %v", err)
	}

	content, err := servergen.GeneratePackageManifest(routes[0].PackagePath, routes)
	if err != nil {
		t.Fatalf("GeneratePackageManifest failed: %v", err)
	}

	text := string(content)
	for _, expected := range []string{
		"Code generated by servergen",
		`serverpkg "github.com/webdeveloperben/tyche/server"`,
		"serverpkg.RegisterGeneratedManifest(",
		"serverpkg.RegisterGeneratedCodec(serverpkg.GeneratedRouteMeta{",
		`raw_ID := req.PathValue("id")`,
		`in.ID = raw_ID`,
		"bufPtr := serverpkg.AcquireGeneratedJSONBuffer()",
		"b = strconv.AppendQuote(b, out.Body.ID)",
		"bodyBytes, err := serverpkg.ReadRequestJSONBodyFast(req)",
		"var decoded struct {",
		`JoinValidationPointer(serverpkg.JoinValidationPointer("", "meta"), "code")`,
		"regexp.MustCompile",
		"mail.ParseAddress",
		"for i_body_Aliases := range in.Body.Aliases",
		"elemtmp_body_Children",
		`if err := json.Unmarshal(bodyBytes, &in.Body); err != nil`,
		`OperationID: "get-thing"`,
		`OperationID: "create-thing"`,
		`OperationID: "bulk-create-thing"`,
		`OperationID: "unsupported-thing"`,
		`OperationID: "flat-thing"`,
		`OperationID: "upload-thing"`,
		`HasGeneratedCodec: false`,
		// Success responses must be wrapped in the {"data": …} envelope that
		// the OpenAPI spec, servertest.DecodeData, and the generated client all
		// expect. The hand-built body path opens the envelope directly...
		`{\"data\":{`,
		// ...and the opaque-body path routes through WriteSuccess (enveloped),
		// never the raw WriteJSON.
		"serverpkg.WriteSuccess(w, status, out.Body)",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected generated output to contain %q\n%s", expected, text)
		}
	}
	if strings.Contains(text, "serverpkg.ParseRequest[GetThingRequest](req)") {
		t.Fatalf("expected sample route to use generated parse path, got fallback\n%s", text)
	}
	// Guard against regressing to an un-enveloped success body: generated
	// codecs must not write responses via the raw WriteJSON helper.
	if strings.Contains(text, "serverpkg.WriteJSON(") {
		t.Fatalf("generated codecs must emit the {\"data\":…} envelope via WriteSuccess, not raw WriteJSON\n%s", text)
	}
	if strings.Contains(text, `RegisterGeneratedCodec(serverpkg.GeneratedRouteMeta{
		PackagePath: "github.com/webdeveloperben/tyche/servergen/testdata/samplepkg",
		OperationID: "unsupported-thing"`) {
		t.Fatalf("expected unsupported route to skip generated codec registration\n%s", text)
	}
	if strings.Contains(text, `RegisterGeneratedCodec(serverpkg.GeneratedRouteMeta{
		PackagePath: "github.com/webdeveloperben/tyche/servergen/testdata/samplepkg",
		OperationID: "upload-thing"`) {
		t.Fatalf("expected multipart route to skip generated codec registration\n%s", text)
	}
}
