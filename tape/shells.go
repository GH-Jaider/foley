package tape

// shell is a Set Shell target: the exact invocation and environment VHS
// uses, prompt included — the default WaitPattern (`>$`) matches that
// prompt, so this table is functional, not cosmetic.
type shell struct {
	// generic marks a shell outside the table: launched bare, prompt
	// unknown — the executor warns about Wait patterns.
	generic bool
	command []string
	env     []string
}

// shells reproduces VHS's shell table (shell.go of the pinned release,
// MIT) verbatim: a migrated tape must meet the same prompt on the same
// flags. Update by re-reading upstream when the vendor pin moves.
func shellFor(name string) (shell, error) {
	switch name {
	case "bash":
		// VHS disables history outright (+o history); foley keeps
		// SESSION history — it only contains what the tape typed, so
		// it is deterministic, and Up recalls the demo's own commands
		// like a real terminal. HISTFILE stays empty (nothing personal
		// read or written) and histexpand is OFF: with a live history
		// list, a typed `!1` would otherwise silently EXPAND against
		// the session (review finding, probe-confirmed). Literal `!`
		// also beats VHS's "event not found" error.
		return shell{
			env:     []string{"PS1=\\[\\e[38;2;90;86;224m\\]> \\[\\e[0m\\]", "BASH_SILENCE_DEPRECATION_WARNING=1", "HISTFILE="},
			command: []string{"bash", "--noprofile", "--norc", "--login", "+o", "histexpand"},
		}, nil
	case "zsh":
		return shell{
			env:     []string{`PROMPT=%F{#5B56E0}> %F{reset_color}`},
			command: []string{"zsh", "--histnostore", "--no-rcs"},
		}, nil
	case "fish":
		return shell{
			command: []string{
				"fish", "--login", "--no-config", "--private",
				"-C", "function fish_greeting; end",
				"-C", `function fish_prompt; set_color 5B56E0; echo -n "> "; set_color normal; end`,
			},
		}, nil
	case "nu":
		return shell{
			command: []string{"nu", "--execute", "$env.PROMPT_COMMAND = {'\033[;38;2;91;86;224m>\033[m '}; $env.PROMPT_COMMAND_RIGHT = {''}"},
		}, nil
	case "osh":
		return shell{
			env:     []string{"PS1=\\[\\e[38;2;90;86;224m\\]> \\[\\e[0m\\]"},
			command: []string{"osh", "--norc"},
		}, nil
	case "xonsh":
		return shell{
			command: []string{"xonsh", "--no-rc", "-D", "PROMPT=\033[;38;2;91;86;224m>\033[m "},
		}, nil
	case "powershell":
		return shell{
			command: []string{
				"powershell", "-NoLogo", "-NoExit", "-NoProfile", "-Command",
				`Set-PSReadLineOption -HistorySaveStyle SaveNothing; function prompt { Write-Host '>' -NoNewLine -ForegroundColor Blue; return ' ' }`,
			},
		}, nil
	case "pwsh":
		return shell{
			command: []string{
				"pwsh", "-Login", "-NoLogo", "-NoExit", "-NoProfile", "-Command",
				`Set-PSReadLineOption -HistorySaveStyle SaveNothing; Function prompt { Write-Host -ForegroundColor Blue -NoNewLine '>'; return ' ' }`,
			},
		}, nil
	case "cmd":
		return shell{command: []string{"cmd.exe", "/k", "prompt=^> "}}, nil
	default:
		// Any other shell launches BARE (#760): no pinned prompt, no
		// rc suppression — foley knows nothing about it. The caller
		// warns (the default `>` Wait pattern will not match; the tape
		// should Wait /regex/ or set Env PS1) instead of refusing: the
		// recording still works, honestly degraded.
		return shell{command: []string{name}, generic: true}, nil
	}
}
