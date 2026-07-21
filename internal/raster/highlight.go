package raster

import (
	"image"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/GH-Jaider/foley/internal/vtengine"
)

// The highlight cue: paint the theme's Selection color under
// chosen text — a director's hand pointing at the frame. Pure paint:
// the grid content never changes, the fg never changes, only the cell
// background under the match, exactly like a real selection.

// HighlightSpec is one highlight: a regex over each row's text, or a
// cell rectangle (Col,Row,W,H). Exactly one form is set. With Pick,
// Occurrence narrows a pattern to that match of the FRAME — 0-based in
// screen order, the same standard as the rect's cells. Name lets a
// targeted off close just this highlight.
type HighlightSpec struct {
	Pattern        *regexp.Regexp
	Col, Row, W, H int
	Rect           bool
	Occurrence     int
	Pick           bool
	Name           string
}

// hlItem is a spec active on [from, to) of the virtual timeline; an
// open item has to == -1.
type hlItem struct {
	spec HighlightSpec
	from time.Duration
	to   time.Duration
}

// HighlightTrack holds the active highlights and serves the driver's
// Overlay contract (SetTime/Breakpoints — snap on/off, no fades).
// Unlike KeysTrack (fed by OnKey ON the driver's goroutine), this one
// is mutated by the RECORDING goroutine while realtime's loop reads it
// at render — hence the mutex; in deterministic mode everything is one
// goroutine and it never contends.
type HighlightTrack struct {
	mu    sync.Mutex
	items []hlItem
	t     time.Duration
}

// NewHighlightTrack returns an empty track.
func NewHighlightTrack() *HighlightTrack { return &HighlightTrack{} }

// Activate turns a highlight on from the given virtual instant.
func (ht *HighlightTrack) Activate(spec HighlightSpec, at time.Duration) {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	ht.items = append(ht.items, hlItem{spec: spec, from: at, to: -1})
}

// Clear turns every open highlight off at the given virtual instant.
func (ht *HighlightTrack) Clear(at time.Duration) {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	for i := range ht.items {
		if ht.items[i].to < 0 {
			ht.items[i].to = at
		}
	}
}

// ClearNamed turns off every open highlight carrying the name.
func (ht *HighlightTrack) ClearNamed(name string, at time.Duration) {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	for i := range ht.items {
		if ht.items[i].to < 0 && ht.items[i].spec.Name == name {
			ht.items[i].to = at
		}
	}
}

// SetTime fixes the overlay clock for the next render.
func (ht *HighlightTrack) SetTime(t time.Duration) {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	ht.t = t
}

// Breakpoints reports the on/off instants in [from, to) — highlights
// snap, so those are the only frames they need.
func (ht *HighlightTrack) Breakpoints(from, to time.Duration) []time.Duration {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	var out []time.Duration
	add := func(t time.Duration) {
		if t >= from && t < to {
			out = append(out, t)
		}
	}
	for _, it := range ht.items {
		add(it.from)
		if it.to >= 0 {
			add(it.to)
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

// active snapshots the specs visible at the track's clock.
func (ht *HighlightTrack) active() []HighlightSpec {
	ht.mu.Lock()
	defer ht.mu.Unlock()
	var out []HighlightSpec
	for _, it := range ht.items {
		if ht.t >= it.from && (it.to < 0 || ht.t < it.to) {
			out = append(out, it.spec)
		}
	}
	return out
}

// drawHighlights paints the Selection color under the matched cells —
// called between cell backgrounds and text, so the glyphs stay crisp
// on top, exactly like a real selection.
func (r *Rasterizer) drawHighlights(dst *image.RGBA, f *vtengine.Frame) {
	if r.highlights == nil {
		return
	}
	specs := r.highlights.active()
	if len(specs) == 0 {
		return
	}
	sel := r.opts.SelectionColor
	fill := func(x0, x1, y int) {
		if x0 < 0 {
			x0 = 0
		}
		if x1 > f.Geometry.Cols {
			x1 = f.Geometry.Cols
		}
		if x0 >= x1 || y < 0 || y >= f.Geometry.Rows {
			return
		}
		fillRect(dst, image.Rect(
			r.orgX+x0*r.cellW, r.orgY+y*r.cellH,
			r.orgX+x1*r.cellW, r.orgY+(y+1)*r.cellH,
		), sel)
	}
	for _, spec := range specs {
		if spec.Rect {
			for y := spec.Row; y < spec.Row+spec.H; y++ {
				fill(spec.Col, spec.Col+spec.W, y)
			}
			continue
		}
		// Gather this FRAME's matches in screen order (top-to-bottom,
		// left-to-right) so the occurrence selector is deterministic.
		type match struct{ x0, x1, y int }
		var found []match
		for y := 0; y < f.Geometry.Rows; y++ {
			text, starts, ends := rowText(f, y)
			for _, m := range spec.Pattern.FindAllStringIndex(text, -1) {
				if m[0] == m[1] {
					continue // empty match: nothing to paint
				}
				i0, i1 := runeIndex(text, m[0]), runeIndex(text, m[1])
				found = append(found, match{starts[i0], ends[i1-1], y})
			}
		}
		if spec.Pick {
			if spec.Occurrence < len(found) {
				found = found[spec.Occurrence : spec.Occurrence+1]
			} else {
				found = nil // fewer matches than asked: nothing this frame
			}
		}
		for _, m := range found {
			fill(m.x0, m.x1, m.y)
		}
	}
}

// rowText renders one row as a string plus, per RUNE, the cell column
// range it occupies ([start, end) — wide cells span two columns): the
// bridge from regex matches back to cells. The string ends at the LAST
// GLYPH, not the last grid column — the empty-cell padding is a grid
// artifact, and `.*` must never paint into the void (same trailing
// semantics as the screen text the waits match).
func rowText(f *vtengine.Frame, y int) (text string, starts, ends []int) {
	var sb []rune
	lastInk := 0 // rune count up to and including the last non-blank
	for x := 0; x < f.Geometry.Cols; {
		c := f.CellAt(x, y)
		w := max(int(c.Width), 1)
		if len(c.Runes) == 0 {
			sb = append(sb, ' ')
			starts, ends = append(starts, x), append(ends, x+1)
			x++
			continue
		}
		for _, rn := range c.Runes {
			sb = append(sb, rn)
			starts, ends = append(starts, x), append(ends, x+w)
			if rn != ' ' {
				lastInk = len(sb)
			}
		}
		x += w
	}
	return string(sb[:lastInk]), starts[:lastInk], ends[:lastInk]
}

// runeIndex converts a byte offset of s into a rune index.
func runeIndex(s string, byteOff int) int {
	n := 0
	for i := range s {
		if i >= byteOff {
			return n
		}
		n++
	}
	return n
}
