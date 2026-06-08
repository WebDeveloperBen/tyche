package cli

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/webdeveloperben/tyche/servergen"
)

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "servergen",
		Short:         "Generate typed server codecs and run commands with generated code",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}

	root.AddCommand(newGenerateCommand())
	root.AddCommand(newCleanCommand())
	root.AddCommand(newBuildCommand())
	root.AddCommand(newRunCommand())
	root.AddCommand(newTestCommand())
	return root
}

func newGenerateCommand() *cobra.Command {
	var rootDir string

	cmd := &cobra.Command{
		Use:   "generate [patterns...]",
		Short: "Generate route manifests and codecs into the working tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRoot(rootDir)
			if err != nil {
				return err
			}
			return runGenerate(root, defaultPatterns(args))
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Project root to generate route codecs in")
	return cmd
}

func newCleanCommand() *cobra.Command {
	var rootDir string

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove generated route codec files from the working tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRoot(rootDir)
			if err != nil {
				return err
			}
			return servergen.CleanupGeneratedFiles(root, defaultPatterns(args), map[string]struct{}{})
		},
	}

	cmd.Flags().StringVar(&rootDir, "root", "", "Project root to clean generated files from")
	return cmd
}

func runGenerate(rootDir string, patterns []string) error {
	return servergen.WriteGeneratedFiles(rootDir, patterns)
}

func newBuildCommand() *cobra.Command {
	var output string
	var rootDir string
	var patterns []string

	cmd := &cobra.Command{
		Use:   "build [package]",
		Short: "Build a package from a temporary generated worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkg := args[0]
			if output == "" {
				return errors.New("output path is required")
			}
			root, err := resolveRoot(rootDir)
			if err != nil {
				return err
			}
			outputPath := resolvePath(root, output)
			return withGeneratedWorktree(root, defaultGenerationPatterns(patterns, pkg), func(tmpDir string) error {
				return runCommand(tmpDir, "go", "build", "-o", outputPath, pkg)
			})
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Build output path")
	cmd.Flags().StringVar(&rootDir, "root", "", "Project root to copy into the temporary worktree")
	cmd.Flags().StringSliceVar(&patterns, "patterns", nil, "Package patterns to generate codecs for before building")
	return cmd
}

func newRunCommand() *cobra.Command {
	var rootDir string
	var patterns []string

	cmd := &cobra.Command{
		Use:   "run [package]",
		Short: "Run a package from a temporary generated worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkg := args[0]
			root, err := resolveRoot(rootDir)
			if err != nil {
				return err
			}
			return withGeneratedWorktree(root, defaultGenerationPatterns(patterns, pkg), func(tmpDir string) error {
				return runCommand(tmpDir, "go", "run", pkg)
			})
		},
	}

	cmd.Flags().StringVar(&rootDir, "root", "", "Project root to copy into the temporary worktree")
	cmd.Flags().StringSliceVar(&patterns, "patterns", nil, "Package patterns to generate codecs for before running")
	return cmd
}

func newTestCommand() *cobra.Command {
	var rootDir string
	var verbose bool
	var patterns []string

	cmd := &cobra.Command{
		Use:   "test [packages...]",
		Short: "Run tests from a temporary generated worktree",
		RunE: func(cmd *cobra.Command, args []string) error {
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
			return withGeneratedWorktree(root, defaultPatterns(patterns), func(tmpDir string) error {
				return runCommand(tmpDir, "go", testArgs...)
			})
		},
	}

	cmd.Flags().StringVar(&rootDir, "root", "", "Project root to copy into the temporary worktree")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", true, "Run tests in verbose mode")
	cmd.Flags().StringSliceVar(&patterns, "patterns", nil, "Package patterns to generate codecs for before testing")
	return cmd
}

func withGeneratedWorktree(rootDir string, patterns []string, fn func(tmpDir string) error) error {
	ignoreMatcher, err := loadIgnoreMatcher(rootDir)
	if err != nil {
		return err
	}
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

func loadIgnoreMatcher(rootDir string) (func(string, string) bool, error) {
	ignorePath := filepath.Join(rootDir, ".servergenignore")
	content, err := os.ReadFile(ignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return func(string, string) bool { return false }, nil
		}
		return nil, err
	}

	patterns := make([]string, 0)
	for line := range strings.SplitSeq(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, filepath.Clean(line))
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
	// Surface any error from flushing/closing the written file.
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
	if fallback != "" {
		return []string{"./..."}
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
