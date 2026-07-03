// Command tyche is the unified tyche CLI. It bundles the previous servergen
// and clientgen binaries into a single tool with subcommands: init, config,
// generate, client, build, run, test, clean.
package main

import (
	"fmt"
	"os"

	tychecli "github.com/webdeveloperben/tyche/servergen/cli"
)

func main() {
	if err := tychecli.NewRootCommand().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "tyche: %v\n", err)
		os.Exit(1)
	}
}
