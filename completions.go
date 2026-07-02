package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// periodTokens are all valid --period values, derived from the same tables
// the CLI validates against so completions can't drift.
func periodTokens() string {
	set := map[string]bool{"standup": true}
	maps.Copy(set, map[string]bool{})
	for k := range presets {
		set[k] = true
	}
	for k := range rollingDays {
		set[k] = true
	}
	return strings.Join(slices.Sorted(maps.Keys(set)), " ")
}

const formatTokens = "term md json html"

// completionScript returns the completion script for the given shell.
// The bash function is named _git_recap on purpose: git's own bash completion
// looks up that name, so `git recap <TAB>` completes too.
func completionScript(shell string) (string, error) {
	switch shell {
	case "bash":
		return fmt.Sprintf(bashCompletion, periodTokens(), formatTokens), nil
	case "zsh":
		return fmt.Sprintf(zshCompletion, periodTokens(), formatTokens), nil
	case "fish":
		return fmt.Sprintf(fishCompletion, periodTokens(), formatTokens), nil
	default:
		return "", fmt.Errorf("usage: git-recap completion bash|zsh|fish")
	}
}

func runCompletion(shell string) error {
	s, err := completionScript(shell)
	if err != nil {
		return err
	}
	fmt.Print(s)
	return nil
}

const bashCompletion = `# bash completion for git-recap. Load with:
#   source <(git-recap completion bash)
_git_recap() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  case "$prev" in
    --period) COMPREPLY=($(compgen -W "%s" -- "$cur")); return ;;
    --format) COMPREPLY=($(compgen -W "%s" -- "$cur")); return ;;
    --recaps-folder) COMPREPLY=($(compgen -d -- "$cur")); return ;;
  esac
  if [[ $COMP_CWORD -eq 1 && "$cur" != -* ]]; then
    COMPREPLY=($(compgen -W "config index completion version help" -- "$cur"))
    return
  fi
  COMPREPLY=($(compgen -W "--period --from --to --profile --org --repo --pick --fetch --diffstat --format --write --recaps-folder --frontmatter -i" -- "$cur"))
}
complete -F _git_recap git-recap
`

const zshCompletion = `#compdef git-recap
# zsh completion for git-recap. Install with:
#   git-recap completion zsh > "${fpath[1]}/_git-recap" && compinit
_git_recap() {
  _arguments \
    '1: :(config index completion version help)' \
    '--period[period preset]:period:(%s)' \
    '--format[stdout format]:format:(%s)' \
    '--from[range start YYYY-MM-DD]:date:' \
    '--to[range end YYYY-MM-DD]:date:' \
    '--profile[profile to use]:name:' \
    '--org[only these orgs]:orgs:' \
    '--repo[only these repo names]:repos:' \
    '--pick[fuzzy-pick repos]' \
    '--fetch[git fetch each repo first]' \
    '--diffstat[include files changed and +/− lines]' \
    '--write[save the recap to the recaps folder]' \
    '--recaps-folder[recaps folder for this run]:folder:_files -/' \
    '--frontmatter[add YAML frontmatter to markdown]' \
    '-i[interactive builder]'
}
_git_recap "$@"
`

const fishCompletion = `# fish completion for git-recap. Install with:
#   git-recap completion fish > ~/.config/fish/completions/git-recap.fish
complete -c git-recap -f
complete -c git-recap -n __fish_use_subcommand -a "config index completion version help"
complete -c git-recap -l period -x -a "%s"
complete -c git-recap -l format -x -a "%s"
complete -c git-recap -l from -x
complete -c git-recap -l to -x
complete -c git-recap -l profile -x
complete -c git-recap -l org -x
complete -c git-recap -l repo -x
complete -c git-recap -l pick
complete -c git-recap -l fetch
complete -c git-recap -l diffstat
complete -c git-recap -l write
complete -c git-recap -l recaps-folder -r -a "(__fish_complete_directories)"
complete -c git-recap -l frontmatter
complete -c git-recap -s i
`
