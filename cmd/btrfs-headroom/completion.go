// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"fmt"
	"io"
)

func runCompletion(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		completionUsage(stderr)
		return 64
	}

	script, ok := completionScripts[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "btrfs-headroom: unsupported completion shell %q\n", args[0])
		completionUsage(stderr)
		return 64
	}
	if _, err := io.WriteString(stdout, script); err != nil {
		fmt.Fprintf(stderr, "btrfs-headroom: write completion: %v\n", err)
		return 74
	}
	return 0
}

func completionUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: btrfs-headroom completion bash|zsh|fish")
}

var completionScripts = map[string]string{
	"bash": bashCompletion,
	"zsh":  zshCompletion,
	"fish": fishCompletion,
}

const bashCompletion = `# bash completion for btrfs-headroom

_btrfs_headroom_complete_values()
{
    local prefix=$1
    local current=$2
    local values=$3
    local value

    while IFS= read -r value; do
        COMPREPLY+=("${prefix}${value}")
    done < <(compgen -W "$values" -- "$current")
}

_btrfs_headroom_complete_paths()
{
    local kind=$1
    local prefix=$2
    local current=$3
    local candidate

    if [[ $kind == directory ]]; then
        while IFS= read -r candidate; do
            COMPREPLY+=("${prefix}${candidate}")
        done < <(compgen -d -- "$current")
    else
        while IFS= read -r candidate; do
            COMPREPLY+=("${prefix}${candidate}")
        done < <(compgen -f -- "$current")
    fi
    compopt -o filenames 2>/dev/null || true
}

_btrfs_headroom()
{
    local cur=
    local prev=
    local command=
    local command_index=0
    local formats=
    local options=
    local i

    COMPREPLY=()
    cur=${COMP_WORDS[COMP_CWORD]}
    if (( COMP_CWORD > 0 )); then
        prev=${COMP_WORDS[COMP_CWORD-1]}
    fi

    for ((i = 1; i < COMP_CWORD; i++)); do
        case ${COMP_WORDS[i]} in
            scan|check|guard|completion|help|version)
                command=${COMP_WORDS[i]}
                command_index=$i
                break
                ;;
        esac
    done

    if [[ -z $command ]]; then
        if [[ $cur == -* ]]; then
            _btrfs_headroom_complete_values "" "$cur" \
                "-h --help -version --version"
        else
            _btrfs_headroom_complete_values "" "$cur" \
                "scan check guard completion help version"
        fi
        return
    fi

    case $command in
        completion)
            _btrfs_headroom_complete_values "" "$cur" "bash zsh fish"
            return
            ;;
        help|version)
            return
            ;;
        scan)
            formats="human json prometheus"
            options="--format --output --mountinfo"
            ;;
        check)
            formats="human json nagios prometheus"
            options="--format --output --mountinfo"
            ;;
        guard)
            formats="human json nagios prometheus"
            options="--format --fail-at --unknown --mountinfo"
            ;;
    esac

    case $prev in
        --format)
            _btrfs_headroom_complete_values "" "$cur" "$formats"
            return
            ;;
        --fail-at)
            _btrfs_headroom_complete_values "" "$cur" "warning critical"
            return
            ;;
        --unknown)
            _btrfs_headroom_complete_values "" "$cur" "block allow"
            return
            ;;
        --output|--mountinfo)
            _btrfs_headroom_complete_paths file "" "$cur"
            return
            ;;
    esac

    case $cur in
        --format=*)
            _btrfs_headroom_complete_values "--format=" "${cur#*=}" "$formats"
            return
            ;;
        --fail-at=*)
            _btrfs_headroom_complete_values "--fail-at=" "${cur#*=}" \
                "warning critical"
            return
            ;;
        --unknown=*)
            _btrfs_headroom_complete_values "--unknown=" "${cur#*=}" \
                "block allow"
            return
            ;;
        --output=*)
            _btrfs_headroom_complete_paths file "--output=" "${cur#*=}"
            return
            ;;
        --mountinfo=*)
            _btrfs_headroom_complete_paths file "--mountinfo=" "${cur#*=}"
            return
            ;;
    esac

    for ((i = command_index + 1; i < COMP_CWORD; i++)); do
        if [[ ${COMP_WORDS[i]} == -- ]]; then
            _btrfs_headroom_complete_paths directory "" "$cur"
            return
        fi
    done

    if [[ $cur == -* ]]; then
        _btrfs_headroom_complete_values "" "$cur" "$options"
    else
        _btrfs_headroom_complete_paths directory "" "$cur"
    fi
}

complete -F _btrfs_headroom btrfs-headroom
`

