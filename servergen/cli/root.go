package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/webdeveloperben/tyche/internal/config"
	"github.com/webdeveloperben/tyche/servergen"
)

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "tyche",
		Short:         "Generate typed server codecs, run commands with generated code, and configure projects",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}

	root.AddCommand(newInitCommand())
	root.AddCommand(newConfigShowCommand())
	root.AddCommand(newGenerateCommand())
	root.AddCommand(newCleanCommand())
	root.AddCommand(newBuildCommand())
	root.AddCommand(newRunCommand())
	root.AddCommand(newTestCommand())
	root.AddCommand(newClientCommand())
	return root
}

type commandCtx struct {
	cmd    *cobra.Command
	loaded *config.LoadResult
	quiet  bool
}

func loadConfig(ctx *commandCtx, configPath, cwd string) error {
	loaded, err := config.Load(config.LoadOptions{
		ExplicitPath:  configPath,
		EnvConfigPath: os.Getenv("TYCHE_CONFIG"),
		CWD:           cwd,
	})
	if err != nil {
		return err
	}
	ctx.loaded = loaded
	if loaded == nil {
		return nil
	}
	if !ctx.quiet && loaded.Path != "" {
		fmt.Fprintf(ctx.cmd.ErrOrStderr(), "tyche: using config %s\n", loaded.Path)
	}
	if loaded.README != "" && !ctx.quiet {
		for line := range strings.SplitSeq(loaded.README, "\n") {
			fmt.Fprintf(ctx.cmd.ErrOrStderr(), "  %s\n", line)
		}
	}
	return nil
}

func newInitCommand() *cobra.Command {
	var (
		rootDir     string
		module      string
		spec        string
		typeNaming  string
		force       bool
		skipPrompts bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a tyche.json config file in the project root",
		Long: "Writes a starter tyche.json next to go.mod. With no flags, prompts for\n" +
			"the spec path, client module, and type-naming strategy. Each prompt has a\n" +
			"default shown in brackets; pressing enter accepts. --module, --spec, and\n" +
			"--type-naming skip their respective prompts. Non-interactive by default:\n" +
			"if stdin is not a TTY, refuse to prompt and tell the user to pass the\n" +
			"flags. Refuses to overwrite an existing file without --force.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRoot(rootDir)
			if err != nil {
				return err
			}
			return runInit(cmd, root, initFlags{
				Module:      module,
				Spec:        spec,
				TypeNaming:  typeNaming,
				Force:       force,
				SkipPrompts: skipPrompts,
			})
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Project root to scaffold into (default: cwd)")
	cmd.Flags().StringVar(&module, "module", "", "Go module path for the generated client (skips the prompt)")
	cmd.Flags().StringVar(&spec, "spec", "", "Path to the OpenAPI document (skips the prompt)")
	cmd.Flags().StringVar(&typeNaming, "type-naming", "", "Generated type naming strategy: structural or operation-scoped (skips the prompt)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing tyche.json")
	cmd.Flags().BoolVar(&skipPrompts, "yes", false, "Skip prompts; require all answers via flags")
	return cmd
}

type initFlags struct {
	Module      string
	Spec        string
	TypeNaming  string
	Force       bool
	SkipPrompts bool
}

func runInit(cmd *cobra.Command, root string, f initFlags) error {
	if f.Spec == "" {
		f.Spec = "./api/openapi.json"
	}
	if f.TypeNaming == "" {
		f.TypeNaming = "structural"
	}
	if f.Module == "" && !f.SkipPrompts {
		if !isTerminal(os.Stdin) {
			return errors.New("non-interactive shell; pass --module, --spec, and --type-naming to scaffold")
		}
		fmt.Fprint(cmd.OutOrStdout(), "Client module path (e.g. github.com/acme/api/client): ")
		var raw string
		if _, err := fmt.Fscan(os.Stdin, &raw); err != nil {
			return fmt.Errorf("read module: %w", err)
		}
		f.Module = strings.TrimSpace(raw)
	}
	if f.Module == "" {
		return errors.New("client module path is required (use --module or answer the prompt)")
	}

	dest := filepath.Join(root, "tyche.json")
	if _, err := os.Stat(dest); err == nil && !f.Force {
		return fmt.Errorf("%s already exists; pass --force to overwrite", dest)
	}

	content := scaffoldConfig(f)
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}

	// Round-trip: parse the file we just wrote to catch any validator errors
	// immediately, before the user hits them on the next CLI run.
	if _, err := config.Load(config.LoadOptions{ExplicitPath: dest}); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("scaffolded file failed validation: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", dest)
	return nil
}

