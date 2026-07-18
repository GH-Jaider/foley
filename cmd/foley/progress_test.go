package main

import (
	"strings"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/tape"
)

// TestRenderProgress pins the pulse's plain formats: deterministic
// (now/total), realtime (elapsed, no declared end), developing.
func TestRenderProgress(t *testing.T) {
	det := renderProgress(tape.ProgressEvent{
		Phase: tape.ProgressRecording,
		Now:   12300 * time.Millisecond, Total: 31800 * time.Millisecond, Frames: 47,
	})
	if det != "foley: rec 12.3s/31.8s · frame 47" {
		t.Fatalf("deterministic line = %q", det)
	}
	rt := renderProgress(tape.ProgressEvent{
		Phase: tape.ProgressRecording, Now: 74 * time.Second, Frames: 132,
	})
	if rt != "foley: rec 1:14 · frame 132" {
		t.Fatalf("realtime line = %q", rt)
	}
	dev := renderProgress(tape.ProgressEvent{
		Phase: tape.ProgressDeveloping, Output: "demo.gif", Frames: 96,
	})
	if !strings.Contains(dev, "developing demo.gif") || !strings.Contains(dev, "96") {
		t.Fatalf("developing line = %q", dev)
	}
}

// TestProgressSilentOffTTY pins the CI/pipe contract: a non-terminal
// stderr gets NOTHING from the renderer — today's silence, untouched.
func TestProgressSilentOffTTY(t *testing.T) {
	var sb strings.Builder
	p := newProgressRenderer(&sb)
	p.pulse(tape.ProgressEvent{Phase: tape.ProgressRecording, Now: time.Second, Total: 2 * time.Second})
	p.pulse(tape.ProgressEvent{Phase: tape.ProgressDeveloping, Output: "x.gif"})
	p.done()
	if sb.Len() != 0 {
		t.Fatalf("non-TTY renderer wrote %q", sb.String())
	}
	// The warn stream passes through untouched off-TTY.
	if _, err := p.warnWriter().Write([]byte("w\n")); err != nil {
		t.Fatal(err)
	}
	if sb.String() != "w\n" {
		t.Fatalf("warn passthrough = %q", sb.String())
	}
}
