// Package driver owns the recording timeline (ADR-012). In deterministic
// mode virtual time advances ONLY by script declaration: every action is a
// step — write bytes, settle (wall-clock wait for the app to quiesce,
// consuming ZERO virtual time), advance the declared duration. The output
// absorbed by a settle is attributed to the step's virtual instant, which
// is what makes recordings byte-identical regardless of how fast the app
// responds. Frames are emitted to the Sink as (image, exact duration)
// pairs, coalescing unchanged states — a 2s Sleep is one frame of 2s.
//
// Waits are synchronization, not choreography: they re-evaluate a
// predicate as output arrives, their timeout runs on the wall clock, and
// they consume no virtual time. Hide/Show trim the emitted timeline
// (actions still run; the video is shorter). The wall-clock mode lives in
// Realtime (realtime.go); Timeline is the surface shared by both clocks.
//
// The engine's Options.Responses writer must be wired to the same
// transport by whoever constructs the pair — capability-probing TUIs
// block waiting for those replies.
//
// Accepted risk, both clocks: Transport.Write can block if the pty
// saturates in both directions (the app stops reading input while its
// output backlog is full). Keystroke writes are tiny and the kernel pty
// buffer absorbs them, so this stays theoretical for tape-shaped input.
package driver

import (
	"context"
	"errors"
	"fmt"
	"image"
	"regexp"
	"time"

	"github.com/GH-Jaider/foley/internal/ptyx"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/key"
)

// ErrWaitTimeout is returned (wrapped, with a screen dump) when a Wait
// predicate does not match within its timeout.
var ErrWaitTimeout = errors.New("driver: wait timeout")

// ErrWaitInterrupted is returned (wrapped, with a screen dump) when a
// Wait can no longer succeed for reasons other than time: the
// application exited or the timeline finished first.
var ErrWaitInterrupted = errors.New("driver: wait interrupted")

// Transport carries bytes between the driver and the application: writes
// go to the app's pty, chunks come back with arrival timestamps.
// *ptyx.Proc satisfies it; tests use a synthetic one.
type Transport interface {
	Write(p []byte) (int, error)
	Chunks() <-chan ptyx.Chunk
}

// RenderFunc turns a frame into pixels, reusing dst when it has the right
// bounds. Wiring binds the rasterizer and its image source, e.g.:
//
//	func(f *vtengine.Frame, dst *image.RGBA) (*image.RGBA, error) {
//		return ras.Render(f, engine, dst)
//	}
type RenderFunc func(f *vtengine.Frame, dst *image.RGBA) (*image.RGBA, error)

// Sink receives the recording. Images are borrowed only for the duration
// of the call — the driver reuses the buffer for its NEXT render, so an
// encoder must consume (or copy) before returning, never retain the
// pointer. Durations are exact virtual-timeline spans; quantizing to a
// frame rate (if the target format needs one) is the encoder's business.
type Sink interface {
	// Add appends a frame shown for d. d is zero only when Finish flushes
	// the final state of a tape that ended on an instant action; the sink
	// decides what a zero-duration closing frame means (a typical encoder
	// clamps it to one frame period rather than lose the last state).
	Add(img *image.RGBA, d time.Duration) error
	// Still delivers a named screenshot outside the timeline.
	Still(name string, img *image.RGBA) error
}

// SettleOptions are the wall-clock knobs of a settle (ADR-012 D5).
type SettleOptions struct {
	// First bounds the wait for the first byte after a step's write; a
	// step may legitimately produce no output at all.
	First time.Duration
	// Quiet is the silence that ends a settle once output has arrived.
	Quiet time.Duration
	// Max caps the whole settle, streaming apps included.
	Max time.Duration
}

// Overlay is a time-driven composited layer (the keys band, ADR-016).
// The driver sets its clock to each frame's START instant before
// rendering, and splits time advances at its breakpoints so the
// overlay's animation lands on exact frames. Implementations live in
// the raster; the contract is structural — neither package imports the
// other.
type Overlay interface {
	SetTime(t time.Duration)
	Breakpoints(from, to time.Duration) []time.Duration
}

