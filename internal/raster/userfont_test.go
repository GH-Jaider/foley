package raster_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

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
