package cli

import (
	"fmt"

	"github.com/webdeveloperben/tyche/internal/app"
)

// TestCmd is `tyche test`. It runs `go test` against a temporary copy
// of the project with codecs generated in place.
type TestCmd struct {
	Verbose  bool     `help:"Run tests in verbose mode (passes -v to go test)." short:"v"`
	Patterns []string `help:"Package patterns to test (default: ./...)." default:""`
}

func (c *TestCmd) Run(g *GlobalFlags) error {
	p := g.printer()
	root, err := app.ResolveRoot(g.Root)
	if err != nil {
		return Exit(1, err)
	}
	packages := c.Patterns
	if len(packages) == 0 {
		packages = []string{"./..."}
	}
	args := []string{"test"}
	if c.Verbose {
		args = append(args, "-v")
	}
	args = append(args, packages...)
	if err := app.WithWorktree(app.WorktreeOptions{
		Root:     root,
		Patterns: c.Patterns,
		GoArgs:   args,
	}); err != nil {
		return Exit(1, err)
	}
	return p.Result(fmt.Sprintf("tested %v", packages))
}
