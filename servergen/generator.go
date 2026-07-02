package servergen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/webdeveloperben/tyche/server/validation"
	"golang.org/x/tools/go/packages"
)

const GeneratedFilename = "zz_server_routes_gen.go"

type RouteSpec struct {
	PackageName      string
	PackagePath      string
	Dir              string
	ServerImportPath string
	OperationID      string
	Method           string
	Path             string
	InputType        string
	OutputType       string
	InputBind        InputBindSpec
	OutputWrite      OutputWriteSpec
}

type BindFieldSpec struct {
	FieldName string
	ParamName string
	Source    string
	TypeExpr  string
	Kind      string
	Pointer   bool
	Required  bool
	Rules     validation.FieldRules
}

type InputBindSpec struct {
	Manual bool
	Fields []BindFieldSpec
	Body   *BodyBindSpec
}

type BodyFieldSpec struct {
	FieldName     string
	JSONName      string
	TypeExpr      string
	Kind          string
	Pointer       bool
	Slice         bool
	Opaque        bool
	Required      bool
	Rules         validation.FieldRules
	Nested        *BodyBindSpec
	NestedType    string
	NestedPtr     bool
	ElemType      string
	ElemKind      string
	ElemPtr       bool
	ElemNested    *BodyBindSpec
	ElemStruct    string
	ElemStructPtr bool
}

type BodyBindSpec struct {
	Required     bool
	Target       string
	DecodeTarget string
	Fields       []BodyFieldSpec
	Direct       *BodyFieldSpec
}

type HeaderFieldSpec struct {
	FieldName string
	Header    string
	TypeExpr  string
	Kind      string
	Pointer   bool
	Required  bool
}

type OutputBodyFieldSpec struct {
	FieldName string
	JSONName  string
	TypeExpr  string
	Kind      string
	Pointer   bool
}

type OutputBodySpec struct {
	TargetExpr      string
	Fields          []OutputBodyFieldSpec
	HasSimpleStatus bool
}

type OutputWriteSpec struct {
	Manual        bool
	BodyFieldName string
	BodyTypeExpr  string
	StatusField   string
	StaticStatus  int
	Headers       []HeaderFieldSpec
	Body          *OutputBodySpec
}

func LoadRoutes(patterns []string) ([]RouteSpec, error) {
	return LoadRoutesInDir("", patterns)
}

func LoadRoutesInDir(dir string, patterns []string) ([]RouteSpec, error) {
	routes, _, err := loadRoutesAndPackages(dir, patterns)
	return routes, err
}

func WriteGeneratedFiles(dir string, patterns []string) error {
	routes, pkgs, err := loadRoutesAndPackages(dir, patterns)
	if err != nil {
		return err
	}

	grouped := GroupRoutesByPackage(routes)
	desiredOutputs := make(map[string]struct{})
	loadedPackageDirs := make(map[string]struct{})
	for _, pkg := range pkgs {
		if pkgDir, ok := packageDir(pkg); ok {
			loadedPackageDirs[pkgDir] = struct{}{}
		}
	}
	for pkgPath, pkgRoutes := range grouped {
		pkgDir, ok := FindPackageDir(pkgRoutes, pkgPath)
		if !ok {
			return fmt.Errorf("package dir not found for %s", pkgPath)
		}

		content, err := GeneratePackageManifest(pkgPath, pkgRoutes)
		if err != nil {
			return fmt.Errorf("generate %s: %w", pkgPath, err)
		}
		if len(content) == 0 {
			continue
		}

		outputPath := filepath.Join(pkgDir, GeneratedFilename)
		desiredOutputs[outputPath] = struct{}{}
		if err := os.WriteFile(outputPath, content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", outputPath, err)
		}
	}

	if patternsCoverWholeTree(patterns) {
		if err := CleanupGeneratedFiles(dir, patterns, desiredOutputs); err != nil {
			return err
		}
		return nil
	}

	for pkgDir := range loadedPackageDirs {
		outputPath := filepath.Join(pkgDir, GeneratedFilename)
		if _, hasRoutes := grouped[packagePathForDir(pkgs, pkgDir)]; hasRoutes {
			continue
		}
		if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale %s: %w", outputPath, err)
		}
	}

	return nil
}

