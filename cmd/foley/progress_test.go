package main

import (
	"strings"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/tape"
)

// TestRenderProgress pins the pulse's frames — colorless for exact
// equality, colored for the ink being there.
func TestRenderProgress(t *testing.T) {
	det := renderProgress(tape.ProgressEvent{
		Phase: tape.ProgressRecording,
		Now:   12300 * time.Millisecond, Total: 31800 * time.Millisecond, Frames: 47,
	}, 0, false)
	// 18 cells x 12.3/31.8 = 6 filled.
	if det != "● REC ▕▪▪▪▪▪▪············▏ 12.3s/31.8s · frame 47" {
		t.Fatalf("deterministic line = %q", det)
	}
	// The clock legitimately runs past the promise (waits): clamp full.
	over := renderProgress(tape.ProgressEvent{
		Phase: tape.ProgressRecording,
		Now:   40 * time.Second, Total: 31800 * time.Millisecond, Frames: 90,
	}, 0, false)
	if !strings.Contains(over, "▕▪▪▪▪▪▪▪▪▪▪▪▪▪▪▪▪▪▪▏") {
		t.Fatalf("over-total bar not clamped full: %q", over)
	}
	rt := renderProgress(tape.ProgressEvent{
		Phase: tape.ProgressRecording, Now: 74 * time.Second, Frames: 132,
	}, 0, false)
	if rt != "● REC 1:14 · frame 132" {
		t.Fatalf("realtime line = %q", rt)
	}
	dev := renderProgress(tape.ProgressEvent{
		Phase: tape.ProgressDeveloping, Output: "demo.gif", Frames: 96,
	}, 1, false)
	if dev != "◓ developing demo.gif · 96 frames" {
		t.Fatalf("developing line = %q", dev)
	}

	// Color mode carries the ink; the dot blinks and the reel turns.
	colored := renderProgress(tape.ProgressEvent{
		Phase: tape.ProgressRecording, Now: time.Second, Total: 2 * time.Second,
	}, 0, true)
	if !strings.Contains(colored, sgrRed) || !strings.Contains(colored, sgrDim) || !strings.Contains(colored, sgrReset) {
		t.Fatalf("colored line lacks ink: %q", colored)
	}
	blinkOff := renderProgress(tape.ProgressEvent{
		Phase: tape.ProgressRecording, Now: time.Second, Total: 2 * time.Second,
	}, 1, true)
	if colored == blinkOff {
		t.Fatal("the REC dot must blink between ticks")
	}
	if reelFrame(0) == reelFrame(1) {
		t.Fatal("the developing reel must turn")
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
