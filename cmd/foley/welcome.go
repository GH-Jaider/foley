package main

import (
	"bytes"
	"image/gif"
	"io"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/GH-Jaider/foley/assets/logo"
	"github.com/GH-Jaider/foley/internal/preview"
)

// The front door wears the brand. On a terminal that speaks kitty
// graphics (decided by the same handshake `play` uses) the embedded
// projector plays — the film chip typing the name, REC light blinking.
// Anywhere else with color, the logo is REPLICATED IN CELLS: block
// glyphs on truecolor, sprocket holes left at the terminal's own
// background (holes are holes). No TTY or NO_COLOR: no art, just the
// words.

// logoHandshakeTimeout mirrors play's: a graphics terminal answers in
// milliseconds; the DA1 sentinel closes the wait early everywhere else.
const logoHandshakeTimeout = 250 * time.Millisecond

// showLogo draws the brand above the welcome. Failures at any step
// fall through silently to the next door — the welcome text is the
// content; the art is the dress.
func showLogo(stdout io.Writer) {
	f, ok := stdout.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return
	}
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		defer func() { _ = tty.Close() }()
		if preview.Supported(tty, logoHandshakeTimeout) {
			if g, gerr := gif.DecodeAll(bytes.NewReader(logo.GIF)); gerr == nil {
				if preview.ShowGIF(tty, g, 18) == nil {
					return
				}
			}
		}
	}
	_, noColor := os.LookupEnv("NO_COLOR")
	if !noColor && os.Getenv("TERM") != "dumb" {
		_, _ = io.WriteString(stdout, logo.CellArt())
	}
}
