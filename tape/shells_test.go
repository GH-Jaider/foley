package tape

import (
	"slices"
	"testing"
)

// TestRunEnvPinsShell: the recorded world's login shell IS the tape's
// shell — $SHELL must point at the launched shell's resolved path, so
// tools that consult it (tmux default-shell, vim :terminal) stay
// deterministic across machines. The table's own vars survive
// alongside, and the pin never mutates the table.
func TestRunEnvPinsShell(t *testing.T) {
	sh, err := shellFor("bash")
	if err != nil {
		t.Fatal(err)
	}
	env := runEnv(sh, "/opt/homebrew/bin/bash")
	if !slices.Contains(env, "SHELL=/opt/homebrew/bin/bash") {
		t.Fatalf("env lacks the SHELL pin: %v", env)
	}
	if !slices.Contains(env, "HISTFILE=") {
		t.Fatalf("the table's own env must survive: %v", env)
	}
	again, err := shellFor("bash")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(again.env, "SHELL=/opt/homebrew/bin/bash") {
		t.Fatal("runEnv leaked the pin into the shell table")
	}
}
