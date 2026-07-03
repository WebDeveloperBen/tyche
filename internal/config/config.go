// Package config loads and validates the project's tyche.json file.
//
// The format is JSON via the standard library's encoding/json — no extra
// dependencies. A tyche.json sits next to go.mod and holds the inputs the
// tyche CLI would otherwise take as flags (spec path, client output dir,
// servergen patterns, ignore list, etc.). Flags always override file values
// so a one-off override never has to edit the file.
//
// Discovery walks up from the working directory until it finds a tyche.json
// or tyche.config.json, stopping at the first go.mod boundary. An explicit
// --config path bypasses discovery.
package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// CurrentVersion is the only config version this build understands. Any other
// value fails Validate so future format changes can fail fast.
const CurrentVersion = 1

// File is the in-memory shape of tyche.json. Unknown top-level and nested
// fields are ignored for forward compatibility; a future --strict-config flag
// can flip that.
type File struct {
	Client  *ClientBlock `json:"client,omitempty"`
	Server  *ServerBlock `json:"server,omitempty"`
	README  string       `json:"_README,omitempty"`
	Spec    string       `json:"spec,omitempty"`
	Version int          `json:"version"`
}

// ClientBlock mirrors the clientgen CLI flags. A zero field is treated as
// "absent" and the CLI default takes over.
type ClientBlock struct {
	Out        string `json:"out,omitempty"`
	Module     string `json:"module,omitempty"`
	Package    string `json:"package,omitempty"`
	Go         string `json:"go,omitempty"`
	ClientName string `json:"client_name,omitempty"`
	TypeNaming string `json:"type_naming,omitempty"`
}

// ServerBlock mirrors servergen flags.
type ServerBlock struct {
	Patterns []string `json:"patterns,omitempty"`
	Ignore   []string `json:"ignore,omitempty"`
}

// validTypeNaming is the closed set of accepted type-naming values plus the
// empty string (which means "absent; use the default").
var validTypeNaming = map[string]bool{
	"":                 true,
	"structural":       true,
	"operation-scoped": true,
}

// goModulePathRE is a deliberately strict subset of the Go module path
// grammar: lowercase letters, digits, dot, underscore, tilde, dash, with at
// least one slash. It rejects spaces, uppercase, and paths that don't look
// like a domain.
var goModulePathRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._~\-]*(\/[a-z0-9][a-z0-9._~\-]*)+$`)

// Validate checks the loaded config is well-formed. It validates the
// *structure* of the file: version, format of fields, and presence of
// required values. It does not check that external resources (like the spec
// file) exist; those checks happen at the point of use so a freshly
// scaffolded config that points at a not-yet-generated spec is still valid.
func (f *File) Validate() error {
	if f == nil {
		return errors.New("config: nil file")
	}
	if f.Version != CurrentVersion {
		return fmt.Errorf("config: unsupported version %d (expected %d); see the tyche docs for migration", f.Version, CurrentVersion)
	}
	if f.Client != nil {
		if f.Client.Module == "" {
			return errors.New("config: client.module is required when client block is present")
		}
		if !goModulePathRE.MatchString(f.Client.Module) {
			return fmt.Errorf("config: client.module %q is not a valid Go module path", f.Client.Module)
		}
		if !validTypeNaming[f.Client.TypeNaming] {
			return fmt.Errorf("config: client.type_naming %q must be one of [structural, operation-scoped]", f.Client.TypeNaming)
		}
	}
	if f.Server != nil {
		for _, p := range f.Server.Ignore {
			cleaned := filepath.Clean(p)
			if cleaned == "" || cleaned == "." || strings.HasPrefix(cleaned, "..") {
				return fmt.Errorf("config: server.ignore entry %q is not a valid path", p)
			}
		}
	}
	return nil
}
