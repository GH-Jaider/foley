package raster

import (
	"image"
	"image/color"
	"sort"
	"time"

	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/key"
)

// The keys band (ADR-016, geometry and behavior from Jaider's mockup):
// keycap chips floating UNDER the window — one cap per keystroke,
// repeats coalesce ("j ×4"), chords are one cap ("⌃C"). foley is the
// one typing, so the input track is emitted, not captured — exact
// virtual timestamps, zero hooks, deterministic by construction.
//
// The band's three laws (the mockup's own manifesto):
//   - a cap NEVER moves once placed: it appears, lives and fades in
//     place — each keypress touches only its own pixels;
//   - the band clears by CUTS, never by scroll: a phrase flush (idle of
//     the virtual clock) or a width cut (capacity reached) — like
//     subtitling;
//   - the pty never knows: keys is a cue, the canvas grows Height+52.

// Strip geometry (logical px) and timing (virtual time).
const (
	keysCapPad = 8 // frame horizontal padding
	keysCapGap = 7 // gap between frames
	keysCapRad = 2 // softly squared, like a film frame

	// The stage: the band area is a distinct surface OUTSIDE the
	// terminal (darker than the window in every theme), with the strip
	// floating on it — breathing room above and below. That is what
	// reads as "fuera del terminal".
	keysStageTop = 12
	keysStageBot = 8

	// Sprocket geometry: committed perforations — big enough to read
	// as film holes at 1×, generously and regularly spaced.
	keysSprocketW   = 8
	keysSprocketH   = 5
	keysSprocketPad = 3  // above and below each sprocket row
	keysSprocketGap = 12 // between holes

	keysIdle      = 1500 * time.Millisecond // take flush: strip idle this long → fade
	keysFlushFade = 450 * time.Millisecond  // gentle take fade...
	keysFlushStep = 4                       // ...quantized (bounded frames)
	keysCutFade   = 150 * time.Millisecond  // width cut (the splice): one quick step
	keysRepeat    = 600 * time.Millisecond  // same-cap coalescing window

	// keysMaxCaps caps a take (the mockup's CAPACITY); the real limit
	// is min(this, what the strip width fits — set at assembly).
	keysMaxCaps = 16
)

// KeysBandFor is the band's logical height for a cap cell height
// (the grid cell scaled by the chosen size): stage above, the strip —
// a frame row between two sprocket rows — and stage below.
func KeysBandFor(capCell int) int {
	return keysStageTop + capCell + 6 + 2*(keysSprocketH+2*keysSprocketPad) + keysStageBot
}

// keyCap is one keycap: a keystroke, or a coalesced run of the same
// keystroke.
type keyCap struct {
	sym   string // symbol form (⌃C, ⏎, ␣) — used when the face covers it
	ascii string // fallback form (Ctrl+C, Enter, Space)
	mod   bool   // chord/special: accent styling
	count int    // coalesced repeats (label gains " ×N")
	first time.Duration
	last  time.Duration
}

// phrase is one run of caps between cuts. Closed phrases carry their
// fade schedule; the open phrase (at most one, the last) has end == -1.
type phrase struct {
	caps   []keyCap
	reveal time.Duration // caps draw from max(cap.first, reveal): after a width cut, the new phrase waits for the old one's quick fade
	end    time.Duration // when the fade starts; -1 while open
	quick  bool          // width cut (fast fade) vs idle flush (gentle)
}

// KeysTrack accumulates the injected input track and serves the
// driver's overlay contract structurally (SetTime/Breakpoints) — the
// driver stays raster-agnostic, the raster driver-agnostic. Chips are
// a pure function of the track and the render time.
type KeysTrack struct {
	phrases  []phrase
	capacity int // caps per phrase; assembly sets it from the band width
	t        time.Duration
}

// NewKeysTrack returns an empty input track.
func NewKeysTrack() *KeysTrack { return &KeysTrack{capacity: keysMaxCaps} }

// setCapacity bounds phrase length to what the band actually fits
// (called once at assembly — same inputs, same capacity: deterministic).
func (kt *KeysTrack) setCapacity(n int) {
	if n > 0 && n < kt.capacity {
		kt.capacity = n
	}
}

