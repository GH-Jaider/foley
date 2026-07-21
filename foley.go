// Package foley records scripted terminal demos without a terminal
// window: the demo command runs on a real pty, an embedded terminal
// engine keeps the state, and foley rasterizes every frame itself —
// byte-identical output across machines, zero screen-capture permissions.
//
// A Recorder is created with New, driven with Timeline actions (Type,
// Press, Sleep, WaitText, Hide/Show, Screenshot) and closed into files
// with Output — the format follows the extension (.gif, .mp4, .webm,
// .webp, .cast, .txt):
//
//	rec, err := foley.New(foley.Options{Command: []string{"my-tui"}})
//	// handle err, defer rec.Close()
//	_ = rec.Type(ctx, "hello", 50*time.Millisecond)
//	_ = rec.Press(ctx, key.Key{Name: key.NameEnter}, 0)
//	_ = rec.WaitText(ctx, regexp.MustCompile(`done`), 5*time.Second)
//	_ = rec.Sleep(ctx, 2*time.Second)
//	_ = rec.Output(ctx, "demo.gif")
//
// In Deterministic mode (the default) time is virtual: it advances only
// by the durations the script declares, the recording is byte-identical
// regardless of machine speed, and it renders faster than real time. In
// Realtime mode recording starts at New and wall time is captured as it
// happens.
package foley

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // MarginFill image decoding
	_ "image/png"  // MarginFill image decoding
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GH-Jaider/foley/internal/driver"
	"github.com/GH-Jaider/foley/internal/encode"
	"github.com/GH-Jaider/foley/internal/fontpack"
	"github.com/GH-Jaider/foley/internal/ptyx"
	"github.com/GH-Jaider/foley/internal/raster"
	"github.com/GH-Jaider/foley/internal/theme"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/factory"
	"github.com/GH-Jaider/foley/key"
)

// Sentinel errors.
var (
	// ErrFinished is returned by actions after the first Output: the
	// timeline is sealed once encoding starts.
	ErrFinished = errors.New("foley: recording already finished")
	// ErrUnsupportedOutput is returned by Output for an extension it
	// does not know.
	ErrUnsupportedOutput = errors.New("foley: unsupported output format")
	// ErrWaitTimeout reports a WaitText whose pattern never appeared
	// within its timeout; the message carries a screen dump.
	ErrWaitTimeout = driver.ErrWaitTimeout
	// ErrWaitInterrupted reports a WaitText that can no longer succeed
	// (the application exited first).
	ErrWaitInterrupted = driver.ErrWaitInterrupted
)

// RGB is one 8-bit color of a Theme.
type RGB struct{ R, G, B uint8 }

// Theme is foley's neutral color model: defaults plus the 16 ANSI slots.
// The remaining 240 palette entries are always the standard xterm cube
// and grayscale ramp. A zero Cursor follows the foreground. Selection
// paints the highlight cue — terminal themes always carry it;
// it finally has a job.
type Theme struct {
	Foreground RGB
	Background RGB
	Cursor     RGB
	Selection  RGB
	ANSI       [16]RGB
}

// DefaultTheme is the built-in dark theme (Catppuccin Mocha).
func DefaultTheme() Theme {
	return Theme{
		Foreground: RGB{0xcd, 0xd6, 0xf4},
		Background: RGB{0x1e, 0x1e, 0x2e},
		Selection:  RGB{0x58, 0x5b, 0x70},
		ANSI: [16]RGB{
			{0x45, 0x47, 0x5a},
			{0xf3, 0x8b, 0xa8},
			{0xa6, 0xe3, 0xa1},
			{0xf9, 0xe2, 0xaf},
			{0x89, 0xb4, 0xfa},
			{0xf5, 0xc2, 0xe7},
			{0x94, 0xe2, 0xd5},
			{0xba, 0xc2, 0xde},
			{0x58, 0x5b, 0x70},
			{0xf3, 0x8b, 0xa8},
			{0xa6, 0xe3, 0xa1},
			{0xf9, 0xe2, 0xaf},
			{0x89, 0xb4, 0xfa},
			{0xf5, 0xc2, 0xe7},
			{0x94, 0xe2, 0xd5},
			{0xa6, 0xad, 0xc8},
		},
	}
}

// WindowBarStyle selects VHS's window bar variants.
type WindowBarStyle uint8

// The window bar styles (VHS names: "Colorful", "ColorfulRight",
// "Rings", "RingsRight").
const (
	WindowBarNone WindowBarStyle = iota
	WindowBarColorful
	WindowBarColorfulRight
	WindowBarRings
	WindowBarRingsRight
	// Foley genre extensions (dress-reachable; VHS silently draws no
	// bar for styles it does not know — still degradable).
	WindowBarLinuxControls
	WindowBarGnomeCSD
)

// KeysSize scales the input reel's caps relative to the grid's font.
// The zero value is medium — the grid's own size.
type KeysSize uint8

// The reel sizes.
const (
	KeysMedium KeysSize = iota
	KeysSmall
	KeysLarge
)

// KeysNotation picks the reel's cap vocabulary: what a
// cap prints for named keys. The zero value is keycap.
type KeysNotation uint8

// The notations.
const (
	// KeysKeycap prints what a real keycap prints: lowercase words in
	// the grid font (esc, enter, tab…), drawn arrows, a blank spacebar.
	KeysKeycap KeysNotation = iota
	// KeysIcons swaps the words for compact drawn symbols (enter, tab,
	// bksp, del) — esc stays a word: keyboards never icon it.
	KeysIcons
)

// keysAccentNames maps a KeysAccent ANSI color name to its BRIGHT
// palette slot (8–15) — the reel's default is the theme's bright
// magenta, so names resolve in the same register.
func keysAccentSlot(name string) (int, bool) {
	names := [8]string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}
	for i, n := range names {
		if name == n {
			return 8 + i, true
		}
	}
	return 0, false
}

