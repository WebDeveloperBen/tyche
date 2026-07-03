// Package cli is the user-facing surface of the tyche binary. It maps CLI
// verbs (`tyche init`, `tyche generate`, …) onto the pure use-case
// orchestrators in internal/app, and is the only package in the binary
// that depends on the CLI framework (github.com/alecthomas/kong). The
// servergen, clientgen, and server libraries never import this package,
// so embedding one of them in another tool does not pull in Kong.
package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"

	"github.com/webdeveloperben/tyche/internal/app"
	"github.com/webdeveloperben/tyche/internal/output"
	"github.com/webdeveloperben/tyche/internal/version"
)

// CLI is the top-level Kong command tree. Every subcommand is a struct
// field whose type implements `Run(*GlobalFlags) error`. The struct tags
// drive help text, flag binding, env-var lookup, and argument validation.
// Kong resolves precedence: command line > struct default > env var.
type CLI struct {
	Version    VersionCmd    `cmd:"" help:"Print the tyche version and exit."`
	Client     ClientCmd     `cmd:"" help:"Regenerate the typed client from a tyche OpenAPI spec."`
	Completion CompletionCmd `cmd:"" hidden:"" help:"Print shell completion script (bash|zsh|fish|powershell)."`
	GlobalFlags
	Init     InitCmd     `cmd:"" help:"Scaffold a tyche.json config file in the project root."`
	Build    BuildCmd    `cmd:"" help:"Build a package from a temporary generated worktree."`
	Run      RunCmd      `cmd:"" help:"Run a package from a temporary generated worktree."`
	Generate GenerateCmd `cmd:"" help:"Generate typed route codecs into the working tree."`
	Clean    CleanCmd    `cmd:"" help:"Remove generated route codec files from the working tree."`
	Test     TestCmd     `cmd:"" help:"Run tests from a temporary generated worktree."`
	Config   ConfigCmd   `cmd:"" help:"Inspect and validate the resolved tyche.json."`
}

// GlobalFlags are the flags inherited by every subcommand.
type GlobalFlags struct {
	outW   io.Writer
	errW   io.Writer
	Config string `help:"Path to a tyche.json config file (overrides discovery)." short:"c" env:"TYCHE_CONFIG"`
	Root   string `help:"Project root (default: current directory)." short:"r" default:""`
	Format string `help:"Output format: human (default), json, or quiet." enum:"human,json,quiet" default:"human"`
	Quiet  bool   `help:"Suppress the 'using config ...' info line." short:"q"`
}

// stdout / stderr return the streams to write to: the test-injected
// overrides when set, otherwise the process's actual streams.
func (g *GlobalFlags) stdout() io.Writer {
	if g.outW != nil {
		return g.outW
	}
	return os.Stdout
}

func (g *GlobalFlags) stderr() io.Writer {
	if g.errW != nil {
		return g.errW
	}
	return os.Stderr
}

// loadOptions translates CLI flags into app.LoadOptions. The Printer is
// used as the info callback so "using config ..." lines land in the right
// place (stderr in human mode, JSON line in json mode, dropped in quiet).
func (g *GlobalFlags) loadOptions(p output.Printer) app.LoadOptions {
	return app.LoadOptions{
		Root:       g.Root,
		ConfigPath: g.Config,
		EnvConfig:  os.Getenv("TYCHE_CONFIG"),
		PrintInfo:  !g.Quiet,
		InfoCallback: func(msg string) {
			p.Info(msg)
		},
	}
}

// printer returns the output sink for the requested mode, using the
// current stdout/stderr. Subcommands call this once and pass the result
// to their Run methods.
func (g *GlobalFlags) printer() output.Printer {
	return g.printerFor(g.Format)
}

// printerFor is printer() with an explicit format override, so a
// subcommand can select a mode (e.g. `config show --json`) without
// mutating the shared GlobalFlags.
func (g *GlobalFlags) printerFor(format string) output.Printer {
	return output.New(output.ParseMode(format), g.stdout(), g.stderr())
}

