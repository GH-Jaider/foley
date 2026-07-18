package tape_test

import (
	"context"
	"strings"
	"testing"

	"github.com/GH-Jaider/foley/tape"
)

// TestAnyShellFallback pins #760: a shell outside VHS's table parses,
// warns LOUDLY about Wait patterns, and proceeds to launch bare — here
// dying honestly at PATH lookup because the binary does not exist.
func TestAnyShellFallback(t *testing.T) {
	tp, err := tape.Parse("Output d.gif\nSet Shell foleyprobeshell\nType \"x\"\n")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := tape.Run(context.Background(), tp, tape.RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "foleyprobeshell") {
		t.Fatalf("run err = %v, want the missing binary named", err)
	}
	found := false
	for _, w := range rep.Warnings {
		if strings.Contains(w, "not in foley's table") && strings.Contains(w, "Wait") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the bare-launch warning is missing: %v", rep.Warnings)
	}
}

// TestExtraEnv pins #621: -env pairs win over the tape's own Env in
// the merged child environment, and a CLI prompt variable warns that
// it does NOT retune bare Wait.
func TestExtraEnv(t *testing.T) {
	merged := tape.MergeEnvForTest(
		[]string{"A=os", "SECRET=old"},
		[]string{"A=tape"},
		[]string{"SECRET=real", "TOKEN=t1"},
	)
	got := map[string]bool{}
	for _, kv := range merged {
		got[kv] = true
	}
	if !got["A=tape"] || !got["SECRET=real"] || !got["TOKEN=t1"] {
		t.Fatalf("merged env = %v", merged)
	}

	tp, err := tape.Parse("Output d.gif\nSet Shell foleyprobeshell\nType \"x\"\n")
	if err != nil {
		t.Fatal(err)
	}
	rep, _ := tape.Run(context.Background(), tp, tape.RunOptions{ExtraEnv: []string{"PS1=❯ "}})
	found := false
	for _, w := range rep.Warnings {
		if strings.Contains(w, "-env PS1") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the -env PS1 warning is missing: %v", rep.Warnings)
	}
}