// ParseKeysAccent validates a KeysAccent value without a theme: ""
// (the theme's bright magenta), "off", "#hex", or an ANSI color name.
// foley.New resolves it against the recording's theme; `foley
// validate` calls this so a typo dies before anything records.
func ParseKeysAccent(s string) error {
	switch {
	case s == "" || s == "off":
		return nil
	case strings.HasPrefix(s, "#"):
		_, err := parseVHSHex(s)
		return err
	default:
		if _, ok := keysAccentSlot(s); !ok {
			return fmt.Errorf("keys accent %q: an ANSI color name (black|red|green|yellow|blue|magenta|cyan|white), #hex, or off", s)
		}
		return nil
	}
}

// Mode selects the recording clock.
type Mode uint8

// Recording modes.
const (
	// Deterministic advances time only by script declaration: identical
	// recordings on any machine, rendered faster than real time.
	Deterministic Mode = iota
	// Realtime samples wall time as it happens, starting at New.
	Realtime
)

// Settle tunes how a Deterministic recording waits for the application
// to quiesce after each step (all wall-clock; zero fields get defaults:
// 150ms / 40ms / 2s). See the PRD's determinism model.
type Settle struct {
	// First bounds the wait for the first byte after a step.
	First time.Duration
	// Quiet is the silence that ends a settle.
	Quiet time.Duration
	// Max caps a whole settle, streaming apps included.
	Max time.Duration
}

// Options configures a Recorder. Command is required; zero values
// elsewhere mean the documented defaults.
type Options struct {
	// Command is the demo command argv; Command[0] resolves via PATH.
	// It runs on a real pty (no shell involved — wrap with `sh -c`
	// explicitly if shell syntax is wanted).
	Command []string
	// Dir is the command's working directory (empty = inherit).
	Dir string
	// Env is the exact child environment. nil inherits the process
	// environment THROUGH foley's terminal identity: the
	// host terminal's variables scrubbed, foley's declared. Callers
	// building an explicit Env should start from
	// TerminalEnv(os.Environ()) for the same behavior.
	Env []string

	// Cols, Rows size the terminal grid. Default 80x24. When zero and
	// the Pixel fields are set, the grid derives from them instead.
	Cols, Rows int
	// PixelWidth, PixelHeight size the recording the way VHS does — in
	// pixels of a logical window — and PixelPadding is the inner margin
	// that shrinks the content area:
	//
	//	cols = (PixelWidth - 2*PixelPadding) / cellW
	//
	// Used only when Cols/Rows are zero — and with the Pixel fields set,
	// the output canvas is EXACTLY PixelWidth x PixelHeight (times
	// Scale), padding border included, like VHS.
	PixelWidth, PixelHeight, PixelPadding int
	// Window chrome (VHS parity), all in logical pixels of the canvas.
	// Margin is the band outside the window block, painted MarginFill —
	// a hex color ("#6B50FF", "#fff") or an image file path; empty means
	// the theme background. WindowBar draws VHS's title bar (BarSize
	// defaults to 30; WindowBarColor hex, empty = theme background).
	// BorderRadius rounds the window block, revealing MarginFill.
	Margin         int
	MarginFill     string
	WindowBar      WindowBarStyle
	WindowBarSize  int
	WindowBarColor string
	// WindowTitle draws static text in the bar (never auto-derived from
	// the host: recordings must not leak hostnames). WindowTitleLeft
	// aligns it left of center (macOS genre centers).
	WindowTitle     string
	WindowTitleLeft bool
	// WindowTitleFollow lets the bar follow the title the recorded
	// APPLICATION declares via OSC 0/2 — tmux, vim, the shell — like a
	// real terminal's tab, with WindowTitle as the fallback
	// until one is set. Footage, not host state: deterministic.
	WindowTitleFollow bool
	// KeysOverlay draws the injected input track as film-strip frames
	// on a stage band UNDER the window. The canvas GROWS by
	// the band height — the footage is never covered and the grid
	// never shrinks (a deliberate, documented divergence from VHS's
	// "declared size = canvas": VHS has no such feature). KeysSize
	// picks the reel size; zero = medium.
	KeysOverlay bool
	KeysSize    KeysSize
	// KeysNotation picks the cap vocabulary; zero =
	// keycap (words + drawn arrows).
	KeysNotation KeysNotation
	// KeysAccent colors the special/chord caps: empty = the theme's
	// bright magenta; an ANSI color name ("blue"), a "#hex", or "off"
	// to mute the hierarchy. (The MarginFill string idiom.)
	KeysAccent string
	// KeysPlain drops the film strip: caps float straight on the
	// margin fill.
	KeysPlain    bool
	BorderRadius int
	// GIFLoop is the gif's loop count, ffmpeg semantics (#633): 0 =
	// loop forever (the default), -1 = play once, N = repeat N more
	// times. Only .gif outputs read it.
	GIFLoop int
	// OutputScale picks the recording's output resolution: 2 (the
	// default) is the retina house output — every logical pixel is a
	// 2x2 block; 1 halves it to logical size with the exact integer
	// area mean (files at roughly a quarter of the weight, single-pixel
	// hairlines soften). Weight versus crispness — the user's call.
	OutputScale int
	// CaptureCast retains the raw pty output stream in memory, stamped
	// on the timeline — required for a .cast Output (asciicast v2).
	// Off by default: a long recording's byte stream is real memory.
	CaptureCast bool
	// KeepFrames leaves the staging frames directory (PNG frames +
	// manifest) on disk after Close instead of deleting it — the
	// caller owns deletion from then on. `foley play` replays it;
	// FramesDir names it.
	KeepFrames bool
	// Zoom reserves the camera: the scene renders on a 2×
	// supersampled master and Recorder.Zoom/ZoomOff drive a viewport
	// over it — every frame stays an exact integer downscale, so zoomed
	// frames are as sharp as the base render. Costs ~4× render time and
	// memory while recording, and even at rest every frame passes
	// through the master (a subtly smoother look — NOT byte-identical
	// to the plain render), so leave it off unless the recording zooms:
	// with it off the pipeline is untouched, byte for byte. The keys
	// reel stays pinned to the glass, outside the camera's world.
	Zoom bool
	// FontSize is the font size in logical pixels. Default 16.
	FontSize int
	// Scale multiplies every metric for supersampling. Default 2.
	Scale int
	// Theme colors the recording. Zero value = DefaultTheme().
	Theme Theme
	// FontsDir holds the pinned fonts (see scripts/fonts.sh). Empty
	// falls back to $FOLEY_FONTS, then "fonts".
	FontsDir string
	// FontFamily selects a font family by NAME from foley's pinned
	// catalog (`foley fonts` lists it) — NEVER from the
	// system: an unknown name is a loud assembly warning and the
	// default family renders. Asking for the default by name is a
	// no-op. Ignored when FontFile or FontFiles is set.
	FontFamily string
	// FontFile loads ONE user font file as the primary face:
	// a CWD-relative .ttf/.otf that drives the cell metrics, titles
	// the window bar and serves all four style slots. The pinned pack
	// stays for coverage fallback, emoji stay Noto, block sprites stay
	// synthesized. Determinism is parametrized: same tape + same font
	// bytes → same frames. Empty = the pinned default.
	FontFile string
	// FontFiles loads a user font FAMILY, one file per style —
	// Regular is required (metrics derive from it); absent styles
	// render with the Regular face. Takes precedence over FontFile.
	FontFiles FontFiles

	// Mode selects the clock. Default Deterministic.
	Mode Mode
	// ModifyOtherKeys selects how modified keys WITHOUT a legacy form
	// (Ctrl+Enter, Shift+Tab...) reach the application when it has not
	// pushed a keyboard protocol. False — the default — matches
	// xterm/xterm.js (and therefore VHS): Ctrl+Enter degrades to a plain
	// Enter. True keeps xterm's modern CSI-27 encodings for apps that
	// understand them.
	ModifyOtherKeys bool
	// FPS is the Realtime sampling rate. Default 60. Ignored in
	// Deterministic mode, where frames follow state changes exactly.
	FPS int
	// Settle tunes Deterministic quiescence detection.
	Settle Settle
}

