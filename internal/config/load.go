package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LoadOptions controls Load's behaviour.
type LoadOptions struct {
	// ExplicitPath, when set, loads this exact file and errors if missing.
	ExplicitPath string
	// CWD is the directory discovery starts from. Defaults to os.Getwd().
	CWD string
	// EnvConfigPath mirrors the TYCHE_CONFIG environment variable; takes
	// precedence over discovery but is overridden by ExplicitPath.
	EnvConfigPath string
}

// LoadResult is the outcome of Load. When File is nil and Err is nil, no
// tyche.json was found and the caller should fall back to flag-only mode.
type LoadResult struct {
	// File is the parsed config, or nil when no file was found.
	File *File
	// Path is the absolute path of the loaded file, or "" when none was found.
	Path string
	// README is the optional _README hint from the loaded file. The caller
	// should print it once to stderr.
	README string
}

// Load reads tyche.json using the precedence: ExplicitPath > EnvConfigPath
// > discovery. The returned File is always validated; invalid configurations
// surface as an error before the caller can act on them.
func Load(opts LoadOptions) (*LoadResult, error) {
	res, err := loadUnvalidated(opts)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	if err := res.File.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", res.Path, err)
	}
	return res, nil
}

// loadUnvalidated is the internal loader that skips Validate. It exists so
// `init` can re-load a freshly written file and let the user see the
// validation error in context rather than as a parse failure.
func loadUnvalidated(opts LoadOptions) (*LoadResult, error) {
	if opts.ExplicitPath != "" {
		return loadFile(opts.ExplicitPath)
	}
	if opts.EnvConfigPath != "" {
		return loadFile(opts.EnvConfigPath)
	}
	return discover(opts.CWD)
}

func loadFile(path string) (*LoadResult, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("config: resolve path %q: %w", path, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("config: read %q: %w", abs, err)
	}
	file, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", abs, err)
	}
	return &LoadResult{File: file, Path: abs, README: file.README}, nil
}

func parse(data []byte) (*File, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f File
	if err := dec.Decode(&f); err != nil {
		return nil, wrapSyntaxError(data, err)
	}
	return &f, nil
}

// wrapSyntaxError turns a json.SyntaxError into a friendlier "line N col M"
// message. Falls back to the raw error when the type doesn't match.
func wrapSyntaxError(data []byte, err error) error {
	var syn *json.SyntaxError
	if !errors.As(err, &syn) {
		return err
	}
	line, col := offsetToLineCol(data, syn.Offset)
	return fmt.Errorf("invalid JSON at line %d col %d: %s", line, col, syn.Error())
}

func offsetToLineCol(data []byte, offset int64) (int, int) {
	if offset < 0 || int(offset) > len(data) {
		return 0, 0
	}
	line, col := 1, 1
	for i := range offset {
		if data[i] == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	return line, col
}

// discover walks up from cwd looking for tyche.json or tyche.config.json,
// stopping at the first directory that contains a go.mod. Returns
// (nil, nil) when nothing is found so the caller can fall back to flags.
func discover(cwd string) (*LoadResult, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("config: get working directory: %w", err)
		}
	}
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("config: resolve cwd: %w", err)
	}
	for {
		primary := filepath.Join(dir, "tyche.json")
		alias := filepath.Join(dir, "tyche.config.json")
		hasPrimary := fileExists(primary)
		hasAlias := fileExists(alias)
		if hasPrimary && hasAlias {
			return nil, fmt.Errorf("config: found both %s and %s in %s; remove one", primary, alias, dir)
		}
		if hasPrimary {
			return loadFile(primary)
		}
		if hasAlias {
			return loadFile(alias)
		}
		// Workspace boundary: stop walking at the first go.mod.
		if hasGoMod(dir) {
			return nil, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Hit the filesystem root.
			return nil, nil
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func hasGoMod(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "go.mod"))
	return err == nil
}
