package raster

import (
	"image"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/fontpack"
	"github.com/GH-Jaider/foley/internal/testassets"
)

// loadPackT loads the pinned font pack or skips with the recipe.
func loadPackT(t *testing.T) *fontpack.Pack {
	t.Helper()
	pack, err := fontpack.Load(filepath.Join("..", "fontpack", "fonts"))
	testassets.Require(t, err, "make fonts")
	return pack
}

// TestEaseInOutCubicQ16 pins the house curve at the points where the
// fixed-point math is EXACT by hand: cubic powers of dyadic fractions
// stay dyadic, so these are equalities, not approximations.
func TestEaseInOutCubicQ16(t *testing.T) {
	cases := []struct{ t, want int64 }{
		{0, 0},
		{q16One, q16One},
		{-5, 0},
		{q16One + 5, q16One},
		{q16One / 2, q16One / 2}, // 4(½)³ = ½ exactly
		{q16One / 4, 4096},       // 4(¼)³ = 1/16 = 0.0625
		{3 * q16One / 4, 61440},  // 1-(½)³/2 = 0.9375
		{q16One / 8, 512},        // 4(⅛)³ = 1/128
	}
	for _, c := range cases {
		if got := easeInOutCubicQ16(c.t); got != c.want {
			t.Fatalf("ease(%d) = %d, want %d", c.t, got, c.want)
		}
	}
	// Monotonic across the whole domain — a camera that backs up
	// mid-move would read as a glitch.
	prev := int64(-1)
	for i := int64(0); i <= q16One; i += 256 {
		v := easeInOutCubicQ16(i)
		if v < prev {
			t.Fatalf("ease not monotonic at t=%d: %d < %d", i, v, prev)
		}
		prev = v
	}
}

// TestDownscaleAreaTable is the scaler's audit table: exact means at
// 2:1, pure copy at 1:1, a hand-computed rational 3:2, the pinned
// round-half-up rule, offset source regions, and the constant-image
// property at an awkward ratio.
func TestDownscaleAreaTable(t *testing.T) {
	px := func(img *image.RGBA, x, y int, v uint8) {
		o := img.PixOffset(x, y)
		img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = v, v, v, 0xff
	}
	at := func(img *image.RGBA, x, y int) uint8 { return img.Pix[img.PixOffset(x, y)] }

	t.Run("two_to_one_exact_mean", func(t *testing.T) {
		src := image.NewRGBA(image.Rect(0, 0, 2, 2))
		px(src, 0, 0, 10)
		px(src, 1, 0, 20)
		px(src, 0, 1, 30)
		px(src, 1, 1, 40)
		dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
		downscaleArea(dst, src, src.Bounds())
		if got := at(dst, 0, 0); got != 25 {
			t.Fatalf("2:1 mean = %d, want 25", got)
		}
	})
	t.Run("round_half_up", func(t *testing.T) {
		src := image.NewRGBA(image.Rect(0, 0, 2, 2))
		px(src, 0, 0, 10)
		px(src, 1, 0, 10)
		px(src, 0, 1, 10)
		px(src, 1, 1, 11) // mean 10.25 → 10; make 10.5: values 10,10,11,11
		dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
		downscaleArea(dst, src, src.Bounds())
		if got := at(dst, 0, 0); got != 10 {
			t.Fatalf("10.25 rounds to %d, want 10", got)
		}
		px(src, 0, 1, 11)
		downscaleArea(dst, src, src.Bounds())
		if got := at(dst, 0, 0); got != 11 {
			t.Fatalf("10.5 rounds to %d, want 11 (half-up)", got)
		}
	})
	t.Run("one_to_one_copy", func(t *testing.T) {
		src := image.NewRGBA(image.Rect(0, 0, 4, 4))
		px(src, 2, 1, 200)
		dst := image.NewRGBA(image.Rect(0, 0, 2, 2))
		downscaleArea(dst, src, image.Rect(1, 0, 3, 2)) // offset 1:1 crop
		if got := at(dst, 1, 1); got != 200 {
			t.Fatalf("1:1 crop copy = %d, want 200", got)
		}
	})
	t.Run("rational_three_to_two", func(t *testing.T) {
		// 3×3 → 2×2: output (0,0) weights (2,1)⊗(2,1)/9. Values chosen
		// so the weighted sum is exact: 4·9+2·18+2·27+1·36 = 162 = 9·18.
		src := image.NewRGBA(image.Rect(0, 0, 3, 3))
		px(src, 0, 0, 9)
		px(src, 1, 0, 18)
		px(src, 0, 1, 27)
		px(src, 1, 1, 36)
		dst := image.NewRGBA(image.Rect(0, 0, 2, 2))
		downscaleArea(dst, src, src.Bounds())
		if got := at(dst, 0, 0); got != 18 {
			t.Fatalf("3:2 weighted mean = %d, want 18", got)
		}
	})
	t.Run("constant_stays_constant", func(t *testing.T) {
		src := image.NewRGBA(image.Rect(0, 0, 7, 5))
		for y := 0; y < 5; y++ {
			for x := 0; x < 7; x++ {
				px(src, x, y, 137)
			}
		}
		dst := image.NewRGBA(image.Rect(0, 0, 3, 2))
		downscaleArea(dst, src, src.Bounds())
		for y := 0; y < 2; y++ {
			for x := 0; x < 3; x++ {
				if got := at(dst, x, y); got != 137 {
					t.Fatalf("constant image broke at %d,%d: %d", x, y, got)
				}
			}
		}
	})
}

