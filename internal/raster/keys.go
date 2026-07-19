package raster

import (
	"image"
	"image/color"
	"sort"
	"time"

	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/key"
)

// The keys band (ADR-016, v3 geometry and vocabulary): keycap chips on
// a film strip UNDER the window — one cap per keystroke, repeats
// coalesce ("j ×4"), chords are one cap ("^C"). foley is the one
// typing, so the input track is emitted, not captured — exact virtual
// timestamps, zero hooks, deterministic by construction.
//
// The band's three laws (the mockup's own manifesto):
//   - a cap NEVER moves once placed: it appears, lives and fades in
//     place — each keypress touches only its own pixels;
//   - the band clears by CUTS, never by scroll: a phrase flush (idle of
//     the virtual clock; Enter closes the shell's natural take sooner)
//     or a width cut (capacity reached) — like subtitling;
//   - the pty never knows: keys is a cue, the canvas grows by the band.
//
// v3 doctrine: the dress is the desk, the theme is the film. The band
// area is the margin fill running under the window; the strip floats
// on it and its perforations are HOLES — painted with the fill behind
// them, never an invented gray. Caps print what a real keycap prints.

// Strip geometry (logical px) and timing (virtual time).
const (
	keysCapPad = 8 // frame horizontal padding
	keysCapGap = 7 // gap between frames
	keysCapRad = 3 // softly squared, like a film frame

	// Breathing room between the margin's matte and the strip: the
	// celluloid floats on the margin (the stage died in v3 — the margin
	// IS the stage), balanced above and below.
	keysBandPadTop = 12
	keysBandPadBot = 12

	// Sprocket geometry (v3, from a real strip): square holes with a
	// small radius, tight pitch (~2× the hole width), rows hugging the
	// strip edges. The hole COLOR is not here: a hole is painted with
	// the margin fill behind the strip (punchHole), Jaider's rule.
	keysSprocketW   = 7
	keysSprocketH   = 7
	keysSprocketPad = 2
	keysSprocketGap = 7
	keysSprocketRad = 2

	keysIdle      = 1500 * time.Millisecond // take flush: strip idle this long → fade
	keysIdleEnter = 600 * time.Millisecond  // after Enter — the shell's take ended
	keysFlushFade = 450 * time.Millisecond  // gentle take fade...
	keysFlushStep = 4                       // ...quantized (bounded frames)
	keysCutFade   = 150 * time.Millisecond  // width cut (the splice): one quick step
	keysRepeat    = 600 * time.Millisecond  // same-cap coalescing window

	// keysMaxCaps caps a take (the mockup's CAPACITY); the real limit
	// is min(this, what the strip width fits — set at assembly).
	keysMaxCaps = 16
)

// KeysBandFor is the band's logical height for a cap cell height
// (the grid cell scaled by the chosen size): matte above, the strip —
// a frame row between two sprocket rows — and matte below.
func KeysBandFor(capCell int) int {
	return keysBandPadTop + capCell + 6 + 2*(keysSprocketH+2*keysSprocketPad) + keysBandPadBot
}

// KeysNotation is the cap vocabulary (ADR-016 v3).
type KeysNotation uint8

const (
	// KeysKeycap prints what a real keycap prints: lowercase words in
	// the grid font, drawn arrows, a blank spacebar.
	KeysKeycap KeysNotation = iota
	// KeysIcons swaps the words for compact drawn symbols (enter, tab,
	// bksp, del) — esc stays a word: keyboards never icon it.
	KeysIcons
)

// KeysStyle is the band's styling knobs (ADR-016 v3): notation, accent
// override, and the plain (stripless) variant.
type KeysStyle struct {
	// Notation picks the cap vocabulary.
	Notation KeysNotation
	// Accent overrides the special-cap color; nil keeps the theme's
	// bright magenta. AccentOff mutes the hierarchy entirely.
	Accent    *color.RGBA
	AccentOff bool
	// Plain drops the celluloid: caps float straight on the margin.
	Plain bool
}

// keyCap is one keycap: a keystroke, or a coalesced run of the same
// keystroke. Its face is either a text label (the grid font), a drawn
// icon, or the blank spacebar — never a mix (ADR-016 v3).
type keyCap struct {
	label string
	icon  keyIcon
	space bool
	mod   bool // special/chord: accent styling
	enter bool // closes the take: shorter flush idle
	count int  // coalesced repeats (label gains " ×N")
	w     int  // drawn width in scaled px (set via the bound measure)
	first time.Duration
	last  time.Duration
}

