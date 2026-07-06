// Package app holds the use-case orchestrators the tyche CLI runs. Every
// function in this package is pure (no CLI types, no Kong flags, no Cobra
// commands) and takes plain Go values, so it can be called from the CLI,
// from a future GUI, or from a programmatic embedding of tyche in another
// tool. The CLI layer in internal/cli is a thin adapter that parses
// arguments and calls into here.
//
// Keeping this layer separate from internal/cli has two payoffs:
//
//  1. The servergen and clientgen libraries never need to know about CLI
//     types, so importing them does not pull Kong into a user's binary.
//  2. Tests for the business logic do not need a CLI framework — they call
//     app.X(...) directly with a tempdir.
package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/webdeveloperben/tyche/clientgen"
	"github.com/webdeveloperben/tyche/internal/config"
	"github.com/webdeveloperben/tyche/servergen"
)

// LoadOptions is the cross-cutting input to most app functions. It captures
// the CLI's discovery state so the app layer does not need to know how the
// caller chose to discover the project root.
type LoadOptions struct {
	InfoCallback func(string)
	Root         string
	ConfigPath   string
	EnvConfig    string
	PrintInfo    bool // whether to emit the "using config ..." banner
}

// ErrNoConfig is re-exported from the config package so CLI handlers can
// distinguish "no config file found" from real load failures without
// importing internal/config directly.
var ErrNoConfig = config.ErrNoConfig

// LoadConfig loads the tyche.json file using the same precedence as the CLI:
// explicit path > env var > discovery. The optional InfoCallback receives
// informational messages ("using config ...", "  <README line>") so the CLI
// layer can route them through its Printer. If no callback is set, the
// messages are dropped — useful for tests and quiet mode. When no config is
// discovered, LoadConfig returns (nil, config.ErrNoConfig).
func LoadConfig(opts LoadOptions) (*config.LoadResult, error) {
	if opts.InfoCallback == nil {
		opts.InfoCallback = func(string) {}
	}
	loaded, err := config.Load(config.LoadOptions{
		ExplicitPath:  opts.ConfigPath,
		EnvConfigPath: opts.EnvConfig,
		CWD:           opts.Root,
	})
	if err != nil {
		return nil, err
	}
	if opts.PrintInfo && loaded.Path != "" {
		opts.InfoCallback(fmt.Sprintf("using config %s", loaded.Path))
	}
	if opts.PrintInfo && loaded.README != "" {
		for line := range strings.SplitSeq(loaded.README, "\n") {
			opts.InfoCallback("  " + line)
		}
	}
	return loaded, nil
}

// ResolveRoot returns the directory to operate in: the explicit --root if
// non-empty, otherwise the current working directory. It is the single place
// that decides "where is the project".
func ResolveRoot(root string) (string, error) {
	if root != "" {
		return root, nil
	}
	return os.Getwd()
}

// ResolvePath joins path to rootDir unless path is absolute.
func ResolvePath(rootDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(rootDir, path)
}

// --- scaffold (tyche init) ----------------------------------------------

// ScaffoldOptions configures Scaffold. Module, Spec, and TypeNaming become
// the values written into tyche.json.
type ScaffoldOptions struct {
	Root       string
	Module     string
	Spec       string
	TypeNaming string
	Force      bool
}

// Scaffold writes a starter tyche.json at Root/tyche.json. The file is
// round-tripped through config.Load so a malformed scaffold is caught
// immediately and removed. The path of the written file is returned.
func Scaffold(opts ScaffoldOptions) (string, error) {
	if opts.Spec == "" {
		opts.Spec = "./api/openapi.json"
	}
	if opts.TypeNaming == "" {
		opts.TypeNaming = "structural"
	}
	if opts.Module == "" {
		return "", errors.New("client module path is required (use --module or answer the prompt)")
	}

	dest := filepath.Join(opts.Root, "tyche.json")
	if _, err := os.Stat(dest); err == nil && !opts.Force {
		return "", fmt.Errorf("%s already exists; pass --force to overwrite", dest)
	}

	content := renderScaffold(opts)
	if err := os.WriteFile(dest, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", dest, err)
	}

	// Round-trip: parse the file we just wrote to catch any validator errors
	// immediately, before the user hits them on the next CLI run.
	if _, err := config.Load(config.LoadOptions{ExplicitPath: dest}); err != nil {
		if rmErr := os.Remove(dest); rmErr != nil {
			return "", errors.Join(
				fmt.Errorf("scaffolded file failed validation: %w", err),
				fmt.Errorf("could not remove the malformed %s: %w", dest, rmErr),
			)
		}
		return "", fmt.Errorf("scaffolded file failed validation: %w", err)
	}
	return dest, nil
}

