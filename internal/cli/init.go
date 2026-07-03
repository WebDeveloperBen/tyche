package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/webdeveloperben/tyche/internal/app"
	"github.com/webdeveloperben/tyche/internal/ui"
)

// InitCmd is the `tyche init` command. It writes a starter tyche.json
// next to go.mod, prompting for the client module path when stdin is a
// TTY and --module was not given.
type InitCmd struct {
	Module     string `help:"Go module path for the generated client (skips the prompt)." short:"m" default:""`
	Spec       string `help:"Path to the OpenAPI document (skips the prompt)." default:"./api/openapi.json"`
	TypeNaming string `help:"Generated type naming strategy: structural or operation-scoped." enum:"structural,operation-scoped" default:"structural"`
	Force      bool   `help:"Overwrite an existing tyche.json." short:"f"`
	Yes        bool   `help:"Skip prompts; require all answers via flags." short:"y"`
}

// Run implements the command. The Printer is used to emit informational
// lines; the actual write goes through app.Scaffold.
func (c *InitCmd) Run(g *GlobalFlags) error {
	p := g.printer()

	root, err := app.ResolveRoot(g.Root)
	if err != nil {
		return Exit(1, err)
	}

	if c.Module == "" && !c.Yes {
		if !ui.IsTerminal(os.Stdin) {
			return Exit(1, errors.New("non-interactive shell; pass --module to scaffold (--spec and --type-naming are optional)"))
		}
		prompt := ui.Prompt{Out: g.stdout(), In: os.Stdin}
		answer, perr := prompt.AskDefault("Client module path (e.g. github.com/acme/api/client):", "")
		if perr != nil {
			return Exit(1, fmt.Errorf("read module: %w", perr))
		}
		c.Module = answer
	}
	if c.Module == "" {
		return Exit(1, errors.New("client module path is required (use --module or answer the prompt)"))
	}

	written, err := app.Scaffold(app.ScaffoldOptions{
		Root:       root,
		Module:     c.Module,
		Spec:       c.Spec,
		TypeNaming: c.TypeNaming,
		Force:      c.Force,
	})
	if err != nil {
		return Exit(1, err)
	}
	return p.Result("wrote " + written)
}