// Options configures a Driver. Engine, Transport, Render and Sink are
// required; zero Settle fields get defaults (150ms / 40ms / 2s).
type Options struct {
	Engine    vtengine.Engine
	Transport Transport
	Render    RenderFunc
	Sink      Sink
	Settle    SettleOptions
	// OnKey observes every injected keystroke at its virtual instant,
	// with the Hide state — concealed setup must not leak its typing.
	OnKey func(k key.Key, at time.Duration, hidden bool)
	// Overlay, when non-nil, animates over the frames (see Overlay).
	Overlay Overlay
	// OnOutput observes every pty byte chunk as the engine ingests it,
	// stamped with the timeline instant — the .cast emitter's feed.
	OnOutput func(data []byte, at time.Duration)
}

// Driver runs a deterministic recording timeline. Not safe for concurrent
// use: one driver per recording, driven from one goroutine.
type Driver struct {
	opts Options

	frame vtengine.Frame // reused snapshot target

	now    time.Duration // virtual timeline position
	hidden bool
	dirty  bool // engine changed since the pending frame was rendered
	// restless counts settles where the app kept writing with no input
	// to answer: the settle hit its Max ceiling (nonstop stream), or
	// output arrived in a settle no keystroke preceded (animation,
	// background work). The launch settle is exempt from the unprompted
	// rule — the first paint answers exec, not input. Waits never count:
	// output someone explicitly waits for was asked for.
	restless int
	settled  bool // some settle already ran (launch-paint exemption)
	launched bool // the pre-first-write launch settle already ran

	emitBuf  *image.RGBA // reused render target for timeline frames
	stillBuf *image.RGBA // reused render target for screenshots

	pending    *image.RGBA   // rendered state not yet sent to the sink
	pendingDur time.Duration // visible span accumulated by pending
}

// New validates options and applies settle defaults.
func New(opts Options) (*Driver, error) {
	if opts.Engine == nil || opts.Transport == nil || opts.Render == nil || opts.Sink == nil {
		return nil, errors.New("driver: Engine, Transport, Render and Sink are all required")
	}
	if opts.Settle.First <= 0 {
		opts.Settle.First = 150 * time.Millisecond
	}
	if opts.Settle.Quiet <= 0 {
		opts.Settle.Quiet = 40 * time.Millisecond
	}
	if opts.Settle.Max <= 0 {
		opts.Settle.Max = 2 * time.Second
	}
	return &Driver{opts: opts}, nil
}

// Now reports the virtual timeline position.
func (d *Driver) Now() time.Duration { return d.now }

// ingest feeds pty bytes to the engine and the OnOutput observer,
// stamped at the current virtual instant.
func (d *Driver) ingest(data []byte) error {
	if d.opts.OnOutput != nil {
		d.opts.OnOutput(data, d.now)
	}
	_, err := d.opts.Engine.Write(data)
	return err
}

// Type presses each rune of s as one step of perKey virtual time. Zero
// perKey is paste semantics: the whole string is ONE write and ONE
// settle — per-rune settling would spend real seconds of wall clock on
// keystrokes the timeline shows for zero time.
func (d *Driver) Type(ctx context.Context, s string, perKey time.Duration) error {
	if perKey == 0 {
		var buf []byte
		for _, r := range s {
			b, err := d.opts.Engine.EncodeKey(vtengine.KeyEvent{Key: key.RuneKey(r), Type: vtengine.KeyTap})
			if err != nil {
				return fmt.Errorf("driver: Type %q: %w", r, err)
			}
			buf = append(buf, b...)
			if d.opts.OnKey != nil {
				// Paste semantics: every rune lands at the same instant
				// — the track shows the string arriving whole.
				d.opts.OnKey(key.RuneKey(r), d.now, d.hidden)
			}
		}
		return d.step(ctx, buf, 0)
	}
	for _, r := range s {
		if err := d.Press(ctx, key.RuneKey(r), perKey); err != nil {
			return fmt.Errorf("driver: Type %q: %w", r, err)
		}
	}
	return nil
}

// Press encodes one key through the engine (which tracks the app's active
// keyboard protocol), writes it to the app and advances dur.
func (d *Driver) Press(ctx context.Context, k key.Key, dur time.Duration) error {
	b, err := d.opts.Engine.EncodeKey(vtengine.KeyEvent{Key: k, Type: vtengine.KeyTap})
	if err != nil {
		return fmt.Errorf("driver: Press: %w", err)
	}
	if d.opts.OnKey != nil {
		d.opts.OnKey(k, d.now, d.hidden)
	}
	return d.step(ctx, b, dur)
}

