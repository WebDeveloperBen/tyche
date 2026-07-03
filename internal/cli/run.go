package cli

import (
	"fmt"

	"github.com/webdeveloperben/tyche/internal/app"
)

// RunCmd is `tyche run`. It runs `go run` against a temporary copy of
// the project with codecs generated in place.
type RunCmd struct {
	Package  string   `arg:"" help:"Package to run (e.g. ./cmd/api)." default:""`
	Patterns []string `help:"Package patterns to generate codecs for before running." default:""`
}

func (c *RunCmd) Run(g *GlobalFlags) error {
	p := g.printer()
	root, err := app.ResolveRoot(g.Root)
	if err != nil {
		return Exit(1, err)
	}
	if err := app.WithWorktree(app.WorktreeOptions{
		Root:     root,
		Patterns: c.Patterns,
		GoArgs:   []string{"run", c.Package},
	}); err != nil {
		return Exit(1, err)
	}
	return p.Result(fmt.Sprintf("ran %s", c.Package))
}
