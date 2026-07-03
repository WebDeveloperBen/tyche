// Package output is the formatting layer for the tyche CLI. It owns the
// human / json / quiet output modes that every command goes through, so
// formatting stays in one place and individual commands stay focused on
// behaviour. The package also renders errors in a way that matches the
// shape used elsewhere in tyche (RFC 9457-style problem+json for machine
// output, terse single-line human errors for terminals).
package output

import (
	"encoding/json"
	"fmt"
	"io"
)

// Mode is the output format selected by the --output flag (or its --json /
// --quiet shorthands).
type Mode string

const (
	ModeHuman Mode = "human"
	ModeJSON  Mode = "json"
	ModeQuiet Mode = "quiet"
)

// ParseMode normalises a user-supplied --output string to a known Mode.
// Empty and "human" both map to ModeHuman. Unknown values fall back to
// ModeHuman so an accidental typo never breaks a pipeline; the caller can
// surface the bad value as a warning if it cares to.
func ParseMode(s string) Mode {
	switch s {
	case "", "human", "h":
		return ModeHuman
	case "json", "j":
		return ModeJSON
	case "quiet", "q":
		return ModeQuiet
	default:
		return ModeHuman
	}
}

// Printer is the output sink for command results.
//
//   - Human: the default. Human-readable text on stdout, leading "tyche:"
//     banners on stderr. Suitable for a terminal.
//   - JSON: machine-readable. Result values are written as JSON to stdout;
//     info messages are JSON lines on stderr; errors are problem+json lines
//     on stderr. Suitable for `| jq`.
//   - Quiet: only the requested data goes to stdout, no decorations. Info
//     messages and error context are dropped. Use it in CI and scripts.
//
// Every command's Run method receives a Printer via its struct, so command
// code stays free of formatting decisions.
type Printer interface {
	// Info writes an informational message. In human mode this is a
	// "tyche: <msg>" line to stderr. In JSON mode it is a JSON line to
	// stderr. In quiet mode it is dropped.
	Info(msg string, args ...any)

	// Result writes a successful result. In human mode it is rendered as
	// text; in JSON mode it is emitted as JSON; in quiet mode only a
	// string result is written verbatim.
	Result(v any) error

	// Error writes an error. The error is rendered with one-line
	// "tyche: error: <msg>" form in human mode, and as a problem+json
	// object in JSON mode. In quiet mode it is dropped — the caller still
	// exits non-zero. The exit-code decision is the caller's responsibility.
	Error(err error)
}

// New returns a Printer for the given mode, writing to stdout/stderr. The
// default is Human if mode is unknown.
func New(mode Mode, stdout, stderr io.Writer) Printer {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	switch mode {
	case ModeJSON:
		return &jsonPrinter{out: stdout, err: stderr}
	case ModeQuiet:
		return &quietPrinter{out: stdout}
	default:
		return &humanPrinter{out: stdout, err: stderr}
	}
}

// --- human printer -------------------------------------------------------

type humanPrinter struct {
	out io.Writer
	err io.Writer
}

func (p *humanPrinter) Info(msg string, args ...any) {
	_, _ = fmt.Fprintf(p.err, "tyche: "+msg+"\n", args...)
}

func (p *humanPrinter) Result(v any) error { return writeHuman(p.out, v) }

func (p *humanPrinter) Error(err error) {
	_, _ = fmt.Fprintf(p.err, "tyche: error: %s\n", err.Error())
}

func writeHuman(out io.Writer, v any) error {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case string:
		_, err := fmt.Fprintln(out, x)
		return err
	case error:
		_, err := fmt.Fprintln(out, x.Error())
		return err
	case fmt.Stringer:
		_, err := fmt.Fprintln(out, x.String())
		return err
	}
	// Commands must hand human mode a string/error/Stringer, not an
	// arbitrary struct or map — the "%v" Go-syntax rendering that would
	// produce (e.g. "map[out:./client]") is not user-facing output. Callers
	// that need structured data should select --format=json instead.
	return fmt.Errorf("output: cannot render %T in human mode (use --format=json)", v)
}

// --- json printer --------------------------------------------------------

type jsonPrinter struct {
	out io.Writer
	err io.Writer
}

func (p *jsonPrinter) Info(msg string, args ...any) {
	_ = encodeLine(p.err, map[string]any{"level": "info", "message": fmt.Sprintf(msg, args...)})
}

func (p *jsonPrinter) Result(v any) error {
	if v == nil {
		return nil
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode json result: %w", err)
	}
	_, err = fmt.Fprintln(p.out, string(data))
	return err
}

func (p *jsonPrinter) Error(err error) {
	_ = encodeLine(p.err, map[string]any{
		"type":   "about:blank",
		"title":  "error",
		"detail": err.Error(),
	})
}

func encodeLine(w io.Writer, body map[string]any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

// --- quiet printer -------------------------------------------------------

type quietPrinter struct {
	out io.Writer
}

func (p *quietPrinter) Info(string, ...any) {}

func (p *quietPrinter) Result(v any) error {
	// Quiet mode: only the requested data, no decorations. Strings are
	// printed verbatim; everything else is dropped — callers that need
	// machine output should use --output=json instead.
	if s, ok := v.(string); ok && s != "" {
		_, err := fmt.Fprintln(p.out, s)
		return err
	}
	return nil
}

func (p *quietPrinter) Error(error) {}
