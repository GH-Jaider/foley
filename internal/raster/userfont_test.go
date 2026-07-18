package raster_test

import (
	"bytes"
	"image/color"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/fontpack"
	"github.com/GH-Jaider/foley/internal/raster"
	"github.com/GH-Jaider/foley/internal/testassets"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/fake"
)

// renderStyled renders one row mixing regular, bold, italic and
// bold-italic cells with the given user fonts (zero value = pack only)
// and returns the raw pixels plus the assembly warnings.
func renderStyled(t *testing.T, uf raster.UserFonts) ([]byte, []string) {
	t.Helper()
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	r, err := raster.New(raster.Options{
		Pack: pack, UserFonts: uf, FontSizePx: 16, Scale: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	geo := vtengine.Geometry{Cols: 32, Rows: 1}
	geo.CellW, geo.CellH = r.LogicalCellSize()
	e := fake.New(vtengine.Options{Geometry: geo})
	defer func() { _ = e.Close() }()
	fg := vtengine.RGB{R: 0xcd, G: 0xd6, B: 0xf4}
	x := 0
	write := func(s string, st vtengine.Style) {
		for _, rn := range s {
			e.SetCell(x, 0, string(rn), st)
			x++
		}
	}
	write("plain ", vtengine.Style{FG: fg})
	write("bold ", vtengine.Style{FG: fg, Bold: true})
	write("italic ", vtengine.Style{FG: fg, Italic: true})
	write("both fi", vtengine.Style{FG: fg, Bold: true, Italic: true})
	var f vtengine.Frame
	if err := e.Snapshot(&f); err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(&f, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	return img.Pix, r.Warnings()
}

func jbFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := fontpack.LoadFile(filepath.Join("..", "fontpack", "fonts", name))
	testassets.Require(t, err, "make fonts")
	return b
}

// TestUserFontFamilyNeutrality: a user FAMILY made of the pinned
// JetBrains files must be byte-identical to the pack path across all
// four styles — pinning the whole per-style plumbing (ADR-015).
func TestUserFontFamilyNeutrality(t *testing.T) {
	base, warns := renderStyled(t, raster.UserFonts{})
	if len(warns) != 0 {
		t.Fatalf("pack-only render warned: %v", warns)
	}
	user, warns := renderStyled(t, raster.UserFonts{
		Label:      "jb-family",
		Regular:    jbFile(t, "JetBrainsMono-Regular.ttf"),
		Bold:       jbFile(t, "JetBrainsMono-Bold.ttf"),
		Italic:     jbFile(t, "JetBrainsMono-Italic.ttf"),
		BoldItalic: jbFile(t, "JetBrainsMono-BoldItalic.ttf"),
	})
	if len(warns) != 0 {
		t.Fatalf("family user font must not warn: %v", warns)
	}
	if !bytes.Equal(base, user) {
		t.Fatal("user family (same bytes as the pack) renders differently — the per-style path is not neutral")
	}
}

// TestUserFontSingleFlattensStyles: a single-file user font serves all
// styles — bold cells render, but with the regular face (different
// pixels than the pack's true bold).
func TestUserFontSingleFlattensStyles(t *testing.T) {
	base, _ := renderStyled(t, raster.UserFonts{})
	user, warns := renderStyled(t, raster.UserFonts{
		Label:   "./jb.ttf",
		Regular: jbFile(t, "JetBrainsMono-Regular.ttf"),
	})
	if len(warns) != 0 {
		t.Fatalf("single-file user font must not warn: %v", warns)
	}
	if bytes.Equal(base, user) {
		t.Fatal("single-file user font must flatten bold/italic — identical pixels mean styles leaked from the pack")
	}
}

// TestUserFontWithoutLatin: a primary with no basic latin (the emoji
// face) cannot drive the grid — metrics and text fall back to the pack
// and the assembly warns LOUDLY. Rendering must equal the pack path.
func TestUserFontWithoutLatin(t *testing.T) {
	base, _ := renderStyled(t, raster.UserFonts{})
	user, warns := renderStyled(t, raster.UserFonts{
		Label:   "./emoji.ttf",
		Regular: jbFile(t, "NotoColorEmoji.ttf"),
	})
	if len(warns) != 1 || !strings.Contains(warns[0], "basic latin") {
		t.Fatalf("latin-less user font must warn once about coverage, got %v", warns)
	}
	if !bytes.Equal(base, user) {
		t.Fatal("latin-less primary must fall back to pack rendering byte-for-byte")
	}
}

// TestUserFontUnparseable: garbage bytes are a LOUD error naming the
// font, never a silent fallback.
func TestUserFontUnparseable(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	_, err = raster.New(raster.Options{
		Pack:       pack,
		UserFonts:  raster.UserFonts{Label: "./bad.ttf", Regular: []byte("not a font")},
		FontSizePx: 16, Scale: 2,
	})
	if err == nil || !strings.Contains(err.Error(), "./bad.ttf") {
		t.Fatalf("unparseable user font must fail naming the font, got %v", err)
	}
}

// TestUserFontFamilyNeedsRegular: styles without a regular face are a
// LOUD error — metrics have nothing to derive from.
func TestUserFontFamilyNeedsRegular(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	_, err = raster.New(raster.Options{
		Pack:       pack,
		UserFonts:  raster.UserFonts{Label: "x", Bold: jbFile(t, "JetBrainsMono-Bold.ttf")},
		FontSizePx: 16, Scale: 2,
	})
	if err == nil || !strings.Contains(err.Error(), "regular") {
		t.Fatalf("family without regular must fail loudly, got %v", err)
	}
}

// TestHighlightPaintsSelection is the pixel proof (ADR-018): the cells
// under a regex match carry the Selection color while unmatched cells
// keep the theme background — and the glyphs still draw on top.
func TestHighlightPaintsSelection(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	track := raster.NewHighlightTrack()
	sel := color.RGBA{R: 0x58, G: 0x5b, B: 0x70, A: 0xff}
	r, err := raster.New(raster.Options{
		Pack: pack, FontSizePx: 16, Scale: 2,
		Highlights: track, SelectionColor: sel,
	})
	if err != nil {
		t.Fatal(err)
	}
	geo := vtengine.Geometry{Cols: 20, Rows: 2}
	geo.CellW, geo.CellH = r.LogicalCellSize()
	e := fake.New(vtengine.Options{Geometry: geo})
	defer func() { _ = e.Close() }()
	bg := vtengine.RGB{R: 0x1e, G: 0x1e, B: 0x2e}
	e.SetColors(vtengine.Colors{FG: vtengine.RGB{R: 0xcd, G: 0xd6, B: 0xf4}, BG: bg})
	x := 0
	for _, rn := range "ok error ok" {
		e.SetCell(x, 0, string(rn), vtengine.Style{FG: vtengine.RGB{R: 0xcd, G: 0xd6, B: 0xf4}})
		x++
	}
	track.Activate(raster.HighlightSpec{Pattern: regexp.MustCompile("error.*")}, 0)
	track.SetTime(time.Second)

	var f vtengine.Frame
	if err := e.Snapshot(&f); err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(&f, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	cw, ch := r.CellSize()
	probe := func(cellX int) color.RGBA {
		// Top-left corner of the cell: background territory, no glyph.
		return img.RGBAAt(cellX*cw+1, 0*ch+1)
	}
	// "error" occupies cells 3..7 ("ok " is 0-2).
	if got := probe(4); got != sel {
		t.Fatalf("matched cell = %v, want selection %v", got, sel)
	}
	if got := probe(1); got == sel {
		t.Fatalf("unmatched cell carries the selection color")
	}
	// `.*` ends at the LAST GLYPH ("ok" at cells 9-10), never in the
	// empty-cell padding beyond it.
	if got := probe(9); got != sel {
		t.Fatalf("glyph inside the greedy match = %v, want selection", got)
	}
	if got := probe(15); got == sel {
		t.Fatalf(".* painted into the empty-cell void")
	}
	// After Clear, the same render shows plain background again.
	track.Clear(2 * time.Second)
	track.SetTime(3 * time.Second)
	img, err = r.Render(&f, e, img)
	if err != nil {
		t.Fatal(err)
	}
	if got := probe(4); got == sel {
		t.Fatalf("cleared highlight still painted")
	}
	// Occurrence selector: with TWO "ok" matches on screen, `first`
	// paints only the first one (cells 0-1; probe 1 dodges the cursor
	// cell), never the second (cells 9-10).
	track.Activate(raster.HighlightSpec{Pattern: regexp.MustCompile("ok"), Occurrence: 0, Pick: true}, 5*time.Second)
	track.SetTime(6 * time.Second)
	img, err = r.Render(&f, e, img)
	if err != nil {
		t.Fatal(err)
	}
	if got := probe(1); got != sel {
		t.Fatalf("first occurrence not painted: %v", got)
	}
	if got := probe(9); got == sel {
		t.Fatalf("second occurrence painted despite index 0")
	}
	_ = img
}
