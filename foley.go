// Package foley records scripted terminal demos without a terminal
// window: the demo command runs on a real pty, an embedded terminal
// engine keeps the state, and foley rasterizes every frame itself —
// byte-identical output across machines, zero screen-capture permissions.
//
// A Recorder is created with New, driven with Timeline actions (Type,
// Press, Sleep, WaitText, Hide/Show, Screenshot) and closed into files
// with Output — the format follows the extension (.gif, .mp4, .webm):
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
	"strings"
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
// and grayscale ramp. A zero Cursor follows the foreground.
type Theme struct {
	Foreground RGB
	Background RGB
	Cursor     RGB
	ANSI       [16]RGB
}

// DefaultTheme is the built-in dark theme (Catppuccin Mocha).
func DefaultTheme() Theme {
	return Theme{
		Foreground: RGB{0xcd, 0xd6, 0xf4},
		Background: RGB{0x1e, 0x1e, 0x2e},
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
	// Env is the exact child environment (nil = inherit).
	Env []string

	// Cols, Rows size the terminal grid. Default 80x24. When zero and
	// the Pixel fields are set, the grid derives from them instead.
	Cols, Rows int
	// PixelWidth, PixelHeight size the recording the way VHS does — in
	// pixels of a logical window — and PixelPadding is the inner margin
	// that shrinks the content area (ADR-008 D6):
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
	BorderRadius    int
	// FontSize is the font size in logical pixels. Default 16.
	FontSize int
	// Scale multiplies every metric for supersampling. Default 2.
	Scale int
	// Theme colors the recording. Zero value = DefaultTheme().
	Theme Theme
	// FontsDir holds the pinned fonts (see scripts/fonts.sh). Empty
	// falls back to $FOLEY_FONTS, then "fonts".
	FontsDir string

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

	finished  bool
	closed    bool
	shots     int
	finalText string
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
	if opts.FontsDir == "" {
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

// RestlessSettles reports how many Deterministic-mode settle windows saw
// the app writing with no input to answer (animation, background work).
// Deterministic recordings collapse that self-paced motion into settled
// keyframes by design — Mode Realtime captures it as it happened. Always
// zero in Realtime mode.
func (r *Recorder) RestlessSettles() int { return r.timeline.RestlessSettles() }

// Output finishes the timeline (first call) and encodes the recording to
// path; the format follows the extension: .gif, .mp4, .webm, .txt (the
// final screen as text) — or a PNG frame sequence into a directory when
// the path has no extension or ends in .png (VHS's frames output).
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
		return encode.GIF(ctx, r.framesDir, path)
	case ".mp4":
		return encode.MP4(ctx, r.framesDir, path)
	case ".webm":
		return encode.WebM(ctx, r.framesDir, path)
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
	if r.framesDir != "" {
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

// assembleRecorder wires pty, engine, rasterizer, driver and sink. The
// engine argument lets tests substitute the fake; New passes nil to get
// the real one.
func assembleRecorder(opts Options, eng vtengine.Engine) (*Recorder, error) {
	pack, err := fontpack.Load(opts.FontsDir)
	if err != nil {
		return nil, fmt.Errorf("foley: fonts: %w (run scripts/fonts.sh and set FontsDir or $FOLEY_FONTS)", err)
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
	probe, err := raster.New(raster.Options{Pack: pack, FontSizePx: opts.FontSize, Scale: opts.Scale})
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
	chromeWanted := opts.PixelPadding > 0 || opts.Margin > 0 || barH > 0 || opts.BorderRadius > 0
	if pixelPath {
		// Pixel path: the canvas is EXACTLY the declared size, always.
		win.CanvasW, win.CanvasH = opts.PixelWidth, opts.PixelHeight
	} else if chromeWanted {
		// Grid path with chrome: the canvas grows around the grid.
		win.CanvasW = opts.Cols*cellW + 2*(opts.Margin+opts.PixelPadding)
		win.CanvasH = opts.Rows*cellH + 2*(opts.Margin+opts.PixelPadding) + barH
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

	ras, err := raster.New(raster.Options{Pack: pack, FontSizePx: opts.FontSize, Scale: opts.Scale, Window: win})
	if err != nil {
		return nil, err
	}
	geo := vtengine.Geometry{Cols: opts.Cols, Rows: opts.Rows, CellW: cellW, CellH: cellH}

	proc, err := ptyx.Start(ptyx.Options{
		Command: opts.Command,
		Dir:     opts.Dir,
		Env:     opts.Env,
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

	var timeline driver.Timeline
	switch opts.Mode {
	case Deterministic:
		timeline, err = driver.New(driver.Options{
			Engine: eng, Transport: proc, Render: render, Sink: sink,
			Settle: driver.SettleOptions(opts.Settle),
		})
	case Realtime:
		timeline, err = driver.NewRealtime(driver.RealtimeOptions{
			Engine: eng, Transport: proc, Render: render, Sink: sink,
			FPS: opts.FPS,
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
		timeline:  timeline,
		proc:      proc,
		engine:    eng,
		sink:      sink,
		framesDir: framesDir,
	}, nil
}