// Recorder is one recording session: a live application, its embedded
// terminal, and the growing frame stream. Not safe for concurrent use.
type Recorder struct {
	timeline  driver.Timeline
	proc      *ptyx.Proc
	engine    vtengine.Engine
	sink      *encode.PNGSink
	framesDir string

	finished   bool
	closed     bool
	shots      int
	finalText  string
	keepFrames bool

	// assemblyWarnings surfaces raster findings (e.g. a proportional
	// user font) — nothing in the pipeline is allowed to stay silent.
	assemblyWarnings []string
	// highlights is the highlight track behind Highlight/ClearHighlights.
	highlights *raster.HighlightTrack
	// camera is the camera track behind Zoom/ZoomOff; nil unless
	// Options.Zoom reserved it. ras maps cells to master viewports.
	camera *raster.CameraTrack
	ras    *raster.Rasterizer
	// cast collects the pty byte stream when Options.CaptureCast asked
	// for it (the .cast Output's feed).
	cast *castLog
	// gifLoop carries Options.GIFLoop to the encoder.
	gifLoop int
}

// castLog captures (bytes, timeline instant) pairs. Mutex: realtime's
// loop goroutine appends while Output reads after the loop stopped —
// and the lock keeps it honest if that ordering ever shifts.
type castLog struct {
	mu         sync.Mutex
	cols, rows int
	events     []encode.CastEvent
}

func (c *castLog) add(data []byte, at time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, encode.CastEvent{At: at, Data: data})
}

func (c *castLog) write(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return encode.WriteCast(path, c.cols, c.rows, c.events)
}

// HighlightSpec selects what a highlight paints: a regex
// matched against each row's text, or a rectangle in CELLS
// (Col,Row,W,H). Exactly one form should be set. With Pick, Occurrence
// narrows a pattern to that match per frame — 0-based in screen order,
// the same standard as the rect's cells. Name lets ClearHighlight
// target just this one.
type HighlightSpec struct {
	Pattern        *regexp.Regexp
	Col, Row, W, H int
	Rect           bool
	Occurrence     int
	Pick           bool
	Name           string
}

// Highlight paints the theme's Selection color under the spec's cells
// from this instant of the timeline until ClearHighlights (or the end).
// Post-production: the pty never knows.
func (r *Recorder) Highlight(spec HighlightSpec) {
	r.highlights.Activate(raster.HighlightSpec(spec), r.timeline.Now())
}

// ClearHighlights turns every active highlight off at this instant.
func (r *Recorder) ClearHighlights() {
	r.highlights.Clear(r.timeline.Now())
}

// ClearHighlight turns off only the highlights carrying the name.
func (r *Recorder) ClearHighlight(name string) {
	r.highlights.ClearNamed(name, r.timeline.Now())
}

// DefaultZoomDur is the camera's house transition — the duration IS the
// shot: one curve, one default, tuned to read as intent.
const DefaultZoomDur = 600 * time.Millisecond

// MaxZoomDur caps a camera transition: each second of transition
// renders ~30 physical frames, so an absurd duration would silently
// explode the recording (a 1h "transition" is ~109,000 frames — unlike
// a 1h Sleep, which is one frame with a long delay). Ten seconds is
// already a very slow cinematic push; past it foley refuses LOUDLY.
const MaxZoomDur = 10 * time.Second

// Zoom eases the camera onto the given CELL rect (0-based, the house
// standard) starting at this instant of the timeline; it stays framed
// there until the next Zoom or ZoomOff. The rect grows to the output's
// aspect around its center and is clamped inside the window. dur <= 0
// means DefaultZoomDur. Refuses past the 2× sharp cap — foley never
// ships a blurry frame.
func (r *Recorder) Zoom(col, row, w, h int, dur time.Duration) error {
	if r.camera == nil {
		return errors.New("foley: zoom needs Options.Zoom — the camera reserves its 2× master before recording starts")
	}
	dur, err := zoomDur(dur)
	if err != nil {
		return err
	}
	target, err := r.ras.ZoomTarget(col, row, w, h)
	if err != nil {
		// The raster's error is self-contained and domain-tagged
		// ("zoom: …"); another prefix would only stutter downstream.
		return err
	}
	r.camera.MoveTo(target, r.timeline.Now(), dur)
	return nil
}

