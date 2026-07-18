package raster

import (
	"regexp"
	"testing"
	"time"
)

// TestHighlightTrackIntervals pins the on/off model: active on
// [from, to), Clear closes every open item, breakpoints are exactly
// the snap instants in [from, to).
func TestHighlightTrackIntervals(t *testing.T) {
	ht := NewHighlightTrack()
	re := regexp.MustCompile("x")
	ht.Activate(HighlightSpec{Pattern: re}, time.Second)
	ht.Activate(HighlightSpec{Rect: true, Col: 1, Row: 1, W: 2, H: 1}, 2*time.Second)
	ht.Clear(3 * time.Second)

	at := func(d time.Duration) int {
		ht.SetTime(d)
		return len(ht.active())
	}
	if n := at(999 * time.Millisecond); n != 0 {
		t.Fatalf("pre-activation active = %d", n)
	}
	if n := at(time.Second); n != 1 {
		t.Fatalf("after first activation active = %d", n)
	}
	if n := at(2500 * time.Millisecond); n != 2 {
		t.Fatalf("both active = %d", n)
	}
	if n := at(3 * time.Second); n != 0 {
		t.Fatalf("after clear active = %d", n)
	}

	cuts := ht.Breakpoints(0, 10*time.Second)
	want := []time.Duration{time.Second, 2 * time.Second, 3 * time.Second}
	if len(cuts) != len(want) {
		t.Fatalf("breakpoints = %v, want %v", cuts, want)
	}
	for i := range want {
		if cuts[i] != want[i] {
			t.Fatalf("breakpoints = %v, want %v", cuts, want)
		}
	}
	if got := ht.Breakpoints(0, time.Second); len(got) != 0 {
		t.Fatalf("[from, to) must exclude to: %v", got)
	}
}

// TestRuneIndex pins the byte→rune bridge for multi-byte text.
func TestRuneIndex(t *testing.T) {
	s := "a❯b"
	if i := runeIndex(s, 0); i != 0 {
		t.Fatalf("offset 0 = rune %d", i)
	}
	if i := runeIndex(s, 1); i != 1 {
		t.Fatalf("offset 1 = rune %d", i)
	}
	if i := runeIndex(s, 4); i != 2 {
		t.Fatalf("offset 4 = rune %d (after the 3-byte ❯)", i)
	}
	if i := runeIndex(s, len(s)); i != 3 {
		t.Fatalf("offset end = rune %d", i)
	}
}

// TestHighlightNamedClear pins the targeted off: only the named
// highlight closes; the rest stay lit.
func TestHighlightNamedClear(t *testing.T) {
	ht := NewHighlightTrack()
	ht.Activate(HighlightSpec{Pattern: regexp.MustCompile("a"), Name: "uno"}, 0)
	ht.Activate(HighlightSpec{Pattern: regexp.MustCompile("b")}, 0)
	ht.ClearNamed("uno", time.Second)
	ht.SetTime(2 * time.Second)
	act := ht.active()
	if len(act) != 1 || act[0].Pattern.String() != "b" {
		t.Fatalf("active = %+v, want only the unnamed b", act)
	}
}
