package main

import (
	"strings"
	"testing"
)

// TestCompletionScripts pins the completion door: each shell gets a
// non-empty script carrying its structural marker, and the errors are
// loud. The scripts' SYNTAX is validated by the lint gate (`make
// lint`), which pipes each one through its real shell's dry-run parser
// — a test here cannot: external binaries only run through execx
// (depguard), and bash/zsh are not product tools.
func TestCompletionScripts(t *testing.T) {
	cases := []struct{ shell, marker string }{
		{"bash", "complete -o default -F _foley foley"},
		{"zsh", "#compdef foley"},
		{"fish", "complete -c foley"},
	}
	for _, c := range cases {
		t.Run(c.shell, func(t *testing.T) {
			exit, stdout, stderr := cli([]string{"completion", c.shell}, "")
			if exit != 0 {
				t.Fatalf("exit = %d, stderr = %q", exit, stderr)
			}
			if !strings.Contains(stdout, c.marker) {
				t.Fatalf("script lacks %q:\n%s", c.marker, stdout)
			}
			// Every subcommand completes in every shell — a new
			// subcommand must reach the scripts, loudly.
			for _, sub := range []string{"play", "validate", "sew", "wardrobe", "completion"} {
				if !strings.Contains(stdout, sub) {
					t.Fatalf("%s script does not know the %q subcommand", c.shell, sub)
				}
			}
		})
	}

	t.Run("errors", func(t *testing.T) {
		if exit, _, stderr := cli([]string{"completion", "powershell"}, ""); exit != 2 || !strings.Contains(stderr, "unknown shell") {
			t.Fatalf("unknown shell: exit %d, stderr %q", exit, stderr)
		}
		if exit, _, stderr := cli([]string{"completion"}, ""); exit != 2 || !strings.Contains(stderr, "usage:") {
			t.Fatalf("no shell: exit %d, stderr %q", exit, stderr)
		}
	})
}