// TestCameraTrackContinuity pins the invariant: the camera NEVER
// teleports — a retarget mid-transition departs from the interpolated
// state of that exact instant, and off returns to identity.
func TestCameraTrackContinuity(t *testing.T) {
	world := image.Rect(0, 0, 4800, 2400)
	ct := NewCameraTrack(world)
	target := image.Rect(1200, 600, 3600, 1800) // centered 2× zoom

	at := func(d time.Duration) image.Rectangle {
		ct.SetTime(d)
		return ct.Viewport()
	}
	if at(0) != world {
		t.Fatalf("initial viewport = %v, want identity", at(0))
	}
	ct.MoveTo(target, time.Second, 600*time.Millisecond)
	if at(time.Second) != world {
		t.Fatalf("at move start = %v, want still identity (ease(0)=0)", at(time.Second))
	}
	if got := at(1600 * time.Millisecond); got != target {
		t.Fatalf("at move end = %v, want target", got)
	}
	// Exact midpoint: ease(½)=½ exactly → the mean rect.
	mid := at(1300 * time.Millisecond)
	want := image.Rect(600, 300, 4200, 2100)
	if mid != want {
		t.Fatalf("midpoint = %v, want %v", mid, want)
	}

	// Retarget mid-move: continuity — the new move's departure equals
	// the state the instant it was issued.
	before := at(1300 * time.Millisecond)
	ct.MoveTo(world, 1300*time.Millisecond, 400*time.Millisecond)
	if got := at(1300 * time.Millisecond); got != before {
		t.Fatalf("retarget teleported: %v != %v", got, before)
	}
	if got := at(1700 * time.Millisecond); got != world {
		t.Fatalf("after retarget end = %v, want identity", got)
	}

	// Off identity mid-move, back at identity once settled.
	if at(1500*time.Millisecond) == world {
		t.Fatal("mid-move camera must be off identity")
	}
	if at(3*time.Second) != world {
		t.Fatal("settled camera must be back at identity")
	}

	// The OVERLAP is continuous too: while the superseded move's window
	// is still open, the new move governs — sample the whole span and
	// bound the per-step delta (a teleport is a hundreds-of-px jump; the
	// eased path moves at most a few px per 10ms here).
	prev := at(1300 * time.Millisecond)
	for tt := 1310 * time.Millisecond; tt <= 1800*time.Millisecond; tt += 10 * time.Millisecond {
		cur := at(tt)
		for _, d := range []int{
			cur.Min.X - prev.Min.X, cur.Min.Y - prev.Min.Y,
			cur.Max.X - prev.Max.X, cur.Max.Y - prev.Max.Y,
		} {
			if d < -60 || d > 60 {
				t.Fatalf("camera teleported at %v: %v -> %v", tt, prev, cur)
			}
		}
		prev = cur
	}
}

// TestCameraBreakpoints pins the quantization: ~33ms steps, floor 4,
// exact end included, [from, to) half-open, sorted and deduplicated.
func TestCameraBreakpoints(t *testing.T) {
	ct := NewCameraTrack(image.Rect(0, 0, 100, 50))
	ct.MoveTo(image.Rect(10, 10, 60, 35), time.Second, 600*time.Millisecond)
	cuts := ct.Breakpoints(0, 10*time.Second)
	if len(cuts) != 19 { // 600/33 = 18 steps → 19 instants
		t.Fatalf("cuts = %d (%v), want 19", len(cuts), cuts)
	}
	if cuts[0] != time.Second || cuts[len(cuts)-1] != 1600*time.Millisecond {
		t.Fatalf("cuts span %v..%v, want 1s..1.6s", cuts[0], cuts[len(cuts)-1])
	}
	prev := time.Duration(-1)
	for _, c := range cuts {
		if c <= prev {
			t.Fatalf("cuts not strictly increasing: %v", cuts)
		}
		prev = c
	}
	if got := ct.Breakpoints(0, time.Second); len(got) != 0 {
		t.Fatalf("[from, to) must exclude to: %v", got)
	}

	// A short snap still animates: floor of 4 steps.
	ct2 := NewCameraTrack(image.Rect(0, 0, 100, 50))
	ct2.MoveTo(image.Rect(0, 0, 50, 25), 0, 50*time.Millisecond)
	if got := ct2.Breakpoints(0, time.Second); len(got) != 5 {
		t.Fatalf("snap cuts = %d (%v), want 5 (4 steps)", len(got), got)
	}
}

// TestCameraMoveToRestIsNoOp pins the waste guard: a move to where the
// camera already rests emits NO cuts — no transition's worth of
// identical frames for `zoom off` at identity or re-zooming the same
// rect.
func TestCameraMoveToRestIsNoOp(t *testing.T) {
	world := image.Rect(0, 0, 100, 50)
	target := image.Rect(10, 10, 60, 35)
	ct := NewCameraTrack(world)

	ct.Reset(0, 600*time.Millisecond) // off before any zoom
	if got := ct.Breakpoints(0, 10*time.Second); len(got) != 0 {
		t.Fatalf("off-at-identity emitted cuts: %v", got)
	}

	ct.MoveTo(target, time.Second, 600*time.Millisecond)
	before := ct.Breakpoints(0, time.Minute)
	ct.MoveTo(target, 3*time.Second, 600*time.Millisecond) // settled: same rect again
	after := ct.Breakpoints(0, time.Minute)
	if len(after) != len(before) {
		t.Fatalf("re-zooming the settled rect added cuts: %d -> %d", len(before), len(after))
	}
}

// TestSuperSampleGeometryIdentity is THE camera guarantee: the master
// (SuperSample 2) has EXACTLY the same logical grid as the plain
// render — same pty winsize, same wrapping, same footage. Scaled
// metrics are exact multiples, never re-rounded.
func TestSuperSampleGeometryIdentity(t *testing.T) {
	pack := loadPackT(t)
	win := Window{CanvasW: 700, CanvasH: 400, Padding: 14}
	base, err := New(Options{Pack: pack, FontSizePx: 16, Scale: 2, Window: win})
	if err != nil {
		t.Fatal(err)
	}
	master, err := New(Options{Pack: pack, FontSizePx: 16, Scale: 2, SuperSample: 2, Window: win})
	if err != nil {
		t.Fatal(err)
	}
	bw, bh := base.LogicalCellSize()
	mw, mh := master.LogicalCellSize()
	if bw != mw || bh != mh {
		t.Fatalf("logical cell diverged: base %dx%d vs master %dx%d — the footage would change", bw, bh, mw, mh)
	}
	bcw, bch := base.CellSize()
	mcw, mch := master.CellSize()
	if mcw != 2*bcw || mch != 2*bch {
		t.Fatalf("master cell %dx%d is not exactly 2× base %dx%d", mcw, mch, bcw, bch)
	}
	if master.SuperSample() != 2 || base.SuperSample() != 1 {
		t.Fatalf("supersample factors: master %d base %d", master.SuperSample(), base.SuperSample())
	}
}

// TestCompositeWorldHUDSeam pins the compositor's stratification: the
// WORLD part of the output is the viewport, the HUD band under it is a
// fixed 2:1 of the master's band — never zoomed — and the seam lands on
// the exact row. Constant colors survive every scaler path exactly, so
// these are equalities.
func TestCompositeWorldHUDSeam(t *testing.T) {
	pack := loadPackT(t)
	win := Window{CanvasW: 200, CanvasH: 120, KeysBand: 20}
	r, err := New(Options{Pack: pack, FontSizePx: 16, Scale: 2, SuperSample: 2, Window: win})
	if err != nil {
		t.Fatal(err)
	}
	world := r.WorldRect()
	master := image.NewRGBA(image.Rect(0, 0, win.CanvasW*4, win.CanvasH*4))
	fill := func(rect image.Rectangle, red, green, blue uint8) {
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			o := master.PixOffset(rect.Min.X, y)
			for x := rect.Min.X; x < rect.Max.X; x++ {
				master.Pix[o], master.Pix[o+1], master.Pix[o+2], master.Pix[o+3] = red, green, blue, 0xff
				o += 4
			}
		}
	}
	band := image.Rect(0, world.Max.Y, master.Bounds().Max.X, master.Bounds().Max.Y)
	fill(world, 200, 10, 10) // world: red
	fill(band, 10, 10, 200)  // HUD band: blue

	check := func(out *image.RGBA, what string) {
		t.Helper()
		outW := out.Bounds().Dx()
		worldOutH := world.Dy() / 2
		at := func(x, y int) [3]uint8 {
			o := out.PixOffset(x, y)
			return [3]uint8{out.Pix[o], out.Pix[o+1], out.Pix[o+2]}
		}
		if got := at(outW/2, worldOutH/2); got != [3]uint8{200, 10, 10} {
			t.Fatalf("%s: world pixel = %v, want red", what, got)
		}
		if got := at(outW/2, worldOutH-1); got != [3]uint8{200, 10, 10} {
			t.Fatalf("%s: last world row = %v, want red (seam leaked)", what, got)
		}
		if got := at(outW/2, worldOutH); got != [3]uint8{10, 10, 200} {
			t.Fatalf("%s: first band row = %v, want blue (HUD zoomed or shifted)", what, got)
		}
		if got := at(outW/2, out.Bounds().Dy()-1); got != [3]uint8{10, 10, 200} {
			t.Fatalf("%s: last band row = %v, want blue", what, got)
		}
	}

	// Camera at rest: one contiguous 2:1 pass.
	check(r.Composite(master, world, nil), "identity")
	// Camera engaged at full 2× (a 1:1 world crop): the band must stay
	// a 2:1 of the master band, glued and unzoomed.
	vp := image.Rect(0, 0, world.Dx()/2, world.Dy()/2)
	check(r.Composite(master, vp, nil), "full zoom")
	// A transition frame (general rational ratio) with a reused dst of
	// the right size.
	vp = image.Rect(100, 60, 100+world.Dx()*3/4, 60+world.Dy()*3/4)
	dst := image.NewRGBA(image.Rect(0, 0, win.CanvasW*2, win.CanvasH*2))
	out := r.Composite(master, vp, dst)
	if out != dst {
		t.Fatal("right-sized dst was not reused")
	}
	check(out, "transition")
}

// TestZoomTarget pins the cell→viewport math: aspect expansion around
// the center, the LOUD 2× cap with its recipe, huge rects settling at
// identity, and edge clamping by shift.
func TestZoomTarget(t *testing.T) {
	pack := loadPackT(t)
	win := Window{CanvasW: 700, CanvasH: 400, Padding: 14}
	r, err := New(Options{Pack: pack, FontSizePx: 16, Scale: 2, SuperSample: 2, Window: win})
	if err != nil {
		t.Fatal(err)
	}
	world := r.WorldRect()

	// A half-world rect: legal, aspect-correct, inside the world.
	cw, chh := r.CellSize()
	colsHalf := world.Dx() / (2 * cw)
	rowsHalf := world.Dy() / (2 * chh)
	vp, err := r.ZoomTarget(1, 1, colsHalf, rowsHalf)
	if err != nil {
		t.Fatal(err)
	}
	if !vp.In(world) {
		t.Fatalf("viewport %v escapes the world %v", vp, world)
	}
	if vp.Dx()*world.Dy() < world.Dx()*(vp.Dy()-2) || vp.Dx()*world.Dy() > world.Dx()*(vp.Dy()+2) {
		t.Fatalf("viewport %v aspect diverges from world %v", vp, world)
	}

	// Tiny rect: over the sharp cap → LOUD error with the recipe.
	if _, err := r.ZoomTarget(0, 0, 2, 1); err == nil || !strings.Contains(err.Error(), "sharp limit") {
		t.Fatalf("tiny rect must refuse loudly, got %v", err)
	}

	// A rect bigger than the world settles at identity.
	vp, err = r.ZoomTarget(0, 0, 500, 200)
	if err != nil {
		t.Fatal(err)
	}
	if vp != world {
		t.Fatalf("huge rect = %v, want identity %v", vp, world)
	}

	// A corner rect clamps by shifting, not by shrinking.
	vp, err = r.ZoomTarget(0, 0, colsHalf, rowsHalf)
	if err != nil {
		t.Fatal(err)
	}
	if vp.Min.X < world.Min.X || vp.Min.Y < world.Min.Y {
		t.Fatalf("corner viewport %v escapes %v", vp, world)
	}
}
