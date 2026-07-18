package foley

import (
	"runtime/debug"
	"strings"
)

// foley IS the terminal (ADR-021): a real emulator does not ask which
// terminal it is running inside — it declares itself. TerminalEnv
// takes an INHERITED environment, scrubs the host terminal's identity
// out of it, and declares foley's own. Without this, the same tape
// records different bytes depending on the terminal it was launched
// from (a fastfetch in a demo would name YOUR terminal), which is
// exactly what the determinism thesis forbids.

// hostTermVars are exact variable names owned by whatever terminal
// foley itself runs inside — never by the recording.
//
//nolint:gochecknoglobals // immutable scrub table
var hostTermVars = map[string]bool{
	"TERM":                 true,
	"COLORTERM":            true,
	"TERM_PROGRAM":         true,
	"TERM_PROGRAM_VERSION": true,
	"TERM_SESSION_ID":      true,
	"TERMINFO":             true,
	"TERMINFO_DIRS":        true,
	"TMUX":                 true,
	"TMUX_PANE":            true,
	"VTE_VERSION":          true,
}

// hostTermPrefixes catch the terminal families' own variable sets.
//
//nolint:gochecknoglobals // immutable scrub table
var hostTermPrefixes = []string{
	"KITTY_", "GHOSTTY_", "WEZTERM_", "ITERM_", "ALACRITTY_",
	"KONSOLE_", "WARP_", "WT_", "GNOME_TERMINAL_",
}

// TerminalEnv makes an inherited environment the one foley's terminal
// presents to the recorded application: the host terminal's identity
// scrubbed, foley's declared. Explicit layers (a tape's Env, the shell
// table, -env) merge ON TOP and still win — this is the BASE, not a
// veto. Options.Env == nil uses it automatically; callers building an
// explicit Env should start from TerminalEnv(os.Environ()).
func TerminalEnv(inherited []string) []string {
	out := make([]string, 0, len(inherited)+4)
	for _, kv := range inherited {
		k, _, ok := strings.Cut(kv, "=")
		if !ok || hostTermVars[k] {
			continue
		}
		host := false
		for _, p := range hostTermPrefixes {
			if strings.HasPrefix(k, p) {
				host = true
				break
			}
		}
		if !host {
			out = append(out, kv)
		}
	}
	return append(out,
		// What the embedded engine actually implements and the raster
		// actually delivers — declared, not inherited.
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"TERM_PROGRAM=foley",
		"TERM_PROGRAM_VERSION="+foleyVersion(),
	)
}

// foleyVersion reports the module version baked by the Go toolchain —
// a real tag for module installs, "dev" for local builds. The library
// keeps no mutable global for the CLI to inject one.
func foleyVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
