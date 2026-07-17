package tape_test

import (
	"os"
	"strings"
	"testing"

	"github.com/GH-Jaider/foley/tape"
)

// TestCueScanner: the `# foley:` namespace is strict (typos are LOUD
// errors), plain comments are never cues, and every dress form parses.
func TestCueScanner(t *testing.T) {
	t.Run("dress_forms", func(t *testing.T) {
		cases := []struct {
			src  string
			want func(tape.DressRef) bool
		}{
			{"# foley: dress warp", func(d tape.DressRef) bool { return d.Name == "warp" }},
			{"# foley: dress ./brand.dress.json", func(d tape.DressRef) bool { return d.Path == "./brand.dress.json" }},
			{`# foley: dress {"padding": 10}`, func(d tape.DressRef) bool { return d.JSON != "" }},
			{"# foley: dress none", func(d tape.DressRef) bool { return d.None }},
		}
		for _, c := range cases {
			tp, err := tape.Parse("Output d.gif\n" + c.src + "\nType \"hi\"\n")
			if err != nil {
				t.Fatalf("%q: %v", c.src, err)
			}
			if len(tp.Cues) != 1 || tp.Cues[0].Kind != tape.CueDress || !c.want(tp.Cues[0].Dress) {
				t.Fatalf("%q parsed to %+v", c.src, tp.Cues)
			}
			if tp.Cues[0].Line != 2 {
				t.Fatalf("%q: line = %d, want 2", c.src, tp.Cues[0].Line)
			}
		}
	})
	t.Run("loud_errors", func(t *testing.T) {
		cases := []struct{ src, want string }{
			{"# foley: dross warp", "unknown cue"},
			{"# foley: dress", "missing argument"},
			{"# foley: dress nosuchbuiltin", "unknown built-in"},
			{"# foley: dress {\"typo\": 1}", "typo"},
			{"# foley:", "empty"},
			{"# foley: dress warp\n# foley: dress kitty", "lines 2 and 3"},
			{"# foley: dress {\"padding\": 10} none", "trailing data"},
			{"# foley: dress {\"windowBar\": \"Colorfull\"}", "windowBar"},
			{"# foley: dress {\"padding\": -1}", "negative"},
			{"# foley: dress {\"marginFill\": \"#12345\"}", "hex"},
			{"# foley: dress {\"theme\": \"NoSuchTheme\"}", "unknown theme"},
			{"# foley: dress {\"theme\": 42}", "curated name string"},
			{"# foley: dress {\"theme\": {\"background\": \"#12345\"}}", "background"},
			{"# foley: dress {\"fontSize\": 0}", "fontSize"},
			{"Type \"x\" # foley: dress warp", "own line"},
			{"Type \"x\" # foley: dross warp", "own line"},
		}
		for _, c := range cases {
			_, err := tape.Parse("Output d.gif\n" + c.src + "\nType \"x\"\n")
			if err == nil {
				t.Fatalf("%q must fail loudly", c.src)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%q error %q lacks %q", c.src, err, c.want)
			}
		}
		// L2: a tab separator must still parse the kind cleanly.
		if _, err := tape.Parse("Output d.gif\n# foley: dress\twarp\nType \"x\"\n"); err != nil {
			t.Fatalf("tab-separated cue must parse: %v", err)
		}
	})
	t.Run("generous_marker_strict_body", func(t *testing.T) {
		tp, err := tape.Parse("Output d.gif\n# Foley : dress warp\nType \"x\"\n")
		if err != nil || len(tp.Cues) != 1 {
			t.Fatalf("generous marker: err=%v cues=%v", err, tp.Cues)
		}
		if _, err := tape.Parse("Output d.gif\n# FOLEY: dross x\nType \"x\"\n"); err == nil {
			t.Fatal("strict body must still fail")
		}
	})
	t.Run("quoted_marker_is_data", func(t *testing.T) {
		tp, err := tape.Parse("Output d.gif\nType \"# foley: dress warp\"\nType \"x\"\n")
		if err != nil || len(tp.Cues) != 0 {
			t.Fatalf("quoted marker: err=%v cues=%v", err, tp.Cues)
		}
	})
	t.Run("crlf", func(t *testing.T) {
		tp, err := tape.Parse("Output d.gif\r\n# foley: dress warp\r\nType \"x\"\r\n")
		if err != nil || tp.DressCue().Name != "warp" {
			t.Fatalf("CRLF: err=%v ref=%+v", err, tp.DressCue())
		}
	})
	t.Run("plain_comments_are_not_cues", func(t *testing.T) {
		tp, err := tape.Parse("Output d.gif\n# foley is great\n# foleys: nope\nType \"x\"\n")
		if err != nil {
			t.Fatal(err)
		}
		if len(tp.Cues) != 0 {
			t.Fatalf("cues = %+v, want none", tp.Cues)
		}
	})
}

