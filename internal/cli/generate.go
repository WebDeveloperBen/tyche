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

	loaded, err := app.LoadConfig(g.loadOptions(p))
	if err != nil {
		return Exit(1, err)
	}
	_ = loaded // file is informational; the patterns come from flags + config

	root, err := app.ResolveRoot(g.Root)
	if err != nil {
		return Exit(1, err)
	}
	if err := app.Generate(root, c.Patterns); err != nil {
		return Exit(1, err)
	}
	return p.Result("generated route codecs into " + root)
}
