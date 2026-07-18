package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/GH-Jaider/foley/tape"
)

// The pulse, dressed as film: a perforated strip fills toward the
// script's declared total while the REC dot blinks, and the reel spins
// while ffmpeg develops — the CLI speaks the same visual language as
// the keys reel. One stderr line redrawn in place, animated by a
// private ticker. TTY only: CI and pipes stay exactly as silent as
// ever; NO_COLOR (or TERM=dumb) keeps the layout and drops the ink.

const (
	// pulseFPS is the animation redraw rate — enough for a blink and a
	// spin, invisible in cost.
	pulseFPS = 8
	// barCells is the film strip's inner width in frames.
	barCells = 18

	sgrRed   = "\x1b[31m"
	sgrDim   = "\x1b[2m"
	sgrReset = "\x1b[0m"
)

// reelFrame is the developing spinner: a reel turning.
func reelFrame(tick int) string {
	return string([]rune("◐◓◑◒")[tick%4])
}

// progressRenderer owns the line and its animation. The mutex covers
// the ticker goroutine, the recording goroutine's pulses and warning
// interleaves.
type progressRenderer struct {
	mu    sync.Mutex
	w     io.Writer
	tty   bool
	color bool
	ev    tape.ProgressEvent
	hasEv bool
	tick  int
	line  string
	stop  chan struct{}
}

// newProgressRenderer detects the stage: a terminal gets the show,
// anything else gets silence.
func newProgressRenderer(w io.Writer) *progressRenderer {
	f, ok := w.(*os.File)
	tty := ok && term.IsTerminal(int(f.Fd()))
	_, noColor := os.LookupEnv("NO_COLOR")
	return &progressRenderer{
		w:     w,
		tty:   tty,
		color: tty && !noColor && os.Getenv("TERM") != "dumb",
	}
}

// pulse is the RunOptions.Progress callback; the first one raises the
// curtain (starts the animation ticker).
func (p *progressRenderer) pulse(ev tape.ProgressEvent) {
	if !p.tty {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ev, p.hasEv = ev, true
	if p.stop == nil {
		p.stop = make(chan struct{})
		go p.animate(p.stop)
	}
	p.redrawLocked()
}

func (p *progressRenderer) animate(stop chan struct{}) {
	t := time.NewTicker(time.Second / pulseFPS)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			p.mu.Lock()
			p.tick++
			p.redrawLocked()
			p.mu.Unlock()
		}
	}
}

func (p *progressRenderer) redrawLocked() {
	if !p.hasEv {
		return
	}
	line := renderProgress(p.ev, p.tick, p.color)
	if line == p.line {
		return
	}
	p.line = line
	_, _ = fmt.Fprintf(p.w, "\r\x1b[2K%s", line)
}

// done cuts: stops the animation and clears the line so the final
// output prints clean. Safe to call more than once; a later pulse
// (watch mode's next take) raises the curtain again.
func (p *progressRenderer) done() {
	if !p.tty {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stop != nil {
		close(p.stop)
		p.stop = nil
	}
	if p.line != "" || p.hasEv {
		p.line = ""
		p.hasEv = false
		_, _ = fmt.Fprint(p.w, "\r\x1b[2K")
	}
}

// warnWriter returns the writer warnings should stream through: it
// clears the strip, lets the warning print whole, and redraws — a
// mid-take warning never lands glued to the pulse.
func (p *progressRenderer) warnWriter() io.Writer {
	if !p.tty {
		return p.w
	}
	return progressWarnWriter{p}
}

type progressWarnWriter struct{ p *progressRenderer }

func (w progressWarnWriter) Write(b []byte) (int, error) {
	w.p.mu.Lock()
	defer w.p.mu.Unlock()
	_, _ = fmt.Fprint(w.p.w, "\r\x1b[2K")
	n, err := w.p.w.Write(b)
	if w.p.line != "" {
		_, _ = fmt.Fprintf(w.p.w, "\r\x1b[2K%s", w.p.line)
	}
	return n, err
}

// renderProgress formats one animation frame of the pulse. Pure — the
// tests pin it without a terminal.
func renderProgress(ev tape.ProgressEvent, tick int, color bool) string {
	dim := func(s string) string {
		if color {
			return sgrDim + s + sgrReset
		}
		return s
	}
	if ev.Phase == tape.ProgressDeveloping {
		return fmt.Sprintf("%s developing %s %s %d frames",
			reelFrame(tick), ev.Output, dim("·"), ev.Frames)
	}
	// The REC dot blinks in color; without ink it holds steady.
	dot := "●"
	if color {
		if tick%2 == 0 {
			dot = sgrRed + "●" + sgrReset
		} else {
			dot = sgrRed + sgrDim + "●" + sgrReset
		}
	}
	if ev.Total > 0 {
		return fmt.Sprintf("%s REC %s %.1fs/%.1fs %s frame %d",
			dot, filmBar(ev.Now, ev.Total, color),
			ev.Now.Seconds(), ev.Total.Seconds(), dim("·"), ev.Frames)
	}
	// Realtime: a camera rolling on the wall clock has no declared end
	// — elapsed and frames, honestly barless.
	return fmt.Sprintf("%s REC %d:%02d %s frame %d",
		dot, int(ev.Now.Minutes()), int(ev.Now.Seconds())%60, dim("·"), ev.Frames)
}

// filmBar draws the strip: filled frames toward the declared total,
// clamped full when waits run the clock past the promise.
func filmBar(now, total time.Duration, color bool) string {
	filled := int(int64(barCells) * int64(now) / int64(total))
	if filled > barCells {
		filled = barCells
	}
	if filled < 0 {
		filled = 0
	}
	full := strings.Repeat("▪", filled)
	rest := strings.Repeat("·", barCells-filled)
	if !color {
		return "▕" + full + rest + "▏"
	}
	return sgrDim + "▕" + sgrReset + full + sgrDim + rest + "▏" + sgrReset
}
