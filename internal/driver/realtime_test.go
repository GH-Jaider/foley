package driver_test

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/driver"
	"github.com/GH-Jaider/foley/internal/ptyx"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/fake"
	"github.com/GH-Jaider/foley/key"
)

// Realtime tests assert CONTENT and ORDER, never wall timings: waitFor
// conditions are eventually-true with generous timeouts (one-sided — a
// slow runner delays them, it cannot flip them).
type rtRig struct {
	d  *driver.Realtime
	tr *transport
	r  *recorder
}

func newRtRig(t *testing.T) *rtRig {
	t.Helper()
	e := fake.New(vtengine.Options{Geometry: vtengine.Geometry{Cols: 20, Rows: 4}})
	tr := newTransport(false)
	r := newRecorder()
	d, err := driver.NewRealtime(driver.RealtimeOptions{
		Engine: e, Transport: tr, Render: r.render, Sink: r,
		FPS: 200, // 5ms ticks keep the tests brisk; nothing asserts tick timing
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Finish() })
	return &rtRig{d: d, tr: tr, r: r}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestRealtimeEmitsOnChangeWithDirtySkip(t *testing.T) {
	g := newRtRig(t)
	g.tr.feed("A")
	waitFor(t, "the initial frame to flush", func() bool { return len(g.r.snapFrames()) >= 1 })
	g.tr.feed("B")
	waitFor(t, "the A frame to flush", func() bool { return len(g.r.snapFrames()) >= 2 })
	if err := g.d.Finish(); err != nil {
		t.Fatal(err)
	}
	frames := g.r.snapFrames()
	if len(frames) != 3 {
		t.Fatalf("frames = %+v, want 3", frames)
	}
	for i, want := range []string{"", "A", "AB"} {
		if frames[i].text != want {
			t.Fatalf("frame %d text = %q, want %q (all: %+v)", i, frames[i].text, want, frames)
		}
		if frames[i].dur <= 0 {
			t.Fatalf("frame %d has non-positive duration: %+v", i, frames)
		}
	}
}

func TestRealtimeWaitText(t *testing.T) {
	g := newRtRig(t)
	go func() {
		time.Sleep(5 * time.Millisecond)
		g.tr.feed("READY")
	}()
	if err := g.d.WaitText(context.Background(), regexp.MustCompile(`READY`), 5*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestRealtimeWaitTimeoutDumpsScreen(t *testing.T) {
	g := newRtRig(t)
	g.tr.feed("other stuff")
	err := g.d.WaitText(context.Background(), regexp.MustCompile(`NEVER`), 20*time.Millisecond)
	if !errors.Is(err, driver.ErrWaitTimeout) {
		t.Fatalf("err = %v, want ErrWaitTimeout", err)
	}
}

func TestRealtimeWaitInterruptedOnAppExit(t *testing.T) {
	g := newRtRig(t)
	close(g.tr.ch)
	err := g.d.WaitText(context.Background(), regexp.MustCompile(`NEVER`), 5*time.Second)
	if !errors.Is(err, driver.ErrWaitInterrupted) {
		t.Fatalf("err = %v, want ErrWaitInterrupted", err)
	}
}

func TestRealtimeWaitCanceledThenRetry(t *testing.T) {
	g := newRtRig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := g.d.WaitText(ctx, regexp.MustCompile(`NEVER`), 5*time.Second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("canceled wait = %v, want DeadlineExceeded", err)
	}
	// The canceled predicate must be uninstalled: a fresh Wait works.
	g.tr.feed("READY")
	if err := g.d.WaitText(context.Background(), regexp.MustCompile(`READY`), 5*time.Second); err != nil {
		t.Fatalf("retry after cancel = %v", err)
	}
}

func TestRealtimeHideShowTrimsTimeline(t *testing.T) {
	g := newRtRig(t)
	if err := g.d.Hide(); err != nil {
		t.Fatal(err)
	}
	g.tr.feed("X")
	// WaitText is the deterministic sync point: it runs on the loop and
	// proves the chunk was consumed before Show renders.
	if err := g.d.WaitText(context.Background(), regexp.MustCompile(`X`), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Show(); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Finish(); err != nil {
		t.Fatal(err)
	}
	frames := g.r.snapFrames()
	if len(frames) != 2 {
		t.Fatalf("frames = %+v, want 2 (hidden span must emit nothing)", frames)
	}
	if frames[0].text != "" || frames[1].text != "X" {
		t.Fatalf("frame texts = %+v, want blank then X", frames)
	}
}

func TestRealtimeScreenshotWhileHidden(t *testing.T) {
	g := newRtRig(t)
	if err := g.d.Hide(); err != nil {
		t.Fatal(err)
	}
	g.tr.feed("secret")
	if err := g.d.WaitText(context.Background(), regexp.MustCompile(`secret`), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Screenshot("proof"); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Finish(); err != nil {
		t.Fatal(err)
	}
	stills := g.r.snapStills()
	if len(stills) != 1 || stills[0] != (stillRec{"proof", "secret"}) {
		t.Fatalf("stills = %+v", stills)
	}
	// Hidden from Hide to Finish: only the pre-Hide blank frame emits.
	if frames := g.r.snapFrames(); len(frames) != 1 || frames[0].text != "" {
		t.Fatalf("frames = %+v, want only the pre-Hide blank", frames)
	}
}

// TestRealtimeContinuousWriterStillGetsFrames: micro-quiescence defers
// ticks that land mid-burst, but its cap must keep a NONSTOP writer
// rendering — starvation would record nothing.
func TestRealtimeContinuousWriterStillGetsFrames(t *testing.T) {
	g := newRtRig(t)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for i := 0; ; i++ {
			select {
			case g.tr.ch <- ptyx.Chunk{Data: []byte{byte('a' + i%26)}}:
			case <-stop:
				return
			}
		}
	}()
	waitFor(t, "frames despite a nonstop writer", func() bool { return len(g.r.snapFrames()) >= 2 })
}

func TestRealtimePressWritesAndActionsAfterFinishFail(t *testing.T) {
	g := newRtRig(t)
	if err := g.d.Type(context.Background(), "hi", 0); err != nil {
		t.Fatal(err)
	}
	// Typing goes app-ward through the transport, not into the engine.
	waitFor(t, "transport to receive the keys", func() bool { return string(g.tr.snapWritten()) == "hi" })
	if err := g.d.Finish(); err != nil {
		t.Fatal(err)
	}
	if err := g.d.Finish(); err != nil { // idempotent
		t.Fatal(err)
	}
	if err := g.d.Sleep(context.Background(), 0); err != nil {
		t.Fatal(err) // Sleep needs no loop
	}
	if err := g.d.Press(context.Background(), key.RuneKey('a'), 0); err == nil {
		t.Fatal("Press after Finish must fail")
	}
}

// TestRealtimeOverlayTicksOnlyOnBreakpoints pins the keys wiring on the
// wall clock (ADR-016): OnKey observes the press on the loop goroutine,
// and an overlay only forces tick renders at its breakpoints — an idle
// overlay must NOT spam one frame per tick. Content and order asserts
// only; no wall timings.
func TestRealtimeOverlayTicksOnlyOnBreakpoints(t *testing.T) {
	e := fake.New(vtengine.Options{Geometry: vtengine.Geometry{Cols: 20, Rows: 4}})
	tr := newTransport(false)
	r := newRecorder()
	ov := &scriptedOverlay{cuts: []time.Duration{30 * time.Millisecond}}
	var mu sync.Mutex
	var pressed []key.Key
	d, err := driver.NewRealtime(driver.RealtimeOptions{
		Engine: e, Transport: tr, Render: r.render, Sink: r,
		FPS: 200,
		OnKey: func(k key.Key, _ time.Duration, hidden bool) {
			mu.Lock()
			defer mu.Unlock()
			if !hidden {
				pressed = append(pressed, k)
			}
		},
		Overlay: ov,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := d.Press(ctx, key.RuneKey('a'), 0); err != nil {
		t.Fatal(err)
	}
	if err := d.Sleep(ctx, 120*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := d.Finish(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(pressed) != 1 || pressed[0] != key.RuneKey('a') {
		t.Fatalf("OnKey saw %v, want the single 'a'", pressed)
	}
	// Structural frame bound: the initial render, ONE breakpoint-forced
	// render, and the trailing flush — never a frame per tick (~24 ticks
	// ran). The grid never changed (echo off), so any extra frame means
	// the overlay spammed.
	frames := r.snapFrames()
	if len(frames) < 2 || len(frames) > 3 {
		t.Fatalf("frames = %d (%+v), want 2-3: initial + breakpoint(+trailing)", len(frames), frames)
	}
}
