package foley

import (
	"regexp"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/raster"
	"github.com/GH-Jaider/foley/key"
)

// TestOverlayMux pins the fan-out: breakpoints from every member,
// merged, sorted, deduplicated — the driver sees one clean stream.
func TestOverlayMux(t *testing.T) {
	hl := raster.NewHighlightTrack()
	hl.Activate(raster.HighlightSpec{Pattern: regexp.MustCompile("x")}, time.Second)
	hl.Clear(2 * time.Second)
	keys := raster.NewKeysTrack()
	keys.AddKey(key.RuneKey('a'), 1500*time.Millisecond, false)

	m := overlayMux{hl, keys}
	m.SetTime(42 * time.Millisecond) // must not panic across members

	cuts := m.Breakpoints(0, 10*time.Second)
	if len(cuts) < 3 {
		t.Fatalf("breakpoints = %v, want members merged", cuts)
	}
	seen := map[time.Duration]bool{}
	for i, c := range cuts {
		if i > 0 && cuts[i-1] >= c {
			t.Fatalf("breakpoints not strictly increasing: %v", cuts)
		}
		seen[c] = true
	}
	for _, want := range []time.Duration{time.Second, 1500 * time.Millisecond, 2 * time.Second} {
		if !seen[want] {
			t.Fatalf("breakpoints %v lack %v", cuts, want)
		}
	}
}
