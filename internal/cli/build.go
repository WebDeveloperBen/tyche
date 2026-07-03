package cli

import (
	"errors"
	"fmt"

	"github.com/webdeveloperben/tyche/internal/app"
)

// BuildCmd is `tyche build`. It runs `go build` against a temporary
// copy of the project with codecs generated in place, so the working
// tree is never modified.
type BuildCmd struct {
	Package  string   `arg:"" help:"Package to build (e.g. ./cmd/api)." default:""`
	Output   string   `help:"Output binary path." short:"o" default:""`
	Patterns []string `help:"Package patterns to generate codecs for before building." default:""`
}

func (c *BuildCmd) Run(g *GlobalFlags) error {
	p := g.printer()
	if c.Output == "" {
		return Exit(2, errors.New("output path is required (use --output)"))
	}
	root, err := app.ResolveRoot(g.Root)
	if err != nil {
		return Exit(1, err)
	}
	out := app.ResolvePath(root, c.Output)
	args := []string{"build", "-o", out, c.Package}
	if err := app.WithWorktree(app.WorktreeOptions{
		Root:     root,
		Patterns: c.Patterns,
		GoArgs:   args,
	}); err != nil {
		return Exit(1, err)
	}
	return p.Result(fmt.Sprintf("built %s", c.Package))
}