const zshCompletion = `#compdef btrfs-headroom

_btrfs-headroom()
{
    local context state state_descr line
    typeset -A opt_args
    local -a commands

    commands=(
        'scan:inspect Btrfs allocator headroom'
        'check:inspect headroom and return a health exit code'
        'guard:run a read-only preflight gate'
        'completion:generate shell completion code'
        'help:show command usage'
        'version:print the program version'
    )

    _arguments -C \
        '(-h --help)-h[show command usage]' \
        '(-h --help)--help[show command usage]' \
        '-version[print the program version]' \
        '--version[print the program version]' \
        '1:command:->command' \
        '*::argument:->args'

    case $state in
        command)
            _describe 'command' commands
            ;;
        args)
            case $line[1] in
                scan)
                    _arguments \
                        '--format=[select output format]:format:(human json prometheus)' \
                        '--output=[write output atomically to a file]:output file:_files' \
                        '--mountinfo=[read mount discovery state from a file]:mountinfo file:_files' \
                        '*:mount point:_directories'
                    ;;
                check)
                    _arguments \
                        '--format=[select output format]:format:(human json nagios prometheus)' \
                        '--output=[write output atomically to a file]:output file:_files' \
                        '--mountinfo=[read mount discovery state from a file]:mountinfo file:_files' \
                        '*:mount point:_directories'
                    ;;
                guard)
                    _arguments \
                        '--format=[select output format]:format:(human json nagios prometheus)' \
                        '--fail-at=[select blocking health threshold]:threshold:(warning critical)' \
                        '--unknown=[select incomplete observation policy]:policy:(block allow)' \
                        '--mountinfo=[read mount discovery state from a file]:mountinfo file:_files' \
                        '*:mount point:_directories'
                    ;;
                completion)
                    _values 'shell' bash zsh fish
                    ;;
            esac
            ;;
    esac
}

_btrfs-headroom "$@"
`

const fishCompletion = `# fish completion for btrfs-headroom

function __fish_btrfs_headroom_needs_command
    not __fish_seen_subcommand_from scan check guard completion help version
end

complete -c btrfs-headroom -n __fish_btrfs_headroom_needs_command -f \
    -a scan -d 'Inspect Btrfs allocator headroom'
complete -c btrfs-headroom -n __fish_btrfs_headroom_needs_command -f \
    -a check -d 'Inspect headroom and return a health exit code'
complete -c btrfs-headroom -n __fish_btrfs_headroom_needs_command -f \
    -a guard -d 'Run a read-only preflight gate'
complete -c btrfs-headroom -n __fish_btrfs_headroom_needs_command -f \
    -a completion -d 'Generate shell completion code'
complete -c btrfs-headroom -n __fish_btrfs_headroom_needs_command -f \
    -a help -d 'Show command usage'
complete -c btrfs-headroom -n __fish_btrfs_headroom_needs_command -f \
    -a version -d 'Print the program version'
complete -c btrfs-headroom -n __fish_btrfs_headroom_needs_command \
    -s h -l help -d 'Show command usage'
complete -c btrfs-headroom -n __fish_btrfs_headroom_needs_command \
    -l version -d 'Print the program version'
complete -c btrfs-headroom -n __fish_btrfs_headroom_needs_command \
    -o version -d 'Print the program version'

complete -c btrfs-headroom -n '__fish_seen_subcommand_from scan' \
    -l format -r -f -a 'human json prometheus' -d 'Select output format'
complete -c btrfs-headroom -n '__fish_seen_subcommand_from check guard' \
    -l format -r -f -a 'human json nagios prometheus' -d 'Select output format'
complete -c btrfs-headroom -n '__fish_seen_subcommand_from scan check' \
    -l output -r -F -d 'Write output atomically to a file'
complete -c btrfs-headroom -n '__fish_seen_subcommand_from scan check guard' \
    -l mountinfo -r -F -d 'Read mount discovery state from a file'
complete -c btrfs-headroom -n '__fish_seen_subcommand_from guard' \
    -l fail-at -r -f -a 'warning critical' -d 'Select blocking health threshold'
complete -c btrfs-headroom -n '__fish_seen_subcommand_from guard' \
    -l unknown -r -f -a 'block allow' -d 'Select incomplete observation policy'
complete -c btrfs-headroom -n '__fish_seen_subcommand_from completion' \
    -f -a 'bash zsh fish' -d 'Shell'
`