func renderScaffold(opts ScaffoldOptions) string {
	return fmt.Sprintf(`{
  "_README": "tyche config. Run `+"`tyche generate`"+` to regenerate server codecs and `+"`tyche client`"+` to regenerate the typed client. Flags always override file values; pass --config to point at a different file.",
  "version": 1,
  "spec": %q,
  "client": {
    "out": "./client",
    "module": %q,
    "type_naming": %q
  },
  "server": {
    "patterns": ["./..."],
    "ignore": ["./tmp", "./bin", "./.git"]
  }
}
`, opts.Spec, opts.Module, opts.TypeNaming)
}

// --- config show ---------------------------------------------------------

// ConfigShowResult is the rendered shape of `tyche config show`. The CLI
// layer formats this through its Printer; --json emits it as JSON. The
// nested blocks are typed (not map[string]any) so the JSON schema is
// statically guaranteed and a typo can't silently drop a field.
type ConfigShowResult struct {
	Client  *ConfigClient `json:"client,omitempty"`
	Path    string        `json:"path,omitempty"`
	Server  *ConfigServer `json:"server,omitempty"`
	Spec    string        `json:"spec,omitempty"`
	Version int           `json:"version"`
}

// ConfigClient mirrors config.ClientBlock for `config show` output.
type ConfigClient struct {
	Out        string `json:"out,omitempty"`
	Module     string `json:"module,omitempty"`
	Package    string `json:"package,omitempty"`
	Go         string `json:"go,omitempty"`
	ClientName string `json:"client_name,omitempty"`
	TypeNaming string `json:"type_naming,omitempty"`
}

// ConfigServer mirrors config.ServerBlock for `config show` output.
type ConfigServer struct {
	Patterns []string `json:"patterns,omitempty"`
	Ignore   []string `json:"ignore,omitempty"`
}

// ShowConfig resolves tyche.json and returns a printable representation.
// Returns (nil, config.ErrNoConfig) when no config file was found — the CLI
// decides how to phrase that to the user.
func ShowConfig(opts LoadOptions) (*ConfigShowResult, error) {
	loaded, err := LoadConfig(opts)
	if err != nil {
		return nil, err
	}
	f := loaded.File
	out := &ConfigShowResult{Path: loaded.Path, Version: f.Version}
	if f.Spec != "" {
		out.Spec = f.Spec
	}
	if f.Client != nil {
		out.Client = &ConfigClient{
			Out:        f.Client.Out,
			Module:     f.Client.Module,
			Package:    f.Client.Package,
			Go:         f.Client.Go,
			ClientName: f.Client.ClientName,
			TypeNaming: f.Client.TypeNaming,
		}
	}
	if f.Server != nil {
		out.Server = &ConfigServer{
			Patterns: f.Server.Patterns,
			Ignore:   f.Server.Ignore,
		}
	}
	return out, nil
}

// --- codegen (tyche generate / clean) -----------------------------------

// Generate writes typed route codecs into the working tree at root for every
// package pattern. Patterns default to ["./..."]. It is the implementation
// behind `tyche generate` and the prefetch step of `tyche build|run|test`.
func Generate(root string, patterns []string) error {
	return servergen.WriteGeneratedFiles(root, defaultPatterns(patterns))
}

// Cleanup removes previously generated codec files from root for the given
// patterns. It is the implementation behind `tyche clean`.
func Cleanup(root string, patterns []string) error {
	return servergen.CleanupGeneratedFiles(root, defaultPatterns(patterns), map[string]struct{}{})
}

func defaultPatterns(args []string) []string {
	if len(args) == 0 {
		return []string{"./..."}
	}
	return args
}

// --- worktree (tyche build / run / test) --------------------------------

// WorktreeOptions configures WithWorktree. The function is the implementation
// behind `tyche build|run|test`: it copies the project into a tmpdir,
// regenerates codecs there, runs the go subcommand, and cleans up.
type WorktreeOptions struct {
	Root       string
	ConfigPath string // honours --config for server pattern/ignore resolution
	EnvConfig  string // honours TYCHE_CONFIG for the same
	Patterns   []string
	GoArgs     []string // e.g. ["build", "-o", "./bin/api", "./cmd/api"]
}