// AddKey records one injected keystroke. Hidden input is dropped: if
// the setup is not shown, its typing must not leak either.
func (kt *KeysTrack) AddKey(k key.Key, at time.Duration, hidden bool) {
	if hidden {
		return
	}
	sym, ascii, mod := keyCapLabel(k)
	if sym == "" {
		return
	}
	cur := kt.open()
	// An idle gap flushes the phrase: it faded out before this key.
	if cur != nil && at-cur.caps[len(cur.caps)-1].last >= keysIdle {
		cur.end = cur.caps[len(cur.caps)-1].last + keysIdle
		cur = nil
	}
	if cur != nil {
		// Coalesce a repeat of the SAME cap inside the window.
		lc := &cur.caps[len(cur.caps)-1]
		if lc.sym == sym && lc.mod == mod && at-lc.last <= keysRepeat {
			lc.count++
			lc.last = at
			return
		}
		// Width cut: the full phrase fades fast; the new one reveals
		// after the cut (the mockup clears, then pushes).
		if len(cur.caps) >= kt.capacity {
			cur.end, cur.quick = at, true
			kt.phrases = append(kt.phrases, phrase{reveal: at + keysCutFade, end: -1})
			cur = &kt.phrases[len(kt.phrases)-1]
		}
	}
	if cur == nil {
		kt.phrases = append(kt.phrases, phrase{end: -1})
		cur = &kt.phrases[len(kt.phrases)-1]
	}
	cur.caps = append(cur.caps, keyCap{sym: sym, ascii: ascii, mod: mod, count: 1, first: at, last: at})
}

func (kt *KeysTrack) open() *phrase {
	if len(kt.phrases) == 0 {
		return nil
	}
	p := &kt.phrases[len(kt.phrases)-1]
	if p.end >= 0 {
		return nil
	}
	return p
}

// SetTime fixes the overlay clock for the next render (the frame's
// START instant on the virtual timeline).
func (kt *KeysTrack) SetTime(t time.Duration) { kt.t = t }

// alphaAt is the phrase's opacity at t. An open phrase is opaque but
// fades once idle passes — closure is implicit: no later key arrived,
// so the flush stands wherever the timeline ends up.
func (p *phrase) alphaAt(t time.Duration) int {
	if len(p.caps) == 0 {
		return 0
	}
	end, quick := p.end, p.quick
	if end < 0 {
		end, quick = p.caps[len(p.caps)-1].last+keysIdle, false
	}
	if t < end {
		return 255
	}
	if quick {
		if t < end+keysCutFade {
			return 110
		}
		return 0
	}
	step := keysFlushFade / keysFlushStep
	i := int((t - end) / step)
	if i >= keysFlushStep {
		return 0
	}
	return 255 * (keysFlushStep - i) / (keysFlushStep + 1)
}

