package cli

import (
	"fmt"
	"io"

	"github.com/alecthomas/kong"
)

// CompletionCmd is `tyche completion <shell>`. It prints a shell
// completion script for the requested shell to stdout, suitable for
// `tyche completion bash > /etc/bash_completion.d/tyche` or
// `tyche completion zsh > "${fpath[1]}/_tyche"`.
type CompletionCmd struct {
	Shell string `arg:"" enum:"bash,zsh,fish,powershell" help:"Shell to generate completion for."`
}

func (c *CompletionCmd) Run(g *GlobalFlags) error {
	parser, err := kong.New(&CLI{}, kong.Name("tyche"))
	if err != nil {
		return Exit(2, fmt.Errorf("init parser: %w", err))
	}
	switch c.Shell {
	case "bash":
		return runBashCompletion(g.stdout(), parser)
	case "zsh":
		return runZshCompletion(g.stdout(), parser)
	case "fish":
		return runFishCompletion(g.stdout(), parser)
	case "powershell":
		return runPowershellCompletion(g.stdout(), parser)
	}
	return Exit(2, fmt.Errorf("unsupported shell: %s", c.Shell))
}

// runBashCompletion emits a small bash completion script. It is not as
// polished as cobra's generator — Kong does not include one — but it
// covers the surface (top-level subcommands + flag names) and is the
// reason completion is in the CLI at all.
func runBashCompletion(w io.Writer, _ *kong.Kong) error {
	script := bashCompletionHeader + bashCompletionBody
	_, err := io.WriteString(w, script)
	return err
}

const bashCompletionHeader = `# bash completion for tyche
# Source this file or drop it into /etc/bash_completion.d/tyche.
`

const bashCompletionBody = `
_tyche_completions() {
  local cur prev cmds
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  cmds="init config generate clean build run test client version completion help"
  if [[ ${COMP_CWORD} -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "${cmds}" -- "${cur}") )
    return 0
  fi
  case "${COMP_WORDS[1]}" in
    config)
      COMPREPLY=( $(compgen -W "show" -- "${cur}") )
      ;;
    completion)
      COMPREPLY=( $(compgen -W "bash zsh fish powershell" -- "${cur}") )
      ;;
  esac
  return 0
}
complete -F _tyche_completions tyche
`

// runZshCompletion emits a minimal zsh completion entry. Kong has no
// built-in zsh generator, so we ship a hand-rolled #compdef block that
// delegates to the bash function. Good enough to start; replace with a
// proper zsh script if the project grows.
func runZshCompletion(w io.Writer, _ *kong.Kong) error {
	_, err := io.WriteString(w, `#compdef tyche
# Minimal zsh completion: delegates to the bash function.
autoload -U +X bashcompinit && bashcompinit
_tyche_completions() {
  local cur prev cmds
  cur="${words[CURRENT]}"
  prev="${words[CURRENT-1]}"
  cmds="init config generate clean build run test client version completion help"
  if [[ ${CURRENT} -eq 2 ]]; then
    compadd -- ${cmds}
    return 0
  fi
  case "${words[2]}" in
    config) compadd -- show ;;
    completion) compadd -- bash zsh fish powershell ;;
  esac
  return 0
}
compdef _tyche_completions tyche
`)
	return err
}

// runFishCompletion emits a minimal fish completion script.
func runFishCompletion(w io.Writer, _ *kong.Kong) error {
	_, err := io.WriteString(w, `# fish completion for tyche
complete -c tyche -n "__fish_use_subcommand" -a "init config generate clean build run test client version completion help"
complete -c tyche -n "__fish_seen_subcommand_from config" -a "show"
complete -c tyche -n "__fish_seen_subcommand_from completion" -a "bash zsh fish powershell"
`)
	return err
}

// runPowershellCompletion emits a minimal PowerShell completion script.
func runPowershellCompletion(w io.Writer, _ *kong.Kong) error {
	_, err := io.WriteString(w, `# PowerShell completion for tyche
using namespace System.Management.Automation
Register-ArgumentCompleter -Native -CommandName 'tyche' -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $subcommands = @('init', 'config', 'generate', 'clean', 'build', 'run', 'test', 'client', 'version', 'completion', 'help')
    $subcommands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`)
	return err
}
