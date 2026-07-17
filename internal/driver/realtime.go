package driver

import (
	"context"
	"errors"
	"fmt"
	"image"
	"regexp"
	"sync"
	"time"

	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/key"
)

// RealtimeOptions configures a Realtime timeline. Engine, Transport,
// Render and Sink are required; FPS defaults to 60.
type RealtimeOptions struct {
	Engine    vtengine.Engine
	Transport Transport
	Render    RenderFunc
	Sink      Sink
	FPS       int
	// OnKey and Overlay mirror the deterministic driver's (ADR-016);
	// here timestamps are wall-clock since the recording started, and
	// the overlay animates on the FPS ticks (no span splitting).
	OnKey   func(k key.Key, at time.Duration, hidden bool)
	Overlay Overlay
}

// Realtime is the wall-clock timeline (ADR-012 D7): recording starts at
// NewRealtime, application output is consumed as it arrives, and frames
// are sampled at FPS with dirty-skip — an unchanged screen extends the
// current frame for free. Durations are real elapsed time between emitted
// states, so the same Sink works for both clocks.
//
// Sampling semantics, stated plainly: a state change is attributed to
// the tick that observes it. A tick landing inside a repaint burst
// defers (micro-quiescence, see rtQuietWindow), so attribution lags the
// bytes by up to 1+rtMaxDeferredTicks frame periods, and an app that
// saturates the quiet window records at FPS/(1+rtMaxDeferredTicks) —
// bounded, in exchange for never sampling a torn half-repaint. ptyx
// chunk timestamps remain available if finer attribution is ever
// needed. Dirtiness is engine-level, not pixel-level — an app that
// rewrites identical content produces consecutive identical frames.
//
// One event-loop goroutine owns the engine, the render buffers and the
// sink; action methods post requests to it and are safe to call from the
// recording goroutine. Sleep and the per-key delay of Type pass real
// time. Errors that happen between actions (a failing sink on a tick)
// surface on the next action or on Finish. One Wait may be active at a
// time (Timeline use is sequential by design).
type Realtime struct {
	opts  RealtimeOptions
	reqs  chan rtReq
	stop  chan struct{}
	done  chan struct{}
	err   error // written by the loop before done closes
	start time.Time

	// lp is the loop-owned state; request closures run on the loop
	// goroutine, which is the only toucher after NewRealtime returns.
	lp *rtLoop

	stopOnce sync.Once
}

type rtReq struct {
	do    func() error
	reply chan error
}

// rtLoop is the event-loop-owned state.
type rtLoop struct {
	r     *Realtime
	frame vtengine.Frame

	emitBuf  *image.RGBA
	stillBuf *image.RGBA

	pending      *image.RGBA
	pendingStart time.Time
	hidden       bool
	dirty        bool
	firstErr     error

	waitPred    func(*vtengine.Frame) bool
	waitReply   chan error
	waitTimer   *time.Timer
	waitTimeout time.Duration

	// appExited remembers a closed chunk channel: it fires exactly once,
	// and a Wait installed AFTER it must still fail fast.
	appExited bool

	// lastChunk and deferred implement micro-quiescence: a tick landing
	// inside a repaint burst would snapshot a torn, half-drawn UI (a
	// state no human ever perceives on a real terminal), so rendering
	// defers until the stream has been quiet for a moment — bounded, so
	// a continuously-writing app still gets frames.
	lastChunk time.Time
	deferred  int
}

const (
	// rtQuietWindow is how long the byte stream must be quiet before a
	// tick may snapshot (repaint bursts span single-digit milliseconds).
	rtQuietWindow = 6 * time.Millisecond
	// rtMaxDeferredTicks caps quiescence deferrals. Consequence, stated
	// plainly: an app that never leaves a 6ms gap records at
	// FPS/(rtMaxDeferredTicks+1) — a bounded worst case of a third of
	// the requested rate, in exchange for never sampling mid-repaint.
	// Deliberately internal, not an Options knob: tuning it is choosing
	// between torn frames and sampling lag, and both bounds are already
	// at the perceptual floor.
	rtMaxDeferredTicks = 2
)

// NewRealtime validates options and starts the recording immediately.
func NewRealtime(opts RealtimeOptions) (*Realtime, error) {
	if opts.Engine == nil || opts.Transport == nil || opts.Render == nil || opts.Sink == nil {
		return nil, errors.New("driver: Engine, Transport, Render and Sink are all required")
	}
	if opts.FPS <= 0 {
		opts.FPS = 60
	}
	r := &Realtime{
		opts:  opts,
		reqs:  make(chan rtReq),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
		start: time.Now(),
	}
	go r.loop()
	return r, nil
}

