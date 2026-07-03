package cli

import (
	"fmt"

	"github.com/webdeveloperben/tyche/internal/app"
	"github.com/webdeveloperben/tyche/internal/output"
)

// ConfigCmd groups the `tyche config ...` subcommands. Currently there is
// only `show`, but the structure leaves room for `validate`, `path`, or
// `init-template` without growing the top-level namespace.
type ConfigCmd struct {
	Show ConfigShowCmd `cmd:"" help:"Print the resolved tyche.json (path, values, source of each field)."`
}

// ConfigShowCmd is `tyche config show`.
type ConfigShowCmd struct {
	JSON bool `help:"Emit resolved config as JSON (shorthand for --format=json)."`
}

func (c *ConfigShowCmd) Run(g *GlobalFlags) error {
	// --json is a shorthand for --format=json. Resolve the effective format
	// locally instead of mutating the shared GlobalFlags.
	format := g.Format
	if c.JSON {
		format = "json"
	}
	mode := output.ParseMode(format)
	p := g.printerFor(format)

	result, err := app.ShowConfig(g.loadOptions(p))
	if err != nil {
		return Exit(1, err)
	}

	if result == nil {
		switch mode {
		case output.ModeJSON:
			// "no config" is a structured response so a consumer can branch.
			return p.Result(map[string]any{"found": false, "message": "no config file found"})
		case output.ModeQuiet:
			// Quiet: no decoration, and nothing to emit.
			return nil
		default:
			return p.Result("tyche: no config file found")
		}
	}

	switch mode {
	case output.ModeJSON:
		// Emit the typed struct; the printer's JSON encoder marshals it.
		return p.Result(result)
	case output.ModeQuiet:
		// Quiet: only the requested datum, no banner or table — the
		// resolved config path is the single useful value here.
		return p.Result(result.Path)
	default:
		return p.Result(formatConfigShow(result))
	}
}

// formatConfigShow renders the resolved config for human output. JSON
// output goes through the printer's default JSON encoding; this is the
// column-aligned, multi-line form users see in their terminal.
func formatConfigShow(r *app.ConfigShowResult) string {
	if r == nil {
		return "tyche: no config file found"
	}
	var b string
	if r.Path != "" {
		b = fmt.Sprintf("tyche: config %s\n", r.Path)
	}
	b += fmt.Sprintf("  version        = %d                       (file)\n", r.Version)
	if r.Spec != "" {
		b += fmt.Sprintf("  spec           = %s        (file)\n", r.Spec)
	}
	if c := r.Client; c != nil {
		if c.Out != "" {
			b += fmt.Sprintf("  client.out     = %s         (file)\n", c.Out)
		}
		if c.Module != "" {
			b += fmt.Sprintf("  client.module  = %s  (file)\n", c.Module)
		}
		if c.Package != "" {
			b += fmt.Sprintf("  client.package = %s         (file)\n", c.Package)
		}
		if c.Go != "" {
			b += fmt.Sprintf("  client.go      = %s                       (file)\n", c.Go)
		}
		if c.ClientName != "" {
			b += fmt.Sprintf("  client.client_name = %s          (file)\n", c.ClientName)
		}
		if c.TypeNaming != "" {
			b += fmt.Sprintf("  client.type_naming = %s       (file)\n", c.TypeNaming)
		}
	}
	if s := r.Server; s != nil {
		if len(s.Patterns) > 0 {
			b += fmt.Sprintf("  server.patterns = %v  (file)\n", s.Patterns)
		}
		if len(s.Ignore) > 0 {
			b += fmt.Sprintf("  server.ignore   = %v  (file)\n", s.Ignore)
		}
	}
	return b
}
