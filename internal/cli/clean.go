package cli

import "github.com/webdeveloperben/tyche/internal/app"

// CleanCmd is `tyche clean`. It removes generated route codec files from
// the working tree.
type CleanCmd struct {
	Patterns []string `help:"Package patterns to clean (default: ./...)." default:""`
}

func (c *CleanCmd) Run(g *GlobalFlags) error {
	p := g.printer()
	root, err := app.ResolveRoot(g.Root)
	if err != nil {
		return Exit(1, err)
	}
	if err := app.Cleanup(root, c.Patterns); err != nil {
		return Exit(1, err)
	}
	return p.Result("cleaned generated files from " + root)
}