// Now reports real time elapsed since the recording started.
func (r *Realtime) Now() time.Duration { return time.Since(r.start) }

// RestlessSettles is always zero: the wall clock records continuously —
// nothing collapses, nothing to flag.
func (r *Realtime) RestlessSettles() int { return 0 }

// Type presses each rune of s, spacing keystrokes by perKey of real
// time. Zero perKey is paste semantics: one encoded write, no spacing —
// mirroring the deterministic clock.
func (r *Realtime) Type(ctx context.Context, s string, perKey time.Duration) error {
	if perKey == 0 {
		return r.do(ctx, func() error {
			var buf []byte
			for _, rn := range s {
				b, err := r.opts.Engine.EncodeKey(vtengine.KeyEvent{Key: key.RuneKey(rn), Type: vtengine.KeyTap})
				if err != nil {
					return fmt.Errorf("driver: Type %q: %w", rn, err)
				}
				buf = append(buf, b...)
				if r.opts.OnKey != nil {
					r.opts.OnKey(key.RuneKey(rn), time.Since(r.start), r.lp.hidden)
				}
			}
			if _, err := r.opts.Transport.Write(buf); err != nil {
				return fmt.Errorf("driver: transport write: %w", err)
			}
			return nil
		})
	}
	for _, rn := range s {
		if err := r.Press(ctx, key.RuneKey(rn), perKey); err != nil {
			return fmt.Errorf("driver: Type %q: %w", rn, err)
		}
	}
	return nil
}

