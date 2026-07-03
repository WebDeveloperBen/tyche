package cli

import (
	"github.com/webdeveloperben/tyche/internal/app"
)

// GenerateCmd is `tyche generate`. It writes typed route codecs into
// the working tree for the given package patterns.
type GenerateCmd struct {
	Patterns []string `help:"Package patterns to generate codecs for (default: ./...)." default:""`
}

func (c *GenerateCmd) Run(g *GlobalFlags) error {
	p := g.printer()

	// LoadConfig honours --config / TYCHE_CONFIG and emits the "using
	// config ..." banner. We reuse the loaded server block below so
	// `generate` applies the configured patterns, matching build/run/test.
	loaded, err := app.LoadConfig(g.loadOptions(p))
	if err != nil {
		return Exit(1, err)
	}

	root, err := app.ResolveRoot(g.Root)
	if err != nil {
		return Exit(1, err)
	}

	fallback := c.Patterns
	if len(fallback) == 0 {
		fallback = []string{"./..."}
	}
	patterns := fallback
	if loaded != nil && loaded.File != nil && loaded.File.Server != nil {
		patterns, _ = loaded.File.Server.ApplyServer(fallback)
	}

	if err := app.Generate(root, patterns); err != nil {
		return Exit(1, err)
	}
	return p.Result("generated route codecs into " + root)
}
