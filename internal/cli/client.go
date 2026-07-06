package cli

import (
	"errors"
	"fmt"

	"github.com/webdeveloperben/tyche/internal/app"
	"github.com/webdeveloperben/tyche/internal/output"
)

// ClientCmd is `tyche client`. It regenerates the typed Go client from
// the configured OpenAPI document.
type ClientCmd struct {
	Spec       string `help:"Path to the OpenAPI JSON document (overrides tyche.json)." default:""`
	Out        string `help:"Output directory for the generated client module (overrides tyche.json)." default:""`
	Module     string `help:"Go module path for the generated client (overrides tyche.json)." short:"m" default:""`
	Package    string `help:"Package name for generated files (default: derived from module)." default:""`
	Go         string `help:"go directive for the generated go.mod (default: 1.22)." default:""`
	ClientName string `help:"Generated client type name (default: Client)." default:""`
	TypeNaming string `help:"Generated type naming strategy: structural or operation-scoped." enum:"structural,operation-scoped" default:"structural"`
}

func (c *ClientCmd) Run(g *GlobalFlags) error {
	p := g.printer()
	loaded, err := app.LoadConfig(g.loadOptions(p))
	if err != nil && !errors.Is(err, app.ErrNoConfig) {
		return Exit(1, err)
	}

	// Fill any unset flag from the loaded config; explicit flags win.
	if loaded != nil && loaded.File != nil {
		f := loaded.File
		if c.Spec == "" {
			c.Spec = f.Spec
		}
		if f.Client != nil {
			if c.Out == "" {
				c.Out = f.Client.Out
			}
			if c.Module == "" {
				c.Module = f.Client.Module
			}
			if c.Package == "" {
				c.Package = f.Client.Package
			}
			if c.Go == "" {
				c.Go = f.Client.Go
			}
			if c.ClientName == "" {
				c.ClientName = f.Client.ClientName
			}
			if c.TypeNaming == "" {
				c.TypeNaming = f.Client.TypeNaming
			}
		}
	}

	configPath := ""
	if loaded != nil {
		configPath = loaded.Path
	}
	res, err := app.RegenerateClient(app.ClientOptions{
		SpecPath:   c.Spec,
		OutDir:     c.Out,
		Module:     c.Module,
		Package:    c.Package,
		GoVersion:  c.Go,
		ClientName: c.ClientName,
		TypeNaming: c.TypeNaming,
		ConfigPath: configPath,
	})
	if err != nil {
		return Exit(1, err)
	}
	// JSON consumers get the typed result (out_dir, file_count); human and
	// quiet callers get a one-line summary instead of a raw Go map.
	if output.ParseMode(g.Format) == output.ModeJSON {
		return p.Result(res)
	}
	return p.Result(fmt.Sprintf("generated %d files into %s", res.FileCount, res.OutDir))
}
