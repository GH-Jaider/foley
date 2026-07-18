package tape_test

import (
	"testing"
	"time"

	"github.com/GH-Jaider/foley/tape"
)

// TestDeclaredTotal pins the progress total as an exact mirror of how
// Run spends virtual time: one perKey per rune, presses by count,
// sleeps as declared, paste at zero — all through PlaybackSpeed.
func TestDeclaredTotal(t *testing.T) {
	src := "Output d.gif\n" +
		"Set TypingSpeed 50ms\n" +
		"Type \"abc\"\n" + // 3 runes x 50ms = 150ms
		"Type@10ms \"hi\"\n" + // 2 x 10ms = 20ms
		"Enter 2\n" + // 2 presses x 50ms = 100ms
		"Sleep 730ms\n" +
		"Wait\n" // synchronization: no declared time
	tp, err := tape.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := tape.EffectiveSettingsForTest(tp, tape.RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := tape.DeclaredTotalForTest(tp, settings), 1000*time.Millisecond; got != want {
		t.Fatalf("declared total = %v, want %v", got, want)
	}

	// PlaybackSpeed 2 halves the promise.
	tp2, err := tape.Parse("Output d.gif\nSet PlaybackSpeed 2\nSet TypingSpeed 50ms\nType \"abcd\"\nSleep 800ms\n")
	if err != nil {
		t.Fatal(err)
	}
	s2, err := tape.EffectiveSettingsForTest(tp2, tape.RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// (4 x 50ms + 800ms) / 2 = 500ms
	if got, want := tape.DeclaredTotalForTest(tp2, s2), 500*time.Millisecond; got != want {
		t.Fatalf("scaled declared total = %v, want %v", got, want)
	}
}
