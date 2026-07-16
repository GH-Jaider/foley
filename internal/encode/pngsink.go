package encode

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// manifestName is the ffconcat list PNGSink writes on Close.
const manifestName = "frames.ffconcat"

// manifestFramerate pins every entry's decoder timebase to 1/1000. The
// concat demuxer otherwise inherits image2's default 1/25 and silently
// QUANTIZES every duration to 40ms ticks (found the hard way: 60ms
// keystrokes came out alternating 80/40ms). Per-file `option` directives
// need ffmpeg >= 6.1 — execx enforces that minimum.
const manifestFramerate = "option framerate 1000\n"

// PNGSinkOptions configures a PNGSink. Dir is required.
type PNGSinkOptions struct {
	// Dir receives the frames, stills and manifest; created if missing.
	Dir string
	// ZeroDuration replaces the one legal zero duration — the final
	// state of a recording that ended on an instant action (see
	// driver.Sink on d == 0) — so the closing frame keeps a visible
	// span. The zero value means 20ms; recorders wanting a longer close
	// set it (or script a trailing pause, which this never overrides).
	ZeroDuration time.Duration
}

// PNGSink is the encode interchange (ADR-013 D1): frame-%05d.png files
// plus an ffconcat manifest carrying the driver's exact durations.
// Stills land next to them as still-<name>.png. It implements
// driver.Sink; images are consumed synchronously (encoded to disk before
// Add returns), honoring the borrowed-buffer contract.
type PNGSink struct {
	opts     PNGSinkOptions
	manifest bytes.Buffer
	n        int
	lastFile string
	closed   bool
}

// NewPNGSink creates the directory if needed and starts an empty
// recording.
func NewPNGSink(opts PNGSinkOptions) (*PNGSink, error) {
	if opts.Dir == "" {
		return nil, errors.New("encode: Dir is required")
	}
	if opts.ZeroDuration <= 0 {
		opts.ZeroDuration = 20 * time.Millisecond
	}
	if err := os.MkdirAll(opts.Dir, 0o750); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	s := &PNGSink{opts: opts}
	s.manifest.WriteString("ffconcat version 1.0\n")
	return s, nil
}

// Add writes the frame and appends its manifest entry.
func (s *PNGSink) Add(img *image.RGBA, d time.Duration) error {
	if s.closed {
		return errors.New("encode: sink already closed")
	}
	name := fmt.Sprintf("frame-%05d.png", s.n)
	s.n++
	if err := s.writePNG(name, img); err != nil {
		return err
	}
	if d <= 0 {
		d = s.opts.ZeroDuration
	}
	fmt.Fprintf(&s.manifest, "file '%s'\n%sduration %.6f\n", name, manifestFramerate, d.Seconds())
	s.lastFile = name
	return nil
}

// Still writes a named screenshot outside the timeline.
func (s *PNGSink) Still(name string, img *image.RGBA) error {
	if s.closed {
		return errors.New("encode: sink already closed")
	}
	return s.writePNG("still-"+sanitize(name)+".png", img)
}

// Close finalizes the manifest. The concat demuxer honors the last
// duration only when the last file is listed once more — Close appends
// that entry. A recording with zero frames is an upstream bug and errors.
func (s *PNGSink) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if s.n == 0 {
		return errors.New("encode: recording has no frames")
	}
	fmt.Fprintf(&s.manifest, "file '%s'\n%s", s.lastFile, manifestFramerate)
	path := filepath.Join(s.opts.Dir, manifestName)
	if err := os.WriteFile(path, s.manifest.Bytes(), 0o600); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}

// Frames reports how many timeline frames were added.
func (s *PNGSink) Frames() int { return s.n }

func (s *PNGSink) writePNG(name string, img *image.RGBA) error {
	f, err := os.Create(filepath.Join(s.opts.Dir, name)) //nolint:gosec // dir is caller-owned; name is generated or sanitized
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return fmt.Errorf("encode: %s: %w", name, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("encode: %s: %w", name, err)
	}
	return nil
}

// sanitize keeps still names filesystem-honest: anything outside
// [A-Za-z0-9._-] becomes '_' (no separators, no traversal).
func sanitize(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "still"
	}
	return b.String()
}