// Press encodes one key through the engine and writes it to the app, then
// lets dur of real time pass.
func (r *Realtime) Press(ctx context.Context, k key.Key, dur time.Duration) error {
	err := r.do(ctx, func() error {
		b, err := r.opts.Engine.EncodeKey(vtengine.KeyEvent{Key: k, Type: vtengine.KeyTap})
		if err != nil {
			return fmt.Errorf("driver: Press: %w", err)
		}
		if r.opts.OnKey != nil {
			r.opts.OnKey(k, time.Since(r.start), r.lp.hidden)
		}
		if _, err := r.opts.Transport.Write(b); err != nil {
			return fmt.Errorf("driver: transport write: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return sleepCtx(ctx, dur)
}

// Sleep passes real time; the loop keeps sampling frames meanwhile.
func (r *Realtime) Sleep(ctx context.Context, d time.Duration) error {
	return sleepCtx(ctx, d)
}

// Wait blocks until pred matches a snapshot (re-evaluated as output
// arrives) or timeout passes. It consumes real time like everything in
// this mode, and the frames sampled meanwhile are emitted normally.
func (r *Realtime) Wait(ctx context.Context, pred func(*vtengine.Frame) bool, timeout time.Duration) error {
	reply := make(chan error, 1)
	if err := r.do(ctx, func() error {
		return r.lp.installWait(pred, reply, timeout)
	}); err != nil {
		return err
	}
	select {
	case err := <-reply:
		return err
	case <-ctx.Done():
		// Uninstall our predicate so a later Wait does not trip over it;
		// the identity check makes this a no-op if the wait resolved
		// concurrently (its buffered reply is simply dropped).
		_ = r.do(context.Background(), func() error { r.lp.cancelWait(reply); return nil })
		return ctx.Err()
	}
}

// WaitText waits until the visible screen text matches re.
func (r *Realtime) WaitText(ctx context.Context, re *regexp.Regexp, timeout time.Duration) error {
	return r.Wait(ctx, func(f *vtengine.Frame) bool { return re.MatchString(f.Text()) }, timeout)
}

// Hide stops frame emission (the app keeps running and real time keeps
// passing; the video omits the span).
func (r *Realtime) Hide() error {
	return r.do(context.Background(), func() error { return r.lp.hide() })
}

// Show resumes emission with the then-current state.
func (r *Realtime) Show() error {
	return r.do(context.Background(), func() error { return r.lp.show() })
}

// Screenshot renders the current state as a named still; works hidden.
func (r *Realtime) Screenshot(name string) error {
	return r.do(context.Background(), func() error { return r.lp.screenshot(name) })
}

// ScreenText returns the current visible screen text.
func (r *Realtime) ScreenText() (string, error) {
	var text string
	err := r.do(context.Background(), func() error {
		if !r.lp.snapshot() {
			return r.lp.firstErr
		}
		text = r.lp.frame.Text()
		return nil
	})
	return text, err
}

// Finish stops sampling, flushes the trailing frame and returns the first
// error the loop hit, if any. Idempotent.
func (r *Realtime) Finish() error {
	r.stopOnce.Do(func() { close(r.stop) })
	<-r.done
	return r.err
}

// do runs fn on the event loop and returns its error.
func (r *Realtime) do(ctx context.Context, fn func() error) error {
	req := rtReq{do: fn, reply: make(chan error, 1)}
	select {
	case r.reqs <- req:
	case <-r.done:
		return r.errAfterDone()
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-req.reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Realtime) errAfterDone() error {
	if r.err != nil {
		return r.err
	}
	return errors.New("driver: realtime timeline already finished")
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// --- event loop ---

// loop owns engine, buffers and sink. It renders the initial state, then
// multiplexes application output, the frame ticker, action requests and a
// possible active wait until Finish.
func (r *Realtime) loop() {
	lp := &rtLoop{r: r}
	r.lp = lp
	defer close(r.done)

	ticker := time.NewTicker(time.Second / time.Duration(r.opts.FPS))
	defer ticker.Stop()

	lp.renderPending(time.Now()) // the recording opens with the initial state

	chunks := r.opts.Transport.Chunks()
	var waitTimeoutC <-chan time.Time
	for {
		if lp.waitTimer != nil {
			waitTimeoutC = lp.waitTimer.C
		} else {
			waitTimeoutC = nil
		}
		select {
		case <-r.stop:
			lp.finish(time.Now())
			r.err = lp.firstErr
			return
		case req := <-r.reqs:
			req.reply <- req.do()
		case ch, ok := <-chunks:
			if !ok {
				chunks = nil // app exited; keep ticking until Finish
				lp.appExited = true
				lp.failWait(ErrWaitInterrupted, "application exited")
				continue
			}
			lp.onChunk(ch.Data)
		case now := <-ticker.C:
			lp.onTick(now)
		case <-waitTimeoutC:
			lp.failWait(ErrWaitTimeout, "no match")
		}
	}
}

func (lp *rtLoop) fail(err error) {
	if lp.firstErr == nil && err != nil {
		lp.firstErr = err
	}
}

// snapshot mirrors the deterministic driver: dirtiness accumulates across
// every snapshot so wait evaluations cannot swallow it.
func (lp *rtLoop) snapshot() bool {
	if err := lp.r.opts.Engine.Snapshot(&lp.frame); err != nil {
		lp.fail(err)
		return false
	}
	lp.dirty = lp.dirty || lp.frame.Dirty
	return true
}

func (lp *rtLoop) onChunk(data []byte) {
	lp.lastChunk = time.Now()
	if _, err := lp.r.opts.Engine.Write(data); err != nil {
		lp.fail(err)
		return
	}
	if lp.waitPred == nil {
		return
	}
	if !lp.snapshot() {
		return
	}
	if lp.waitPred(&lp.frame) {
		lp.waitReply <- nil
		lp.clearWait()
	}
}

func (lp *rtLoop) onTick(now time.Time) {
	if lp.hidden || lp.firstErr != nil {
		return
	}
	if !lp.lastChunk.IsZero() && now.Sub(lp.lastChunk) < rtQuietWindow && lp.deferred < rtMaxDeferredTicks {
		lp.deferred++ // mid-burst: wait for micro-quiescence, bounded
		return
	}
	lp.deferred = 0
	if !lp.snapshot() {
		return
	}
	if lp.pending == nil || !lp.dirty {
		// Unchanged screen: the pending span grows for free — unless
		// the OVERLAY changed since the pending frame started (a cap
		// born, a take fading). Idle overlays must not spam frames.
		ov := lp.r.opts.Overlay
		if ov == nil || lp.pending == nil {
			return
		}
		from := lp.pendingStart.Sub(lp.r.start)
		if len(ov.Breakpoints(from, now.Sub(lp.r.start))) == 0 {
			return
		}
	}
	lp.flush(now)
	lp.renderCurrent(now) // lp.frame is fresh from this tick's snapshot
}

// renderPending snapshots and renders the current state as the new
// pending frame (cold entry points: loop start, Show).
func (lp *rtLoop) renderPending(now time.Time) {
	if !lp.snapshot() {
		return
	}
	lp.renderCurrent(now)
}

// renderCurrent renders lp.frame as-is into the new pending frame.
func (lp *rtLoop) renderCurrent(now time.Time) {
	if ov := lp.r.opts.Overlay; ov != nil {
		ov.SetTime(now.Sub(lp.r.start))
	}
	img, err := lp.r.opts.Render(&lp.frame, lp.emitBuf)
	if err != nil {
		lp.fail(err)
		return
	}
	lp.emitBuf = img
	lp.pending = img
	lp.pendingStart = now
	lp.dirty = false
}

func (lp *rtLoop) flush(now time.Time) {
	if lp.pending == nil {
		return
	}
	img := lp.pending
	lp.pending = nil
	if d := now.Sub(lp.pendingStart); d > 0 {
		lp.fail(lp.r.opts.Sink.Add(img, d))
	}
}

// flushFinal sends the pending frame even with a non-positive span: the
// closing state of a recording must not vanish (the sink decides what a
// zero-duration final frame means).
func (lp *rtLoop) flushFinal(now time.Time) {
	if lp.pending == nil {
		return
	}
	img := lp.pending
	lp.pending = nil
	d := now.Sub(lp.pendingStart)
	if d < 0 {
		d = 0
	}
	lp.fail(lp.r.opts.Sink.Add(img, d))
}

// finish closes the recording: if output arrived after the last sampling
// tick (a Wait that matched right before Finish, typically), the final
// state was never rendered — render and emit it even with a near-zero
// span, mirroring the deterministic Finish (see Sink.Add on d == 0). The
// recording must end on what the screen actually shows.
func (lp *rtLoop) finish(now time.Time) {
	lp.failWait(ErrWaitInterrupted, "timeline finished")
	if lp.hidden {
		return
	}
	if lp.snapshot() && lp.dirty && lp.pending != nil {
		lp.flush(now)
		lp.renderCurrent(now)
	}
	lp.flushFinal(now)
}

func (lp *rtLoop) hide() error {
	if lp.hidden {
		return lp.firstErr
	}
	lp.flush(time.Now())
	lp.hidden = true
	return lp.firstErr
}

func (lp *rtLoop) show() error {
	if !lp.hidden {
		return lp.firstErr
	}
	lp.hidden = false
	lp.renderPending(time.Now())
	return lp.firstErr
}

func (lp *rtLoop) screenshot(name string) error {
	if !lp.snapshot() {
		return lp.firstErr
	}
	if ov := lp.r.opts.Overlay; ov != nil {
		ov.SetTime(time.Since(lp.r.start))
	}
	img, err := lp.r.opts.Render(&lp.frame, lp.stillBuf)
	if err != nil {
		return err
	}
	lp.stillBuf = img
	return lp.r.opts.Sink.Still(name, img)
}

func (lp *rtLoop) installWait(pred func(*vtengine.Frame) bool, reply chan error, timeout time.Duration) error {
	if lp.waitPred != nil {
		return errors.New("driver: a Wait is already active")
	}
	if !lp.snapshot() {
		return lp.firstErr
	}
	if pred(&lp.frame) {
		reply <- nil
		return nil
	}
	if lp.appExited {
		// The closed-channel signal already fired; without this check a
		// wait installed after the app died would sit out its timeout.
		lp.waitTimeout = timeout
		return lp.waitError(ErrWaitInterrupted, "application exited")
	}
	lp.waitPred = pred
	lp.waitReply = reply
	lp.waitTimeout = timeout
	lp.waitTimer = time.NewTimer(timeout)
	return nil
}

func (lp *rtLoop) failWait(base error, why string) {
	if lp.waitPred == nil {
		return
	}
	lp.waitReply <- lp.waitError(base, why)
	lp.clearWait()
}

func (lp *rtLoop) waitError(base error, why string) error {
	return fmt.Errorf("%w: %s after %v at %v; screen:\n%s",
		base, why, lp.waitTimeout, lp.r.Now(), lp.frame.Text())
}

// cancelWait uninstalls the wait identified by its reply channel; a
// mismatch means that wait already resolved and this is a no-op.
func (lp *rtLoop) cancelWait(reply chan error) {
	if lp.waitReply == reply {
		lp.clearWait()
	}
}

func (lp *rtLoop) clearWait() {
	if lp.waitTimer != nil {
		lp.waitTimer.Stop()
	}
	lp.waitPred, lp.waitReply, lp.waitTimer = nil, nil, nil
}