// sameFace reports whether a new keystroke repeats this cap's face —
// the coalescing identity.
func (c *keyCap) sameFace(o keyCap) bool {
	return c.label == o.label && c.icon == o.icon && c.space == o.space && c.mod == o.mod
}

// idleAfter is the flush idle measured from this cap: Enter closed the
// shell's natural take, so the strip breathes sooner; anything else
// waits the general idle (TUIs have no terminator key).
func (c *keyCap) idleAfter() time.Duration {
	if c.enter {
		return keysIdleEnter
	}
	return keysIdle
}

// phrase is one run of caps between cuts. Closed phrases carry their
// fade schedule; the open phrase (at most one, the last) has end == -1.
type phrase struct {
	caps   []keyCap
	width  int           // drawn px so far (caps + gaps) — the cut accounting
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
	notation KeysNotation
	t        time.Duration
	// The width cut measures REAL caps (assembly binds the raster's
	// own width math — shaping is deterministic, so so is the cut):
	// the strip fills to its edge, then splices. Unbound (tests,
	// headless misuse) falls back to a cap count.
	measure func(*keyCap) int
	gap     int
	limit   int
}

// NewKeysTrack returns an empty input track speaking the given
// notation (the cap face derives at AddKey time — coalescing compares
// faces, so the vocabulary is fixed per recording).
func NewKeysTrack(notation KeysNotation) *KeysTrack {
	return &KeysTrack{notation: notation}
}

// bind wires the width-cut accounting to the raster (called once at
// assembly): measure is the cap's drawn width, gap the inter-cap gap
// and limit the strip's usable width, all in scaled px.
func (kt *KeysTrack) bind(measure func(*keyCap) int, gap, limit int) {
	kt.measure, kt.gap, kt.limit = measure, gap, limit
}

// fits reports whether the phrase can grow by w px — a new cap also
// pays the inter-cap gap (the first one does not). Unbound tracks
// fall back to counting caps.
func (p *phrase) fits(kt *KeysTrack, w int, newCap bool) bool {
	if kt.measure == nil {
		return !newCap || len(p.caps) < keysMaxCaps
	}
	need := p.width + w
	if newCap && len(p.caps) > 0 {
		need += kt.gap
	}
	return need <= kt.limit
}

