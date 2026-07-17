package raster_test

import (
	"bytes"
	"image/color"
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

// TestGoldenChrome pins the window chrome byte-exactly: margin fill,
// Colorful bar with VHS's dot geometry, visual padding, and rounded
// corners revealing the margin — plus grid content (text and sprites)
// at its shifted origin. VHS parity semantics per chrome.go.
func TestGoldenChrome(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")

	// Probe metrics first: the canvas must be derived like foley does.
	probe, err := raster.New(raster.Options{Pack: pack, FontSizePx: 16, Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	cw, ch := probe.LogicalCellSize()

	const (
		cols, rows      = 24, 5
		margin, padding = 10, 8
		barSize, radius = 18, 8
	)
	win := raster.Window{
		CanvasW:    cols*cw + 2*(margin+padding),
		CanvasH:    rows*ch + 2*(margin+padding) + barSize,
		Padding:    padding,
		Margin:     margin,
		MarginFill: raster.Fill{Color: color.RGBA{R: 0x6b, G: 0x50, B: 0xff, A: 0xff}},
		Bar:        raster.BarColorful,
		BarSize:    barSize,
		BarColor:   color.RGBA{R: 0x20, G: 0x20, B: 0x28, A: 0xff},
		Radius:     radius,
	}
	r, err := raster.New(raster.Options{Pack: pack, FontSizePx: 16, Scale: 2, Window: win})
	if err != nil {
		t.Fatal(err)
	}

	geo := vtengine.Geometry{Cols: cols, Rows: rows, CellW: cw, CellH: ch}
	e := fake.New(vtengine.Options{Geometry: geo})
	defer func() { _ = e.Close() }()
	e.SetColors(vtengine.Colors{
		FG: vtengine.RGB{R: 0xc0, G: 0xc5, B: 0xd4},
		BG: vtengine.RGB{R: 0x16, G: 0x18, B: 0x20},
	})
	e.SetCursor(vtengine.Cursor{Visible: false})
	writeAt := func(x, y int, s string, st vtengine.Style) {
		for _, rn := range s {
			e.SetCell(x, y, string(rn), st)
			x++
		}
	}
	writeAt(0, 0, "chrome parity", vtengine.Style{FG: vtengine.RGB{R: 0xd9, G: 0x77, B: 0x57}, Bold: true})
	writeAt(0, 2, "╭─ dressed ─╮ ▀▄█░", vtengine.Style{FG: vtengine.RGB{R: 0xa6, G: 0xe3, B: 0xa1}})
	writeAt(0, 4, "corner reveal below", vtengine.Style{FG: vtengine.RGB{R: 0xc0, G: 0xc5, B: 0xd4}})

	var f vtengine.Frame
	if err := e.Snapshot(&f); err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(&f, e, nil)
	if err != nil {
		t.Fatal(err)
	}

	s := 2 // Scale
	wantW, wantH := win.CanvasW*s, win.CanvasH*s
	if img.Bounds().Dx() != wantW || img.Bounds().Dy() != wantH {
		t.Fatalf("canvas = %dx%d, want %dx%d (the declared window size, exactly)",
			img.Bounds().Dx(), img.Bounds().Dy(), wantW, wantH)
	}
	// Spot checks before the byte compare, so failures explain themselves:
	// the outermost corner pixel is pure margin fill (radius reveal)...
	if got := img.RGBAAt(margin*s, margin*s); got != (color.RGBA{R: 0x6b, G: 0x50, B: 0xff, A: 0xff}) {
		t.Fatalf("window corner pixel = %v, want the margin fill (rounded reveal)", got)
	}
	// ...the first bar dot sits at VHS's geometry with VHS's red...
	dotRad := (barSize * s) / 6
	dotGap := (barSize*s - 2*dotRad) / 2
	dx := margin*s + dotGap + dotRad
	dy := margin*s + dotRad + dotGap
	if got := img.RGBAAt(dx, dy); got != (color.RGBA{R: 0xFF, G: 0x4F, B: 0x4D, A: 0xFF}) {
		t.Fatalf("first bar dot = %v at (%d,%d), want VHS red #FF4F4D", got, dx, dy)
	}
	// ...and the padding band is the theme background.
	padX := (margin+padding/2)*s + 2
	padY := (margin + barSize + padding/2) * s
	if got := img.RGBAAt(padX, padY); got != (color.RGBA{R: 0x16, G: 0x18, B: 0x20, A: 0xff}) {
		t.Fatalf("padding pixel = %v, want theme background", got)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	goldenPath := filepath.Join("testdata", "golden", "chrome-v1.png")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, buf.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("golden updated: %s", goldenPath)
		return
	}
	want, err := os.ReadFile(goldenPath) //nolint:gosec // repo testdata
	testassets.Require(t, err, "run this test with -update")
	if !bytes.Equal(buf.Bytes(), want) {
		diffPath := filepath.Join(t.TempDir(), "chrome-got.png")
		_ = os.WriteFile(diffPath, buf.Bytes(), 0o600)
		t.Fatalf("chrome differs from golden; got written to %s", diffPath)
	}
}
