package main

import (
	"fmt"
	"io"
)

// Shell completions ship INSIDE the binary (`foley completion <shell>`
// prints the script): one source, versioned with the flags it
// completes, packaging-friendly (brew/goreleaser call the same
// command). Dynamic values (dress names) are resolved at completion
// time by calling foley itself — the script never carries a list that
// can drift.

const bashCompletion = `# bash completion for foley.
# Install for this shell and your ~/.bashrc:
#   eval "$(foley completion bash)"
_foley() {
	local cur prev
	cur="${COMP_WORDS[COMP_CWORD]}"
	prev="${COMP_WORDS[COMP_CWORD-1]}"
	local subs="play validate new sew doctor themes fonts wardrobe completion"
	local flags="-mode -studio -dir -env -cols -rows -dress -keys -theme -o -output-scale -gif-loop -watch -fonts -modify-other-keys -version -h"

	case "$prev" in
	-mode) COMPREPLY=($(compgen -W "deterministic realtime" -- "$cur")); return ;;
	-output-scale) COMPREPLY=($(compgen -W "1 2" -- "$cur")); return ;;
	-dress|-from) COMPREPLY=($(compgen -W "$(foley wardrobe 2>/dev/null | cut -d' ' -f1) none" -- "$cur")); return ;;
	-keys) COMPREPLY=($(compgen -W "off on small medium large plain" -- "$cur")); return ;;
	-dir|-fonts) COMPREPLY=($(compgen -d -- "$cur")); return ;;
	-theme|-env|-o|-gif-loop|-cols|-rows) return ;;
	esac

	if [ "$COMP_CWORD" -eq 1 ] && [ "${cur#-}" = "$cur" ]; then
		# Subcommand position: offer the subcommands; when none match,
		# the -o default fallback below completes tape files instead.
		COMPREPLY=($(compgen -W "$subs" -- "$cur"))
		return
	fi
	case "${COMP_WORDS[1]}" in
	completion) COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur")); return ;;
	wardrobe) COMPREPLY=($(compgen -W "$(foley wardrobe 2>/dev/null | cut -d' ' -f1)" -- "$cur")); return ;;
	sew) case "$cur" in -*) COMPREPLY=($(compgen -W "-from" -- "$cur"));; esac; return ;;
	doctor) COMPREPLY=($(compgen -W "-fonts" -- "$cur")); return ;;
	themes|fonts) return ;;
	esac
	case "$cur" in
	-*) COMPREPLY=($(compgen -W "$flags" -- "$cur")) ;;
	esac
}
complete -o default -F _foley foley
`

const zshCompletion = `#compdef foley
# zsh completion for foley.
# Install (any directory in $fpath, then restart the shell):
#   foley completion zsh > "${fpath[1]}/_foley"
# Or for this shell only:
#   eval "$(foley completion zsh)"; compdef _foley foley

_foley_dresses() {
	local -a names
	names=(${(f)"$(foley wardrobe 2>/dev/null | cut -d' ' -f1)"} none)
	_describe -t dresses 'dress' names
}

_foley() {
	local -a subs
	subs=(
		'play:watch the tape right here, in this terminal'
		'validate:the spotting session — lint + cue sheet, nothing records'
		'new:write a starter tape'
		'sew:make a dress to edit'
		'doctor:check fonts, engine and ffmpeg'
		'themes:list the theme catalog'
		'fonts:list the pinned font families'
		'wardrobe:list dresses, or expand one'
		'completion:print a shell completion script'
	)
	if (( CURRENT == 2 )); then
		_describe -t commands 'foley command' subs
		_files -g '*.tape'
		return
	fi
	case "$words[2]" in
	completion) _values 'shell' bash zsh fish ;;
	wardrobe) _foley_dresses ;;
	new) _files -g '*.tape' ;;
	themes|fonts) ;;
	sew)
		_arguments \
			'-from[start from an existing dress]:dress:_foley_dresses' \
			'*:name:'
		;;
	doctor)
		_arguments '-fonts[pinned fonts directory]:dir:_files -/'
		;;
	play)
		_arguments \
			'-mode[recording clock]:mode:(deterministic realtime)' \
			'-modify-other-keys[modern CSI-27 chords]' \
			'-fonts[pinned fonts directory]:dir:_files -/' \
			'-dress[replace the dress layer]:dress:_foley_dresses' \
			'-keys[replace the keys layer]:keys:' \
			'-theme[replace the theme]:theme:' \
			'*:tape:_files -g "*.tape"'
		;;
	*)
		_arguments \
			'-mode[recording clock]:mode:(deterministic realtime)' \
			'-studio[closed set — your machine stays off camera]' \
			'-dir[working directory for the tape'\''s command]:dir:_files -/' \
			'*-env[KEY=VALUE for the recording]:pair:' \
			'-cols[terminal grid columns]:cols:' \
			'-rows[terminal grid rows]:rows:' \
			'-dress[replace the dress layer]:dress:_foley_dresses' \
			'-keys[replace the keys layer]:keys:' \
			'-theme[record dark/light pairs from one tape]:theme:' \
			'*-o[output path, format by extension]:file:_files' \
			'-output-scale[2 retina, 1 logical]:scale:(1 2)' \
			'-gif-loop[gif loop count]:n:' \
			'-watch[re-record every time the tape is saved]' \
			'-fonts[pinned fonts directory]:dir:_files -/' \
			'-modify-other-keys[modern CSI-27 chords]' \
			'-version[print the foley version]' \
			'*:tape:_files -g "*.tape"'
		;;
	esac
}
_foley "$@"
`

