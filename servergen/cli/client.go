package cli

import (
	"github.com/spf13/cobra"

	clientcli "github.com/webdeveloperben/tyche/clientgen/cli"
)

// newClientCommand returns the `servergen client` subcommand. It shares its
// flags and generation logic with the standalone clientgen binary via the
// clientgen/cli package.
func newClientCommand() *cobra.Command {
	return clientcli.NewCommand("client")
}
