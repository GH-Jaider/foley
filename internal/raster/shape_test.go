package raster_test

// Evaluation: can the pure-Go text stack (go-text/typesetting +
// x/image/vector) deliver ligatures and glyph rasterization of the quality
// foley needs? This test is the executable half of that decision — it
// shapes a ligature-heavy string with the pinned JetBrains Mono, asserts
// substitution actually happened, and rasterizes a proof PNG for visual
// judgment.

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"

	"github.com/GH-Jaider/foley/internal/testassets"
)

const fontPath = "../fontpack/fonts/JetBrainsMono-Regular.ttf"

func loadFace(t *testing.T) *font.Face {
	t.Helper()
	f, err := os.Open(fontPath)
	testassets.Require(t, err, "make fonts")
	defer func() { _ = f.Close() }()
	face, err := font.ParseTTF(f)
	if err != nil {
		t.Fatalf("ParseTTF: %v", err)
	}
	return face
}

func TestLigatureShapingPureGo(t *testing.T) {
	face := loadFace(t)
	text := []rune("a -> b => c != d fi ffi")

	shaper := shaping.HarfbuzzShaper{}
	out := shaper.Shape(shaping.Input{
		Text:      text,
		RunStart:  0,
		RunEnd:    len(text),
		Face:      face,
		Size:      fixed.I(32),
		Script:    language.Latin,
		Language:  language.NewLanguage("en"),
		Direction: 0, // LTR
	})

	// Monospace fonts keep the glyph count stable and substitute glyph IDs
	// contextually (calt): detect ligation by comparing the glyph chosen
	// for '>' in "->" context vs isolated.
	shapeOne := func(rs []rune) []shaping.Glyph {
		o := shaper.Shape(shaping.Input{
			Text: rs, RunStart: 0, RunEnd: len(rs), Face: face,
			Size: fixed.I(32), Script: language.Latin,
			Language: language.NewLanguage("en"),
		})
		return o.Glyphs
	}
	iso := shapeOne([]rune("x> "))
	ctx := shapeOne([]rune("-> "))
	gidIso, gidCtx := iso[1].GlyphID, ctx[1].GlyphID
	t.Logf("gid '>' isolated=%d, in \"->\"=%d, total glyphs=%d/%d", gidIso, gidCtx, len(out.Glyphs), len(text))
	if gidIso == gidCtx {
		t.Errorf("calt not applied: '>' has the same glyph isolated and in -> context")
	}

	// Proof render for visual judgment.
	const W, H = 640, 64
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	for i := range img.Pix {
		img.Pix[i] = 0x1e // uniform dark background
	}
	drawGlyphRun(t, img, face, out, 8, 44)

	fout, err := os.Create(filepath.Join(t.TempDir(), "ligatures-purego.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fout.Close() }()
	if err := png.Encode(fout, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("proof written to %s", fout.Name())
}

func drawGlyphRun(t *testing.T, img *image.RGBA, face *font.Face, out shaping.Output, x0, baseline float32) {
	t.Helper()
	penX := x0
	scale := float32(out.Size.Round()) / float32(face.Upem())
	for _, g := range out.Glyphs {
		data := face.GlyphData(g.GlyphID)
		outline, ok := data.(font.GlyphOutline)
		if !ok {
			t.Fatalf("glyph %d has no outline (%T)", g.GlyphID, data)
		}
		r := vector.NewRasterizer(img.Bounds().Dx(), img.Bounds().Dy())
		ox := penX + float32(g.XOffset)/64
		oy := baseline - float32(g.YOffset)/64
		var started bool
		for _, seg := range outline.Segments {
			p := seg.Args
			switch seg.Op {
			case ot.SegmentOpMoveTo:
				r.MoveTo(ox+p[0].X*scale, oy-p[0].Y*scale)
				started = true
			case ot.SegmentOpLineTo:
				r.LineTo(ox+p[0].X*scale, oy-p[0].Y*scale)
			case ot.SegmentOpQuadTo:
				r.QuadTo(ox+p[0].X*scale, oy-p[0].Y*scale, ox+p[1].X*scale, oy-p[1].Y*scale)
			case ot.SegmentOpCubeTo:
				r.CubeTo(ox+p[0].X*scale, oy-p[0].Y*scale, ox+p[1].X*scale, oy-p[1].Y*scale, ox+p[2].X*scale, oy-p[2].Y*scale)
			}
		}
		if started {
			r.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 0xcd, G: 0xd6, B: 0xf4, A: 0xff}), image.Point{})
		}
		penX += float32(g.Advance) / 64
	}
}
