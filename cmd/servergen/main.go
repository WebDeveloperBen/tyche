package main

import (
	"fmt"
	"os"

	servergencli "github.com/webdeveloperben/tyche/servergen/cli"
)

func main() {
	if err := servergencli.NewRootCommand().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "servergen: %v\n", err)
		os.Exit(1)
	}
}
