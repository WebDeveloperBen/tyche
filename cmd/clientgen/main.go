package main

import (
	"fmt"
	"os"

	clientcli "github.com/webdeveloperben/tyche/clientgen/cli"
)

func main() {
	if err := clientcli.NewCommand("clientgen").Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "clientgen: %v\n", err)
		os.Exit(1)
	}
}
