package cli

import (
	"fmt"
	"strings"
)

const bashCompletionTemplate = `#!/usr/bin/env bash
# bash completion for tempest
_tempest_completion() {
    local cur prev words cword
    _init_completion -n : || return

    local commands="server workflows workflow runs config admin migrate version completion"
    local flags="--server --output --token --namespace"

    case "${prev}" in
        --output)
            COMPREPLY=( $(compgen -W "table json csv" -- "${cur}") )
            return 0
            ;;
        workflow|workflows)
            COMPREPLY=( $(compgen -W "list get submit inspect pause resume cancel" -- "${cur}") )
            return 0
            ;;
        runs)
            COMPREPLY=( $(compgen -W "list get cancel retry logs" -- "${cur}") )
            return 0
            ;;
        config)
            COMPREPLY=( $(compgen -W "show validate set" -- "${cur}") )
            return 0
            ;;
        admin)
            COMPREPLY=( $(compgen -W "token-mint queue-drain cluster-rebalance" -- "${cur}") )
            return 0
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh" -- "${cur}") )
            return 0
            ;;
    esac

    if [[ "${cur}" == -* ]]; then
        COMPREPLY=( $(compgen -W "${flags}" -- "${cur}") )
        return 0
    fi

    COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
}

complete -F _tempest_completion tempest
`

const zshCompletionTemplate = `#compdef tempest
# zsh completion for tempest

_tempest() {
    local -a commands
    commands=(
        'server:start workflow orchestration server'
        'workflows:manage workflow definitions'
        'workflow:manage workflow definitions'
        'runs:manage workflow runs'
        'config:manage configuration'
        'admin:administrative operations'
        'migrate:run schema migrations'
        'version:print version information'
        'completion:generate shell completion script'
    )

    _arguments -C \
        '--server[tempest server address]:address:' \
        '--output[output format]:format:(table json csv)' \
        '--token[authentication token]:token:' \
        '--namespace[workflow namespace]:namespace:' \
        '1: :->command' \
        '*:: :->args'

    case $state in
        command)
            _describe -t commands 'tempest command' commands
            ;;
        args)
            case $words[1] in
                workflow|workflows)
                    local -a wf_subcmds
                    wf_subcmds=(
                        'list:list workflows'
                        'get:get workflow details'
                        'submit:submit new workflow'
                        'inspect:inspect workflow run status'
                        'pause:pause running workflow'
                        'resume:resume paused workflow'
                        'cancel:cancel workflow run'
                    )
                    _describe -t wf_subcmds 'workflow command' wf_subcmds
                    ;;
                completion)
                    local -a shells
                    shells=('bash:generate bash completion' 'zsh:generate zsh completion')
                    _describe -t shells 'shell' shells
                    ;;
            esac
            ;;
    esac
}

_tempest "$@"
`

func completionCmd() *Command {
	return &Command{
		Name:    "completion",
		Summary: "generate shell completion script (bash, zsh)",
		Run: func(ctx *Context) error {
			if len(ctx.Args) == 0 {
				return fmt.Errorf("shell type required: bash or zsh")
			}
			shell := strings.ToLower(strings.TrimSpace(ctx.Args[0]))
			switch shell {
			case "bash":
				ctx.Printf("%s", bashCompletionTemplate)
				return nil
			case "zsh":
				ctx.Printf("%s", zshCompletionTemplate)
				return nil
			default:
				return fmt.Errorf("unsupported shell %q, must be 'bash' or 'zsh'", shell)
			}
		},
	}
}
