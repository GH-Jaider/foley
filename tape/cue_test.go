package tape_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GH-Jaider/foley"
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
			{"# foley: dress macos", func(d tape.DressRef) bool { return d.Name == "macos" }},
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
			{"# foley: dress macos\n# foley: dress macos", "lines 2 and 3"},
			{"# foley: dress {\"padding\": 10} none", "trailing data"},
			{"# foley: dress {\"windowBar\": \"Colorfull\"}", "windowBar"},
			{"# foley: dress {\"padding\": -1}", "negative"},
			{"# foley: dress {\"marginFill\": \"#12345\"}", "hex"},
			{"# foley: dress {\"theme\": \"NoSuchTheme\"}", "unknown theme"},
			{"# foley: dress {\"theme\": 42}", "curated name string"},
			{"# foley: dress {\"theme\": {\"background\": \"#12345\"}}", "background"},
			{"# foley: dress {\"fontSize\": 0}", "fontSize"},
			{"# foley: dress {\"font\": \"Comic Sans\"}", "pinned catalog"},
			{"# foley: dress {\"font\": {\"bold\": \"./b.ttf\"}}", "regular is required"},
			{"# foley: dress {\"font\": {\"regular\": \"Fira Code\"}}", "not a font file path"},
			{"# foley: dress {\"font\": {\"regulr\": \"./r.ttf\"}}", "regulr"},
			{"Type \"x\" # foley: dress macos", "own line"},
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
		if _, err := tape.Parse("Output d.gif\n# foley: dress\tmacos\nType \"x\"\n"); err != nil {
			t.Fatalf("tab-separated cue must parse: %v", err)
		}
	})
	t.Run("generous_marker_strict_body", func(t *testing.T) {
		tp, err := tape.Parse("Output d.gif\n# Foley : dress macos\nType \"x\"\n")
		if err != nil || len(tp.Cues) != 1 {
			t.Fatalf("generous marker: err=%v cues=%v", err, tp.Cues)
		}
		if _, err := tape.Parse("Output d.gif\n# FOLEY: dross x\nType \"x\"\n"); err == nil {
			t.Fatal("strict body must still fail")
		}
	})
	t.Run("quoted_marker_is_data", func(t *testing.T) {
		tp, err := tape.Parse("Output d.gif\nType \"# foley: dress macos\"\nType \"x\"\n")
		if err != nil || len(tp.Cues) != 0 {
			t.Fatalf("quoted marker: err=%v cues=%v", err, tp.Cues)
		}
	})
	t.Run("crlf", func(t *testing.T) {
		tp, err := tape.Parse("Output d.gif\r\n# foley: dress macos\r\nType \"x\"\r\n")
		if err != nil || tp.DressCue().Name != "macos" {
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

// TestDressPrecedence: defaults < dress < explicit Sets (ADR-014),
// exercised through the inline form.
func TestDressPrecedence(t *testing.T) {
	src := "Output d.gif\n# foley: dress {\"windowBar\": \"Colorful\", \"margin\": 24, \"padding\": 40, \"borderRadius\": 10}\nSet BorderRadius 2\nType \"hi\"\n"
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
		fill := "#181818"
		d := tape.Dress{
			Theme:      &tape.DressTheme{Ref: tape.ThemeRef{Name: "Dracula"}},
			FontSize:   &size,
			Font:       &tape.DressFont{Single: "./brand.ttf"},
			MarginFill: &fill,
		}
		exp := strings.Join(d.Expansion(), "\n")
		// MarginFill must come out QUOTED: unquoted, the grammar's lexer
		// would eat "#..." as a comment — the expansion round-trips.
		for _, want := range []string{`Set Theme "Dracula"`, "Set FontSize 18", `Set FontFamily "./brand.ttf"`, `Set MarginFill "#181818"`} {
			if !strings.Contains(exp, want) {
				t.Fatalf("expansion lacks %q:\n%s", want, exp)
			}
		}
	})
	t.Run("font_forms", func(t *testing.T) {
		tp, err := tape.Parse("Output d.gif\n# foley: dress {\"font\": \"Fira Code\"}\nType \"x\"\n")
		if err != nil {
			t.Fatal(err)
		}
		settings, err := tape.EffectiveSettingsForTest(tp, tape.RunOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if settings.FontFamily != "Fira Code" {
			t.Fatalf("FontFamily = %q, want the catalog name Fira Code", settings.FontFamily)
		}
		tp, err = tape.Parse("Output d.gif\n# foley: dress {\"font\": {\"regular\": \"./r.ttf\", \"bold\": \"./b.ttf\"}}\nType \"x\"\n")
		if err != nil {
			t.Fatal(err)
		}
		settings, err = tape.EffectiveSettingsForTest(tp, tape.RunOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if settings.FontFiles.Regular != "./r.ttf" || settings.FontFiles.Bold != "./b.ttf" {
			t.Fatalf("FontFiles = %+v, want the family's ./r.ttf and ./b.ttf", settings.FontFiles)
		}
	})
	t.Run("font_file_precedence", func(t *testing.T) {
		tp, err := tape.Parse("Output d.gif\n# foley: dress {\"font\": \"./brand.ttf\"}\nType \"x\"\n")
		if err != nil {
			t.Fatal(err)
		}
		settings, err := tape.EffectiveSettingsForTest(tp, tape.RunOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if settings.FontFamily != "./brand.ttf" {
			t.Fatalf("FontFamily = %q, want the dress's ./brand.ttf", settings.FontFamily)
		}
		src := "Output d.gif\n# foley: dress {\"font\": \"./brand.ttf\"}\nSet FontFamily \"./mine.otf\"\nType \"x\"\n"
		tp, err = tape.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		settings, err = tape.EffectiveSettingsForTest(tp, tape.RunOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if settings.FontFamily != "./mine.otf" {
			t.Fatalf("FontFamily = %q — the tape's explicit Set FontFamily must beat the dress", settings.FontFamily)
		}
	})
}

// TestDressRebase: paths INSIDE a dress file resolve against the dress
// file's own directory — the kit travels together. Catalog
// names and hex fills pass through untouched.
func TestDressRebase(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "dresses")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	dress := `{"font": {"regular": "./brand.ttf"}, "marginFill": "bg.png"}`
	path := filepath.Join(sub, "brand.json")
	if err := os.WriteFile(path, []byte(dress), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := tape.ResolveDress(tape.DressRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(sub, "brand.ttf"); d.Font.Files.Regular != want {
		t.Fatalf("font = %q, want rebased %q", d.Font.Files.Regular, want)
	}
	if want := filepath.Join(sub, "bg.png"); *d.MarginFill != want {
		t.Fatalf("marginFill = %q, want rebased %q", *d.MarginFill, want)
	}

	named := `{"font": "Fira Code", "marginFill": "#101010"}`
	path2 := filepath.Join(sub, "named.json")
	if err := os.WriteFile(path2, []byte(named), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err = tape.ResolveDress(tape.DressRef{Path: path2})
	if err != nil {
		t.Fatal(err)
	}
	if d.Font.Single != "Fira Code" || *d.MarginFill != "#101010" {
		t.Fatalf("catalog name / hex fill must not rebase, got %q %q", d.Font.Single, *d.MarginFill)
	}
}

// TestBuiltinWardrobe: every embedded dress parses (a broken preset is a
// build defect) and the wardrobe lists the canonical four.
func TestBuiltinWardrobe(t *testing.T) {
	names := tape.BuiltinDresses()
	for _, want := range []string{"bare", "gnome", "macos", "noir", "paper"} {
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
	tp, err := tape.Parse("Output d.gif\n# foley: dress {\"margin\": 24, \"windowBar\": \"Rings\"}\nSet BorderRadius 2\nType \"x\"\n")
	if err != nil {
		t.Fatal(err)
	}
	before := tp.Settings

	gnome, err := tape.EffectiveSettingsForTest(tp, tape.RunOptions{Dress: tape.DressRef{Name: "gnome"}})
	if err != nil {
		t.Fatal(err)
	}
	// gnome's values, and NONE of the tape dress's (margin 24 must go).
	if gnome.WindowBar != "GnomeCSD" || gnome.WindowBarSize != 30 || gnome.Padding != 14 || gnome.Margin != 0 {
		t.Fatalf("override did not REPLACE the layer: bar=%q/%d padding=%d margin=%d",
			gnome.WindowBar, gnome.WindowBarSize, gnome.Padding, gnome.Margin)
	}
	if gnome.BorderRadius != 2 {
		t.Fatalf("explicit Set must beat the CLI dress: radius = %d", gnome.BorderRadius)
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
	if err := os.WriteFile("common.tape", []byte("# foley: dress macos\nType \"shared\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := tape.Parse("Output d.gif\nSource common.tape\nType \"x\"\n")
	if err == nil || !strings.Contains(err.Error(), "top-level") {
		t.Fatalf("sourced cue must be loud, got: %v", err)
	}
}

// TestKeysCue: the second cue (ADR-016) — parses bare and with a size,
// rejects unknown sizes and duplicates, layers under the CLI override.
func TestKeysCue(t *testing.T) {
	tp, err := tape.Parse("Output d.gif\n# foley: keys\nType \"x\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if on, size := tp.KeysCue(); !on || size != foley.KeysMedium {
		t.Fatalf("bare keys cue = %v %v, want on medium", on, size)
	}
	settings, err := tape.EffectiveSettingsForTest(tp, tape.RunOptions{})
	if err != nil || !settings.KeysOverlay {
		t.Fatalf("effective keys: err=%v on=%v", err, settings.KeysOverlay)
	}
	settings, err = tape.EffectiveSettingsForTest(tp, tape.RunOptions{Keys: tape.KeysOff})
	if err != nil || settings.KeysOverlay {
		t.Fatal("-keys off must strip the reel")
	}

	small, err := tape.Parse("Output d.gif\n# foley: keys small\nType \"x\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if on, size := small.KeysCue(); !on || size != foley.KeysSmall {
		t.Fatalf("keys small = %v %v", on, size)
	}

	plain, err := tape.Parse("Output d.gif\nType \"x\"\n")
	if err != nil {
		t.Fatal(err)
	}
	settings, err = tape.EffectiveSettingsForTest(plain, tape.RunOptions{Keys: tape.KeysOnLarge})
	if err != nil || !settings.KeysOverlay || settings.KeysSize != foley.KeysLarge {
		t.Fatalf("-keys large must add the reel: %+v", settings.KeysSize)
	}

	if _, err := tape.Parse("Output d.gif\n# foley: keys bottom\nType \"x\"\n"); err == nil || !strings.Contains(err.Error(), "small|medium|large") {
		t.Fatalf("unknown keys size must fail loudly, got %v", err)
	}
	if _, err := tape.Parse("Output d.gif\n# foley: keys\n# foley: keys\nType \"x\"\n"); err == nil || !strings.Contains(err.Error(), "lines 2 and 3") {
		t.Fatalf("two keys cues must fail with lines, got %v", err)
	}
}
