package raster_test

import (
	"bytes"
	"flag"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/GH-Jaider/foley/internal/fontpack"
	"github.com/GH-Jaider/foley/internal/raster"
	"github.com/GH-Jaider/foley/internal/testassets"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/fake"
)

//nolint:gochecknoglobals // flag registration must be package-level
var updateGolden = flag.Bool("update", false, "rewrite golden files")

// TestGoldenScene renders a scene exercising every rasterizer feature —
// ligatures, bold/italic, palette and true color, inverse, faint, every
// underline style, strikethrough, wide graphemes, color emoji, a kitty
// placement and the cursor — and compares the PNG byte-for-byte.
func TestGoldenScene(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	r, err := raster.New(raster.Options{Pack: pack, FontSizePx: 16, Scale: 2})
	if err != nil {
		t.Fatal(err)
	}

	geo := vtengine.Geometry{Cols: 44, Rows: 7}
	geo.CellW, geo.CellH = r.LogicalCellSize()
	e := fake.New(vtengine.Options{Geometry: geo})
	defer func() { _ = e.Close() }()

	catppuccin := vtengine.Colors{
		FG: vtengine.RGB{R: 0xcd, G: 0xd6, B: 0xf4},
		BG: vtengine.RGB{R: 0x1e, G: 0x1e, B: 0x2e},
	}
	catppuccin.Palette[2] = vtengine.RGB{R: 0xa6, G: 0xe3, B: 0xa1}
	e.SetColors(catppuccin)

	writeAt := func(x, y int, s string, st vtengine.Style) {
		for _, rn := range s {
			e.SetCell(x, y, string(rn), st)
			x++
		}
	}

	blue := vtengine.RGB{R: 0x89, G: 0xb4, B: 0xfa}
	pink := vtengine.RGB{R: 0xf3, G: 0x8b, B: 0xa8}
	green := vtengine.RGB{R: 0xa6, G: 0xe3, B: 0xa1}

	writeAt(0, 0, "foley -> raster v1 => != fi ffi", vtengine.Style{FG: blue})
	writeAt(0, 1, "bold", vtengine.Style{FG: catppuccin.FG, Bold: true})
	{ // italic + colors + inverse + faint on one row
		st := vtengine.Style{FG: pink, Italic: true}
		x := 6
		for _, rn := range "italic" {
			e.SetCell(x, 1, string(rn), st)
			x++
		}
		x = 14
		for _, rn := range "inverse" {
			e.SetCell(x, 1, string(rn), vtengine.Style{FG: green, BG: catppuccin.BG, HasBG: true, Inverse: true})
			x++
		}
		x = 23
		for _, rn := range "faint" {
			e.SetCell(x, 1, string(rn), vtengine.Style{FG: catppuccin.FG, Faint: true})
			x++
		}
		x = 30
		for _, rn := range "both" {
			e.SetCell(x, 1, string(rn), vtengine.Style{FG: blue, Bold: true, Italic: true})
			x++
		}
	}
	writeAt(0, 2, "single", vtengine.Style{FG: catppuccin.FG, Underline: vtengine.UnderlineSingle})
	{
		styles := []struct {
			s  string
			u  vtengine.UnderlineStyle
			x0 int
		}{
			{"double", vtengine.UnderlineDouble, 8},
			{"curly", vtengine.UnderlineCurly, 16},
			{"dotted", vtengine.UnderlineDotted, 23},
			{"dashed", vtengine.UnderlineDashed, 31},
		}
		for _, sp := range styles {
			x := sp.x0
			for _, rn := range sp.s {
				e.SetCell(x, 2, string(rn), vtengine.Style{
					FG: catppuccin.FG, Underline: sp.u,
					UnderlineColor: pink,
				})
				x++
			}
		}
	}
	writeAt(0, 3, "strike", vtengine.Style{FG: catppuccin.FG, Strikethrough: true})
	// CJK stays out of scene v1: JetBrains Mono lacks it and the fallback
	// font is the pending PRD §14.4 decision (future golden).
	e.SetGrapheme(9, 3, "🚀", 2, vtengine.Style{})
	e.SetGrapheme(12, 3, "✨", 2, vtengine.Style{})

	// kitty placement: 8x8 checkerboard scaled to 6x2 cells.
	pix := make([]byte, 8*8*4)
	for i := 0; i < 8*8; i++ {
		on := (i%8+i/8)%2 == 0
		if on {
			pix[i*4], pix[i*4+1], pix[i*4+2] = 0xcb, 0xa6, 0xf7
		} else {
			pix[i*4], pix[i*4+1], pix[i*4+2] = 0x31, 0x32, 0x44
		}
		pix[i*4+3] = 0xff
	}
	e.SetImage(vtengine.ImageData{ID: 7, W: 8, H: 8, Generation: 1, Pix: pix})
	e.AddPlacement(vtengine.Placement{
		ImageID: 7, Col: 1, Row: 4,
		PixelW: uint32(6 * geo.CellW), PixelH: uint32(2 * geo.CellH), //nolint:gosec // test values
		SrcW: 8, SrcH: 8,
	})

	// Sprites (synthesized, sprites.go): box drawing with double/heavy/
	// rounded corners joining across cells, dashes, diagonals, block
	// eighths, shades, quadrants and braille. The arc and diagonals are
	// the only supersampled shapes — this golden pins them cross-arch.
	writeAt(9, 4, "╔═╦═╗ ┏━┳━┓ ╭──╮ ┌─┬─┐ ▛▀▀▜", vtengine.Style{FG: green})
	writeAt(9, 5, "╚═╩═╝ ┗━┻━┛ ╰──╯ └─┴─┘ ▙▄▄▟", vtengine.Style{FG: green})
	writeAt(0, 6, "╌┄┈ ╎┆┊ ╱╲╳ ▁▂▃▄▅▆▇█ ▉▋▍▏▐▔▕ ░▒▓ ▖▘▝▗▚▞ ⠁⠿⣿", vtengine.Style{FG: pink})

	e.SetCursor(vtengine.Cursor{X: 33, Y: 0, Visible: true, Shape: vtengine.CursorBlock})

	var f vtengine.Frame
	if err := e.Snapshot(&f); err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(&f, e, nil)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	goldenPath := filepath.Join("testdata", "golden", "scene-v1.png")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, buf.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("golden updated: %s (%d bytes)", goldenPath, buf.Len())
		return
	}
	want, err := os.ReadFile(goldenPath) //nolint:gosec // fixed testdata path
	testassets.Require(t, err, "regenerate with go test ./internal/raster/ -update")
	if !bytes.Equal(buf.Bytes(), want) {
		diffPath := filepath.Join(t.TempDir(), "got.png")
		_ = os.WriteFile(diffPath, buf.Bytes(), 0o600)
		t.Fatalf("render differs from golden (%d vs %d bytes); actual at %s", buf.Len(), len(want), diffPath)
	}
}