// Sleep advances virtual time. It still settles first, so output already
// in flight is deterministically part of the slept-over frame.
func (d *Driver) Sleep(ctx context.Context, dur time.Duration) error {
	return d.step(ctx, nil, dur)
}

// Wait blocks until pred matches a snapshot, feeding the engine as output
// arrives. The timeout is wall-clock and the wait consumes zero virtual
// time (ADR-012 D3). The frame passed to pred is borrowed for the call.
// On timeout the error wraps ErrWaitTimeout and includes the screen text.
func (d *Driver) Wait(ctx context.Context, pred func(*vtengine.Frame) bool, timeout time.Duration) error {
	f, err := d.snapshot()
	if err != nil {
		return err
	}
	if pred(f) {
		return nil
	}
	tm := time.NewTimer(timeout)
	defer tm.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch, ok := <-d.opts.Transport.Chunks():
			if !ok {
				return d.waitFailed(ErrWaitInterrupted, timeout, "application exited")
			}
			if err := d.ingest(ch.Data); err != nil {
				return err
			}
			f, err := d.snapshot()
			if err != nil {
				return err
			}
			if pred(f) {
				return nil
			}
		case <-tm.C:
			return d.waitFailed(ErrWaitTimeout, timeout, "no match")
		}
	}
}

// WaitText waits until the visible screen text matches re.
func (d *Driver) WaitText(ctx context.Context, re *regexp.Regexp, timeout time.Duration) error {
	return d.Wait(ctx, func(f *vtengine.Frame) bool { return re.MatchString(f.Text()) }, timeout)
}

func (d *Driver) waitFailed(base error, timeout time.Duration, why string) error {
	return fmt.Errorf("%w: %s after %v at virtual %v; screen:\n%s",
		base, why, timeout, d.now, d.frame.Text())
}

// Hide stops frame emission: subsequent actions run and advance virtual
// time, but the video omits them (the emitted timeline is shorter).
func (d *Driver) Hide() error {
	if d.hidden {
		return nil
	}
	err := d.flush()
	d.hidden = true
	return err
}

// Show resumes emission; the next advance renders the then-current state.
// The error is always nil here (Timeline symmetry with the wall clock).
func (d *Driver) Show() error {
	d.hidden = false
	return nil
}

// Screenshot renders the current settled state and hands it to the sink
// as a named still. It works while hidden and advances no time.
func (d *Driver) Screenshot(name string) error {
	if _, err := d.snapshot(); err != nil {
		return err
	}
	if d.opts.Overlay != nil {
		d.opts.Overlay.SetTime(d.now)
	}
	img, err := d.opts.Render(&d.frame, d.stillBuf)
	if err != nil {
		return err
	}
	d.stillBuf = img
	return d.opts.Sink.Still(name, img)
}

// ScreenText snapshots and returns the visible screen text.
func (d *Driver) ScreenText() (string, error) {
	f, err := d.snapshot()
	if err != nil {
		return "", err
	}
	return f.Text(), nil
}

// Finish flushes the trailing frame — unlike mid-stream flushes it sends
// the final state even with zero accumulated duration (see Sink.Add). A
// recording that should not end abruptly declares its final pause in the
// script (e.g. a last Sleep).
func (d *Driver) Finish() error {
	if d.pending == nil {
		return nil
	}
	err := d.opts.Sink.Add(d.pending, d.pendingDur)
	d.pending, d.pendingDur = nil, 0
	return err
}

// step is the universal primitive: write, settle, advance (ADR-012 D1).
// The very first step runs a LAUNCH settle before writing anything: the
// shell's initial paint (its prompt) must land before the first
// keystroke, or the keystroke races it and the recording opens with the
// key BEFORE the prompt (found live by the dress examples: "e> echo").
func (d *Driver) step(ctx context.Context, b []byte, dur time.Duration) error {
	if !d.launched {
		d.launched = true
		// prompted=true: the launch paint answers exec, not input — it
		// must never count as restless (same exemption RestlessSettles
		// documents).
		if err := d.settle(ctx, true); err != nil {
			return err
		}
	}
	if len(b) > 0 {
		if _, err := d.opts.Transport.Write(b); err != nil {
			return fmt.Errorf("driver: transport write: %w", err)
		}
	}
	if err := d.settle(ctx, len(b) > 0); err != nil {
		return err
	}
	return d.advance(dur)
}

