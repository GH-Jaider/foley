package encode

import (
	"testing"
	"time"
)

func TestParseDurations(t *testing.T) {
	manifest := []byte(`ffconcat version 1.0
file 'frame-00000.png'
option framerate 1000
duration 0.060000
file 'frame-00001.png'
option framerate 1000
duration 1.300000
file 'frame-00001.png'
option framerate 1000
`)
	total, last, err := parseDurations(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1360*time.Millisecond {
		t.Fatalf("total = %v", total)
	}
	if last != 1300*time.Millisecond {
		t.Fatalf("last = %v", last)
	}
}

func TestParseDurationsMalformed(t *testing.T) {
	for _, bad := range []string{
		"duration 0.5\n",     // wrong fraction width
		"duration abc.def\n", // not numbers
		"duration 1\n",       // no fraction
	} {
		if _, _, err := parseDurations([]byte(bad)); err == nil {
			t.Fatalf("malformed %q must error", bad)
		}
	}
	if _, _, err := parseDurations([]byte("ffconcat version 1.0\n")); err == nil {
		t.Fatal("manifest without durations must error")
	}
}

func TestCentiseconds(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want int64
	}{
		{1300 * time.Millisecond, 130},
		{60 * time.Millisecond, 6},
		{20 * time.Millisecond, 2},
		{4 * time.Millisecond, 1}, // GIF's floor: sub-centisecond is inexpressible
	}
	for _, c := range cases {
		if got := centiseconds(c.d); got != c.want {
			t.Fatalf("centiseconds(%v) = %d, want %d", c.d, got, c.want)
		}
	}
}
