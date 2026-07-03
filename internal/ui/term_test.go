package ui

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestIsTerminal_FileInfoCheck(t *testing.T) {
	// /dev/null is a character device on macOS and Linux. We can use
	// it as a stand-in for a "device" file that is not a regular
	// file. isTerminal expects ModeCharDevice, which /dev/null has.
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = devNull.Close() })
	// /dev/null is a character device, so this should be true.
	if !IsTerminal(devNull) {
		t.Errorf("IsTerminal(%s) = false, want true (it is a char device)", os.DevNull)
	}
}

func TestIsTerminal_RegularFile(t *testing.T) {
	// A regular file (e.g. a temp file) is not a character device.
	f := t.TempDir() + "/file"
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fh, err := os.Open(f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fh.Close() })
	if IsTerminal(fh) {
		t.Errorf("IsTerminal on a regular file = true, want false")
	}
}

func TestPrompt_AskReadsLine(t *testing.T) {
	var out bytes.Buffer
	p := Prompt{Out: &out, In: strings.NewReader("  my answer  \n")}
	got, err := p.Ask("Module?")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if got != "my answer" {
		t.Errorf("Ask = %q, want %q", got, "my answer")
	}
	if !strings.Contains(out.String(), "Module?") {
		t.Errorf("prompt did not write question: %q", out.String())
	}
}

func TestPrompt_AskDefaultReturnsFallback(t *testing.T) {
	var out bytes.Buffer
	p := Prompt{Out: &out, In: strings.NewReader("\n")}
	got, err := p.AskDefault("Module?", "fallback")
	if err != nil {
		t.Fatalf("AskDefault: %v", err)
	}
	if got != "fallback" {
		t.Errorf("AskDefault on empty input = %q, want %q", got, "fallback")
	}
	if !strings.Contains(out.String(), "[fallback]") {
		t.Errorf("prompt did not show fallback hint: %q", out.String())
	}
}

func TestPrompt_AskDefaultReturnsAnswer(t *testing.T) {
	var out bytes.Buffer
	p := Prompt{Out: &out, In: strings.NewReader("typed-it\n")}
	got, err := p.AskDefault("Module?", "fallback")
	if err != nil {
		t.Fatalf("AskDefault: %v", err)
	}
	if got != "typed-it" {
		t.Errorf("AskDefault = %q, want %q", got, "typed-it")
	}
}

func TestPrompt_AskWithExplicitReaderWriter(t *testing.T) {
	// With both fields set to in-memory streams, Ask reads the line from
	// In and writes the question to Out — no stdio involved.
	var out bytes.Buffer
	p := Prompt{In: strings.NewReader("hello\n"), Out: &out}
	got, err := p.Ask("Q?")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if got != "hello" {
		t.Errorf("Ask = %q, want %q", got, "hello")
	}
}
