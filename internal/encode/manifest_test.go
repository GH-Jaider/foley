package encode

import (
	"image"
	"testing"
	"time"
)

// TestManifest pins the playback listing: every frame with its exact
// duration, in order, and the trailing repeated file (the concat
// boundary marker) dropped.
func TestManifest(t *testing.T) {
	dir := t.TempDir()
	s, err := NewPNGSink(PNGSinkOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := s.Add(img, 120*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(img, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	frames, err := Manifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 {
		t.Fatalf("frames = %d (%+v), want 2 — the boundary marker must be dropped", len(frames), frames)
	}
	if frames[0].Dur != 120*time.Millisecond || frames[1].Dur != 2*time.Second {
		t.Fatalf("durations = %v/%v, want 120ms/2s", frames[0].Dur, frames[1].Dur)
	}
	for _, f := range frames {
		if f.Path == "" {
			t.Fatalf("frame with empty path: %+v", frames)
		}
	}
}