// ZoomOff eases the camera back to the full frame. dur <= 0 means
// DefaultZoomDur.
func (r *Recorder) ZoomOff(dur time.Duration) error {
	if r.camera == nil {
		return errors.New("foley: zoom needs Options.Zoom — the camera reserves its 2× master before recording starts")
	}
	dur, err := zoomDur(dur)
	if err != nil {
		return err
	}
	r.camera.Reset(r.timeline.Now(), dur)
	return nil
}

// zoomDur resolves a transition duration: zero/negative means the house
// default, past MaxZoomDur is a LOUD refusal (the frame-count bomb).
func zoomDur(dur time.Duration) (time.Duration, error) {
	if dur <= 0 {
		return DefaultZoomDur, nil
	}
	if dur > MaxZoomDur {
		return 0, fmt.Errorf("foley: zoom transition %v exceeds the %v cap — each second renders ~30 physical frames; a slower reveal is a longer Sleep while framed, not a longer transition", dur, MaxZoomDur)
	}
	return dur, nil
}

// ZoomCheck validates a zoom rect against the sharp cap WITHOUT moving
// the camera — the tape runner pre-flights every cue before a single
// key is typed, so a bad zoom fails the run at frame zero, not mid-take.
func (r *Recorder) ZoomCheck(col, row, w, h int) error {
	if r.camera == nil {
		return errors.New("foley: zoom needs Options.Zoom — the camera reserves its 2× master before recording starts")
	}
	_, err := r.ras.ZoomTarget(col, row, w, h)
	return err
}

// AssemblyWarnings reports findings from recorder assembly (e.g. a
// proportional user font). The caller decides how to print them;
// the slice is a copy.
func (r *Recorder) AssemblyWarnings() []string {
	return append([]string(nil), r.assemblyWarnings...)
}

// New starts the demo command on a fresh pty and prepares the pipeline.
// The caller must Close the recorder; in Realtime mode recording begins
// immediately.
func New(opts Options) (*Recorder, error) {
	applyDefaults(&opts)
	if len(opts.Command) == 0 {
		return nil, errors.New("foley: Command is required")
	}
	return assembleRecorder(opts, nil)
}

func applyDefaults(opts *Options) {
	if opts.Cols <= 0 && opts.PixelWidth <= 0 {
		opts.Cols = 80
	}
	if opts.Rows <= 0 && opts.PixelHeight <= 0 {
		opts.Rows = 24
	}
	if opts.FontSize <= 0 {
		opts.FontSize = 16
	}
	if opts.Scale <= 0 {
		opts.Scale = 2
	}
	if opts.FPS <= 0 {
		opts.FPS = 60
	}
	if opts.Theme == (Theme{}) {
		opts.Theme = DefaultTheme()
	}
	if opts.FontsDir == "" {
		opts.FontsDir = os.Getenv("FOLEY_FONTS")
	}
	if opts.FontsDir == "" && !fontpack.Embedded {
		// No dir, no env, and no baked-in fonts: the legacy default.
		// A binary built with -tags embedfonts leaves this empty so
		// fontpack.Load("") serves the embedded set.
		opts.FontsDir = "fonts"
	}
}

func colorsFor(t Theme) *vtengine.Colors {
	conv := func(c RGB) vtengine.RGB { return vtengine.RGB{R: c.R, G: c.G, B: c.B} }
	var ansi [16]vtengine.RGB
	for i, c := range t.ANSI {
		ansi[i] = conv(c)
	}
	colors := theme.Resolve(conv(t.Foreground), conv(t.Background), conv(t.Cursor), ansi)
	return &colors
}

// defaultWindowBarSize is VHS's bar height (style.go of the pin).
const defaultWindowBarSize = 30

// parseVHSHex parses VHS's accepted hex color forms: #RGB, RGB, #RRGGBB,
// RRGGBB (draw.go parseHexColor of the pin) — but errors LOUDLY instead
// of silently falling back to near-black.
// keysStyleFor resolves the keys style knobs against the
// recording's theme. Validation is loud even with the reel off — a
// typo'd accent must never pass silently.
func keysStyleFor(opts Options) (raster.KeysStyle, error) {
	st := raster.KeysStyle{Plain: opts.KeysPlain}
	if opts.KeysNotation == KeysIcons {
		st.Notation = raster.KeysIcons
	}
	switch a := opts.KeysAccent; {
	case a == "":
	case a == "off":
		st.AccentOff = true
	case strings.HasPrefix(a, "#"):
		c, err := parseVHSHex(a)
		if err != nil {
			return raster.KeysStyle{}, fmt.Errorf("foley: KeysAccent: %w", err)
		}
		st.Accent = &c
	default:
		slot, ok := keysAccentSlot(a)
		if !ok {
			return raster.KeysStyle{}, fmt.Errorf("foley: KeysAccent %q: an ANSI color name (black|red|green|yellow|blue|magenta|cyan|white), #hex, or off", a)
		}
		rgb := opts.Theme.ANSI[slot]
		st.Accent = &color.RGBA{R: rgb.R, G: rgb.G, B: rgb.B, A: 0xff}
	}
	return st, nil
}

func parseVHSHex(s string) (color.RGBA, error) {
	c := color.RGBA{A: 0xff}
	var err error
	switch len(s) {
	case 7:
		_, err = fmt.Sscanf(s, "#%02x%02x%02x", &c.R, &c.G, &c.B)
	case 6:
		_, err = fmt.Sscanf(s, "%02x%02x%02x", &c.R, &c.G, &c.B)
	case 4:
		_, err = fmt.Sscanf(s, "#%1x%1x%1x", &c.R, &c.G, &c.B)
		c.R *= 17
		c.G *= 17
		c.B *= 17
	case 3:
		_, err = fmt.Sscanf(s, "%1x%1x%1x", &c.R, &c.G, &c.B)
		c.R *= 17
		c.G *= 17
		c.B *= 17
	default:
		err = fmt.Errorf("hex color %q has invalid length", s)
	}
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid hex color %q", s)
	}
	return c, nil
}