// WithWorktree runs a go subcommand against a temporary copy of the project
// with fresh codecs generated in place, so the user's working tree is never
// touched by generated code. It is the implementation behind build/run/test.
func WithWorktree(opts WorktreeOptions) error {
	patterns, ignore := ResolveServerPatterns(opts.Root, opts.ConfigPath, opts.EnvConfig, opts.Patterns)
	ignoreMatcher, err := buildIgnoreMatcher(ignore)
	if err != nil {
		return err
	}

	tmpRoot := filepath.Join(opts.Root, "tmp")
	if err := os.MkdirAll(tmpRoot, 0o750); err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp(tmpRoot, "codegen.")
	if err != nil {
		return err
	}
	// Cleanup runs on every return path, including panics (defer fires during
	// unwind). If RemoveAll fails (e.g. a file still held open on Windows), the
	// temp dir leaks under ./tmp until the next `tyche clean`; that is not worth
	// failing the command over, so the error is intentionally dropped.
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := copyProjectTree(opts.Root, tmpDir, ignoreMatcher); err != nil {
		return err
	}
	if err := servergen.WriteGeneratedFiles(tmpDir, patterns); err != nil {
		return err
	}
	return runGo(tmpDir, opts.GoArgs...)
}

// ResolveServerPatterns returns the package patterns and ignore globs for
// server codegen. It honours tyche.json's server block using the same config
// precedence as the rest of the CLI (explicit --config > env > discovery from
// root) and falls back to the supplied patterns (default ./...) when no config
// or server block is present.
func ResolveServerPatterns(root, configPath, envConfig string, fallback []string) (patterns, ignore []string) {
	loaded, err := config.Load(config.LoadOptions{
		ExplicitPath:  configPath,
		EnvConfigPath: envConfig,
		CWD:           root,
	})
	if err != nil || loaded == nil || loaded.File == nil || loaded.File.Server == nil {
		return defaultPatterns(fallback), nil
	}
	return loaded.File.Server.ApplyServer(defaultPatterns(fallback))
}

// buildIgnoreMatcher validates the ignore globs up front (so a malformed
// pattern in tyche.json fails fast instead of silently matching nothing) and
// returns a matcher that reports whether a walked path should be skipped.
func buildIgnoreMatcher(patterns []string) (func(string, string) bool, error) {
	if len(patterns) == 0 {
		return func(string, string) bool { return false }, nil
	}
	// filepath.Match reports ErrBadPattern for malformed globs; validate
	// each pattern once here rather than swallowing the error on every path.
	for _, pattern := range patterns {
		if _, err := filepath.Match(pattern, ""); err != nil {
			return nil, fmt.Errorf("invalid ignore pattern %q in tyche.json server.ignore: %w", pattern, err)
		}
	}
	return func(relPath, base string) bool {
		cleanRel := filepath.Clean(relPath)
		for _, pattern := range patterns {
			if pattern == cleanRel || pattern == base {
				return true
			}
			if matched, _ := filepath.Match(pattern, base); matched {
				return true
			}
			if matched, _ := filepath.Match(pattern, cleanRel); matched {
				return true
			}
		}
		return false
	}, nil
}

func copyProjectTree(rootDir, dstDir string, ignore func(string, string) bool) error {
	return filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		base := filepath.Base(path)
		if d.IsDir() {
			if shouldSkipDir(relPath, base) || ignore(relPath, base) {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dstDir, relPath), 0o750)
		}
		if shouldSkipFile(base) || ignore(relPath, base) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, filepath.Join(dstDir, relPath), info.Mode())
	})
}

func shouldSkipDir(relPath, base string) bool {
	switch relPath {
	case ".git", "tmp", "bin", "node_modules", ".next", "dist", "coverage", ".turbo":
		return true
	}
	if base == ".git" || base == "node_modules" || base == ".next" || base == "dist" || base == "coverage" || base == ".turbo" {
		return true
	}
	return strings.HasPrefix(relPath, ".git"+string(filepath.Separator))
}

func shouldSkipFile(base string) bool {
	return base == servergen.GeneratedFilename
}

func copyFile(srcPath, dstPath string, mode os.FileMode) error {
	src, err := os.Open(srcPath) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o750); err != nil {
		return err
	}

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode) //nolint:gosec
	if err != nil {
		return err
	}

	// Close dst exactly once, surfacing a copy error in preference to a
	// close error. No early return sits between here and the close.
	_, err = io.Copy(dst, src)
	if closeErr := dst.Close(); err == nil {
		err = closeErr
	}
	return err
}