func scaffoldConfig(f initFlags) string {
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
`, f.Spec, f.Module, f.TypeNaming)
}

func newConfigShowCommand() *cobra.Command {
	var (
		rootDir    string
		configPath string
		asJSON     bool
		quiet      bool
	)

	cmd := &cobra.Command{
		Use:           "config show",
		Short:         "Print the resolved tyche.json (path, values, source of each field)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := &commandCtx{cmd: cmd, quiet: quiet}
			root, err := resolveRoot(rootDir)
			if err != nil {
				return err
			}
			if err := loadConfig(ctx, configPath, root); err != nil {
				return err
			}
			return runConfigShow(cmd, ctx, asJSON)
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Project root to start discovery from")
	cmd.Flags().StringVar(&configPath, "config", "", "Path to a tyche.json config file (overrides discovery)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit resolved config as JSON")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress the 'using config ...' info line")
	return cmd
}

func runConfigShow(cmd *cobra.Command, ctx *commandCtx, asJSON bool) error {
	if ctx.loaded == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "tyche: no config file found")
		return nil
	}
	if asJSON {
		// Marshal a copy without the README hint so the JSON output is
		// directly re-ingestable.
		out := *ctx.loaded.File
		out.README = ""
		data, err := json.MarshalIndent(&out, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "tyche: config %s\n", ctx.loaded.Path)
	f := ctx.loaded.File
	fmt.Fprintf(cmd.OutOrStdout(), "  version        = %d                       (file)\n", f.Version)
	if f.Spec != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  spec           = %s        (file)\n", f.Spec)
	}
	if f.Client != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  client.out     = %s         (file)\n", f.Client.Out)
		fmt.Fprintf(cmd.OutOrStdout(), "  client.module  = %s  (file)\n", f.Client.Module)
		if f.Client.Package != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  client.package = %s         (file)\n", f.Client.Package)
		}
		if f.Client.Go != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  client.go      = %s                       (file)\n", f.Client.Go)
		}
		if f.Client.ClientName != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  client.client_name = %s          (file)\n", f.Client.ClientName)
		}
		if f.Client.TypeNaming != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  client.type_naming = %s       (file)\n", f.Client.TypeNaming)
		}
	}
	if f.Server != nil {
		if len(f.Server.Patterns) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "  server.patterns = %v  (file)\n", f.Server.Patterns)
		}
		if len(f.Server.Ignore) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "  server.ignore   = %v  (file)\n", f.Server.Ignore)
		}
	}
	return nil
}

func newGenerateCommand() *cobra.Command {
	var (
		rootDir    string
		configPath string
		quiet      bool
	)

	cmd := &cobra.Command{
		Use:   "generate [patterns...]",
		Short: "Generate route manifests and codecs into the working tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := &commandCtx{cmd: cmd, quiet: quiet}
			root, err := resolveRoot(rootDir)
			if err != nil {
				return err
			}
			if err := loadConfig(ctx, configPath, root); err != nil {
				return err
			}
			patterns, _ := resolveServerPatterns(ctx, defaultPatterns(args))
			return runGenerate(root, patterns)
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Project root to generate route codecs in")
	cmd.Flags().StringVar(&configPath, "config", "", "Path to a tyche.json config file (overrides discovery)")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress the 'using config ...' info line")
	return cmd
}

func newCleanCommand() *cobra.Command {
	var (
		rootDir    string
		configPath string
		quiet      bool
	)

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove generated route codec files from the working tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := &commandCtx{cmd: cmd, quiet: quiet}
			root, err := resolveRoot(rootDir)
			if err != nil {
				return err
			}
			if err := loadConfig(ctx, configPath, root); err != nil {
				return err
			}
			return servergen.CleanupGeneratedFiles(root, defaultPatterns(args), map[string]struct{}{})
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Project root to clean generated files from")
	cmd.Flags().StringVar(&configPath, "config", "", "Path to a tyche.json config file (overrides discovery)")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress the 'using config ...' info line")
	return cmd
}

func runGenerate(rootDir string, patterns []string) error {
	return servergen.WriteGeneratedFiles(rootDir, patterns)
}

func newBuildCommand() *cobra.Command {
	var (
		output     string
		rootDir    string
		patterns   []string
		configPath string
		quiet      bool
	)

	cmd := &cobra.Command{
		Use:   "build [package]",
		Short: "Build a package from a temporary generated worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := &commandCtx{cmd: cmd, quiet: quiet}
			pkg := args[0]
			if output == "" {
				return errors.New("output path is required")
			}
			root, err := resolveRoot(rootDir)
			if err != nil {
				return err
			}
			if err := loadConfig(ctx, configPath, root); err != nil {
				return err
			}
			mergedPatterns, _ := resolveServerPatterns(ctx, defaultGenerationPatterns(patterns, pkg))
			outputPath := resolvePath(root, output)
			return withGeneratedWorktree(root, mergedPatterns, func(tmpDir string) error {
				return runCommand(tmpDir, "go", "build", "-o", outputPath, pkg)
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "Build output path")
	cmd.Flags().StringVar(&rootDir, "root", "", "Project root to copy into the temporary worktree")
	cmd.Flags().StringSliceVar(&patterns, "patterns", nil, "Package patterns to generate codecs for before building")
	cmd.Flags().StringVar(&configPath, "config", "", "Path to a tyche.json config file (overrides discovery)")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress the 'using config ...' info line")
	return cmd
}

func newRunCommand() *cobra.Command {
	var (
		rootDir    string
		patterns   []string
		configPath string
		quiet      bool
	)

	cmd := &cobra.Command{
		Use:   "run [package]",
		Short: "Run a package from a temporary generated worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := &commandCtx{cmd: cmd, quiet: quiet}
			pkg := args[0]
			root, err := resolveRoot(rootDir)
			if err != nil {
				return err
			}
			if err := loadConfig(ctx, configPath, root); err != nil {
				return err
			}
			mergedPatterns, _ := resolveServerPatterns(ctx, defaultGenerationPatterns(patterns, pkg))
			return withGeneratedWorktree(root, mergedPatterns, func(tmpDir string) error {
				return runCommand(tmpDir, "go", "run", pkg)
			})
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Project root to copy into the temporary worktree")
	cmd.Flags().StringSliceVar(&patterns, "patterns", nil, "Package patterns to generate codecs for before running")
	cmd.Flags().StringVar(&configPath, "config", "", "Path to a tyche.json config file (overrides discovery)")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress the 'using config ...' info line")
	return cmd
}

func newTestCommand() *cobra.Command {
	var (
		rootDir    string
		verbose    bool
		patterns   []string
		configPath string
		quiet      bool
	)

	cmd := &cobra.Command{
		Use:   "test [packages...]",
		Short: "Run tests from a temporary generated worktree",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := &commandCtx{cmd: cmd, quiet: quiet}
			packages := defaultPatterns(args)
			testArgs := []string{"test"}
			if verbose {
				testArgs = append(testArgs, "-v")
			}
			testArgs = append(testArgs, packages...)
			root, err := resolveRoot(rootDir)
			if err != nil {
				return err
			}
			if err := loadConfig(ctx, configPath, root); err != nil {
				return err
			}
			mergedPatterns, _ := resolveServerPatterns(ctx, defaultPatterns(patterns))
			return withGeneratedWorktree(root, mergedPatterns, func(tmpDir string) error {
				return runCommand(tmpDir, "go", testArgs...)
			})
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Project root to copy into the temporary worktree")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", true, "Run tests in verbose mode")
	cmd.Flags().StringSliceVar(&patterns, "patterns", nil, "Package patterns to generate codecs for before testing")
	cmd.Flags().StringVar(&configPath, "config", "", "Path to a tyche.json config file (overrides discovery)")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress the 'using config ...' info line")
	return cmd
}

// resolveServerPatterns returns the patterns to use for servergen. The
// defaults are the user's flag/arg value; if a config was loaded and the
// user did not pass --patterns, the file's server.patterns takes over.
func resolveServerPatterns(ctx *commandCtx, fallback []string) ([]string, []string) {
	if ctx == nil || ctx.loaded == nil || ctx.loaded.File == nil {
		return fallback, nil
	}
	if ctx.cmd != nil && ctx.cmd.Flags().Changed("patterns") {
		return fallback, nil
	}
	patterns, ignore := ctx.loaded.File.Server.ApplyServer(fallback)
	return patterns, ignore
}

func withGeneratedWorktree(rootDir string, patterns []string, fn func(tmpDir string) error) error {
	_, ignorePatterns, _ := resolveServerPatternsFromRoot(rootDir)
	ignoreMatcher := buildIgnoreMatcher(ignorePatterns)
	tmpRoot := filepath.Join(rootDir, "tmp")
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp(tmpRoot, "codegen.")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := copyProjectTree(rootDir, tmpDir, ignoreMatcher); err != nil {
		return err
	}
	if err := servergen.WriteGeneratedFiles(tmpDir, patterns); err != nil {
		return err
	}
	return fn(tmpDir)
}

func resolveServerPatternsFromRoot(rootDir string) ([]string, []string, error) {
	loaded, err := config.Load(config.LoadOptions{CWD: rootDir})
	if err != nil {
		return nil, nil, err
	}
	if loaded == nil || loaded.File == nil || loaded.File.Server == nil {
		return nil, nil, nil
	}
	patterns, ignore := loaded.File.Server.ApplyServer(nil)
	return patterns, ignore, nil
}

func buildIgnoreMatcher(patterns []string) func(string, string) bool {
	if len(patterns) == 0 {
		return func(string, string) bool { return false }
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
	}
}

func copyProjectTree(rootDir, dstDir string, ignoreMatcher func(string, string) bool) error {
	return filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
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
			if shouldSkipDir(relPath, base) || ignoreMatcher(relPath, base) {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dstDir, relPath), 0o755)
		}
		if shouldSkipFile(base) || ignoreMatcher(relPath, base) {
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

func copyFile(srcPath, dstPath string, mode fs.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	if _, err = io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Close()
}

func runCommand(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func defaultPatterns(args []string) []string {
	if len(args) == 0 {
		return []string{"./..."}
	}
	return args
}

func defaultGenerationPatterns(patterns []string, fallback string) []string {
	if len(patterns) != 0 {
		return patterns
	}
	return []string{"./..."}
}

func resolveRoot(rootDir string) (string, error) {
	if rootDir != "" {
		return rootDir, nil
	}
	return os.Getwd()
}

func resolvePath(rootDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(rootDir, path)
}

// isTerminal returns true when the given file is a character device
// (i.e. an interactive TTY). We don't prompt in scripts or CI.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