func CleanupGeneratedFiles(rootDir string, patterns []string, keep map[string]struct{}) error {
	_, pkgs, err := loadRoutesAndPackages(rootDir, patterns)
	if err != nil {
		return err
	}

	for _, pkg := range pkgs {
		pkgDir, ok := packageDir(pkg)
		if !ok {
			continue
		}
		path := filepath.Join(pkgDir, GeneratedFilename)
		if _, ok := keep[path]; ok {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale %s: %w", path, err)
		}
	}
	return nil
}

func patternsCoverWholeTree(patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == "./..." || pattern == "." {
			return true
		}
	}
	return false
}

func loadRoutesAndPackages(dir string, patterns []string) ([]RouteSpec, []*packages.Package, error) {
	env, err := packageLoadEnv()
	if err != nil {
		return nil, nil, err
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  dir,
		Env:  env,
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, nil, err
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, nil, fmt.Errorf("failed to load packages")
	}

	var routes []RouteSpec
	for _, pkg := range pkgs {
		pkgRoutes, err := loadPackageRoutes(pkg)
		if err != nil {
			return nil, nil, err
		}
		routes = append(routes, pkgRoutes...)
	}
	return routes, pkgs, nil
}

func packageLoadEnv() ([]string, error) {
	baseEnv := os.Environ()
	goModCache, ok := os.LookupEnv("GOMODCACHE")
	if !ok || goModCache == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		goModCache = filepath.Join(cacheDir, "go-mod")
	}
	env := append([]string(nil), baseEnv...)
	env = append(env, "GOWORK=off", "GOMODCACHE="+goModCache)
	return env, nil
}

func packagePathForDir(pkgs []*packages.Package, dir string) string {
	for _, pkg := range pkgs {
		pkgDir, ok := packageDir(pkg)
		if ok && pkgDir == dir {
			return pkg.PkgPath
		}
	}
	return ""
}

func packageDir(pkg *packages.Package) (string, bool) {
	if pkg == nil || len(pkg.GoFiles) == 0 {
		return "", false
	}
	return filepath.Dir(pkg.GoFiles[0]), true
}

func loadPackageRoutes(pkg *packages.Package) ([]RouteSpec, error) {
	routes := make([]RouteSpec, 0)
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			serverImportPath, ok := serverRegisterImportPath(call.Fun, pkg.TypesInfo)
			if !ok {
				return true
			}

			route, ok, err := routeSpecFromCall(pkg, call, serverImportPath)
			if err != nil {
				routes = nil
				panic(err)
			}
			if ok {
				routes = append(routes, route)
			}
			return true
		})
	}
	// Typed routes registered in package main cannot use generated codecs: Go's
	// runtime reports "main" as the reflect PkgPath for main-package types,
	// while servergen keys codecs by the full import path, so the two never
	// match and Register would panic at runtime. Fail loudly at generate time
	// with an actionable message instead.
	if len(routes) > 0 && pkg.Name == "main" {
		return nil, fmt.Errorf(
			"package main (%s) registers typed routes (e.g. %s %s): move the route registrations and their input/output types into a non-main package — Go reports \"main\" as the reflect PkgPath for main-package types, so a generated codec's type keys can never match and Register fails at runtime",
			pkg.PkgPath, routes[0].Method, routes[0].Path,
		)
	}
	return routes, nil
}

func routeSpecFromCall(pkg *packages.Package, call *ast.CallExpr, serverImportPath string) (RouteSpec, bool, error) {
	if len(call.Args) != 3 {
		return RouteSpec{}, false, nil
	}

	opLit, ok := call.Args[1].(*ast.CompositeLit)
	if !ok {
		return RouteSpec{}, false, nil
	}

	spec := RouteSpec{
		PackageName:      pkg.Name,
		PackagePath:      pkg.PkgPath,
		ServerImportPath: serverImportPath,
	}
	if pkgDir, ok := packageDir(pkg); ok {
		spec.Dir = pkgDir
	}

	for _, elt := range opLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "OperationID":
			spec.OperationID, _ = literalString(kv.Value, pkg.TypesInfo)
		case "Method":
			spec.Method, _ = literalString(kv.Value, pkg.TypesInfo)
		case "Path":
			spec.Path, _ = literalString(kv.Value, pkg.TypesInfo)
		}
	}

	tv, ok := pkg.TypesInfo.Types[call.Args[2]]
	if !ok {
		return RouteSpec{}, false, nil
	}
	sig, ok := tv.Type.(*types.Signature)
	if !ok || sig.Params().Len() != 2 || sig.Results().Len() != 2 {
		return RouteSpec{}, false, nil
	}

	inputPtr, ok := sig.Params().At(1).Type().(*types.Pointer)
	if !ok {
		return RouteSpec{}, false, nil
	}
	outputPtr, ok := sig.Results().At(0).Type().(*types.Pointer)
	if !ok {
		return RouteSpec{}, false, nil
	}

	spec.InputType = types.TypeString(inputPtr.Elem(), qualifierFor(pkg.Types))
	spec.OutputType = types.TypeString(outputPtr.Elem(), qualifierFor(pkg.Types))
	spec.InputBind = analyseInputType(inputPtr.Elem())
	spec.OutputWrite = analyseOutputType(outputPtr.Elem())

	if spec.Method == "" || spec.Path == "" {
		return RouteSpec{}, false, fmt.Errorf("route %s is missing method or path", spec.OperationID)
	}

	return spec, true, nil
}

