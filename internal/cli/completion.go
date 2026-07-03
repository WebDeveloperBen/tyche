package cli

import (
	"fmt"
	"io"
)

// CompletionCmd is `tyche completion <shell>`. It prints a shell
// completion script for the requested shell to stdout, suitable for
// `tyche completion bash > /etc/bash_completion.d/tyche` or
// `tyche completion zsh > "${fpath[1]}/_tyche"`.
type CompletionCmd struct {
	Shell string `arg:"" enum:"bash,zsh,fish,powershell" help:"Shell to generate completion for."`
}

// completionScripts maps a shell name to its completion script. The keys are
// exactly the enum values on CompletionCmd.Shell, so the `enum:` tag guarantees
// c.Shell is always present here — no default/fallback branch is reachable.
var completionScripts = map[string]string{
	"bash":       bashCompletionHeader + bashCompletionBody,
	"zsh":        zshCompletionScript,
	"fish":       fishCompletionScript,
	"powershell": powershellCompletionScript,
}

func (c *CompletionCmd) Run(g *GlobalFlags) error {
	script, ok := completionScripts[c.Shell]
	if !ok {
		// Unreachable while the enum and completionScripts keys agree; kept
		// as a defensive guard rather than a silent empty write.
		return Exit(2, fmt.Errorf("unsupported shell: %s", c.Shell))
	}
	_, err := io.WriteString(g.stdout(), script)
	return err
}

// completionSubcommands is the list of top-level verbs the completion scripts
// offer. It is asserted against the live Kong command tree by a test so a new
// subcommand cannot silently desync the scripts below.
var completionSubcommands = []string{
	"init", "config", "generate", "clean", "build",
	"run", "test", "client", "version", "completion", "help",
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

// zshCompletionScript is a minimal zsh completion entry. Kong has no built-in
// zsh generator, so we ship a hand-rolled #compdef block that delegates to a
// bash-style function via bashcompinit. Good enough to start; replace with a
// proper _tyche zsh script if the project grows.
const zshCompletionScript = `#compdef tyche
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
`

const fishCompletionScript = `# fish completion for tyche
complete -c tyche -n "__fish_use_subcommand" -a "init config generate clean build run test client version completion help"
complete -c tyche -n "__fish_seen_subcommand_from config" -a "show"
complete -c tyche -n "__fish_seen_subcommand_from completion" -a "bash zsh fish powershell"
`

const powershellCompletionScript = `# PowerShell completion for tyche
using namespace System.Management.Automation
Register-ArgumentCompleter -Native -CommandName 'tyche' -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $subcommands = @('init', 'config', 'generate', 'clean', 'build', 'run', 'test', 'client', 'version', 'completion', 'help')
    $subcommands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`
