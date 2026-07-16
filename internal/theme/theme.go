// Package theme resolves foley's neutral theme model into engine colors:
// the 16 ANSI slots come from the theme, slots 16-231 are the standard
// xterm 6x6x6 color cube and 232-255 the standard grayscale ramp — an
// application using 256-color SGR sees exactly what it would in a real
// terminal, never a black hole of unseeded slots.
package theme

import "github.com/GH-Jaider/foley/internal/vtengine"

// cubeLevels are the xterm color-cube channel values (16 + 36r + 6g + b).
//
//nolint:gochecknoglobals // immutable lookup table of a public standard
var cubeLevels = [6]uint8{0, 95, 135, 175, 215, 255}

// Resolve builds the full engine color set from a neutral theme. A zero
// cursor follows the foreground (the Colors contract).
func Resolve(fg, bg, cursor vtengine.RGB, ansi [16]vtengine.RGB) vtengine.Colors {
	c := vtengine.Colors{FG: fg, BG: bg, Cursor: cursor}
	copy(c.Palette[:16], ansi[:])
	for i := 16; i < 232; i++ {
		n := i - 16
		c.Palette[i] = vtengine.RGB{
			R: cubeLevels[n/36],
			G: cubeLevels[n/6%6],
			B: cubeLevels[n%6],
		}
	}
	for i := 232; i < 256; i++ {
		v := uint8(8 + 10*(i-232)) //nolint:gosec // bounded: 8..238
		c.Palette[i] = vtengine.RGB{R: v, G: v, B: v}
	}
	return c
}
