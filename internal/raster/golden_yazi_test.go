package raster_test

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GH-Jaider/foley/internal/fontpack"
	"github.com/GH-Jaider/foley/internal/raster"
	"github.com/GH-Jaider/foley/internal/testassets"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/fake"
)

// loadTestPNG decodes a committed testdata PNG into engine ImageData —
// real decoded photo content (gradients, curves), not synthetic arrays.
func loadTestPNG(t *testing.T, name string, id uint32) vtengine.ImageData {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name)) //nolint:gosec // repo testdata
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	nrgba := image.NewNRGBA(b)
	draw.Draw(nrgba, b, img, b.Min, draw.Src)
	return vtengine.ImageData{ID: id, W: b.Dx(), H: b.Dy(), Generation: 1, Pix: nrgba.Pix}
}

// TestGoldenYaziScene is the M6 leftover: a file-manager layout (three
// panes of box-drawing borders, a selected row, dotfiles faint) with two
// REAL decoded PNGs (regenerable: testdata/gen_photos.go) placed like
// yazi's previews, both at fractional ratios. Byte-exact like every
// golden.
func TestGoldenYaziScene(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	r, err := raster.New(raster.Options{Pack: pack, FontSizePx: 16, Scale: 2})
	if err != nil {
		t.Fatal(err)
	}

	geo := vtengine.Geometry{Cols: 62, Rows: 14}
	geo.CellW, geo.CellH = r.LogicalCellSize()
	e := fake.New(vtengine.Options{Geometry: geo})
	defer func() { _ = e.Close() }()

	colors := vtengine.Colors{
		FG: vtengine.RGB{R: 0xc0, G: 0xc5, B: 0xd4},
		BG: vtengine.RGB{R: 0x16, G: 0x18, B: 0x20},
	}
	e.SetColors(colors)
	e.SetCursor(vtengine.Cursor{Visible: false})

	dim := vtengine.RGB{R: 0x5c, G: 0x63, B: 0x70}
	accent := vtengine.RGB{R: 0xd9, G: 0x77, B: 0x57}
	sel := vtengine.RGB{R: 0x2a, G: 0x2f, B: 0x3d}

	writeAt := func(x, y int, s string, st vtengine.Style) {
		for _, rn := range s {
			e.SetCell(x, y, string(rn), st)
			x++
		}
	}
	border := vtengine.Style{FG: dim}

	// Three panes: parent (0-13), current (14-37), preview (38-61).
	for _, p := range []struct{ x0, x1 int }{{0, 13}, {14, 37}, {38, 61}} {
		writeAt(p.x0, 0, "╭"+strings.Repeat("─", p.x1-p.x0-1)+"╮", border)
		for y := 1; y < 13; y++ {
			writeAt(p.x0, y, "│", border)
			writeAt(p.x1, y, "│", border)
		}
		writeAt(p.x0, 13, "╰"+strings.Repeat("─", p.x1-p.x0-1)+"╯", border)
	}

	writeAt(2, 0, " ~/pics ", vtengine.Style{FG: accent, Bold: true})
	parent := []string{"docs", "pics", "src"}
	for i, name := range parent {
		st := vtengine.Style{FG: colors.FG}
		if name == "pics" {
			st = vtengine.Style{FG: accent}
		}
		writeAt(2, 1+i, name, st)
	}

	files := []struct {
		name string
		st   vtengine.Style
	}{
		{"..", vtengine.Style{FG: dim}},
		{"leaf.png", vtengine.Style{FG: colors.FG}},
		{"sunset.png", vtengine.Style{FG: colors.FG, BG: sel, HasBG: true, Bold: true}},
		{".hidden", vtengine.Style{FG: dim, Faint: true}},
		{"notes.txt", vtengine.Style{FG: colors.FG}},
	}
	for i, f := range files {
		writeAt(16, 1+i, f.name, f.st)
	}
	writeAt(16, 12, "3/5 · 2 imgs", vtengine.Style{FG: dim, Italic: true})

	// Preview pane: the selected sunset photo scaled to the pane and
	// the leaf as a thumbnail — both at FRACTIONAL ratios (filtered
	// path). The integer-upscale NearestNeighbor rule is pinned by
	// TestIntegerUpscaleStaysCrisp in placement_scale_test.go.
	sunset := loadTestPNG(t, "photo-sunset.png", 1)
	leaf := loadTestPNG(t, "photo-leaf.png", 2)
	e.SetImage(sunset)
	e.SetImage(leaf)
	e.AddPlacement(vtengine.Placement{
		ImageID: 1, Col: 39, Row: 1,
		PixelW: uint32(22 * geo.CellW), PixelH: uint32(11 * geo.CellH), //nolint:gosec // test values
		SrcW: uint32(sunset.W), SrcH: uint32(sunset.H), //nolint:gosec // test values
	})
	e.AddPlacement(vtengine.Placement{
		ImageID: 2, Col: 2, Row: 6,
		PixelW: uint32(6 * geo.CellW), PixelH: uint32(6 * geo.CellH), //nolint:gosec // test values
		SrcW: uint32(leaf.W), SrcH: uint32(leaf.H), //nolint:gosec // test values
	})

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
	goldenPath := filepath.Join("testdata", "golden", "yazi-v1.png")
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
		diffPath := filepath.Join(t.TempDir(), "yazi-got.png")
		_ = os.WriteFile(diffPath, buf.Bytes(), 0o600)
		t.Fatalf("yazi scene differs from golden; got written to %s", diffPath)
	}
}
