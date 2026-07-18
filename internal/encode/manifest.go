package encode

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Frame is one playable entry of a closed recording: a PNG on disk and
// exactly how long it holds on screen.
type Frame struct {
	Path string
	Dur  time.Duration
}

// Manifest lists a closed recording's frames in playback order with
// their exact durations — what `foley play` replays on the wall clock.
// The manifest's trailing repeated file (the quirk that makes the last
// duration effective for ffmpeg's concat demuxer) is a boundary
// marker, not a frame, and is dropped.
func Manifest(framesDir string) ([]Frame, error) {
	raw, err := os.ReadFile(filepath.Join(framesDir, manifestName)) //nolint:gosec // fixed name inside the caller-owned frames dir
	if err != nil {
		return nil, fmt.Errorf("encode: no manifest (did you Close the PNGSink?): %w", err)
	}
	var frames []Frame
	pending := ""
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()
		if name, ok := strings.CutPrefix(line, "file '"); ok {
			pending = strings.TrimSuffix(name, "'")
			continue
		}
		val, ok := strings.CutPrefix(line, "duration ")
		if !ok {
			continue
		}
		if pending == "" {
			return nil, fmt.Errorf("encode: manifest duration without a file: %q", line)
		}
		micros, err := parseMicros(val)
		if err != nil {
			return nil, fmt.Errorf("encode: malformed manifest duration %q: %w", line, err)
		}
		frames = append(frames, Frame{
			Path: filepath.Join(framesDir, pending),
			Dur:  time.Duration(micros) * time.Microsecond,
		})
		pending = ""
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("encode: reading manifest: %w", err)
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("encode: manifest has no frames")
	}
	return frames, nil
}