// resolveFill turns Options.MarginFill into a raster fill. VHS's rule
// disambiguates: a "#" prefix is a color, anything else is an image file
// (video.go marginFillIsColor of the pin). Empty means the theme
// background. Errors are LOUD — a recording with the wrong wallpaper is
// worse than an error.
func resolveFill(spec string, themeBG color.RGBA) (raster.Fill, error) {
	switch {
	case spec == "":
		return raster.Fill{Color: themeBG}, nil
	case strings.HasPrefix(spec, "#"):
		c, err := parseVHSHex(spec)
		if err != nil {
			return raster.Fill{}, fmt.Errorf("foley: MarginFill: %w", err)
		}
		return raster.Fill{Color: c}, nil
	default:
		raw, err := os.ReadFile(spec) //nolint:gosec // the fill path is caller-provided configuration
		if err != nil {
			return raster.Fill{}, fmt.Errorf("foley: MarginFill: %w", err)
		}
		img, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			return raster.Fill{}, fmt.Errorf("foley: MarginFill %s: %w", spec, err)
		}
		return raster.Fill{Image: img}, nil
	}
}

// Type presses each rune of s, spacing keystrokes by perKey.
func (r *Recorder) Type(ctx context.Context, s string, perKey time.Duration) error {
	if r.finished {
		return ErrFinished
	}
	return r.timeline.Type(ctx, s, perKey)
}

// Press sends one key (encoded for the application's active keyboard
// protocol) and advances dur.
func (r *Recorder) Press(ctx context.Context, k key.Key, dur time.Duration) error {
	if r.finished {
		return ErrFinished
	}
	return r.timeline.Press(ctx, k, dur)
}

// Sleep advances the timeline.
func (r *Recorder) Sleep(ctx context.Context, d time.Duration) error {
	if r.finished {
		return ErrFinished
	}
	return r.timeline.Sleep(ctx, d)
}

// WaitText blocks until the visible screen matches re or timeout passes
// (wall clock). In Deterministic mode it consumes no timeline time —
// waits synchronize, they do not choreograph.
func (r *Recorder) WaitText(ctx context.Context, re *regexp.Regexp, timeout time.Duration) error {
	if r.finished {
		return ErrFinished
	}
	return r.timeline.WaitText(ctx, re, timeout)
}

// WaitLine blocks until the CURRENT line (the cursor's row) matches re —
// VHS's Wait+Line semantics. Timeout and timeline behavior are the same
// as WaitText.
func (r *Recorder) WaitLine(ctx context.Context, re *regexp.Regexp, timeout time.Duration) error {
	if r.finished {
		return ErrFinished
	}
	return r.timeline.Wait(ctx, func(f *vtengine.Frame) bool {
		return re.MatchString(f.RowText(f.Cursor.Y))
	}, timeout)
}

// ScreenText returns the current visible screen text (what waits match
// against) — the .txt output format and a debugging aid. Valid until
// Close, including after Output.
func (r *Recorder) ScreenText() (string, error) {
	if r.closed {
		return "", ErrFinished
	}
	return r.timeline.ScreenText()
}

// Hide suppresses frame emission; the application keeps running.
func (r *Recorder) Hide() error {
	if r.finished {
		return ErrFinished
	}
	return r.timeline.Hide()
}

// Show resumes frame emission.
func (r *Recorder) Show() error {
	if r.finished {
		return ErrFinished
	}
	return r.timeline.Show()
}

// Screenshot renders the current state into a standalone PNG at path.
// It works while hidden and consumes no timeline time.
func (r *Recorder) Screenshot(path string) error {
	if r.finished {
		return ErrFinished
	}
	name := fmt.Sprintf("shot-%03d", r.shots)
	r.shots++
	if err := r.timeline.Screenshot(name); err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("foley: %w", err)
		}
	}
	src := filepath.Join(r.framesDir, "still-"+name+".png")
	return copyFile(src, path)
}

// Now reports the timeline position (virtual in Deterministic mode,
// elapsed wall time in Realtime).
func (r *Recorder) Now() time.Duration { return r.timeline.Now() }

// Frames reports how many timeline frames have been emitted so far.
// Live: safe to read while the recording runs (a progress pulse).
func (r *Recorder) Frames() int { return r.sink.Frames() }

// FramesDir is the staging directory holding the PNG frames and their
// manifest. It outlives Close only with Options.KeepFrames — then the
// caller owns deleting it.
func (r *Recorder) FramesDir() string { return r.framesDir }

// RestlessSettles reports how many Deterministic-mode settle windows saw
// the app writing with no input to answer (animation, background work).
// Deterministic recordings collapse that self-paced motion into settled
// keyframes by design — Mode Realtime captures it as it happened. Always
// zero in Realtime mode.
func (r *Recorder) RestlessSettles() int { return r.timeline.RestlessSettles() }

// Output finishes the timeline (first call) and encodes the recording to
// path; the format follows the extension: .gif, .mp4, .webm, .webp,
// .cast (asciicast v2 — needs Options.CaptureCast), .txt (the final
// screen as text) — or a PNG frame sequence into a directory when the
// path has no extension or ends in .png (VHS's frames output).
// Multiple Output calls encode the same recording to multiple formats;
// timeline actions are invalid afterwards.
func (r *Recorder) Output(ctx context.Context, path string) error {
	if err := r.finish(); err != nil {
		return err
	}
	// VHS creates missing parent directories for its outputs; so do we.
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("foley: %w", err)
		}
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".gif":
		return encode.GIF(ctx, r.framesDir, path, r.gifLoop)
	case ".webp":
		return encode.WebP(ctx, r.framesDir, path)
	case ".mp4":
		return encode.MP4(ctx, r.framesDir, path)
	case ".webm":
		return encode.WebM(ctx, r.framesDir, path)
	case ".cast":
		// asciicast v2 (asciinema): the raw byte stream on the exact
		// timeline. finish() already sealed the recording.
		if r.cast == nil {
			return errors.New("foley: .cast output needs Options.CaptureCast — the byte stream is not retained by default")
		}
		if err := r.cast.write(path); err != nil {
			return fmt.Errorf("foley: %w", err)
		}
		return nil
	case ".txt", ".ascii":
		// finish() captured the closing screen before the timeline
		// stopped (the realtime loop cannot answer afterwards).
		if err := os.WriteFile(path, []byte(r.finalText+"\n"), 0o644); err != nil { //nolint:gosec // caller-requested artifact
			return fmt.Errorf("foley: %w", err)
		}
		return nil
	case "", ".png":
		return r.copyFrames(path)
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, filepath.Ext(path))
	}
}

