package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/webdeveloperben/tyche/clientgen"
	"github.com/webdeveloperben/tyche/internal/config"
)

// NewCommand builds the clientgen command under the given use name. It is
// mounted by the tyche binary as the `client` subcommand, and is also
// reusable as a standalone command for custom tooling.
func NewCommand(use string) *cobra.Command {
	var (
		specPath, outDir, module, pkg, goVersion, clientName, typeNaming string
		configPath                                                       string
		quiet                                                            bool
	)

	cmd := &cobra.Command{
		Use:   use,
		Short: "Generate a self-contained typed Go client from a tyche OpenAPI spec",
		Long: "Reads an OpenAPI JSON document (e.g. the spec your tyche server emits) and\n" +
			"writes a dependency-free, typed Go client module: its own go.mod, request/\n" +
			"response types, one method per operation, and typed problem+json errors.\n\n" +
			"Configuration is read from tyche.json (or tyche.config.json) in the current\n" +
			"directory or any parent up to the first go.mod. Use --config to point at a\n" +
			"specific file. Flags always override file values.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			loaded, err := config.Load(config.LoadOptions{
				ExplicitPath:  configPath,
				EnvConfigPath: os.Getenv("TYCHE_CONFIG"),
			})
			if err != nil {
				return err
			}
			if loaded != nil {
				if !quiet && loaded.Path != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "tyche: using config %s\n", loaded.Path)
				}
				if loaded.README != "" && !quiet {
					for line := range strings.SplitSeq(loaded.README, "\n") {
						fmt.Fprintf(cmd.ErrOrStderr(), "  %s\n", line)
					}
				}
				if loaded.File != nil {
					if !cmd.Flags().Changed("spec") && loaded.File.Spec != "" {
						specPath = loaded.File.Spec
					}
					if loaded.File.Client != nil {
						if !cmd.Flags().Changed("out") && loaded.File.Client.Out != "" {
							outDir = loaded.File.Client.Out
						}
						if !cmd.Flags().Changed("module") && loaded.File.Client.Module != "" {
							module = loaded.File.Client.Module
						}
						if !cmd.Flags().Changed("package") && loaded.File.Client.Package != "" {
							pkg = loaded.File.Client.Package
						}
						if !cmd.Flags().Changed("go") && loaded.File.Client.Go != "" {
							goVersion = loaded.File.Client.Go
						}
						if !cmd.Flags().Changed("client-name") && loaded.File.Client.ClientName != "" {
							clientName = loaded.File.Client.ClientName
						}
						if !cmd.Flags().Changed("type-naming") && loaded.File.Client.TypeNaming != "" {
							typeNaming = loaded.File.Client.TypeNaming
						}
					}
				}
			}
			if specPath == "" {
				return errors.New("--spec is required (or set spec in tyche.json)")
			}
			if outDir == "" {
				return errors.New("--out is required (or set client.out in tyche.json)")
			}
			if module == "" {
				return errors.New("--module is required (or set client.module in tyche.json)")
			}

			// Resolve spec relative to the config file's directory when one
			// was loaded. This lets "spec": "./api/openapi.json" in tyche.json
			// resolve correctly even when the CLI is run from elsewhere.
			if loaded != nil && loaded.Path != "" && !filepath.IsAbs(specPath) {
				specPath = filepath.Join(filepath.Dir(loaded.Path), specPath)
			}

			typeNamingStrategy, err := parseTypeNamingStrategy(typeNaming)
			if err != nil {
				return err
			}

			data, err := os.ReadFile(specPath)
			if err != nil {
				return fmt.Errorf("read spec %q: %w", specPath, err)
			}
			doc, err := clientgen.ParseDocument(data)
			if err != nil {
				return err
			}
			res, err := clientgen.Generate(doc, clientgen.Options{
				Module:             module,
				Package:            pkg,
				GoVersion:          goVersion,
				ClientName:         clientName,
				TypeNamingStrategy: typeNamingStrategy,
			})
			if err != nil {
				return err
			}

			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return fmt.Errorf("create out dir %q: %w", outDir, err)
			}
			// Remove previously generated .go files so a regeneration that drops
			// an operation (e.g. no more streaming) doesn't leave stale files
			// referencing deleted symbols.
			if err := removeGeneratedFiles(outDir); err != nil {
				return err
			}
			for _, f := range res.Files {
				dst := filepath.Join(outDir, f.Name)
				// Don't clobber a go.mod the user may have customized (a
				// module-level replace, a bumped go directive, etc.).
				if f.Name == "go.mod" {
					if _, statErr := os.Stat(dst); statErr == nil {
						continue
					}
				}
				if err := os.WriteFile(dst, f.Content, 0o644); err != nil {
					return fmt.Errorf("write %q: %w", dst, err)
				}
			}
			for _, n := range res.Notices {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "notice:", n)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "generated %d files into %s\n", len(res.Files), outDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&specPath, "spec", "", "Path to the OpenAPI JSON document")
	cmd.Flags().StringVar(&outDir, "out", "", "Output directory for the generated client module")
	cmd.Flags().StringVar(&module, "module", "", "Go module path for the generated client, e.g. github.com/you/app/client")
	cmd.Flags().StringVar(&pkg, "package", "", "Package name for generated files (default: derived from module)")
	cmd.Flags().StringVar(&goVersion, "go", "", "go directive for the generated go.mod (default: 1.22)")
	cmd.Flags().StringVar(&clientName, "client-name", "", "Generated client type name (default: Client)")
	cmd.Flags().StringVar(&typeNaming, "type-naming", "structural", "Generated type naming strategy: structural or operation-scoped")
	cmd.Flags().StringVar(&configPath, "config", "", "Path to a tyche.json config file (overrides discovery)")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress the 'using config ...' info line")
	return cmd
}

func parseTypeNamingStrategy(value string) (clientgen.TypeNamingStrategy, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "structural":
		return clientgen.TypeNamingStructural, nil
	case "operation-scoped", "operation_scoped", "operation":
		return clientgen.TypeNamingOperationScoped, nil
	default:
		return clientgen.TypeNamingStructural, fmt.Errorf("unknown --type-naming %q (want structural or operation-scoped)", value)
	}
}

// generatedClientMarker is the header tyche clientgen writes on every generated
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
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if bytes.HasPrefix(bytes.TrimSpace(data), []byte(generatedClientMarker)) {
			if err := os.Remove(p); err != nil {
				return fmt.Errorf("remove stale %q: %w", p, err)
			}
		}
	}
	return nil
}
