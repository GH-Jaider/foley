package raster_test

import (
	"path/filepath"
	"testing"

	"github.com/GH-Jaider/foley/internal/fontpack"
	"github.com/GH-Jaider/foley/internal/raster"
	"github.com/GH-Jaider/foley/internal/testassets"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/fake"
)

// TestPlacementScalerMatchesKitty pins the scaler rule against kitty's
// ground truth: fractional-ratio placements are FILTERED (a 1px texture
// line blends away instead of surviving at full contrast as a phantom
// cut — tenten's live finding), while exact integer upscales stay
// nearest-neighbor crisp.
func TestPlacementScalerMatchesKitty(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	r, err := raster.New(raster.Options{Pack: pack, FontSizePx: 16, Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	cw, ch := r.LogicalCellSize()
	geo := vtengine.Geometry{Cols: 30, Rows: 12, CellW: cw, CellH: ch}
	e := fake.New(vtengine.Options{Geometry: geo})
	defer func() { _ = e.Close() }()

	// Fine texture: light 4px blocks separated by dark 1px lines.
	const w, h = 200, 200
	pix := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			if x%5 == 4 || y%5 == 4 {
				pix[i], pix[i+1], pix[i+2], pix[i+3] = 0x30, 0x70, 0x50, 0xff
			} else {
				pix[i], pix[i+1], pix[i+2], pix[i+3] = 0x60, 0xd0, 0x90, 0xff
			}
		}
	}
	e.SetImage(vtengine.ImageData{ID: 1, W: w, H: h, Generation: 1, Pix: pix})

	// Fractional downscale: 200px into 3 rows of logical cells.
	e.AddPlacement(vtengine.Placement{
		ImageID: 1, Col: 1, Row: 1,
		PixelW: uint32(6 * cw), PixelH: uint32(3 * ch), //nolint:gosec // test values
		SrcW: w, SrcH: h,
	})

	var f vtengine.Frame
	if err := e.Snapshot(&f); err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(&f, e, nil)
	if err != nil {
		t.Fatal(err)
	}

	scaledW, scaledH := r.CellSize()
	x0, y0 := 1*scaledW, 1*scaledH
	ww, hh := 6*scaledW, 3*scaledH
	// Row luminance: with a filtered scaler no row may sit at (or below)
	// the raw dark-line level while its neighbors sit at the raw block
	// level — that full-contrast survival is the phantom-cut signature.
	rowLum := func(y int) float64 {
		var sum float64
		for x := 0; x < ww; x++ {
			c := img.RGBAAt(x0+x, y0+y)
			sum += float64(c.R) + float64(c.G) + float64(c.B)
		}
		return sum / float64(ww)
	}
	const rawLine, rawBlock = 0x30 + 0x70 + 0x50, 0x60 + 0xd0 + 0x90
	phantom := 0
	for y := 1; y < hh-1; y++ {
		l, up, down := rowLum(y), rowLum(y-1), rowLum(y+1)
		if l < rawLine+20 && up > rawBlock-30 && down > rawBlock-30 {
			phantom++
		}
	}
	if phantom > 0 {
		t.Fatalf("%d phantom full-contrast lines survived a fractional downscale (NN sampling instead of filtering)", phantom)
	}
}

// TestZeroAreaPlacementIsSkipped: Render is the robustness boundary — a
// placement with empty source or destination (an engine's unresolvable
// placement, a degenerate rect) must be a no-op, never a divide-by-zero
// in the scaler-ratio math (parada finding, reproduced as a panic).
func TestZeroAreaPlacementIsSkipped(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	r, err := raster.New(raster.Options{Pack: pack, FontSizePx: 16, Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	cw, ch := r.LogicalCellSize()
	geo := vtengine.Geometry{Cols: 10, Rows: 3, CellW: cw, CellH: ch}
	e := fake.New(vtengine.Options{Geometry: geo})
	defer func() { _ = e.Close() }()
	pix := make([]byte, 4*4*4)
	for i := range pix {
		pix[i] = 0xff
	}
	e.SetImage(vtengine.ImageData{ID: 1, W: 4, H: 4, Generation: 1, Pix: pix})
	e.AddPlacement(vtengine.Placement{ImageID: 1})                       // all-zero rects
	e.AddPlacement(vtengine.Placement{ImageID: 1, SrcW: 4, SrcH: 4})     // zero destination
	e.AddPlacement(vtengine.Placement{ImageID: 1, PixelW: 8, PixelH: 8}) // zero source
	var f vtengine.Frame
	if err := e.Snapshot(&f); err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(&f, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Nothing painted: the canvas stays theme background everywhere.
	bg := img.RGBAAt(0, 0)
	b := img.Bounds()
	for y := 0; y < b.Max.Y; y += 3 {
		for x := 0; x < b.Max.X; x += 3 {
			if img.RGBAAt(x, y) != bg {
				t.Fatalf("zero-area placement painted pixel (%d,%d)", x, y)
			}
		}
	}
}
