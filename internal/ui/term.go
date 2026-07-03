// Package ui provides small terminal-interaction primitives the tyche CLI
// uses to detect TTY state and read single-line interactive input. The package
// is intentionally minimal — no TUI framework, no colour library, no escape
// codes — because every command that uses it is short-lived and not a
// long-running process.
package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// IsTerminal reports whether f is connected to a character device (a TTY).
// Commands that prompt should refuse to do so when this returns false so a
// non-interactive invocation in CI or a script does not hang on stdin.
func IsTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// Prompt is a tiny single-line input helper. It writes a question to Out and
// reads one trimmed line from In. The zero value is fine: it uses stdout and
// stdin.
//
// Prompt is deliberately not a TUI. If you find yourself wanting bubbles or
// huh here, you have probably outgrown this command and should be writing
// a script or using a real TUI.
type Prompt struct {
	Out io.Writer
	In  io.Reader
}

// Ask writes question to the configured output (stdout by default) and reads
// a single line of input, trimming trailing whitespace. The leading space in
// the prompt is intentional: it visually separates the question from the
// user's input.
func (p Prompt) Ask(question string) (string, error) {
	out := p.Out
	if out == nil {
		out = os.Stdout
	}
	in := p.In
	if in == nil {
		in = os.Stdin
	}
	if _, err := fmt.Fprint(out, question+" "); err != nil {
		return "", err
	}
	reader, ok := in.(io.RuneReader)
	if !ok {
		reader = bufio.NewReader(in)
	}
	line, err := readLine(reader)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return "", errors.New("no input")
		}
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// AskDefault is Ask with a fallback value: if the user enters an empty line,
// it returns default. The default is shown in the prompt so the user knows
// what they get by pressing enter.
func (p Prompt) AskDefault(question, fallback string) (string, error) {
	q := question
	if fallback != "" {
		q = fmt.Sprintf("%s [%s]", question, fallback)
	}
	answer, err := p.Ask(q)
	if err != nil {
		return "", err
	}
	if answer == "" {
		return fallback, nil
	}
	return answer, nil
}

// readLine reads one line from r, terminated by '\n' or EOF. The
// terminator is dropped from the returned string.
func readLine(r io.RuneReader) (string, error) {
	var b strings.Builder
	for {
		ch, _, err := r.ReadRune()
		if err != nil {
			if err == io.EOF && b.Len() > 0 {
				return b.String(), nil
			}
			return "", err
		}
		if ch == '\n' {
			return b.String(), nil
		}
		// Drop trailing \r so CRLF inputs round-trip cleanly.
		if ch == '\r' {
			continue
		}
		b.WriteRune(ch)
	}
}
