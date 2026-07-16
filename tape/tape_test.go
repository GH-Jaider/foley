package tape_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GH-Jaider/foley"
	"github.com/GH-Jaider/foley/key"
	"github.com/GH-Jaider/foley/tape"
)

// TestParseTyped pins the stringly→typed boundary: one tape exercising
// every command class, asserted value by value.
func TestParseTyped(t *testing.T) {
	src := `Output demo.gif
Output frames/
Require echo
Env GREETING "hola"
Set Shell zsh
Set FontSize 28
Set Width 800
Set Height 400
Set Padding 30
Set TypingSpeed 75ms
Set PlaybackSpeed 2
Set WaitTimeout 30s
Set WaitPattern /listo/
Set Theme "Dracula"
Type@100ms "hola foley"
Enter 2
Backspace@50ms 3
Ctrl+Shift+C
Alt+Enter
Sleep 1.5s
Wait
Wait+Screen@5s /done/
Hide
Show
Screenshot cap.png
Copy "pegado"
Paste
ScrollUp@250ms 4
`
	tp, err := tape.Parse(src)
	if err != nil {
		t.Fatal(err)
	}

	s := tp.Settings
	if s.Shell != "zsh" || s.FontSize != 28 || s.Width != 800 || s.Height != 400 || s.Padding != 30 {
		t.Fatalf("settings = %+v", s)
	}
	if s.TypingSpeed != 75*time.Millisecond || s.PlaybackSpeed != 2 || s.WaitTimeout != 30*time.Second {
		t.Fatalf("timing settings = %+v", s)
	}
	if s.WaitPattern.String() != "listo" {
		t.Fatalf("WaitPattern = %q", s.WaitPattern)
	}
	if s.Theme.Name != "Dracula" || s.Theme.JSON != "" {
		t.Fatalf("theme = %+v", s.Theme)
	}
	// VHS defaults survive where the tape stayed silent.
	if s.Framerate != 50 || !s.CursorBlink || s.LineHeight != 1.0 {
		t.Fatalf("defaults lost: %+v", s)
	}
	if len(tp.Outputs) != 2 || tp.Outputs[0] != "demo.gif" || tp.Outputs[1] != "frames/" {
		t.Fatalf("outputs = %v", tp.Outputs)
	}
	if len(tp.Requires) != 1 || tp.Requires[0] != "echo" {
		t.Fatalf("requires = %v", tp.Requires)
	}
	if tp.Env["GREETING"] != "hola" {
		t.Fatalf("env = %v", tp.Env)
	}

	want := []tape.Command{
		{Kind: tape.KindType, Text: "hola foley", Speed: 100 * time.Millisecond},
		{Kind: tape.KindPress, Key: key.Named(key.NameEnter), Count: 2},
		{Kind: tape.KindPress, Key: key.Named(key.NameBackspace), Count: 3, Speed: 50 * time.Millisecond},
		{Kind: tape.KindPress, Key: key.RuneKey('c').With(key.ModCtrl | key.ModShift), Count: 1}, // physical base key, mods carry the shift
		{Kind: tape.KindPress, Key: key.Named(key.NameEnter).With(key.ModAlt), Count: 1},
		{Kind: tape.KindSleep, Speed: 1500 * time.Millisecond},
		{Kind: tape.KindWait, Scope: tape.WaitLine},
		{Kind: tape.KindWait, Scope: tape.WaitScreen, Timeout: 5 * time.Second},
		{Kind: tape.KindHide},
		{Kind: tape.KindShow},
		{Kind: tape.KindScreenshot, Text: "cap.png"},
		{Kind: tape.KindCopy, Text: "pegado"},
		{Kind: tape.KindPaste},
		{Kind: tape.KindScrollUp, Count: 4, Speed: 250 * time.Millisecond},
	}
	if len(tp.Commands) != len(want) {
		t.Fatalf("got %d commands, want %d:\n%+v", len(tp.Commands), len(want), tp.Commands)
	}
	for i, w := range want {
		g := tp.Commands[i]
		if w.Kind == tape.KindWait {
			// Patterns compare separately (pointer types).
			if g.Kind != w.Kind || g.Scope != w.Scope || g.Timeout != w.Timeout {
				t.Fatalf("command %d = %+v, want %+v", i, g, w)
			}
			continue
		}
		if g != w {
			t.Fatalf("command %d = %+v, want %+v", i, g, w)
		}
	}
	if tp.Commands[6].Pattern != nil {
		t.Fatalf("bare Wait must defer to settings pattern, got %v", tp.Commands[6].Pattern)
	}
	if got := tp.Commands[7].Pattern.String(); got != "done" {
		t.Fatalf("wait pattern = %q", got)
	}
}