const fishCompletion = `# fish completion for foley.
# Install:
#   foley completion fish > ~/.config/fish/completions/foley.fish
complete -c foley -n '__fish_use_subcommand' -a 'play' -d 'watch the tape right here, in this terminal'
complete -c foley -n '__fish_use_subcommand' -a 'validate' -d 'lint + cue sheet, nothing records'
complete -c foley -n '__fish_use_subcommand' -a 'new' -d 'write a starter tape'
complete -c foley -n '__fish_use_subcommand' -a 'sew' -d 'make a dress to edit'
complete -c foley -n '__fish_use_subcommand' -a 'doctor' -d 'check fonts, engine and ffmpeg'
complete -c foley -n '__fish_use_subcommand' -a 'themes' -d 'list the theme catalog'
complete -c foley -n '__fish_use_subcommand' -a 'fonts' -d 'list the pinned font families'
complete -c foley -n '__fish_use_subcommand' -a 'wardrobe' -d 'list dresses, or expand one'
complete -c foley -n '__fish_use_subcommand' -a 'completion' -d 'print a shell completion script'
complete -c foley -n '__fish_seen_subcommand_from completion' -f -a 'bash zsh fish'
complete -c foley -n '__fish_seen_subcommand_from wardrobe' -f -a '(foley wardrobe 2>/dev/null | cut -d" " -f1)'
complete -c foley -o mode -x -a 'deterministic realtime' -d 'recording clock'
complete -c foley -o studio -d 'closed set — your machine stays off camera'
complete -c foley -o dir -x -a '(__fish_complete_directories)' -d "working directory for the tape's command"
complete -c foley -o env -x -d 'KEY=VALUE for the recording (repeatable)'
complete -c foley -o cols -x -d 'terminal grid columns'
complete -c foley -o rows -x -d 'terminal grid rows'
complete -c foley -o dress -x -a '(foley wardrobe 2>/dev/null | cut -d" " -f1) none' -d 'replace the dress layer'
complete -c foley -o keys -x -a 'off on small medium large plain' -d 'replace the keys layer'
complete -c foley -o theme -x -d 'record dark/light pairs from one tape'
complete -c foley -o o -r -d 'output path, format by extension (repeatable)'
complete -c foley -o output-scale -x -a '1 2' -d '2 retina, 1 logical'
complete -c foley -o gif-loop -x -d 'gif loop count'
complete -c foley -o watch -d 're-record every time the tape is saved'
complete -c foley -o fonts -x -a '(__fish_complete_directories)' -d 'pinned fonts directory'
complete -c foley -o modify-other-keys -d 'modern CSI-27 chords'
complete -c foley -o version -d 'print the foley version'
complete -c foley -o from -x -n '__fish_seen_subcommand_from sew' -a '(foley wardrobe 2>/dev/null | cut -d" " -f1)' -d 'start from an existing dress'
`

// runCompletion prints the requested shell's completion script.
func runCompletion(args []string, stdout, stderr io.Writer) int {
	usage := func(w io.Writer) {
		_, _ = fmt.Fprint(w, "usage: foley completion bash|zsh|fish\n\n"+
			"Prints a shell completion script for subcommands, flags and their\n"+
			"values (dress names resolve live via `foley wardrobe`). Install:\n\n"+
			"  bash:  eval \"$(foley completion bash)\"   # and in ~/.bashrc\n"+
			"  zsh:   foley completion zsh > \"${fpath[1]}/_foley\"\n"+
			"  fish:  foley completion fish > ~/.config/fish/completions/foley.fish\n")
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "-help" || args[0] == "--help") {
		usage(stdout)
		return 0
	}
	if len(args) != 1 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "bash":
		_, _ = fmt.Fprint(stdout, bashCompletion)
	case "zsh":
		_, _ = fmt.Fprint(stdout, zshCompletion)
	case "fish":
		_, _ = fmt.Fprint(stdout, fishCompletion)
	default:
		_, _ = fmt.Fprintf(stderr, "foley: completion %q: unknown shell (bash|zsh|fish)\n", args[0])
		return 2
	}
	return 0
}
