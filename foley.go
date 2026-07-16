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
	"context"
	"errors"
	"fmt"
	"image"
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

	// Cols, Rows size the terminal grid. Default 80x24.
	Cols, Rows int
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

	finished bool
	closed   bool
	shots    int
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
	if opts.Cols <= 0 {
		opts.Cols = 80
	}
	if opts.Rows <= 0 {
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
	src := filepath.Join(r.framesDir, "still-"+name+".png")
	return copyFile(src, path)
}

// Now reports the timeline position (virtual in Deterministic mode,
// elapsed wall time in Realtime).
func (r *Recorder) Now() time.Duration { return r.timeline.Now() }

// Output finishes the timeline (first call) and encodes the recording to
// path; the format follows the extension: .gif, .mp4 or .webm. Multiple
// Output calls encode the same recording to multiple formats; timeline
// actions are invalid afterwards.
func (r *Recorder) Output(ctx context.Context, path string) error {
	if err := r.finish(); err != nil {
		return err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".gif":
		return encode.GIF(ctx, r.framesDir, path)
	case ".mp4":
		return encode.MP4(ctx, r.framesDir, path)
	case ".webm":
		return encode.WebM(ctx, r.framesDir, path)
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, filepath.Ext(path))
	}
}

func (r *Recorder) finish() error {
	if r.finished {
		return nil
	}
	r.finished = true
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
	ras, err := raster.New(raster.Options{Pack: pack, FontSizePx: opts.FontSize, Scale: opts.Scale})
	if err != nil {
		return nil, err
	}
	cellW, cellH := ras.LogicalCellSize()
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
