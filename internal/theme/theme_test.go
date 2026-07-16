package theme_test

import (
	"testing"

	"github.com/GH-Jaider/foley/internal/theme"
	"github.com/GH-Jaider/foley/internal/vtengine"
)

func TestResolveXterm256(t *testing.T) {
	var ansi [16]vtengine.RGB
	ansi[1] = vtengine.RGB{R: 0xf3, G: 0x8b, B: 0xa8}
	fg := vtengine.RGB{R: 1, G: 2, B: 3}
	c := theme.Resolve(fg, vtengine.RGB{R: 9}, vtengine.RGB{}, ansi)

	if c.FG != fg || c.Palette[1] != ansi[1] {
		t.Fatalf("base colors not applied: %+v %+v", c.FG, c.Palette[1])
	}
	// The engine resolves a zero cursor to FG (Colors contract); Resolve
	// passes it through untouched.
	if c.Cursor != (vtengine.RGB{}) {
		t.Fatalf("cursor = %+v, want zero passthrough", c.Cursor)
	}
	// xterm cube corners and grayscale ramp, straight from the standard.
	cases := []struct {
		i    int
		want vtengine.RGB
	}{
		{16, vtengine.RGB{}},                        // cube origin
		{21, vtengine.RGB{B: 255}},                  // pure blue corner
		{196, vtengine.RGB{R: 255}},                 // pure red corner
		{46, vtengine.RGB{G: 255}},                  // pure green corner
		{231, vtengine.RGB{R: 255, G: 255, B: 255}}, // cube white
		{110, vtengine.RGB{R: 135, G: 175, B: 215}}, // an interior point
		{232, vtengine.RGB{R: 8, G: 8, B: 8}},       // ramp start
		{255, vtengine.RGB{R: 238, G: 238, B: 238}}, // ramp end
	}
	for _, cse := range cases {
		if got := c.Palette[cse.i]; got != cse.want {
			t.Fatalf("palette[%d] = %+v, want %+v", cse.i, got, cse.want)
		}
	}
}
