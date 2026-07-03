package config

import (
	"github.com/webdeveloperben/tyche/clientgen"
)

// ApplyClient merges a ClientBlock into a clientgen.Options. Zero fields in
// the block are treated as "absent" so the caller's pre-populated defaults
// (declared CLI defaults) survive. Validate must already have run; this
// helper assumes the values are sound.
func (c *ClientBlock) ApplyClient(opts *clientgen.Options) {
	if c == nil {
		return
	}
	if c.Module != "" {
		opts.Module = c.Module
	}
	if c.Package != "" {
		opts.Package = c.Package
	}
	if c.Go != "" {
		opts.GoVersion = c.Go
	}
	if c.ClientName != "" {
		opts.ClientName = c.ClientName
	}
	if c.TypeNaming != "" {
		switch c.TypeNaming {
		case "structural":
			opts.TypeNamingStrategy = clientgen.TypeNamingStructural
		case "operation-scoped":
			opts.TypeNamingStrategy = clientgen.TypeNamingOperationScoped
		}
	}
}

// ApplyServer returns patterns and ignore lists merged with the declared
// defaults. A nil block returns (nil, nil) so the caller falls back to
// defaults unchanged.
func (s *ServerBlock) ApplyServer(defaultPatterns []string) (patterns, ignore []string) {
	if s == nil {
		return defaultPatterns, nil
	}
	if len(s.Patterns) > 0 {
		patterns = s.Patterns
	} else {
		patterns = defaultPatterns
	}
	if len(s.Ignore) > 0 {
		ignore = append([]string(nil), s.Ignore...)
	}
	return patterns, ignore
}
