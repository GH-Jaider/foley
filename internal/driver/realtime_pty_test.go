package driver_test

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/driver"
	"github.com/GH-Jaider/foley/internal/ptyx"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/fake"
)

// TestRealtimeAgainstRealProcess samples a real process that prints over
// time. Assertions are stall-proof by design: frame texts must form a
// prefix progression ("" ⊆ "A" ⊆ "AB") ending in the full output — a
// starved loop may merge intermediate states (fewer frames) but can never
// produce an out-of-order or wrong frame.
func TestRealtimeAgainstRealProcess(t *testing.T) {
	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/sh", "-c", "printf A; sleep 1; printf B"},
		Size:    ptyx.Winsize{Cols: 20, Rows: 4, XPix: 160, YPix: 64},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	e := fake.New(vtengine.Options{Geometry: vtengine.Geometry{Cols: 20, Rows: 4}})
	r := newRecorder()
	d, err := driver.NewRealtime(driver.RealtimeOptions{
		Engine: e, Transport: p, Render: r.render, Sink: r, FPS: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.WaitText(context.Background(), regexp.MustCompile(`AB`), 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := d.Finish(); err != nil {
		t.Fatal(err)
	}

	frames := r.snapFrames()
	if len(frames) < 2 {
		t.Fatalf("frames = %+v, want at least the initial and the final state", frames)
	}
	if last := frames[len(frames)-1].text; last != "AB" {
		t.Fatalf("final frame = %q, want AB (all: %+v)", last, frames)
	}
	for i := 1; i < len(frames); i++ {
		if !strings.HasPrefix(frames[i].text, frames[i-1].text) {
			t.Fatalf("frames out of order at %d: %+v", i, frames)
		}
	}
	// Every span is positive except possibly the last: the final state is
	// emitted even when Finish lands right after the output that formed
	// it (its span is then near zero, and never negative).
	for i, f := range frames[:len(frames)-1] {
		if f.dur <= 0 {
			t.Fatalf("frame %d has non-positive duration: %+v", i, frames)
		}
	}
	if frames[len(frames)-1].dur < 0 {
		t.Fatalf("final frame has negative duration: %+v", frames)
	}
}
