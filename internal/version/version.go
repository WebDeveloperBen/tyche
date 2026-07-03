// Package version exposes the build-time metadata of the tyche binary.
//
// The values are set via -ldflags="-X" at link time. The defaults (used
// during `go run` or `go test`) are "dev" for the version and the current
// commit is empty; the Makefile or release pipeline populates them.
//
// Example:
//
//	go build -ldflags "\
//	    -X github.com/webdeveloperben/tyche/internal/version.Version=1.2.3 \
//	    -X github.com/webdeveloperben/tyche/internal/version.Commit=$(git rev-parse HEAD) \
//	    -X github.com/webdeveloperben/tyche/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
//	    -X github.com/webdeveloperben/tyche/internal/version.BuiltBy=release-please" \
//	    -o ./bin/tyche ./cmd/tyche
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

// These variables are overridden at link time. The defaults let `go run` and
// `go test ./...` produce a usable `tyche version` output without flags.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
	BuiltBy = "source"
)

var (
	once sync.Once
	info Info
)

// Info is the rendered, one-line build identity of the binary.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	Date    string `json:"date,omitempty"`
	BuiltBy string `json:"built_by,omitempty"`
	Go      string `json:"go"`
}

// Get returns the binary's build identity. The Go field is always populated
// from runtime.Version; everything else comes from ldflags, falling back to
// build-info derivation from runtime/debug when ldflags are absent.
func Get() Info {
	once.Do(resolve)
	return info
}

func resolve() {
	info = Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
		BuiltBy: BuiltBy,
		Go:      runtime.Version(),
	}
	// When the binary was built with `go build` (no -ldflags) and lives in
	// a Go module, runtime/debug.ReadBuildInfo can recover the VCS revision
	// and build time. We fill those in if ldflags did not.
	if bi, ok := debug.ReadBuildInfo(); ok {
		if info.Version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			info.Version = bi.Main.Version
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if info.Commit == "" {
					info.Commit = s.Value
				}
			case "vcs.time":
				if info.Date == "" {
					info.Date = s.Value
				}
			case "vcs.modified":
				if s.Value == "true" && info.Commit != "" && !strings.HasSuffix(info.Commit, "-dirty") {
					info.Commit += "-dirty"
				}
			}
		}
	}
}

// String returns a one-line, human-readable summary of the build identity.
// This is what `tyche version` prints to stdout.
func String() string {
	return format(Get())
}

// format renders an Info as a one-line summary. It is a pure function of its
// argument (no package-level state) so it can be tested with synthetic Info
// values without resetting the sync.Once or mutating the ldflag variables.
func format(i Info) string {
	if i.Commit == "" {
		return fmt.Sprintf("tyche %s (built %s with %s)", i.Version, i.BuiltBy, i.Go)
	}
	return fmt.Sprintf("tyche %s (%s, built %s with %s)", i.Version, short(i.Commit), i.BuiltBy, i.Go)
}

// short returns the first 12 characters of a commit, like `git log --oneline`.
func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
