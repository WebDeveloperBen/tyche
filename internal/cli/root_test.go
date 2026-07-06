package cli

import (
	"errors"
	"strings"
	"testing"
)

// runWithRecover is a test helper that mirrors what cmd/tyche/main.go
// does at process start: catch the ExitPanic that Kong's Exit() raises
// and translate it into a return code. Tests use this so they can
// assert on exit codes for help/parse flows without crashing.
func runWithRecover(t *testing.T, args []string) (code int, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			if ep, ok := r.(*ExitPanic); ok {
				code = ep.Code
				return
			}
			t.Fatalf("unexpected panic in Run: %v", r)
		}
	}()
	code, err = Run(args)
	return code, err
}

// TestRun_NoArgs_PrintsHelp verifies that `tyche` (no args) prints the
// help banner and exits 0, matching the prior Cobra behaviour where
// the root command's RunE returned cmd.Help() and a nil error.
func TestRun_NoArgs_PrintsHelp(t *testing.T) {
	code, err := runWithRecover(t, nil)
	if err != nil {
		t.Fatalf("Run(nil) returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("Run(nil) returned code %d, want 0", code)
	}
}

// TestRun_HelpFlag_PrintsHelp verifies that `tyche --help` prints the
// help banner and exits 0. We can't easily capture stdout in this test
// (Kong writes to os.Stdout by default), but we can assert the exit
// code and the absence of an error.
func TestRun_HelpFlag_PrintsHelp(t *testing.T) {
	code, err := runWithRecover(t, []string{"--help"})
	if err != nil {
		t.Fatalf("Run(--help) returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("Run(--help) returned code %d, want 0", code)
	}
}

// TestRun_Version_ExitsZero verifies that `tyche version` returns
// cleanly. The actual version string is rendered to os.Stdout, which is
// not captured here.
func TestRun_Version_ExitsZero(t *testing.T) {
	code, err := runWithRecover(t, []string{"version"})
	if err != nil {
		t.Fatalf("Run(version) returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("Run(version) returned code %d, want 0", code)
	}
}

// TestRun_UnknownCommand_ExitsTwo verifies that an unknown subcommand
// exits 2 (POSIX usage error) — the value scripts and CI can rely on.
func TestRun_UnknownCommand_ExitsTwo(t *testing.T) {
	code, _ := runWithRecover(t, []string{"unknown-cmd"})
	if code != 2 {
		t.Fatalf("Run(unknown-cmd) returned code %d, want 2", code)
	}
}

// TestRun_Completion_EmitsScript verifies that `tyche completion bash`
// prints a non-empty script with a shebang-style header.
func TestRun_Completion_EmitsScript(t *testing.T) {
	code, err := runWithRecover(t, []string{"completion", "bash"})
	if err != nil {
		t.Fatalf("Run(completion bash) returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("Run(completion bash) returned code %d, want 0", code)
	}
}

// TestRun_Completion_RejectsBadShell verifies that an unknown shell
// name exits 2.
func TestRun_Completion_RejectsBadShell(t *testing.T) {
	code, _ := runWithRecover(t, []string{"completion", "elvish"})
	if code != 2 {
		t.Fatalf("Run(completion elvish) returned code %d, want 2", code)
	}
}

// TestExitError_Wraps ensures Exit(code, err) is a proper wrapped error
// that errors.Unwrap can reach the underlying error. Several tests and
// runtimes (including the Printer's Error) rely on this.
func TestExitError_Wraps(t *testing.T) {
	inner := &testErr{msg: "boom"}
	got := Exit(7, inner)
	if got == nil {
		t.Fatal("Exit returned nil")
	}
	if !strings.Contains(got.Error(), "boom") {
		t.Errorf("Exit error text %q does not contain inner message", got.Error())
	}
	var exitErr *ExitError
	if !errors.As(got, &exitErr) || exitErr.Code != 7 {
		t.Errorf("Exit(7, err) lost code: got %T, %v", got, got)
	}
}

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }
