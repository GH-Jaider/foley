package raster

import (
	"errors"
	"fmt"
	"image"
	"sort"
	"sync"
	"time"
)

// The camera (ADR-019): a viewport over the supersampled MASTER frame,
// eased on the virtual clock, downscaled to the output with exact
// integer arithmetic. Constitutional rule: no float ever crosses the
// output boundary — the easing runs in Q16 fixed point and the scaler
// in rational integer weights, so the camera is byte-identical across
// architectures BY CONSTRUCTION.

// q16One is 1.0 in Q16 fixed point.
const q16One = 1 << 16

// easeInOutCubicQ16 maps t ∈ [0, q16One] through the house curve —
// the only curve (the knob is the DURATION, not the shape). Pure
// int64: t³ terms peak below 2^49.
func easeInOutCubicQ16(t int64) int64 {
	switch {
	case t <= 0:
		return 0
	case t >= q16One:
		return q16One
	case t < q16One/2:
		// 4t³ = 4 * t*t*t / one²
		return (4 * t * t * t) >> 32
	default:
		// 1 - (-2t+2)³/2
		u := 2*q16One - 2*t
		return q16One - ((u*u*u)>>32)/2
	}
}

// lerpQ16 interpolates a→b by the Q16 fraction e, in integers.
func lerpQ16(a, b int, e int64) int {
	return a + int((int64(b-a)*e)>>16)
}

// downscaleArea shrinks src into dst with an exact area-average box
// filter: every output pixel is the weighted mean of the source pixels
// it covers, weights as EXACT RATIONALS in integer arithmetic (units
// of 1/outW source pixels along X, 1/outH along Y). This is the
// correct filter for minification — and the camera only ever
// minifies: the viewport is never smaller than the output (the 2× cap
// is exactly the master's supersample). Ratio 1:1 degrades to a copy,
// 2:1 to a clean 2×2 mean. Rounding is round-half-up, pinned by test.
func downscaleArea(dst *image.RGBA, src *image.RGBA, sr image.Rectangle) {
	ob := dst.Bounds()
	outW, outH := ob.Dx(), ob.Dy()
	srcW, srcH := sr.Dx(), sr.Dy()
	if outW <= 0 || outH <= 0 || srcW <= 0 || srcH <= 0 {
		return
	}
	if srcW == outW && srcH == outH {
		// 1:1 — full zoom: a pure copy, no arithmetic at all.
		for y := 0; y < outH; y++ {
			so := src.PixOffset(sr.Min.X, sr.Min.Y+y)
			do := dst.PixOffset(ob.Min.X, ob.Min.Y+y)
			copy(dst.Pix[do:do+4*outW], src.Pix[so:so+4*outW])
		}
		return
	}
	if srcW == 2*outW && srcH == 2*outH {
		// 2:1 — identity view of the 2× master: exact 2×2 mean.
		for y := 0; y < outH; y++ {
			s0 := src.PixOffset(sr.Min.X, sr.Min.Y+2*y)
			s1 := src.PixOffset(sr.Min.X, sr.Min.Y+2*y+1)
			d := dst.PixOffset(ob.Min.X, ob.Min.Y+y)
			for x := 0; x < outW; x++ {
				o0, o1 := s0+8*x, s1+8*x
				dst.Pix[d] = uint8((int(src.Pix[o0]) + int(src.Pix[o0+4]) + int(src.Pix[o1]) + int(src.Pix[o1+4]) + 2) / 4)       //nolint:gosec // mean of bytes
				dst.Pix[d+1] = uint8((int(src.Pix[o0+1]) + int(src.Pix[o0+5]) + int(src.Pix[o1+1]) + int(src.Pix[o1+5]) + 2) / 4) //nolint:gosec
				dst.Pix[d+2] = uint8((int(src.Pix[o0+2]) + int(src.Pix[o0+6]) + int(src.Pix[o1+2]) + int(src.Pix[o1+6]) + 2) / 4) //nolint:gosec
				dst.Pix[d+3] = 0xff
				d += 4
			}
		}
		return
	}
	// General rational ratio (the transition frames): X spans measured
	// in units of 1/outW source pixels, Y in 1/outH — total weight per
	// output pixel is exactly srcW*srcH, so the divide is exact
	// bookkeeping, not approximation.
	total := int64(srcW) * int64(srcH)
	half := total / 2
	type span struct {
		idx []int   // source pixel index
		w   []int64 // its weight in units
	}
	spansFor := func(out, srcN int) []span {
		spans := make([]span, out)
		for o := 0; o < out; o++ {
			lo, hi := o*srcN, (o+1)*srcN // in units of 1/out
			s0, s1 := lo/out, (hi-1)/out
			for s := s0; s <= s1; s++ {
				a, b := s*out, (s+1)*out
				ov := min64(int64(hi), int64(b)) - max64(int64(lo), int64(a))
				if ov > 0 {
					spans[o].idx = append(spans[o].idx, s)
					spans[o].w = append(spans[o].w, ov)
				}
			}
		}
		return spans
	}
	xs := spansFor(outW, srcW)
	ys := spansFor(outH, srcH)
	for y := 0; y < outH; y++ {
		d := dst.PixOffset(ob.Min.X, ob.Min.Y+y)
		for x := 0; x < outW; x++ {
			var r, g, b int64
			for yi, sy := range ys[y].idx {
				wy := ys[y].w[yi]
				row := src.PixOffset(sr.Min.X, sr.Min.Y+sy)
				for xi, sx := range xs[x].idx {
					w := wy * xs[x].w[xi]
					o := row + 4*sx
					r += w * int64(src.Pix[o])
					g += w * int64(src.Pix[o+1])
					b += w * int64(src.Pix[o+2])
				}
			}
			dst.Pix[d] = uint8((r + half) / total)   //nolint:gosec // weighted mean of bytes
			dst.Pix[d+1] = uint8((g + half) / total) //nolint:gosec
			dst.Pix[d+2] = uint8((b + half) / total) //nolint:gosec
			dst.Pix[d+3] = 0xff
			d += 4
		}
	}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// camMove is one camera movement: from → target over [start, start+dur).
// from is the camera's state at start — captured when the move is
// added, so the camera NEVER teleports: a retarget mid-transition
// departs from the interpolated state of that instant.
type camMove struct {
	from, target image.Rectangle
	start        time.Duration
	dur          time.Duration
}

// CameraTrack holds the camera's movements and serves the driver's
// Overlay contract (SetTime/Breakpoints). Like HighlightTrack it is
// mutated by the recording goroutine while realtime's loop reads it —
// mutex; single-goroutine and contention-free in deterministic mode.
type CameraTrack struct {
	mu       sync.Mutex
	identity image.Rectangle // the full world rect on the master
	moves    []camMove
	t        time.Duration
}

// NewCameraTrack returns a camera at identity over the given world.
func NewCameraTrack(world image.Rectangle) *CameraTrack {
	return &CameraTrack{identity: world}
}

// stateAtLocked computes the viewport at t. Moves are in start order
// (the recorder's clock is monotonic) and a LATER move SUPERSEDES any
// still-running earlier one from its own start — its from captured the
// composite state at that instant, so a retarget mid-transition stays
// continuous instead of letting the old move finish and teleporting.
func (ct *CameraTrack) stateAtLocked(t time.Duration) image.Rectangle {
	state := ct.identity
	for _, m := range ct.moves {
		if t < m.start {
			break
		}
		if t >= m.start+m.dur {
			state = m.target
			continue
		}
		e := easeInOutCubicQ16(int64(t-m.start) * q16One / int64(m.dur))
		state = image.Rect(
			lerpQ16(m.from.Min.X, m.target.Min.X, e),
			lerpQ16(m.from.Min.Y, m.target.Min.Y, e),
			lerpQ16(m.from.Max.X, m.target.Max.X, e),
			lerpQ16(m.from.Max.Y, m.target.Max.Y, e),
		)
	}
	return state
}

// Viewport reports the camera rect at the track's current clock.
func (ct *CameraTrack) Viewport() image.Rectangle {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.stateAtLocked(ct.t)
}

// MoveTo animates the camera to target starting at the given instant.
// The departure point is the camera's state AT that instant — never a
// teleport. A move to where the camera already rests is a no-op: it
// must not emit a transition's worth of identical frames (`zoom off`
// at identity, re-zooming the same rect).
func (ct *CameraTrack) MoveTo(target image.Rectangle, at, dur time.Duration) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if dur <= 0 {
		dur = time.Millisecond
	}
	from := ct.stateAtLocked(at)
	if from == target {
		return
	}
	ct.moves = append(ct.moves, camMove{from: from, target: target, start: at, dur: dur})
}

// Reset animates back to identity — `zoom off`, reversible by
// construction.
func (ct *CameraTrack) Reset(at, dur time.Duration) {
	ct.MoveTo(ct.identity, at, dur)
}

// SetTime fixes the camera clock for the next render.
func (ct *CameraTrack) SetTime(t time.Duration) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.t = t
}

// camStepMs quantizes transitions: one frame about every 33ms of
// virtual time, floor 4 steps — bounded frames, smooth motion.
const camStepMs = 33

// Breakpoints reports the animation instants in [from, to): each
// move's quantized steps plus its exact end.
func (ct *CameraTrack) Breakpoints(from, to time.Duration) []time.Duration {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	var out []time.Duration
	add := func(t time.Duration) {
		if t >= from && t < to {
			out = append(out, t)
		}
	}
	for _, m := range ct.moves {
		steps := int(m.dur / (camStepMs * time.Millisecond))
		if steps < 4 {
			steps = 4
		}
		for i := 0; i <= steps; i++ {
			add(m.start + m.dur*time.Duration(i)/time.Duration(steps))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	dedup := out[:0]
	for i, t := range out {
		if i == 0 || t != out[i-1] {
			dedup = append(dedup, t)
		}
	}
	return dedup
}

// WorldRect is the camera's world on the master: the canvas minus the
// keys band — the HUD stays glued to the camera glass (ADR-019).
func (r *Rasterizer) WorldRect() image.Rectangle {
	win := r.opts.Window
	return image.Rect(0, 0, win.CanvasW*r.s, (win.CanvasH-win.KeysBand)*r.s)
}

// SuperSample reports the master's supersample factor (1 = no camera).
func (r *Rasterizer) SuperSample() int { return r.s / r.opts.Scale }

// ZoomTarget maps a CELL rect (0-based, the house standard) to a
// master viewport: expanded to the world's aspect around its center,
// capped at the supersample's sharp limit — beyond it foley would ship
// a blurry frame, and blurry never ships silently: it refuses with the
// recipe — then clamped inside the world.
func (r *Rasterizer) ZoomTarget(col, row, w, h int) (image.Rectangle, error) {
	world := r.WorldRect()
	x0 := r.orgX + col*r.cellW
	y0 := r.orgY + row*r.cellH
	x1, y1 := x0+w*r.cellW, y0+h*r.cellH
	rw, rh := x1-x0, y1-y0
	ww, wh := world.Dx(), world.Dy()
	if rw <= 0 || rh <= 0 || ww <= 0 || wh <= 0 {
		return image.Rectangle{}, errors.New("zoom: empty region")
	}
	// Expand the smaller dimension to the world's aspect, centered.
	if rw*wh < ww*rh {
		rw2 := (rh*ww + wh - 1) / wh
		x0 -= (rw2 - rw) / 2
		x1 = x0 + rw2
	} else {
		rh2 := (rw*wh + ww - 1) / ww
		y0 -= (rh2 - rh) / 2
		y1 = y0 + rh2
	}
	// The sharp cap: the viewport must stay at least 1/SS of the world
	// — at SS=2 that is exactly the output size (crop 1:1).
	ss := r.SuperSample()
	minW, minH := ww/ss, wh/ss
	if x1-x0 < minW || y1-y0 < minH {
		factor := float64(ww) / float64(x1-x0) // display only, never output
		return image.Rectangle{}, fmt.Errorf(
			"zoom: %.1f× exceeds the %d× sharp limit — foley never ships a blurry frame; widen the region to at least %d×%d cells (or rethink the framing: past %d× you are showing under %d columns)",
			factor, ss, (minW+r.cellW-1)/r.cellW, (minH+r.cellH-1)/r.cellH, ss, minW/r.cellW)
	}
	// Viewport larger than the world (a huge rect + aspect growth):
	// settle at identity.
	if x1-x0 >= ww || y1-y0 >= wh {
		return world, nil
	}
	// Clamp: shift inside the world.
	if x0 < world.Min.X {
		x1 += world.Min.X - x0
		x0 = world.Min.X
	}
	if y0 < world.Min.Y {
		y1 += world.Min.Y - y0
		y0 = world.Min.Y
	}
	if x1 > world.Max.X {
		x0 -= x1 - world.Max.X
		x1 = world.Max.X
	}
	if y1 > world.Max.Y {
		y0 -= y1 - world.Max.Y
		y1 = world.Max.Y
	}
	return image.Rect(x0, y0, x1, y1), nil
}

// Composite develops the master through the camera: the WORLD part of
// the frame is the viewport downscaled into the output; the HUD band
// (the keys reel, ADR-016) is glued under it at a fixed 2:1 — pinned to
// the camera glass, never zoomed (ADR-019 stratification). dst is
// reused when it already has the output size.
func (r *Rasterizer) Composite(master *image.RGBA, vp image.Rectangle, dst *image.RGBA) *image.RGBA {
	ss := r.SuperSample()
	mb := master.Bounds()
	outW, outH := mb.Dx()/ss, mb.Dy()/ss
	if dst == nil || dst.Bounds().Dx() != outW || dst.Bounds().Dy() != outH {
		dst = image.NewRGBA(image.Rect(0, 0, outW, outH))
	}
	world := r.WorldRect()
	if vp == world {
		// Camera at rest: world and HUD are one contiguous 2:1 pass.
		downscaleArea(dst, master, mb)
		return dst
	}
	worldOutH := world.Dy() / ss
	wd, _ := dst.SubImage(image.Rect(0, 0, outW, worldOutH)).(*image.RGBA)
	downscaleArea(wd, master, vp)
	if worldOutH < outH {
		hd, _ := dst.SubImage(image.Rect(0, worldOutH, outW, outH)).(*image.RGBA)
		downscaleArea(hd, master, image.Rect(mb.Min.X, world.Max.Y, mb.Max.X, mb.Max.Y))
	}
	return dst
}

// DownscaleHalf shrinks src into dst at exactly 2:1 with the integer
// area mean — the output-scale knob's final pass (this file's scaler:
// deterministic by construction). dst is reallocated when it does not
// match; the result is returned either way.
func DownscaleHalf(dst, src *image.RGBA) *image.RGBA {
	sb := src.Bounds()
	w, h := sb.Dx()/2, sb.Dy()/2
	if dst == nil || dst.Bounds().Dx() != w || dst.Bounds().Dy() != h {
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	downscaleArea(dst, src, sb)
	return dst
}
