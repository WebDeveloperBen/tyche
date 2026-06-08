// Package cli provides the Cobra command for the clientgen CLI.
//
// The command is exposed as a builder so it can serve both as the standalone
// clientgen binary's root command and as the `client` subcommand of servergen,
// sharing identical flags and generation logic.
package cli