// AddKey records one injected keystroke. Hidden input is dropped: if
// the setup is not shown, its typing must not leak either.
func (kt *KeysTrack) AddKey(k key.Key, at time.Duration, hidden bool) {
	if hidden {
		return
	}
	c, ok := keyCapFor(k, kt.notation)
	if !ok {
		return
	}
	c.count, c.first, c.last = 1, at, at
	if kt.measure != nil {
		c.w = kt.measure(&c)
	}
	cur := kt.open()
	// An idle gap flushes the phrase: it faded out before this key.
	if cur != nil {
		lc := &cur.caps[len(cur.caps)-1]
		if at-lc.last >= lc.idleAfter() {
			cur.end = lc.last + lc.idleAfter()
			cur = nil
		}
	}
	if cur != nil {
		// Coalesce a repeat of the SAME cap inside the window — unless
		// the counter's growth would spill past the strip's edge; then
		// the repeat starts the next take instead (a splice: the
		// placed cap never moves nor shrinks, first law).
		lc := &cur.caps[len(cur.caps)-1]
		if lc.sameFace(c) && at-lc.last <= keysRepeat {
			grown := *lc
			grown.count++
			grown.last = at
			if kt.measure != nil {
				grown.w = kt.measure(&grown)
			}
			if delta := grown.w - lc.w; cur.fits(kt, delta, false) {
				cur.width += delta
				*lc = grown
				return
			}
		} else if cur.fits(kt, c.w, true) {
			cur.width += kt.gap + c.w
			cur.caps = append(cur.caps, c)
			return
		}
		// Width cut: the full phrase fades fast; the new one reveals
		// after the cut (the mockup clears, then pushes).
		cur.end, cur.quick = at, true
		kt.phrases = append(kt.phrases, phrase{reveal: at + keysCutFade, end: -1})
		cur = &kt.phrases[len(kt.phrases)-1]
	}
	if cur == nil {
		kt.phrases = append(kt.phrases, phrase{end: -1})
		cur = &kt.phrases[len(kt.phrases)-1]
	}
	cur.width = c.w
	cur.caps = append(cur.caps, c)
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
		lc := &p.caps[len(p.caps)-1]
		end, quick = lc.last+lc.idleAfter(), false
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
// [from, to): cap births, phrase reveals, fade steps. The driver
// splits time advances there so the animation lands on exact frames.
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
			lc := &p.caps[len(p.caps)-1]
			end = lc.last + lc.idleAfter()
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

// keyCapFor renders a key as a cap face (ADR-016 v3): what a real
// keycap prints. Lowercase words in the grid font for named keys —
// ASCII, so coverage never gambles on a cmap — and drawn sprites only
// where keyboards draw: the arrows (plus enter/tab/bksp/del under
// KeysIcons; esc stays a word, keyboards never icon it). Chords are
// always TEXT (`^E`, `alt+b`, `shift+tab`): a cap never mixes an icon
// with a prefix. ok=false drops the keystroke (nothing to show).
func keyCapFor(k key.Key, notation KeysNotation) (keyCap, bool) {
	// Notation for modifiers (ADR-016): CARET for control — the way
	// the terminal itself echoes it — and lowercase words for the rest.
	var prefix string
	if k.Mods&key.ModCtrl != 0 {
		prefix += "^"
	}
	if k.Mods&key.ModAlt != 0 {
		prefix += "alt+"
	}
	if k.Mods&key.ModShift != 0 {
		prefix += "shift+"
	}

	word, icon, special := keyFace(k, notation)
	if word == "" && icon == iconNone {
		return keyCap{}, false
	}
	enter := k.Name == key.NameEnter
	if prefix == "" {
		if k.Name == key.NameSpace || (k.Name == key.NameNone && k.Rune == ' ') {
			// The spacebar is a BLANK cap, wider than the rest — the
			// most recognizable key precisely because it says nothing.
			return keyCap{space: true}, true
		}
		if icon != iconNone {
			return keyCap{icon: icon, mod: true, enter: enter}, true
		}
		return keyCap{label: word, mod: special, enter: enter}, true
	}
	if k.Rune >= 'a' && k.Rune <= 'z' {
		// ^E, not ^e: control chords echo uppercase, terminal custom.
		word = string(k.Rune - 'a' + 'A')
	}
	return keyCap{label: prefix + word, mod: true, enter: enter}, true
}

// keyFace is the bare key's face: its word form (a real keycap's
// print), its drawn form where one exists for the notation, and
// whether it takes accent styling.
func keyFace(k key.Key, notation KeysNotation) (word string, icon keyIcon, special bool) {
	icons := notation == KeysIcons
	switch k.Name {
	case key.NameNone:
		if k.Rune == 0 {
			return "", iconNone, false
		}
		return string(k.Rune), iconNone, false
	case key.NameEnter:
		if icons {
			return "enter", iconEnter, true
		}
		return "enter", iconNone, true
	case key.NameEscape:
		return "esc", iconNone, true // esc is esc — never an icon
	case key.NameBackspace:
		if icons {
			return "bksp", iconBksp, true
		}
		return "bksp", iconNone, true
	case key.NameTab:
		if icons {
			return "tab", iconTab, true
		}
		return "tab", iconNone, true
	case key.NameSpace:
		return "space", iconNone, true // the word serves chords: ^space
	case key.NameDelete:
		if icons {
			return "del", iconDel, true
		}
		return "del", iconNone, true
	case key.NameInsert:
		return "ins", iconNone, true
	case key.NameUp:
		return "up", iconUp, true
	case key.NameDown:
		return "down", iconDown, true
	case key.NameLeft:
		return "left", iconLeft, true
	case key.NameRight:
		return "right", iconRight, true
	case key.NameHome:
		return "home", iconNone, true
	case key.NameEnd:
		return "end", iconNone, true
	case key.NamePageUp:
		return "pgup", iconNone, true
	case key.NamePageDown:
		return "pgdn", iconNone, true
	}
	return "", iconNone, false
}

// capStrip is the cap's rendered face: the label at its size (single
// runes at the reel's cap size, words and chords a step smaller — like
// the small print on a physical keycap) or the drawn icon.
func (r *Rasterizer) capStrip(c keyCap) textStrip {
	if c.icon != iconNone {
		return r.keyIconStrip(c.icon, r.keysCapPx)
	}
	px := r.keysCapPx
	if len([]rune(c.label)) > 1 {
		px = px * 4 / 5
	}
	return r.keyStrip(c.label, px)
}

// capContent is the cap's face and counter strips plus their combined
// content width — the single width truth the cut accounting (via
// capWidth, bound at assembly) and drawKeyChips share.
func (r *Rasterizer) capContent(c keyCap) (strip, cnt textStrip, contentW int) {
	if c.space {
		if c.count > 1 {
			cnt = r.keyStrip(r.keysMult+itoa(c.count), r.keysCapPx*7/10)
		}
		return textStrip{}, cnt, 0
	}
	strip = r.capStrip(c)
	if strip.mask == nil {
		return strip, cnt, 0
	}
	contentW = strip.mask.alpha.Bounds().Dx()
	if c.count > 1 {
		cnt = r.keyStrip(r.keysMult+itoa(c.count), r.keysCapPx*7/10)
		if cnt.mask != nil {
			contentW += 4*r.s + cnt.mask.alpha.Bounds().Dx()
		}
	}
	return strip, cnt, contentW
}

// keysFrameH is the strip's frame-row height in scaled px, derivable
// from the band before drawChrome ever runs.
func (r *Rasterizer) keysFrameH() int {
	return (r.opts.Window.KeysBand - keysBandPadTop - keysBandPadBot - 2*(keysSprocketH+2*keysSprocketPad)) * r.s
}

// capWidth is the cap's drawn width in scaled px.
func (r *Rasterizer) capWidth(c *keyCap) int {
	frameH := r.keysFrameH()
	if c.space {
		// The blank spacebar: 1.5× wide, no face.
		return frameH * 3 / 2
	}
	_, _, contentW := r.capContent(*c)
	w := contentW + 2*keysCapPad*r.s
	if w < frameH {
		w = frameH // square minimum, like a film frame
	}
	return w
}

// textStrip pairs a rendered label with its baseline ascent, so caps
// can align BASELINES across sizes (the key and its counter).
type textStrip struct {
	mask *glyphMask
	asc  int
}

// keyStrip shapes a label once per size and caches it (labels repeat:
// letters, enter, the arrows' chord words). Labels are the grid face —
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

// filmShade is the strip's celluloid tone (v3: COMMITTED — near-black
// on dark themes and still a dark strip on paper; a strip on a light
// table is still a strip. The frames and the punched holes carry the
// read, not the celluloid's tone).
func filmShade(bg color.RGBA) color.RGBA {
	return mixRGBA(bg, color.RGBA{A: 0xff}, 72)
}

// lumOf is the integer perceptual luminance the shade derivations use.
func lumOf(c color.RGBA) int {
	return (299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000
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
	s := r.s
	gap := keysCapGap * s
	frameH := r.bandRect.Dy() - 2*(keysSprocketH+2*keysSprocketPad)*s

	bg, fg := rgba(f.Colors.BG), rgba(f.Colors.FG)
	// Inverted hierarchy: the INVISIBLE keys carry the accent — they
	// are what the band exists to show; plain characters already echo
	// in the footage, so they stay muted neutral. v3 commits the
	// accent: near-pure on dark, pulled toward fg on light (contrast).
	accent := color.RGBA{R: f.Colors.Palette[13].R, G: f.Colors.Palette[13].G, B: f.Colors.Palette[13].B, A: 0xff}
	if r.keysStyle.Accent != nil {
		accent = *r.keysStyle.Accent
	}
	txt := mixRGBA(bg, fg, 80)
	modTxt := mixRGBA(accent, fg, 6)
	if lumOf(bg) >= 128 {
		modTxt = mixRGBA(accent, fg, 25)
	}
	if r.keysStyle.AccentOff {
		modTxt = txt
	}
	subTxt := mixRGBA(bg, fg, 50)

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
			if c.space {
				// The blank spacebar: 1.5× wide, window-toned, no face.
				w := r.capWidth(&c)
				if x+w > r.bandRect.Max.X {
					break
				}
				rect := image.Rect(x, cy-frameH/2, x+w, cy+frameH/2)
				fillRoundedRect(dst, rect, keysCapRad*s, bg, alpha)
				if _, cnt, _ := r.capContent(c); cnt.mask != nil {
					cb := cnt.mask.alpha.Bounds()
					blitMaskFaded(dst, cnt.mask, rect.Min.X+(w-cb.Dx())/2, rect.Min.Y+(frameH-cb.Dy())/2, subTxt, alpha)
				}
				x += w + gap
				continue
			}
			strip, cnt, contentW := r.capContent(c)
			if strip.mask == nil {
				continue
			}
			w := r.capWidth(&c)
			if x+w > r.bandRect.Max.X {
				break // clip guard; the width cut should prevent this
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
