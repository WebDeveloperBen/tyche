package cli

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// TestCompletionSubcommands_MatchModel guards against completion-script drift:
// if a subcommand is added to or removed from the CLI struct, the hardcoded
// lists in completion.go must be updated too. "help" is allowed to appear in
// the scripts without a matching Kong command node (it is a convenience entry,
// not a real subcommand).
func TestCompletionSubcommands_MatchModel(t *testing.T) {
	parser, err := kong.New(&CLI{}, kong.Name("tyche"))
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}

	modelCmds := map[string]bool{}
	for _, child := range parser.Model.Children {
		if child.Type == kong.CommandNode {
			modelCmds[child.Name] = true
		}
	}

	listed := map[string]bool{}
	for _, name := range completionSubcommands {
		listed[name] = true
	}

	// Every real Kong command must be offered by the completion scripts.
	for name := range modelCmds {
		if !listed[name] {
			t.Errorf("subcommand %q exists in the CLI but is missing from completionSubcommands (update completion.go)", name)
		}
	}
	// Everything listed (except the "help" convenience entry) must be a real command.
	for name := range listed {
		if name == "help" {
			continue
		}
		if !modelCmds[name] {
			t.Errorf("completionSubcommands lists %q but it is not a CLI command (stale entry in completion.go)", name)
		}
	}

	// The bash/zsh scripts embed the same list as a space-joined string;
	// make sure that literal stays in sync with completionSubcommands too.
	joined := strings.Join(completionSubcommands, " ")
	if !strings.Contains(bashCompletionBody, joined) {
		t.Errorf("bash completion body does not contain the subcommand list %q", joined)
	}
	if !strings.Contains(zshCompletionScript, joined) {
		t.Errorf("zsh completion script does not contain the subcommand list %q", joined)
	}
}
