package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/webdeveloperben/tyche/clientgen"
)

func newClientCommand() *cobra.Command {
	var specPath, outDir, module, pkg, goVersion, clientName string

	cmd := &cobra.Command{
		Use:   "client",
		Short: "Generate a self-contained typed Go client from a tyche OpenAPI spec",
		Long: "Reads an OpenAPI JSON document (e.g. the spec your tyche server emits) and\n" +
			"writes a dependency-free, typed Go client module: its own go.mod, request/\n" +
			"response types, one method per operation, and typed problem+json errors.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if specPath == "" {
				return errors.New("--spec is required")
			}
			if outDir == "" {
				return errors.New("--out is required")
			}
			if module == "" {
				return errors.New("--module is required")
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
				Module:     module,
				Package:    pkg,
				GoVersion:  goVersion,
				ClientName: clientName,
			})
			if err != nil {
				return err
			}

			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return fmt.Errorf("create out dir %q: %w", outDir, err)
			}
			for _, f := range res.Files {
				dst := filepath.Join(outDir, f.Name)
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

	cmd.Flags().StringVar(&specPath, "spec", "", "Path to the OpenAPI JSON document (required)")
	cmd.Flags().StringVar(&outDir, "out", "", "Output directory for the generated client module (required)")
	cmd.Flags().StringVar(&module, "module", "", "Go module path for the generated client, e.g. github.com/you/app/client (required)")
	cmd.Flags().StringVar(&pkg, "package", "", "Package name for generated files (default: derived from module)")
	cmd.Flags().StringVar(&goVersion, "go", "", "go directive for the generated go.mod (default: 1.22)")
	cmd.Flags().StringVar(&clientName, "client-name", "", "Generated client type name (default: Client)")
	return cmd
}