// ExitError is the structured error type for command failures. The
// process main() inspects it to set the right exit code.
type ExitError struct {
	Err  error
	Code int
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// Exit returns ExitError wrapping err with the given exit code, so the
// subcommand Run methods can signal non-zero exits without printing the
// usage banner.
func Exit(code int, err error) error {
	return &ExitError{Code: code, Err: err}
}

// Run is the entry point. It parses args, runs the resolved command, and
// returns the exit code. main() in cmd/tyche calls this and exits the
// process with the result.
func Run(args []string) (code int, err error) {
	// Catch the panic that Kong's Exit() callback raises and normalise the
	// code to 0 for --help / --version and 2 for parse errors. Anything
	// that is not an *ExitPanic is a genuine bug (nil deref, a panic inside
	// Kong, ...) — we re-panic it so main()'s recover turns it into a
	// non-zero exit. Swallowing it here would make a crash look like
	// success (exit 0), which is exactly what we must not do.
	defer func() {
		if r := recover(); r != nil {
			ep, ok := r.(*ExitPanic)
			if !ok {
				panic(r)
			}
			// Kong chooses 80 for parse / usage errors; the tyche
			// contract is POSIX-exit-2 for that case, and 0 for
			// --help / --version.
			if ep.Code == 0 {
				code = 0
			} else {
				code = 2
			}
			err = nil
		}
	}()
	// `tyche` with no args prints help and exits 0, matching the prior
	// Cobra behaviour. We intercept here because Kong treats a missing
	// subcommand as a parse error.
	if len(args) == 0 {
		return runHelp(args)
	}
	return run(args)
}

// ExitPanic is the panic value Kong's Exit function uses to unwind out
// of Parse / Run without killing the process. main()'s recover() catches
// it and uses the embedded code as the exit status. It is exported so
// cmd/tyche can do the recover at the right boundary.
type ExitPanic struct {
	Code int
}

func (e *ExitPanic) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

// run is the actual parser entry. Split out from Run so the no-args help
// case does not pay the parser construction cost on every call.
func run(args []string) (int, error) {
	var cli CLI
	// Kong's Exit default is os.Exit, which would terminate the process
	// before main() can do its bookkeeping. We replace it with a
	// panic-based exit so we can return a structured code to main().
	parser, err := kong.New(
		&cli,
		kong.Name("tyche"),
		kong.Description("Generate typed Go HTTP servers and clients from an OpenAPI spec."),
		kong.UsageOnError(),
		kong.Vars{
			"version": version.String(),
		},
		kong.ConfigureHelp(kong.HelpOptions{
			Compact:             true,
			Indenter:            kong.SpaceIndenter,
			NoExpandSubcommands: false,
			Summary:             true,
		}),
		kong.Exit(func(code int) { panic(&ExitPanic{Code: code}) }),
	)
	if err != nil {
		return 2, fmt.Errorf("init cli parser: %w", err)
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		// Kong has not printed anything yet. Print the usage banner via
		// FatalIfErrorf (which honours UsageOnError), then force the
		// exit code to 2 (POSIX usage-error) by panicking with our
		// own ExitPanic — overriding whatever Kong's exit code logic
		// would have chosen. The recover in Run()/main() handles the
		// panic. FatalIfErrorf already panics through our kong.Exit
		// override, so the explicit panic below is a defensive backstop
		// that keeps the contract even if Kong's behaviour changes.
		parser.FatalIfErrorf(err)
		panic(&ExitPanic{Code: 2})
	}
	if err := kctx.Run(&cli.GlobalFlags); err != nil {
		var exitErr *ExitError
		if eex, ok := err.(*ExitError); ok {
			exitErr = eex
		} else {
			exitErr = &ExitError{Code: 1, Err: err}
		}
		// Render the error through the chosen printer so --output=json
		// gets a problem+json error rather than a free-form line.
		cli.printer().Error(exitErr.Err)
		return exitErr.Code, exitErr.Err
	}
	return 0, nil
}

// runHelp prints the help banner and exits 0. It is invoked when the
// user runs `tyche` with no arguments, matching the prior Cobra
// behaviour where the root command's RunE returned cmd.Help(). It
// builds a separate parser with a no-op Exit so the help flag does not
// panic through the production run() recover boundary.
func runHelp(_ []string) (int, error) {
	var cli CLI
	parser, err := kong.New(
		&cli,
		kong.Name("tyche"),
		kong.Description("Generate typed Go HTTP servers and clients from an OpenAPI spec."),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact:  true,
			Indenter: kong.SpaceIndenter,
			Summary:  true,
		}),
		// No-op Exit: the help flag will call this with 0, but we do
		// not want to panic (which would unwind through run()'s
		// defer). The defer catches the no-op call and returns 0.
		kong.Exit(func(int) {}),
	)
	if err != nil {
		return 2, err
	}
	defer func() {
		// The help flag calls our no-op kong.Exit, so Parse should
		// return normally. Guard against a stray *ExitPanic only;
		// any other panic is a real bug and must propagate so main()
		// can report it as a non-zero exit rather than a clean 0.
		if r := recover(); r != nil {
			if _, ok := r.(*ExitPanic); !ok {
				panic(r)
			}
		}
	}()
	_, _ = parser.Parse([]string{"--help"})
	return 0, nil
}
