package foley

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTerminalEnv pins ADR-021: the host terminal's identity is
// scrubbed from the inherited environment, foley's is declared, and
// everything else passes through untouched.
func TestTerminalEnv(t *testing.T) {
	got := TerminalEnv([]string{
		"PATH=/usr/bin",
		"HOME=/Users/x",
		"TERM=xterm-kitty",
		"TERM_PROGRAM=ghostty",
		"TERM_PROGRAM_VERSION=1.2.3",
		"KITTY_WINDOW_ID=7",
		"GHOSTTY_RESOURCES_DIR=/x",
		"WEZTERM_PANE=3",
		"ITERM_SESSION_ID=w0",
		"TMUX=/private/tmp/x",
		"COLORTERM=24bit",
		"EDITOR=vim",
	})
	env := map[string]string{}
	for _, kv := range got {
		k, v, _ := strings.Cut(kv, "=")
		if old, dup := env[k]; dup {
			t.Fatalf("duplicate %s (%q and %q) — glibc takes the first, the scrub must not leave two", k, old, v)
		}
		env[k] = v
	}
	if env["PATH"] != "/usr/bin" || env["HOME"] != "/Users/x" || env["EDITOR"] != "vim" {
		t.Fatalf("non-terminal vars must pass through: %v", got)
	}
	if env["TERM"] != "xterm-ghostty" || env["COLORTERM"] != "truecolor" {
		t.Fatalf("TERM/COLORTERM not declared: %v", got)
	}
	if env["TERM_PROGRAM"] != "foley" || env["TERM_PROGRAM_VERSION"] == "" {
		t.Fatalf("identity not declared: %v", got)
	}
	// The declared TERM only exists because foley SHIPS its entry: the
	// env must carry TERMINFO and the entry must actually resolve there.
	tinfo := env["TERMINFO"]
	if tinfo == "" {
		t.Fatalf("TERMINFO not declared alongside xterm-ghostty: %v", got)
	}
	if _, err := os.Stat(filepath.Join(tinfo, "x", "xterm-ghostty")); err != nil {
		t.Fatalf("pinned entry not materialized: %v", err)
	}
	for _, gone := range []string{"KITTY_WINDOW_ID", "GHOSTTY_RESOURCES_DIR", "WEZTERM_PANE", "ITERM_SESSION_ID", "TMUX"} {
		if _, ok := env[gone]; ok {
			t.Fatalf("%s leaked through the scrub: %v", gone, got)
		}
	}
}