// copyFrames delivers the staged frame PNGs into dir (the VHS frames
// output: one exact PNG per emitted state).
func (r *Recorder) copyFrames(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("foley: %w", err)
	}
	entries, err := os.ReadDir(r.framesDir)
	if err != nil {
		return fmt.Errorf("foley: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "frame-") || !strings.HasSuffix(name, ".png") {
			continue
		}
		if err := copyFile(filepath.Join(r.framesDir, name), filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func (r *Recorder) finish() error {
	if r.finished {
		return nil
	}
	r.finished = true
	// Capture the closing screen BEFORE stopping the timeline: the
	// realtime loop cannot answer afterwards, and .txt outputs need it.
	if text, err := r.timeline.ScreenText(); err == nil {
		r.finalText = text
	}
	// Close the sink even when Finish fails: whatever frames exist get a
	// manifest, so a later Output reports the real error instead of a
	// confusing missing-manifest one.
	return errors.Join(r.timeline.Finish(), r.sink.Close())
}

// Close releases everything: the demo process, the engine and the frame
// staging. Idempotent; call it after the last Output.
func (r *Recorder) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	var errs []error
	if !r.finished {
		r.finished = true
		errs = append(errs, r.timeline.Finish())
	}
	if r.proc != nil {
		errs = append(errs, r.proc.Close())
	}
	errs = append(errs, r.engine.Close())
	if r.framesDir != "" && !r.keepFrames {
		errs = append(errs, os.RemoveAll(r.framesDir))
	}
	return errors.Join(errs...)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // src is the recorder's own staging dir
	if err != nil {
		return fmt.Errorf("foley: %w", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst) //nolint:gosec // dst is the caller's requested path
	if err != nil {
		return fmt.Errorf("foley: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("foley: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("foley: %w", err)
	}
	return nil
}

// DefaultFontFamily is the pinned family recordings use unless a tape
// asks otherwise.
const DefaultFontFamily = fontpack.DefaultFamily

// FontFamilies lists the pinned name catalog, sorted — the names
// `Set FontFamily` resolves without touching the system.
func FontFamilies() []string { return fontpack.Families() }

// KnownFontFamily reports whether a name resolves in the catalog
// (case- and spacing-insensitive).
func KnownFontFamily(name string) bool { return fontpack.HasFamily(name) }

// FontFiles names a user font family, one file per style.
type FontFiles struct {
	Regular, Bold, Italic, BoldItalic string
}

func (f FontFiles) empty() bool {
	return f.Regular == "" && f.Bold == "" && f.Italic == "" && f.BoldItalic == ""
}

// overlayMux fans the driver's single Overlay out to several tracks:
// SetTime broadcasts, Breakpoints merge (sorted, deduplicated).
type overlayMux []driver.Overlay

func (m overlayMux) SetTime(t time.Duration) {
	for _, o := range m {
		o.SetTime(t)
	}
}

func (m overlayMux) Breakpoints(from, to time.Duration) []time.Duration {
	var out []time.Duration
	for _, o := range m {
		out = append(out, o.Breakpoints(from, to)...)
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

// resolveUserFonts turns the font options into the raster's user font:
// FontFiles > FontFile > FontFamily (pinned catalog) > none. Explicit
// file paths fail HARD on error (a typo'd path must not record); an
// unknown catalog NAME degrades to the default with a loud warning —
// the tape still records, like every VHS-parity degradation.
func resolveUserFonts(opts Options) (raster.UserFonts, []string, error) {
	var uf raster.UserFonts
	switch {
	case !opts.FontFiles.empty():
		uf.Label = opts.FontFiles.Regular
		for _, s := range []struct {
			dst  *[]byte
			path string
		}{
			{&uf.Regular, opts.FontFiles.Regular},
			{&uf.Bold, opts.FontFiles.Bold},
			{&uf.Italic, opts.FontFiles.Italic},
			{&uf.BoldItalic, opts.FontFiles.BoldItalic},
		} {
			if s.path == "" {
				continue
			}
			b, err := fontpack.LoadFile(s.path)
			if err != nil {
				return raster.UserFonts{}, nil, fmt.Errorf("foley: %w", err)
			}
			*s.dst = b
		}
	case opts.FontFile != "":
		b, err := fontpack.LoadFile(opts.FontFile)
		if err != nil {
			return raster.UserFonts{}, nil, fmt.Errorf("foley: %w", err)
		}
		uf = raster.UserFonts{Label: opts.FontFile, Regular: b}
	case opts.FontFamily != "":
		fam, err := fontpack.LoadFamily(opts.FontsDir, opts.FontFamily)
		if errors.Is(err, fontpack.ErrUnknownFamily) {
			return raster.UserFonts{}, []string{err.Error()}, nil
		}
		if err != nil {
			return raster.UserFonts{}, nil, fmt.Errorf("foley: fonts: %w (run scripts/fonts.sh and set FontsDir or $FOLEY_FONTS)", err)
		}
		if fam.Name == fontpack.DefaultFamily {
			return raster.UserFonts{}, nil, nil // the pack IS this family
		}
		uf = raster.UserFonts{
			Label: fam.Name, Regular: fam.Regular, Bold: fam.Bold,
			Italic: fam.Italic, BoldItalic: fam.BoldItalic,
		}
	}
	return uf, nil, nil
}

// assembleRecorder wires pty, engine, rasterizer, driver and sink. The
// engine argument lets tests substitute the fake; New passes nil to get
// the real one.
func assembleRecorder(opts Options, eng vtengine.Engine) (*Recorder, error) {
	pack, err := fontpack.Load(opts.FontsDir)
	if err != nil {
		return nil, fmt.Errorf("foley: fonts: %w (run scripts/fonts.sh and set FontsDir or $FOLEY_FONTS)", err)
	}
	userFonts, fontWarnings, err := resolveUserFonts(opts)
	if err != nil {
		return nil, err
	}
	// Chrome arithmetic is VHS's: margin and window bar EAT space from
	// the terminal area; Width/Height stay the final canvas.
	if opts.WindowBar != WindowBarNone && opts.WindowBarSize <= 0 {
		opts.WindowBarSize = defaultWindowBarSize
	}
	barH := 0
	if opts.WindowBar != WindowBarNone {
		barH = opts.WindowBarSize
	}

	// Metrics need a probe rasterizer before the real one (cell size
	// decides the grid, the grid decides the window geometry).
	probe, err := raster.New(raster.Options{
		Pack: pack, UserFonts: userFonts,
		FontSizePx: opts.FontSize, Scale: opts.Scale,
	})
	if err != nil {
		return nil, err
	}
	cellW, cellH := probe.LogicalCellSize()
	pixelPath := opts.Cols <= 0 && opts.Rows <= 0 && opts.PixelWidth > 0
	if opts.Cols <= 0 {
		opts.Cols = (opts.PixelWidth - 2*opts.Margin - 2*opts.PixelPadding) / cellW
	}
	if opts.Rows <= 0 {
		opts.Rows = (opts.PixelHeight - 2*opts.Margin - barH - 2*opts.PixelPadding) / cellH
	}
	if opts.Cols < 1 || opts.Rows < 1 {
		return nil, fmt.Errorf("foley: grid resolves to %dx%d — pixel size too small for the font (after margin/bar/padding)", opts.Cols, opts.Rows)
	}

	win := raster.Window{}
	// Zoom needs a definite canvas: the camera's world is the window
	// block, so the camera forces chrome the same way the keys reel does.
	chromeWanted := opts.PixelPadding > 0 || opts.Margin > 0 || barH > 0 || opts.BorderRadius > 0 || opts.KeysOverlay || opts.Zoom
	if pixelPath {
		// Pixel path: the canvas is EXACTLY the declared size, always.
		win.CanvasW, win.CanvasH = opts.PixelWidth, opts.PixelHeight
	} else if chromeWanted {
		// Grid path with chrome: the canvas grows around the grid.
		win.CanvasW = opts.Cols*cellW + 2*(opts.Margin+opts.PixelPadding)
		win.CanvasH = opts.Rows*cellH + 2*(opts.Margin+opts.PixelPadding) + barH
	}
	keysFontPx := 0
	if opts.KeysOverlay && win.CanvasW > 0 {
		// The reel EXTENDS the canvas below the declared size: a cue
		// never eats grid rows and never covers footage. Its
		// height follows the grid's cell scaled by the reel size.
		num := 4
		switch opts.KeysSize {
		case KeysSmall:
			num = 3
		case KeysMedium:
		case KeysLarge:
			num = 5
		}
		keysFontPx = opts.FontSize * num / 4
		band := raster.KeysBandFor(cellH * num / 4)
		win.KeysBand = band
		win.CanvasH += band
	}
	if win.CanvasW > 0 {
		win.Padding = opts.PixelPadding
		win.Margin = opts.Margin
		win.BarSize = opts.WindowBarSize
		win.Radius = opts.BorderRadius
		switch opts.WindowBar {
		case WindowBarNone:
			win.Bar = raster.BarNone
		case WindowBarColorful:
			win.Bar = raster.BarColorful
		case WindowBarColorfulRight:
			win.Bar = raster.BarColorfulRight
		case WindowBarRings:
			win.Bar = raster.BarRings
		case WindowBarRingsRight:
			win.Bar = raster.BarRingsRight
		case WindowBarLinuxControls:
			win.Bar = raster.BarLinuxControls
		case WindowBarGnomeCSD:
			win.Bar = raster.BarGnomeCSD
		}
		win.Title = opts.WindowTitle
		win.TitleFollow = opts.WindowTitleFollow
		if opts.WindowTitleLeft {
			win.TitleAlign = raster.TitleLeft
		}
		bg := opts.Theme.Background
		bgRGBA := color.RGBA{R: bg.R, G: bg.G, B: bg.B, A: 0xff}
		win.MarginFill, err = resolveFill(opts.MarginFill, bgRGBA)
		if err != nil {
			return nil, err
		}
		// Unset bar color = AUTO: the raster derives a shade from the
		// theme so the bar reads as a bar over any palette. (VHS's own
		// default — bar equals the background — renders an invisible
		// strip with floating dots; set WindowBarColor to the theme
		// background explicitly if that exact look is wanted.)
		if opts.WindowBarColor != "" {
			c, err := parseVHSHex(opts.WindowBarColor)
			if err != nil {
				return nil, fmt.Errorf("foley: WindowBarColor: %w", err)
			}
			win.BarColor = c
		}
	}

	keysStyle, err := keysStyleFor(opts)
	if err != nil {
		return nil, err
	}
	var keysTrack *raster.KeysTrack
	if opts.KeysOverlay {
		notation := raster.KeysKeycap
		if opts.KeysNotation == KeysIcons {
			notation = raster.KeysIcons
		}
		keysTrack = raster.NewKeysTrack(notation)
	}
	// The highlight track always exists — Recorder.Highlight is public
	// API, tape or no tape.
	highlightTrack := raster.NewHighlightTrack()
	selRGB := opts.Theme.Selection
	ssample := 1
	if opts.Zoom {
		// The camera's master: render at 2× the output so a full 2×
		// zoom is still a 1:1 crop — never an upscale.
		ssample = 2
	}
	ras, err := raster.New(raster.Options{
		Pack: pack, UserFonts: userFonts,
		FontSizePx: opts.FontSize, Scale: opts.Scale, SuperSample: ssample,
		Window: win,
		Keys:   keysTrack, KeysFontPx: keysFontPx, KeysStyle: keysStyle,
		Highlights:     highlightTrack,
		SelectionColor: color.RGBA{R: selRGB.R, G: selRGB.G, B: selRGB.B, A: 0xff},
	})
	if err != nil {
		return nil, err
	}
	geo := vtengine.Geometry{Cols: opts.Cols, Rows: opts.Rows, CellW: cellW, CellH: cellH}

	childEnv := opts.Env
	var identityWarnings []string
	if childEnv == nil {
		// foley IS the terminal: inheritance passes through
		// the identity layer, never raw.
		childEnv, identityWarnings = TerminalIdentity(os.Environ())
	}
	proc, err := ptyx.Start(ptyx.Options{
		Command: opts.Command,
		Dir:     opts.Dir,
		Env:     childEnv,
		Size: ptyx.Winsize{
			Cols: opts.Cols, Rows: opts.Rows,
			XPix: opts.Cols * cellW, YPix: opts.Rows * cellH,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("foley: %w", err)
	}

	if eng == nil {
		eng, err = factory.New("ghostty", vtengine.Options{
			Geometry: geo,
			// kitty's own default image storage budget.
			KittyStorageLimit: 320 << 20,
			Colors:            colorsFor(opts.Theme),
			Responses:         proc,
			ModifyOtherKeys:   opts.ModifyOtherKeys,
		})
		if err != nil {
			_ = proc.Close()
			return nil, err
		}
	} else if err := eng.Resize(geo); err != nil {
		_ = proc.Close()
		_ = eng.Close()
		return nil, err
	}

	framesDir, err := os.MkdirTemp("", "foley-frames-*")
	if err != nil {
		_ = proc.Close()
		_ = eng.Close()
		return nil, fmt.Errorf("foley: %w", err)
	}
	sink, err := encode.NewPNGSink(encode.PNGSinkOptions{Dir: framesDir})
	if err != nil {
		_ = proc.Close()
		_ = eng.Close()
		_ = os.RemoveAll(framesDir)
		return nil, err
	}

	render := func(f *vtengine.Frame, dst *image.RGBA) (*image.RGBA, error) {
		return ras.Render(f, eng, dst)
	}
	var cast *castLog
	if opts.CaptureCast {
		cast = &castLog{cols: opts.Cols, rows: opts.Rows}
	}
	if opts.OutputScale != 0 && opts.OutputScale != 1 && opts.OutputScale != 2 {
		return nil, fmt.Errorf("foley: OutputScale %d: 2 is the retina default, 1 halves to logical size", opts.OutputScale)
	}
	var camera *raster.CameraTrack
	if opts.Zoom {
		// The camera composits every frame: master render → viewport
		// downscaled into the output (the driver's reuse buffer), HUD
		// band glued at fixed 2:1. The master buffer is private and
		// reused — the driver never sees supersampled pixels.
		camera = raster.NewCameraTrack(ras.WorldRect())
		var masterBuf *image.RGBA
		render = func(f *vtengine.Frame, dst *image.RGBA) (*image.RGBA, error) {
			m, err := ras.Render(f, eng, masterBuf)
			if err != nil {
				return nil, err
			}
			masterBuf = m
			return ras.Composite(m, camera.Viewport(), dst), nil
		}
	}

	if opts.OutputScale == 1 {
		// The final pass halves whatever the pipeline produced (camera
		// included) with the exact 2:1 integer mean. The inner render
		// keeps its own buffer; the driver's dst stays the half-size
		// one it will reuse.
		inner := render
		var fullBuf *image.RGBA
		render = func(f *vtengine.Frame, dst *image.RGBA) (*image.RGBA, error) {
			full, err := inner(f, fullBuf)
			if err != nil {
				return nil, err
			}
			fullBuf = full
			return raster.DownscaleHalf(dst, full), nil
		}
	}

	// Overlay wiring: the driver sees ONE overlay; foley fans it out to
	// the tracks that exist (keys, highlights, camera) — the driver
	// never learns how many there are.
	var onKey func(k key.Key, at time.Duration, hidden bool)
	mux := overlayMux{highlightTrack}
	if keysTrack != nil {
		onKey = keysTrack.AddKey
		mux = append(mux, keysTrack)
	}
	if camera != nil {
		mux = append(mux, camera)
	}
	var overlay driver.Overlay = mux

	var onOutput func(data []byte, at time.Duration)
	if cast != nil {
		onOutput = cast.add
	}
	var timeline driver.Timeline
	switch opts.Mode {
	case Deterministic:
		timeline, err = driver.New(driver.Options{
			Engine: eng, Transport: proc, Render: render, Sink: sink,
			Settle: driver.SettleOptions(opts.Settle),
			OnKey:  onKey, Overlay: overlay, OnOutput: onOutput,
		})
	case Realtime:
		timeline, err = driver.NewRealtime(driver.RealtimeOptions{
			Engine: eng, Transport: proc, Render: render, Sink: sink,
			FPS:   opts.FPS,
			OnKey: onKey, Overlay: overlay, OnOutput: onOutput,
		})
	default:
		err = fmt.Errorf("foley: unknown mode %d", opts.Mode)
	}
	if err != nil {
		_ = proc.Close()
		_ = eng.Close()
		_ = os.RemoveAll(framesDir)
		return nil, err
	}

	return &Recorder{
		timeline:         timeline,
		proc:             proc,
		engine:           eng,
		sink:             sink,
		framesDir:        framesDir,
		keepFrames:       opts.KeepFrames,
		highlights:       highlightTrack,
		camera:           camera,
		ras:              ras,
		cast:             cast,
		gifLoop:          opts.GIFLoop,
		assemblyWarnings: append(append(fontWarnings, identityWarnings...), ras.Warnings()...),
	}, nil
}
