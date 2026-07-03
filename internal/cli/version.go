package cli

import (
	"github.com/webdeveloperben/tyche/internal/version"
)

// VersionCmd is `tyche version`. It prints the binary's build identity
// and exits.
type VersionCmd struct{}

func (c *VersionCmd) Run(g *GlobalFlags) error {
	return g.printer().Result(version.String())
}
