// Command tyche is the unified tyche CLI. It scaffolds tyche.json
// configs, regenerates typed route codecs, regenerates typed Go clients
// from an OpenAPI spec, and runs `go build`/`run`/`test` against a
// temporary copy of the project with fresh codecs in place.
//
// Subcommands:
//
//	tyche init         scaffold a tyche.json
//	tyche config show  print the resolved config
//	tyche generate     emit server route codecs into the working tree
//	tyche clean        remove generated codec files
//	tyche build        go build against a temp worktree
//	tyche run          go run against a temp worktree
//	tyche test         go test against a temp worktree
//	tyche client       regenerate the typed client from a spec
//	tyche version      print build info
//	tyche completion   emit a shell completion script
package main

import (
	"fmt"
	"os"

	"github.com/webdeveloperben/tyche/internal/cli"
)

func main() {
	code := run(os.Args[1:])
	os.Exit(code)
}

// run is the top-level CLI entry. It returns the process exit code and
// uses a panic/recover boundary to translate Kong's Exit() callbacks into
// Go returns — Kong's default is os.Exit, which would prevent main() from
// doing any cleanup, and replacing os.Exit with a panic lets us keep
// structured error handling in internal/cli.
func run(args []string) (code int) {
	defer func() {
		if r := recover(); r != nil {
			if ep, ok := r.(*cli.ExitPanic); ok {
				// Force POSIX usage-error (2) for parse / help errors.
				// Subcommand Run() errors do not go through here; they
				// are returned from cli.Run and translated in main().
				if ep.Code != 0 {
					code = 2
				} else {
					code = 0
				}
				return
			}
			// Unknown panic: surface as a runtime error and exit 1.
			fmt.Fprintf(os.Stderr, "tyche: panic: %v\n", r)
			code = 1
		}
	}()
	code, _ = cli.Run(args)
	return code
}