// settle absorbs application output on the wall clock: up to First for
// the first byte, then until a Quiet gap, capped by Max. A closed
// transport (app exited) ends the settle immediately. Virtual time is
// untouched. Timer Reset without draining is sound on go >= 1.23.
func (d *Driver) settle(ctx context.Context, prompted bool) error {
	s := d.opts.Settle
	unprompted := !prompted && d.settled
	d.settled = true
	sawOutput := false
	deadline := time.NewTimer(s.Max)
	defer deadline.Stop()
	quiet := time.NewTimer(s.First)
	defer quiet.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch, ok := <-d.opts.Transport.Chunks():
			if !ok {
				return nil
			}
			sawOutput = true
			if err := d.ingest(ch.Data); err != nil {
				return err
			}
			quiet.Reset(s.Quiet)
		case <-quiet.C:
			if unprompted && sawOutput {
				d.restless++
			}
			return nil
		case <-deadline.C:
			d.restless++
			return nil
		}
	}
}

// RestlessSettles reports settles where the app kept writing with no
// input to answer — nonstop past the Max ceiling, or unprompted after a
// pure time advance (see the field's classification rules). A nonzero
// count means the app animates (or works) on its own and deterministic
// mode collapsed that motion into settled keyframes; callers surface it
// as a "record with Realtime" hint.
func (d *Driver) RestlessSettles() int { return d.restless }

// snapshot refreshes d.frame and accumulates dirtiness across every
// snapshot the driver takes — a Wait's intermediate snapshots must not
// swallow the change that the next emitted frame has to reflect.
func (d *Driver) snapshot() (*vtengine.Frame, error) {
	if err := d.opts.Engine.Snapshot(&d.frame); err != nil {
		return nil, err
	}
	d.dirty = d.dirty || d.frame.Dirty
	return &d.frame, nil
}

// advance moves virtual time by dur. With an overlay, the span first
// splits at the overlay's breakpoints — a chip fading mid-Sleep needs
// its own frames — and each cut forces a fresh render even when the
// grid did not change (the overlay did).
func (d *Driver) advance(dur time.Duration) error {
	if d.opts.Overlay == nil || d.hidden || dur == 0 {
		return d.advanceSpan(dur)
	}
	start := d.now
	prev := start
	for _, cut := range d.opts.Overlay.Breakpoints(start, start+dur) {
		if cut == start {
			// A state change lands exactly at this span's first frame
			// (e.g. a fade step that closed the previous advance).
			d.dirty = true
			continue
		}
		if cut <= prev || cut >= start+dur {
			continue
		}
		if err := d.advanceSpan(cut - prev); err != nil {
			return err
		}
		d.dirty = true
		prev = cut
	}
	return d.advanceSpan(start + dur - prev)
}

// advanceSpan accounts one span to the current settled state: unchanged
// state extends the pending frame, changed state flushes it and renders
// anew. States that were visible for zero time (perKey == 0 bursts) are
// skipped by flush, so they cost no frames.
func (d *Driver) advanceSpan(dur time.Duration) error {
	d.now += dur
	if d.hidden {
		return nil
	}
	if _, err := d.snapshot(); err != nil {
		return err
	}
	if d.pending != nil && !d.dirty {
		d.pendingDur += dur
		return nil
	}
	if err := d.flush(); err != nil {
		return err
	}
	if d.opts.Overlay != nil {
		// The frame represents the span STARTING at now-dur.
		d.opts.Overlay.SetTime(d.now - dur)
	}
	img, err := d.opts.Render(&d.frame, d.emitBuf)
	if err != nil {
		return err
	}
	d.emitBuf = img
	d.pending, d.pendingDur = img, dur
	d.dirty = false
	return nil
}

// flush sends the pending frame to the sink; zero-duration states vanish
// (they were never visible).
func (d *Driver) flush() error {
	if d.pending == nil {
		return nil
	}
	img, dur := d.pending, d.pendingDur
	d.pending, d.pendingDur = nil, 0
	if dur == 0 {
		return nil
	}
	return d.opts.Sink.Add(img, dur)
}