func GeneratePackageManifest(pkgPath string, routes []RouteSpec) ([]byte, error) {
	if len(routes) == 0 {
		return nil, nil
	}

	pkgName := routes[0].PackageName
	var buf bytes.Buffer
	useEncodingJSON := false
	useErrors := false
	useBytes := false
	useFmt := false
	useIO := false
	useNetMail := false
	useNetHTTP := true
	useNetURL := false
	useRegexp := false
	useStrconv := false
	for _, route := range routes {
		if route.InputBind.Manual {
			for _, field := range route.InputBind.Fields {
				if field.Source == "cookie" {
					useErrors = true
				}
				if field.Kind != "string" {
					useFmt = true
					useStrconv = true
				}
				if field.Required {
					useFmt = true
				}
				markValidationImports(field.Rules, &useFmt, &useRegexp, &useNetMail, &useNetURL)
			}
			if route.InputBind.Body != nil {
				useEncodingJSON = true
				useFmt = true
				if route.InputBind.Body.Direct == nil {
					useErrors = true
					useIO = true
				}
				markGeneratedBodyImports(route.InputBind.Body, &useFmt, &useRegexp, &useNetMail, &useNetURL)
				if generatedBodyNeedsStrconv(route.InputBind.Body) {
					useStrconv = true
				}
			}
		} else {
			useFmt = true
		}

		if route.OutputWrite.Manual {
			if route.OutputWrite.StatusField != "" || len(route.OutputWrite.Headers) > 0 || route.OutputWrite.BodyFieldName != "" {
				useFmt = true
			}
			if route.OutputWrite.Body != nil {
				useStrconv = true
			}
		} else {
			useFmt = true
		}
	}

	buf.WriteString("// Code generated by servergen. DO NOT EDIT.\n\n")
	buf.WriteString("package " + pkgName + "\n\n")
	buf.WriteString("import (\n")
	if useBytes {
		buf.WriteString("\t\"bytes\"\n")
	}
	if useEncodingJSON {
		buf.WriteString("\t\"encoding/json\"\n")
	}
	if useErrors {
		buf.WriteString("\t\"errors\"\n")
	}
	if useFmt {
		buf.WriteString("\t\"fmt\"\n")
	}
	if useIO {
		buf.WriteString("\t\"io\"\n")
	}
	if useNetMail {
		buf.WriteString("\t\"net/mail\"\n")
	}
	if useNetHTTP {
		buf.WriteString("\t\"net/http\"\n")
	}
	if useNetURL {
		buf.WriteString("\t\"net/url\"\n")
	}
	if useRegexp {
		buf.WriteString("\t\"regexp\"\n")
	}
	if useStrconv {
		buf.WriteString("\t\"strconv\"\n")
	}
	buf.WriteString("\tserverpkg " + strconv.Quote(routes[0].ServerImportPath) + "\n")
	buf.WriteString(")\n\n")
	writePatternVars(&buf, routes)
	buf.WriteString("func init() {\n")
	buf.WriteString("\tserverpkg.RegisterGeneratedManifest(\n")
	for _, route := range routes {
		hasGeneratedCodec := route.InputBind.Manual && route.OutputWrite.Manual
		buf.WriteString("\t\tserverpkg.GeneratedRouteMeta{\n")
		buf.WriteString("\t\t\tPackagePath: " + strconv.Quote(route.PackagePath) + ",\n")
		buf.WriteString("\t\t\tOperationID: " + strconv.Quote(route.OperationID) + ",\n")
		buf.WriteString("\t\t\tMethod: " + strconv.Quote(route.Method) + ",\n")
		buf.WriteString("\t\t\tPath: " + strconv.Quote(route.Path) + ",\n")
		buf.WriteString("\t\t\tInputType: " + strconv.Quote(route.InputType) + ",\n")
		buf.WriteString("\t\t\tOutputType: " + strconv.Quote(route.OutputType) + ",\n")
		buf.WriteString("\t\t\tInputTypeKey: " + strconv.Quote(route.PackagePath+"."+route.InputType) + ",\n")
		buf.WriteString("\t\t\tOutputTypeKey: " + strconv.Quote(route.PackagePath+"."+route.OutputType) + ",\n")
		buf.WriteString("\t\t\tHasGeneratedCodec: " + strconv.FormatBool(hasGeneratedCodec) + ",\n")
		buf.WriteString("\t\t},\n")
	}
	buf.WriteString("\t)\n")
	for _, route := range routes {
		if !route.InputBind.Manual || !route.OutputWrite.Manual {
			continue
		}
		buf.WriteString("\tserverpkg.RegisterGeneratedCodec(serverpkg.GeneratedRouteMeta{\n")
		buf.WriteString("\t\tPackagePath: " + strconv.Quote(route.PackagePath) + ",\n")
		buf.WriteString("\t\tOperationID: " + strconv.Quote(route.OperationID) + ",\n")
		buf.WriteString("\t\tMethod: " + strconv.Quote(route.Method) + ",\n")
		buf.WriteString("\t\tPath: " + strconv.Quote(route.Path) + ",\n")
		buf.WriteString("\t\tInputType: " + strconv.Quote(route.InputType) + ",\n")
		buf.WriteString("\t\tOutputType: " + strconv.Quote(route.OutputType) + ",\n")
		buf.WriteString("\t\tInputTypeKey: " + strconv.Quote(route.PackagePath+"."+route.InputType) + ",\n")
		buf.WriteString("\t\tOutputTypeKey: " + strconv.Quote(route.PackagePath+"."+route.OutputType) + ",\n")
		buf.WriteString("\t\tHasGeneratedCodec: true,\n")
		buf.WriteString("\t}, serverpkg.GeneratedRouteCodec{\n")
		buf.WriteString("\t\tParse: func(req *http.Request) (any, error) {\n")
		writeParseBody(&buf, route)
		buf.WriteString("\t\t},\n")
		buf.WriteString("\t\tWrite: func(w http.ResponseWriter, req *http.Request, value any) error {\n")
		writeWriteBody(&buf, route)
		buf.WriteString("\t\t},\n")
		buf.WriteString("\t})\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}

func GroupRoutesByPackage(routes []RouteSpec) map[string][]RouteSpec {
	grouped := make(map[string][]RouteSpec)
	for _, route := range routes {
		grouped[route.PackagePath] = append(grouped[route.PackagePath], route)
	}
	for pkgPath := range grouped {
		sort.Slice(grouped[pkgPath], func(i, j int) bool {
			return grouped[pkgPath][i].OperationID < grouped[pkgPath][j].OperationID
		})
	}
	return grouped
}

func literalString(expr ast.Expr, info *types.Info) (string, bool) {
	if info != nil {
		if tv, ok := info.Types[expr]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
			return constant.StringVal(tv.Value), true
		}
	}
	basic, ok := expr.(*ast.BasicLit)
	if !ok || basic.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(basic.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

func serverRegisterImportPath(fun ast.Expr, info *types.Info) (string, bool) {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Register" {
		return "", false
	}
	if info == nil {
		return "", false
	}
	selection := info.Selections[sel]
	if selection != nil {
		return "", false
	}
	pkgName, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	obj := info.Uses[pkgName]
	if obj == nil {
		return "", false
	}
	if imported, ok := obj.(*types.PkgName); ok && imported.Imported() != nil {
		return imported.Imported().Path(), true
	}
	if obj.Pkg() == nil {
		return "", false
	}
	return obj.Pkg().Path(), true
}

func qualifierFor(current *types.Package) types.Qualifier {
	return func(other *types.Package) string {
		if other == nil {
			return ""
		}
		if current != nil && other.Path() == current.Path() {
			return ""
		}
		return other.Name()
	}
}

func FindPackageDir(routes []RouteSpec, pkgPath string) (string, bool) {
	for _, route := range routes {
		if route.PackagePath == pkgPath && route.Dir != "" {
			return route.Dir, true
		}
	}
	return "", false
}