func runGo(dir string, args ...string) error {
	if _, err := exec.LookPath("go"); err != nil {
		return errors.New("`go` executable not found in PATH; install Go 1.22+ to use this command")
	}
	cmd := exec.CommandContext(context.Background(), "go", args...) //nolint:gosec
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// --- clientgen (tyche client) -------------------------------------------

// ClientOptions configures RegenerateClient. The CLI maps --spec, --out,
// --module, --package, --go, --client-name, and --type-naming onto these
// fields; values resolved from tyche.json fill any zero field.
type ClientOptions struct {
	SpecPath   string
	OutDir     string
	Module     string
	Package    string
	GoVersion  string
	ClientName string
	TypeNaming string // "structural" or "operation-scoped"
	ConfigPath string // for spec path resolution
}

// ClientResult is what `tyche client` returns on success: the output dir and
// the file count.
type ClientResult struct {
	OutDir    string `json:"out_dir"`
	FileCount int    `json:"file_count"`
}

// RegenerateClient reads the OpenAPI spec at opts.SpecPath, runs clientgen,
// and writes the result to opts.OutDir. spec paths in tyche.json are
// resolved relative to the config file's directory.
func RegenerateClient(opts ClientOptions) (*ClientResult, error) {
	if opts.SpecPath == "" {
		return nil, errors.New("--spec is required (or set spec in tyche.json)")
	}
	if opts.OutDir == "" {
		return nil, errors.New("--out is required (or set client.out in tyche.json)")
	}
	if opts.Module == "" {
		return nil, errors.New("--module is required (or set client.module in tyche.json)")
	}

	if opts.ConfigPath != "" && !filepath.IsAbs(opts.SpecPath) {
		opts.SpecPath = filepath.Join(filepath.Dir(opts.ConfigPath), opts.SpecPath)
	}

	strategy, err := parseTypeNamingStrategy(opts.TypeNaming)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(opts.SpecPath)
	if err != nil {
		return nil, fmt.Errorf("read spec %q: %w", opts.SpecPath, err)
	}
	doc, err := clientgen.ParseDocument(data)
	if err != nil {
		return nil, err
	}
	res, err := clientgen.Generate(doc, clientgen.Options{
		Module:             opts.Module,
		Package:            opts.Package,
		GoVersion:          opts.GoVersion,
		ClientName:         opts.ClientName,
		TypeNamingStrategy: strategy,
	})
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(opts.OutDir, 0o750); err != nil {
		return nil, fmt.Errorf("create out dir %q: %w", opts.OutDir, err)
	}
	if err := removeGeneratedFiles(opts.OutDir); err != nil {
		return nil, err
	}
	for _, f := range res.Files {
		dst := filepath.Join(opts.OutDir, f.Name)
		if f.Name == "go.mod" {
			if _, statErr := os.Stat(dst); statErr == nil {
				continue
			}
		}
		if err := os.WriteFile(dst, f.Content, 0o600); err != nil {
			return nil, fmt.Errorf("write %q: %w", dst, err)
		}
	}
	return &ClientResult{OutDir: opts.OutDir, FileCount: len(res.Files)}, nil
}

func parseTypeNamingStrategy(value string) (clientgen.TypeNamingStrategy, error) {
	switch value {
	case "", "structural":
		return clientgen.TypeNamingStructural, nil
	case "operation-scoped", "operation_scoped", "operation":
		return clientgen.TypeNamingOperationScoped, nil
	default:
		return clientgen.TypeNamingStructural, fmt.Errorf("unknown --type-naming %q (want structural or operation-scoped)", value)
	}
}

// generatedClientMarker is the header clientgen writes on every generated
// Go file; it identifies files safe to delete on regeneration.
const generatedClientMarker = "// Code generated by tyche clientgen; DO NOT EDIT."

func removeGeneratedFiles(outDir string) error {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read out dir %q: %w", outDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		p := filepath.Join(outDir, e.Name())
		data, err := os.ReadFile(p) //nolint:gosec
		if err != nil {
			continue
		}
		if startsWithMarker(data) {
			if err := os.Remove(p); err != nil {
				return fmt.Errorf("remove stale %q: %w", p, err)
			}
		}
	}
	return nil
}

func startsWithMarker(data []byte) bool {
	// Trim leading whitespace/newlines, then compare to the marker.
	i := 0
	for i < len(data) && (data[i] == ' ' || data[i] == '\t' || data[i] == '\n' || data[i] == '\r') {
		i++
	}
	if i >= len(data) {
		return false
	}
	return bytes.HasPrefix(data[i:], []byte(generatedClientMarker))
}
