package raster_test

import (
	"image/color"
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

// TestIntegerUpscaleStaysCrisp pins the NearestNeighbor half of the
// scaler rule — the goldens only exercise the filtered half, so without
// this test the NN branch could be deleted with every golden green. An
// exact uniform integer upscale must reproduce each source pixel as a
// solid block: no blended pixels anywhere.
func TestIntegerUpscaleStaysCrisp(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	r, err := raster.New(raster.Options{Pack: pack, FontSizePx: 16, Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	cw, ch := r.LogicalCellSize()
	geo := vtengine.Geometry{Cols: 20, Rows: 6, CellW: cw, CellH: ch}
	e := fake.New(vtengine.Options{Geometry: geo})
	defer func() { _ = e.Close() }()

	red := [4]byte{0xff, 0x00, 0x00, 0xff}
	blue := [4]byte{0x00, 0x00, 0xff, 0xff}
	pix := append(append(append(append([]byte{}, red[:]...), blue[:]...), blue[:]...), red[:]...)
	e.SetImage(vtengine.ImageData{ID: 1, W: 2, H: 2, Generation: 1, Pix: pix})
	// 2x2 source into 8x8 LOGICAL px → 16x16 device px: exact uniform 8x.
	e.AddPlacement(vtengine.Placement{ImageID: 1, Col: 1, Row: 1, PixelW: 8, PixelH: 8, SrcW: 2, SrcH: 2})

	var f vtengine.Frame
	if err := e.Snapshot(&f); err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(&f, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	scaledW, scaledH := r.CellSize()
	x0, y0 := scaledW, scaledH
	wantRed := color.RGBA{R: 0xff, A: 0xff}
	wantBlue := color.RGBA{B: 0xff, A: 0xff}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			want := wantRed
			if (x >= 8) != (y >= 8) {
				want = wantBlue
			}
			if got := img.RGBAAt(x0+x, y0+y); got != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v — integer upscale must be crisp NN, no blending", x, y, got, want)
			}
		}
	}
}

// TestTranslucentPlacementUsesStraightAlpha is the canonical NRGBA/RGBA
// regression detector. ImageData.Pix is STRAIGHT alpha by contract and
// kittyImage converts it to premultiplied before blending; misreading it
// as already-premultiplied would leave every OPAQUE golden byte-identical
// while translucent pixels blend too bright. photo-glass.png (regenerable
// via gen_photos.go) carries the exact values this asserts.
func TestTranslucentPlacementUsesStraightAlpha(t *testing.T) {
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	r, err := raster.New(raster.Options{Pack: pack, FontSizePx: 16, Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	cw, ch := r.LogicalCellSize()
	geo := vtengine.Geometry{Cols: 20, Rows: 8, CellW: cw, CellH: ch}
	e := fake.New(vtengine.Options{Geometry: geo})
	defer func() { _ = e.Close() }()
	e.SetColors(vtengine.Colors{BG: vtengine.RGB{}}) // black: exact blend targets
	e.SetCursor(vtengine.Cursor{Visible: false})

	glass := loadTestPNG(t, "photo-glass.png", 1)
	e.SetImage(glass)
	// 32x32 into 64x64 logical → 128x128 device: exact 4x, NN path — the
	// alpha conversion must hold on BOTH scaler branches.
	e.AddPlacement(vtengine.Placement{
		ImageID: 1, Col: 1, Row: 1, PixelW: 64, PixelH: 64,
		SrcW: uint32(glass.W), SrcH: uint32(glass.H), //nolint:gosec // test values
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
	// Source px (sx,sy) → device px block origin + center offset.
	at := func(sx, sy int) color.RGBA {
		return img.RGBAAt(scaledW+sx*4+2, scaledH+sy*4+2)
	}
	// Fully transparent (x=0, top rows): background shows exactly.
	if got := at(0, 0); got != (color.RGBA{A: 0xff}) {
		t.Fatalf("alpha-0 pixel = %v, want pure background", got)
	}
	// Fully opaque (x=31, top rows): pure red.
	if got := at(31, 0); got != (color.RGBA{R: 0xff, A: 0xff}) {
		t.Fatalf("alpha-255 pixel = %v, want pure red", got)
	}
	// The half-alpha band (straight 255,0,0,128 over black): straight
	// Over gives R≈128. The premultiplied-misread bug gives R=255.
	got := at(8, 16)
	if got.R < 126 || got.R > 130 || got.G != 0 || got.B != 0 {
		t.Fatalf("half-alpha pixel = %v, want R≈128 (straight-alpha Over; 255 means Pix was misread as premultiplied)", got)
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
