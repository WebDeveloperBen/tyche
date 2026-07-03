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
	JSON bool `help:"Emit resolved config as JSON (shorthand for --output=json)." name:"json"`
}

func (c *ConfigShowCmd) Run(g *GlobalFlags) error {
	p := g.printer()
	if c.JSON {
		g.Format = "json"
		p = g.printer()
	}
	result, err := app.ShowConfig(g.loadOptions(p))
	if err != nil {
		return Exit(1, err)
	}
	if result == nil {
		// In JSON mode, "no config" is also a structured response rather
		// than a free-form string, so the consumer can branch on it.
		if output.ParseMode(g.Format) == output.ModeJSON {
			return p.Result(map[string]any{"found": false, "message": "no config file found"})
		}
		return p.Result("tyche: no config file found")
	}
	if output.ParseMode(g.Format) == output.ModeJSON {
		// In JSON mode, emit the raw struct so consumers get a structured
		// object. The Printer's JSON encoder handles the marshalling.
		return p.Result(result)
	}
	return p.Result(formatConfigShow(result))
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
	if r.Client != nil {
		if v, ok := r.Client["out"].(string); ok && v != "" {
			b += fmt.Sprintf("  client.out     = %s         (file)\n", v)
		}
		if v, ok := r.Client["module"].(string); ok && v != "" {
			b += fmt.Sprintf("  client.module  = %s  (file)\n", v)
		}
		if v, ok := r.Client["package"].(string); ok && v != "" {
			b += fmt.Sprintf("  client.package = %s         (file)\n", v)
		}
		if v, ok := r.Client["go"].(string); ok && v != "" {
			b += fmt.Sprintf("  client.go      = %s                       (file)\n", v)
		}
		if v, ok := r.Client["client_name"].(string); ok && v != "" {
			b += fmt.Sprintf("  client.client_name = %s          (file)\n", v)
		}
		if v, ok := r.Client["type_naming"].(string); ok && v != "" {
			b += fmt.Sprintf("  client.type_naming = %s       (file)\n", v)
		}
	}
	if r.Server != nil {
		if v, ok := r.Server["patterns"].([]string); ok && len(v) > 0 {
			b += fmt.Sprintf("  server.patterns = %v  (file)\n", v)
		}
		if v, ok := r.Server["ignore"].([]string); ok && len(v) > 0 {
			b += fmt.Sprintf("  server.ignore   = %v  (file)\n", v)
		}
	}
	return b
}