// Breakpoints reports every overlay-state change taking effect in
// [from, to): cap births and pop-ends, phrase reveals, fade steps. The
// driver splits time advances there so the animation lands on exact
// frames.
func (kt *KeysTrack) Breakpoints(from, to time.Duration) []time.Duration {
	var out []time.Duration
	add := func(t time.Duration) {
		if t >= from && t < to {
			out = append(out, t)
		}
	}
	for i := range kt.phrases {
		p := &kt.phrases[i]
		for _, c := range p.caps {
			birth := c.first
			if p.reveal > birth {
				birth = p.reveal
			}
			add(birth)
		}
		end, quick := p.end, p.quick
		if end < 0 {
			end = p.caps[len(p.caps)-1].last + keysIdle
		}
		if quick {
			add(end)
			add(end + keysCutFade)
			continue
		}
		step := keysFlushFade / keysFlushStep
		for i := 0; i <= keysFlushStep; i++ {
			add(end + time.Duration(i)*step)
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

// keyCapLabel renders a key as a cap: symbol form, ASCII fallback (for
// fonts without the symbols), and whether it takes accent styling.
// Every keystroke is one cap — the mockup's anatomy.
func keyCapLabel(k key.Key) (sym, ascii string, mod bool) {
	// Notation (ADR-016): CARET for control combos (^C — the way the
	// terminal itself echoes them; authenticity over mac aesthetics),
	// Unicode glyphs for named keys and the other modifiers.
	var symMods, asciiMods string
	if k.Mods&key.ModCtrl != 0 {
		symMods, asciiMods = symMods+"^", asciiMods+"Ctrl+"
	}
	if k.Mods&key.ModAlt != 0 {
		symMods, asciiMods = symMods+"⌥", asciiMods+"Alt+"
	}
	if k.Mods&key.ModShift != 0 {
		symMods, asciiMods = symMods+"⇧", asciiMods+"Shift+"
	}
	var base, baseASCII string
	special := true
	switch k.Name {
	case key.NameNone:
		if k.Rune == 0 {
			return "", "", false
		}
		special = false
		base = string(k.Rune)
		if k.Rune == ' ' {
			base, baseASCII, special = "␣", "Space", true
		} else if k.Mods != 0 && k.Rune >= 'a' && k.Rune <= 'z' {
			base = string(k.Rune - 'a' + 'A')
		}
	case key.NameEnter:
		base, baseASCII = "↩", "Enter"
	case key.NameEscape:
		base, baseASCII = "⎋", "Esc"
	case key.NameBackspace:
		base, baseASCII = "⌫", "Bksp"
	case key.NameTab:
		base, baseASCII = "⇥", "Tab"
	case key.NameSpace:
		base, baseASCII = "␣", "Space"
	case key.NameDelete:
		base, baseASCII = "⌦", "Del"
	case key.NameInsert:
		base, baseASCII = "Ins", "Ins"
	case key.NameUp:
		base, baseASCII = "↑", "Up"
	case key.NameDown:
		base, baseASCII = "↓", "Down"
	case key.NameLeft:
		base, baseASCII = "←", "Left"
	case key.NameRight:
		base, baseASCII = "→", "Right"
	case key.NameHome:
		base, baseASCII = "Home", "Home"
	case key.NameEnd:
		base, baseASCII = "End", "End"
	case key.NamePageUp:
		base, baseASCII = "PgUp", "PgUp"
	case key.NamePageDown:
		base, baseASCII = "PgDn", "PgDn"
	}
	if baseASCII == "" {
		baseASCII = base
	}
	return symMods + base, asciiMods + baseASCII, k.Mods != 0 || special
}

// capText picks the cap's label forms: the main label (symbol form
// when the grid face covers every rune, ASCII otherwise) and the
// coalescing counter, separately — the key is the DATA, the counter is
// metadata and draws smaller, in subtext. The coverage scan runs once
// per distinct label, memoized — not once per frame.
func (r *Rasterizer) capText(c keyCap) (main, counter string) {
	main, ok := r.capLabels[c.sym]
	if !ok {
		main = c.sym
		face := r.gridFace()
		for _, rn := range c.sym {
			if _, covered := face.NominalGlyph(rn); !covered {
				main = c.ascii
				break
			}
		}
		r.capLabels[c.sym] = main
	}
	if c.count > 1 {
		counter = r.keysMult + itoa(c.count)
	}
	return main, counter
}

// textStrip pairs a rendered label with its baseline ascent, so caps
// can align BASELINES across sizes (the key and its counter).
type textStrip struct {
	mask *glyphMask
	asc  int
}

// keyStrip shapes a label once per size and caches it (labels repeat:
// ↵, arrows, common letters). Main labels use the reel's cap size —
// the strip speaks the terminal's own type; counters go smaller.
func (r *Rasterizer) keyStrip(label string, px int) textStrip {
	ck := label + "\x00" + itoa(px)
	if m, ok := r.keyStrips[ck]; ok {
		return m
	}
	mask, asc := r.renderTextStrip(label, px)
	ts := textStrip{mask: mask, asc: asc}
	r.keyStrips[ck] = ts
	return ts
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 && i > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// stageShade is the surface the band lives on — clearly NOT the
// terminal: darker than the window in every theme.
func stageShade(bg color.RGBA) color.RGBA {
	black := color.RGBA{A: 0xff}
	if lum := (299*int(bg.R) + 587*int(bg.G) + 114*int(bg.B)) / 1000; lum < 128 {
		return mixRGBA(bg, black, 65)
	}
	return mixRGBA(bg, black, 18)
}

// filmShade is the strip's celluloid tone: between the stage and the
// window — a strip of tape lying on the stage.
func filmShade(bg color.RGBA) color.RGBA {
	black := color.RGBA{A: 0xff}
	if lum := (299*int(bg.R) + 587*int(bg.G) + 114*int(bg.B)) / 1000; lum < 128 {
		return mixRGBA(bg, black, 40)
	}
	return mixRGBA(bg, black, 38)
}

// drawKeyChips composites the frames onto the film strip for the
// track's current clock. Frames place chronologically left→right and
// NEVER move (frames on film don't); each take fades as one. Called
// per frame of the RECORDING — this is the part that animates; the
// strip and its sprockets are chrome.
func (r *Rasterizer) drawKeyChips(dst *image.RGBA, f *vtengine.Frame) {
	if r.keys == nil || r.bandRect.Empty() {
		return
	}
	t := r.keys.t
	s := r.opts.Scale
	gap := keysCapGap * s
	frameH := r.bandRect.Dy() - 2*(keysSprocketH+2*keysSprocketPad)*s

	bg, fg := rgba(f.Colors.BG), rgba(f.Colors.FG)
	// Inverted hierarchy: the INVISIBLE keys carry the accent — they
	// are what the band exists to show; plain characters already echo
	// in the footage, so they stay muted neutral. The hierarchy codes
	// the thesis in color.
	accent := color.RGBA{R: f.Colors.Palette[13].R, G: f.Colors.Palette[13].G, B: f.Colors.Palette[13].B, A: 0xff}
	txt := mixRGBA(bg, fg, 68)
	modTxt := mixRGBA(accent, fg, 18)
	subTxt := mixRGBA(bg, fg, 45)

	cy := r.bandRect.Min.Y + r.bandRect.Dy()/2
	for pi := range r.keys.phrases {
		p := &r.keys.phrases[pi]
		alpha := p.alphaAt(t)
		if alpha == 0 {
			continue
		}
		// The first frame aligns with the prompt's column — grid
		// coherence between footage and subtitle — with a floor so a
		// zero-padding recording never kisses the canvas edge.
		x := max(r.orgX, 8*s)
		for _, c := range p.caps {
			birth := c.first
			if p.reveal > birth {
				birth = p.reveal
			}
			if t < birth {
				break // frames are chronological: nothing later is born
			}
			main, counter := r.capText(c)
			strip := r.keyStrip(main, r.keysCapPx)
			if strip.mask == nil {
				continue
			}
			var cnt textStrip
			if counter != "" {
				cnt = r.keyStrip(counter, r.keysCapPx*7/10)
			}
			contentW := strip.mask.alpha.Bounds().Dx()
			if cnt.mask != nil {
				contentW += 4*s + cnt.mask.alpha.Bounds().Dx()
			}
			w := contentW + 2*keysCapPad*s
			if w < frameH {
				w = frameH // square minimum, like a film frame
			}
			if c.sym == "␣" && w < 2*frameH {
				// The spacebar is WIDE — the band reads by words, the
				// way a real keyboard separates them.
				w = 2 * frameH
			}
			if x+w > r.bandRect.Max.X {
				break // clip guard; capacity should prevent this
			}
			tx := txt
			if c.mod {
				tx = modTxt
			}
			// One flat exposed frame on the strip: window-toned cell,
			// softly squared — no gloss, no shadows.
			rect := image.Rect(x, cy-frameH/2, x+w, cy+frameH/2)
			fillRoundedRect(dst, rect, keysCapRad*s, bg, alpha)
			cx := rect.Min.X + (w-contentW)/2
			b := strip.mask.alpha.Bounds()
			mainY := rect.Min.Y + (frameH-b.Dy())/2
			blitMaskFaded(dst, strip.mask, cx, mainY, tx, alpha)
			if cnt.mask != nil {
				// The counter shares the key's BASELINE: the key is the
				// data, the counter metadata — smaller, subtext, aligned.
				blitMaskFaded(dst, cnt.mask, cx+b.Dx()+4*s, mainY+strip.asc-cnt.asc, subTxt, alpha)
			}
			x += w + gap
		}
	}
}
