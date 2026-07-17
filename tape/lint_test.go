package tape_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/GH-Jaider/foley"
	"github.com/GH-Jaider/foley/tape"
)

// TestLintReportsStaticWarningsWithoutRunning: the same staged-setting
// and chord-degradation warnings Run emits, available before any pty
// exists — and gated by the same options.
func TestLintReportsStaticWarningsWithoutRunning(t *testing.T) {
	tp, err := tape.Parse("Output d.gif\nSet CursorBlink false\nSet Framerate 30\nCtrl+Enter\nType \"hi\"\n")
	if err != nil {
		t.Fatal(err)
	}

	det := tape.Lint(tp, tape.RunOptions{Mode: foley.Deterministic})
	joined := strings.Join(det, "\n")
	for _, want := range []string{"CursorBlink", "Framerate", "Ctrl+Enter"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("deterministic lint lacks %q:\n%s", want, joined)
		}
	}

	// ModifyOtherKeys silences the chord degradation, exactly like Run.
	mok := strings.Join(tape.Lint(tp, tape.RunOptions{ModifyOtherKeys: true}), "\n")
	if strings.Contains(mok, "Ctrl+Enter") {
		t.Fatalf("chord warning survived ModifyOtherKeys:\n%s", mok)
	}

	// Framerate applies in realtime — its deterministic-only warning goes.
	rt := strings.Join(tape.Lint(tp, tape.RunOptions{Mode: foley.Realtime}), "\n")
	if strings.Contains(rt, "Framerate") {
		t.Fatalf("Framerate warning survived realtime mode:\n%s", rt)
	}

	// A clean tape lints clean.
	clean, err := tape.Parse("Output d.gif\nType \"ls\"\nEnter\n")
	if err != nil {
		t.Fatal(err)
	}
	if msgs := tape.Lint(clean, tape.RunOptions{}); len(msgs) != 0 {
		t.Fatalf("clean tape produced warnings: %v", msgs)
	}
}

// TestThemesListsVendoredCatalog: the sorted names behind
// `Set Theme "<name>"`, straight from the vendored themes.json.
func TestThemesListsVendoredCatalog(t *testing.T) {
	names, err := tape.Themes()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 100 {
		t.Fatalf("only %d themes — vendored catalog shrank?", len(names))
	}
	if !sort.StringsAreSorted(names) {
		t.Fatal("names are not sorted")
	}
	found := false
	for _, n := range names {
		if n == "Dracula" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("catalog lacks Dracula — themes.json shape changed?")
	}
}
