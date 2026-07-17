package raster

import (
	"testing"
	"time"

	"github.com/GH-Jaider/foley/key"
)

// TestKeysTrackCaps pins the mockup's cap anatomy: one cap per
// keystroke, repeats coalesce with a counter, chords are one accented
// cap, and hidden input never lands.
func TestKeysTrackCaps(t *testing.T) {
	kt := NewKeysTrack()
	kt.AddKey(key.RuneKey('l'), 0, false)
	kt.AddKey(key.RuneKey('s'), 100*time.Millisecond, false)
	kt.AddKey(key.RuneKey('j'), 300*time.Millisecond, false)
	kt.AddKey(key.RuneKey('j'), 400*time.Millisecond, false)
	kt.AddKey(key.RuneKey('j'), 500*time.Millisecond, false)
	kt.AddKey(key.Named(key.NameEnter), 700*time.Millisecond, false)
	kt.AddKey(key.RuneKey('x'), 800*time.Millisecond, true) // hidden
	kt.AddKey(key.RuneKey('b').With(key.ModCtrl), 900*time.Millisecond, false)

	if len(kt.phrases) != 1 {
		t.Fatalf("phrases = %d, want 1", len(kt.phrases))
	}
	caps := kt.phrases[0].caps
	want := []struct {
		sym   string
		count int
		mod   bool
	}{
		{"l", 1, false},
		{"s", 1, false},
		{"j", 3, false},
		{"↩", 1, true},
		{"^B", 1, true},
	}
	if len(caps) != len(want) {
		t.Fatalf("caps = %+v, want %d", caps, len(want))
	}
	for i, w := range want {
		if caps[i].sym != w.sym || caps[i].count != w.count || caps[i].mod != w.mod {
			t.Fatalf("cap %d = %+v, want %+v", i, caps[i], w)
		}
	}
}

// TestKeysPhrasesFlushAndCut pins the two cut mechanisms: an idle gap
// closes the phrase (gentle fade at lastInput+idle), a full phrase
// cuts fast and the next one reveals after the cut.
func TestKeysPhrasesFlushAndCut(t *testing.T) {
	kt := NewKeysTrack()
	kt.AddKey(key.RuneKey('a'), 0, false)
	kt.AddKey(key.RuneKey('b'), 5*time.Second, false) // past idle: new phrase
	if len(kt.phrases) != 2 {
		t.Fatalf("phrases = %d, want 2 after an idle gap", len(kt.phrases))
	}
	first := &kt.phrases[0]
	if first.end != keysIdle || first.quick {
		t.Fatalf("idle flush = %+v, want gentle fade at %v", first, keysIdle)
	}
	if a := first.alphaAt(keysIdle - 1); a != 255 {
		t.Fatalf("pre-flush alpha = %d", a)
	}
	if a := first.alphaAt(keysIdle + keysFlushFade); a != 0 {
		t.Fatalf("post-flush alpha = %d", a)
	}

	kt = NewKeysTrack()
	kt.setCapacity(3)
	base := time.Duration(0)
	for i, r := range "abcd" {
		kt.AddKey(key.RuneKey(r), base+time.Duration(i)*700*time.Millisecond, false)
	}
	if len(kt.phrases) != 2 {
		t.Fatalf("phrases = %d, want 2 after a width cut", len(kt.phrases))
	}
	cutAt := 3 * 700 * time.Millisecond
	if !kt.phrases[0].quick || kt.phrases[0].end != cutAt {
		t.Fatalf("cut phrase = %+v, want quick at %v", kt.phrases[0], cutAt)
	}
	if kt.phrases[1].reveal != cutAt+keysCutFade {
		t.Fatalf("reveal = %v, want %v", kt.phrases[1].reveal, cutAt+keysCutFade)
	}
	if a := kt.phrases[0].alphaAt(cutAt + keysCutFade/2); a == 0 || a == 255 {
		t.Fatalf("mid-cut alpha = %d, want a quick-fade step", a)
	}
}

// TestKeysBreakpoints pins the frame contract: births, pop ends and
// fade steps land in [from, to), sorted and deduplicated.
func TestKeysBreakpoints(t *testing.T) {
	kt := NewKeysTrack()
	kt.AddKey(key.Named(key.NameEnter), time.Second, false)
	cuts := kt.Breakpoints(0, 10*time.Second)
	if len(cuts) == 0 || cuts[0] != time.Second {
		t.Fatalf("breakpoints = %v, want the birth first", cuts)
	}
	flush := time.Second + keysIdle
	seen := map[time.Duration]bool{}
	for _, c := range cuts {
		if seen[c] {
			t.Fatalf("duplicate breakpoint %v in %v", c, cuts)
		}
		seen[c] = true
	}
	if !seen[flush] {
		t.Fatalf("breakpoints %v lack the flush start %v", cuts, flush)
	}
	// Half-open window: a cut AT from is in, AT to is out.
	if got := kt.Breakpoints(time.Second, 2*time.Second); len(got) == 0 || got[0] != time.Second {
		t.Fatalf("[from, to) must include from: %v", got)
	}
	if got := kt.Breakpoints(0, time.Second); len(got) != 0 {
		t.Fatalf("[from, to) must exclude to: %v", got)
	}
}

// TestKeyCapLabels pins the symbol/ASCII pairs and the accent flag.
func TestKeyCapLabels(t *testing.T) {
	cases := []struct {
		k     key.Key
		sym   string
		ascii string
		mod   bool
	}{
		{key.RuneKey('a'), "a", "a", false},
		{key.RuneKey(' '), "␣", "Space", true},
		{key.Named(key.NameEnter), "↩", "Enter", true},
		{key.Named(key.NameEscape), "⎋", "Esc", true},
		{key.Named(key.NameUp), "↑", "Up", true},
		{key.RuneKey('c').With(key.ModCtrl), "^C", "Ctrl+C", true},
		{key.RuneKey('v').With(key.ModShift), "⇧V", "Shift+V", true},
	}
	for _, c := range cases {
		sym, ascii, mod := keyCapLabel(c.k)
		if sym != c.sym || ascii != c.ascii || mod != c.mod {
			t.Fatalf("keyCapLabel(%+v) = %q %q %v, want %q %q %v", c.k, sym, ascii, mod, c.sym, c.ascii, c.mod)
		}
	}
}