// TestDressPrecedence: defaults < dress < explicit Sets (ADR-014).
func TestDressPrecedence(t *testing.T) {
	src := "Output d.gif\n# foley: dress warp\nSet BorderRadius 2\nType \"hi\"\n"
	tp, err := tape.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	before := tp.Settings
	settings, err := tape.EffectiveSettingsForTest(tp, tape.RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if settings.WindowBar != "Colorful" {
		t.Fatalf("WindowBar = %q, want the dress's Colorful", settings.WindowBar)
	}
	if settings.Margin != 24 || settings.Padding != 40 {
		t.Fatalf("margin/padding = %d/%d, want the dress's 24/40", settings.Margin, settings.Padding)
	}
	if settings.BorderRadius != 2 {
		t.Fatalf("BorderRadius = %d, want 2 — the tape's explicit Set must beat the dress", settings.BorderRadius)
	}
	if tp.Settings != before {
		t.Fatal("effectiveSettings MUTATED the parsed tape — parse once, run many must hold")
	}
}

// TestDressPaintFields: theme and fontSize are dress-able paint
// (ADR-014 v2) — they land where the tape stayed silent and lose to
// explicit Sets, like every other dress field.
func TestDressPaintFields(t *testing.T) {
	dress := `{"theme": "Dracula", "fontSize": 18}`
	t.Run("dress_fills_silence", func(t *testing.T) {
		tp, err := tape.Parse("Output d.gif\n# foley: dress " + dress + "\nType \"x\"\n")
		if err != nil {
			t.Fatal(err)
		}
		settings, err := tape.EffectiveSettingsForTest(tp, tape.RunOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if settings.Theme.Name != "Dracula" {
			t.Fatalf("Theme = %+v, want the dress's Dracula", settings.Theme)
		}
		if settings.FontSize != 18 {
			t.Fatalf("FontSize = %d, want the dress's 18", settings.FontSize)
		}
	})
	t.Run("explicit_sets_win", func(t *testing.T) {
		src := "Output d.gif\n# foley: dress " + dress +
			"\nSet Theme \"Catppuccin Mocha\"\nSet FontSize 30\nType \"x\"\n"
		tp, err := tape.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		settings, err := tape.EffectiveSettingsForTest(tp, tape.RunOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if settings.Theme.Name != "Catppuccin Mocha" {
			t.Fatalf("Theme = %+v — the tape's explicit Set Theme must beat the dress", settings.Theme)
		}
		if settings.FontSize != 30 {
			t.Fatalf("FontSize = %d — the tape's explicit Set FontSize must beat the dress", settings.FontSize)
		}
	})
	t.Run("inline_palette_form", func(t *testing.T) {
		tp, err := tape.Parse("Output d.gif\n# foley: dress {\"theme\": {\"background\": \"#101010\"}}\nType \"x\"\n")
		if err != nil {
			t.Fatal(err)
		}
		settings, err := tape.EffectiveSettingsForTest(tp, tape.RunOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(settings.Theme.JSON, "#101010") {
			t.Fatalf("Theme = %+v, want the inline palette", settings.Theme)
		}
	})
	t.Run("expansion_prints_paint", func(t *testing.T) {
		size := 18
		d := tape.Dress{
			Theme:    &tape.DressTheme{Ref: tape.ThemeRef{Name: "Dracula"}},
			FontSize: &size,
		}
		exp := strings.Join(d.Expansion(), "\n")
		if !strings.Contains(exp, `Set Theme "Dracula"`) || !strings.Contains(exp, "Set FontSize 18") {
			t.Fatalf("expansion lacks the paint fields:\n%s", exp)
		}
	})
}

// TestBuiltinWardrobe: every embedded dress parses (a broken preset is a
// build defect) and the wardrobe lists the canonical four.
func TestBuiltinWardrobe(t *testing.T) {
	names := tape.BuiltinDresses()
	for _, want := range []string{"bare", "gnome", "iterm", "kitty", "macos", "warp"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("wardrobe %v lacks %q", names, want)
		}
	}
	for _, n := range names {
		d, err := tape.ResolveDress(tape.DressRef{Name: n})
		if err != nil {
			t.Fatalf("built-in %q does not resolve: %v", n, err)
		}
		if len(d.Expansion()) == 0 {
			t.Fatalf("built-in %q expands to nothing", n)
		}
	}
}

// TestDressOverrideSemantics pins the -dress layer replacement: the CLI
// ref REPLACES the tape's cue, `none` strips the layer, the tape's
// explicit Sets beat both (ADR-014) — and the Tape is never mutated.
func TestDressOverrideSemantics(t *testing.T) {
	tp, err := tape.Parse("Output d.gif\n# foley: dress warp\nSet BorderRadius 2\nType \"x\"\n")
	if err != nil {
		t.Fatal(err)
	}
	before := tp.Settings

	kitty, err := tape.EffectiveSettingsForTest(tp, tape.RunOptions{Dress: tape.DressRef{Name: "kitty"}})
	if err != nil {
		t.Fatal(err)
	}
	// kitty's values, and NONE of warp's (margin 24 must be gone).
	if kitty.WindowBar != "Colorful" || kitty.WindowBarSize != 24 || kitty.Padding != 16 || kitty.Margin != 0 {
		t.Fatalf("override did not REPLACE the layer: bar=%q/%d padding=%d margin=%d",
			kitty.WindowBar, kitty.WindowBarSize, kitty.Padding, kitty.Margin)
	}
	if kitty.BorderRadius != 2 {
		t.Fatalf("explicit Set must beat the CLI dress: radius = %d", kitty.BorderRadius)
	}

	none, err := tape.EffectiveSettingsForTest(tp, tape.RunOptions{Dress: tape.DressRef{None: true}})
	if err != nil {
		t.Fatal(err)
	}
	if none.WindowBar != "" || none.Margin != 0 || none.Padding != 60 {
		t.Fatalf("none must strip to defaults+Sets: bar=%q margin=%d padding=%d", none.WindowBar, none.Margin, none.Padding)
	}
	if none.BorderRadius != 2 {
		t.Fatalf("none must keep explicit Sets: radius = %d", none.BorderRadius)
	}

	if tp.Settings != before {
		t.Fatal("effectiveSettings mutated the tape across runs")
	}
}

// TestSourcedCuesAreLoud: a cue buried in a Source'd tape cannot take
// effect (the grammar drops comments) — it must be an error, not a
// silent no-op.
func TestSourcedCuesAreLoud(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("common.tape", []byte("# foley: dress warp\nType \"shared\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := tape.Parse("Output d.gif\nSource common.tape\nType \"x\"\n")
	if err == nil || !strings.Contains(err.Error(), "top-level") {
		t.Fatalf("sourced cue must be loud, got: %v", err)
	}
}
