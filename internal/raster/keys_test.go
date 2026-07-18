package raster

import (
	"testing"
	"time"

	"github.com/GH-Jaider/foley/key"
)

// TestKeysTrackCaps pins the cap anatomy: one cap per keystroke,
// repeats coalesce with a counter, chords are one accented cap, and
// hidden input never lands.
func TestKeysTrackCaps(t *testing.T) {
	kt := NewKeysTrack(KeysKeycap)
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
		label string
		count int
		mod   bool
	}{
		{"l", 1, false},
		{"s", 1, false},
		{"j", 3, false},
		{"enter", 1, true},
		{"^B", 1, true},
	}
	if len(caps) != len(want) {
		t.Fatalf("caps = %+v, want %d", caps, len(want))
	}
	for i, w := range want {
		if caps[i].label != w.label || caps[i].count != w.count || caps[i].mod != w.mod {
			t.Fatalf("cap %d = %+v, want %+v", i, caps[i], w)
		}
	}
	if !caps[3].enter {
		t.Fatalf("the enter cap must mark the take's end: %+v", caps[3])
	}
}

// TestKeysPhrasesFlushAndCut pins the two cut mechanisms: an idle gap
// closes the phrase (gentle fade at lastInput+idle), a full phrase
// cuts fast and the next one reveals after the cut.
func TestKeysPhrasesFlushAndCut(t *testing.T) {
	kt := NewKeysTrack(KeysKeycap)
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

	kt = NewKeysTrack(KeysKeycap)
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

// TestKeysEnterClosesTake pins the v3 take semantics: after Enter the
// phrase flushes on the SHORT idle — the shell's take ended — while
// ordinary keys keep the general idle (TUIs have no terminator).
func TestKeysEnterClosesTake(t *testing.T) {
	kt := NewKeysTrack(KeysKeycap)
	kt.AddKey(key.RuneKey('a'), 0, false)
	kt.AddKey(key.Named(key.NameEnter), 100*time.Millisecond, false)
	// 700ms after Enter: past the enter idle (600ms), inside the
	// general one (1.5s) — a new take must open.
	kt.AddKey(key.RuneKey('b'), 800*time.Millisecond, false)
	if len(kt.phrases) != 2 {
		t.Fatalf("phrases = %d, want 2 — Enter closes the take", len(kt.phrases))
	}
	if want := 100*time.Millisecond + keysIdleEnter; kt.phrases[0].end != want {
		t.Fatalf("flush = %v, want %v (enter idle)", kt.phrases[0].end, want)
	}

	// The same 700ms gap WITHOUT an Enter keeps the phrase open.
	kt = NewKeysTrack(KeysKeycap)
	kt.AddKey(key.RuneKey('a'), 0, false)
	kt.AddKey(key.RuneKey('b'), 700*time.Millisecond, false)
	if len(kt.phrases) != 1 {
		t.Fatalf("phrases = %d, want 1 — plain keys keep the general idle", len(kt.phrases))
	}
}

// TestKeysBreakpoints pins the frame contract: births and fade steps
// land in [from, to), sorted and deduplicated.
func TestKeysBreakpoints(t *testing.T) {
	kt := NewKeysTrack(KeysKeycap)
	kt.AddKey(key.Named(key.NameEnter), time.Second, false)
	cuts := kt.Breakpoints(0, 10*time.Second)
	if len(cuts) == 0 || cuts[0] != time.Second {
		t.Fatalf("breakpoints = %v, want the birth first", cuts)
	}
	// Enter closed the take: the flush schedule hangs off the SHORT idle.
	flush := time.Second + keysIdleEnter
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

// TestKeyCapFaces pins the v3 vocabulary: what a real keycap prints —
// lowercase words, drawn arrows, the blank spacebar, text chords; esc
// is never an icon, in either notation.
func TestKeyCapFaces(t *testing.T) {
	cases := []struct {
		name     string
		k        key.Key
		notation KeysNotation
		want     keyCap
	}{
		{"plain rune", key.RuneKey('a'), KeysKeycap, keyCap{label: "a"}},
		{"spacebar", key.RuneKey(' '), KeysKeycap, keyCap{space: true}},
		{"enter word", key.Named(key.NameEnter), KeysKeycap, keyCap{label: "enter", mod: true, enter: true}},
		{"esc word", key.Named(key.NameEscape), KeysKeycap, keyCap{label: "esc", mod: true}},
		{"bksp word", key.Named(key.NameBackspace), KeysKeycap, keyCap{label: "bksp", mod: true}},
		{"arrow is drawn", key.Named(key.NameUp), KeysKeycap, keyCap{icon: iconUp, mod: true}},
		{"ctrl caret", key.RuneKey('c').With(key.ModCtrl), KeysKeycap, keyCap{label: "^C", mod: true}},
		{"shift word chord", key.RuneKey('v').With(key.ModShift), KeysKeycap, keyCap{label: "shift+V", mod: true}},
		{"alt chord of a named key", key.Named(key.NameEnter).With(key.ModAlt), KeysKeycap, keyCap{label: "alt+enter", mod: true, enter: true}},
		{"chorded space is text", key.Named(key.NameSpace).With(key.ModCtrl), KeysKeycap, keyCap{label: "^space", mod: true}},
		{"chorded arrow is text", key.Named(key.NameUp).With(key.ModCtrl), KeysKeycap, keyCap{label: "^up", mod: true}},

		{"icons: enter drawn", key.Named(key.NameEnter), KeysIcons, keyCap{icon: iconEnter, mod: true, enter: true}},
		{"icons: bksp drawn", key.Named(key.NameBackspace), KeysIcons, keyCap{icon: iconBksp, mod: true}},
		{"icons: tab drawn", key.Named(key.NameTab), KeysIcons, keyCap{icon: iconTab, mod: true}},
		{"icons: esc is still esc", key.Named(key.NameEscape), KeysIcons, keyCap{label: "esc", mod: true}},
		{"icons: spacebar stays blank", key.Named(key.NameSpace), KeysIcons, keyCap{space: true}},
	}
	for _, c := range cases {
		got, ok := keyCapFor(c.k, c.notation)
		if !ok {
			t.Fatalf("%s: keyCapFor(%+v) dropped the key", c.name, c.k)
		}
		if got.label != c.want.label || got.icon != c.want.icon || got.space != c.want.space ||
			got.mod != c.want.mod || got.enter != c.want.enter {
			t.Fatalf("%s: keyCapFor(%+v) = %+v, want %+v", c.name, c.k, got, c.want)
		}
	}
	if _, ok := keyCapFor(key.Key{}, KeysKeycap); ok {
		t.Fatal("a zero key must be dropped")
	}
}

// TestKeyIconStrips pins that every drawn face rasterizes: non-empty
// ink at a plausible cap size, cached per size.
func TestKeyIconStrips(t *testing.T) {
	r := &Rasterizer{keyIcons: make(map[keyIconKey]textStrip)}
	for icon := iconUp; icon <= iconDel; icon++ {
		ts := r.keyIconStrip(icon, 22)
		if ts.mask == nil {
			t.Fatalf("icon %d: nil mask", icon)
		}
		ink := 0
		for _, a := range ts.mask.alpha.Pix {
			if a > 0 {
				ink++
			}
		}
		if ink == 0 {
			t.Fatalf("icon %d: no ink at 22px", icon)
		}
		if again := r.keyIconStrip(icon, 22); again.mask != ts.mask {
			t.Fatalf("icon %d: mask not cached per size", icon)
		}
	}
}