func BenchmarkRenderFull120x30(b *testing.B) {
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(b, err, "make fonts")
	r, err := raster.New(raster.Options{Pack: pack, FontSizePx: 16, Scale: 2})
	if err != nil {
		b.Fatal(err)
	}
	geo := vtengine.Geometry{Cols: 120, Rows: 30}
	geo.CellW, geo.CellH = r.CellSize()
	e := fake.New(vtengine.Options{Geometry: geo})
	defer func() { _ = e.Close() }()
	for y := 0; y < 30; y++ {
		st := vtengine.Style{FG: vtengine.RGB{R: 200, G: 200, B: 220}, Bold: y%3 == 0, Underline: vtengine.UnderlineStyle(y % 6)}
		for x := 0; x < 120; x++ {
			e.SetCell(x, y, string(rune('a'+(x+y)%26)), st)
		}
	}
	var f vtengine.Frame
	if err := e.Snapshot(&f); err != nil {
		b.Fatal(err)
	}
	img, err := r.Render(&f, e, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.Render(&f, e, img); err != nil {
			b.Fatal(err)
		}
	}
}

// TestCellSizeIsExactMultipleOfScale pins the geometry invariant behind
// kitty-graphics alignment: the app plans placements in LOGICAL pixels
// (winsize = cell/Scale), so logical*Scale must reconstruct the scaled
// cell EXACTLY — an odd cell drifts 1px per row and slices seams through
// row-strip images (found live by tenten's studio demo).
func TestCellSizeIsExactMultipleOfScale(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	for _, size := range []int{12, 13, 14, 15, 16, 17, 18, 20, 22, 28} {
		for _, scale := range []int{1, 2, 3} {
			r, err := raster.New(raster.Options{Pack: pack, FontSizePx: size, Scale: scale})
			if err != nil {
				t.Fatal(err)
			}
			w, h := r.CellSize()
			lw, lh := r.LogicalCellSize()
			if lw*scale != w || lh*scale != h {
				t.Fatalf("size %d scale %d: cell %dx%d vs logical %dx%d — logical*scale must be exact",
					size, scale, w, h, lw, lh)
			}
		}
	}
}