func TestParseRejectsUnknownThings(t *testing.T) {
	if _, err := tape.Parse(`Type "sin output"`); err == nil {
		t.Fatal("tape without Output must error")
	}
	if _, err := tape.Parse("Output x.gif\nFoo bar"); err == nil {
		t.Fatal("grammar errors must surface")
	}
}

// TestChordNormalization pins the physical-key mapping the encoder
// expects: Ctrl/Alt chords carry the lowercase base key, Shift-only is
// text.
func TestChordNormalization(t *testing.T) {
	tp, err := tape.Parse("Output x.gif\nCtrl+Shift+C\nAlt+X\nShift+a\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []key.Key{
		key.RuneKey('c').With(key.ModCtrl | key.ModShift),
		key.RuneKey('x').With(key.ModAlt),
		key.RuneKey('A').With(key.ModShift),
	}
	for i, w := range want {
		if got := tp.Commands[i].Key; got != w {
			t.Fatalf("chord %d = %+v, want %+v", i, got, w)
		}
	}
}

// TestInlineThemeInheritsDefaults uses the real Whimsy line from
// upstream's all.tape: its "purple"/"cursorColor" keys are dropped (as
// upstream drops them) and the unset slots inherit the default theme —
// never black.
func TestInlineThemeInheritsDefaults(t *testing.T) {
	tp, err := tape.Parse(`Output x.gif
Set Theme { "name": "Whimsy", "background": "#29283b", "purple": "#aa7ff0", "cursorColor": "#b3b0d6" }
`)
	if err != nil {
		t.Fatal(err)
	}
	th, err := tape.ResolveThemeForTest(tp.Settings.Theme)
	if err != nil {
		t.Fatal(err)
	}
	if th.Background != (foley.RGB{R: 0x29, G: 0x28, B: 0x3b}) {
		t.Fatalf("background = %+v", th.Background)
	}
	def := foley.DefaultTheme()
	if th.ANSI[5] != def.ANSI[5] || th.ANSI[5] == (foley.RGB{}) {
		t.Fatalf("magenta must inherit the default, got %+v", th.ANSI[5])
	}
}

// TestDegradedChordWarningVisible pins the user-facing visibility of the
// ModifyOtherKeys choice: affected chords warn once each, naming the
// exact degradation and where to configure it.
func TestDegradedChordWarningVisible(t *testing.T) {
	tp, err := tape.Parse("Output x.gif\nCtrl+Enter\nCtrl+Enter\nAlt+Tab\nCtrl+c\n")
	if err != nil {
		t.Fatal(err)
	}
	var warns []string
	tape.WarnDegradedChordsForTest(tp, func(format string, args ...any) {
		warns = append(warns, fmt.Sprintf(format, args...))
	})
	if len(warns) != 2 { // Ctrl+Enter deduped; Ctrl+c has a legacy form and stays silent
		t.Fatalf("warns = %v", warns)
	}
	for _, w := range warns {
		if !strings.Contains(w, "ModifyOtherKeys") || !strings.Contains(w, "--modify-other-keys") {
			t.Fatalf("warning must point at the knob: %q", w)
		}
	}
	if !strings.Contains(warns[0], "Ctrl+Enter") || !strings.Contains(warns[1], "Alt+Tab") {
		t.Fatalf("warnings must name the chords: %v", warns)
	}
}

func TestLateSetTracked(t *testing.T) {
	tp, err := tape.Parse("Output x.gif\nType \"a\"\nSet FontSize 30\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(tp.LateSets) != 1 || tp.LateSets[0] != "FontSize" {
		t.Fatalf("LateSets = %v", tp.LateSets)
	}
	if tp.Settings.FontSize != 30 {
		t.Fatal("late Set must still apply (last wins)")
	}
}
