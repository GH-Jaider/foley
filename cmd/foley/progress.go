package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/GH-Jaider/foley/tape"
)

// progressRenderer draws the recording's pulse as ONE stderr line
// rewritten in place — only when stderr is a terminal. Non-TTY runs
// (CI, pipes, tests) stay exactly as silent as before. Presentation is
// deliberately plain for now; the styling pass owns the look.
type progressRenderer struct {
	w    io.Writer
	tty  bool
	last time.Time // last draw, for throttling
	line string    // current line, redrawn after a warning interleaves
}

// newProgressRenderer detects whether w is a terminal.
func newProgressRenderer(w io.Writer) *progressRenderer {
	f, ok := w.(*os.File)
	return &progressRenderer{w: w, tty: ok && term.IsTerminal(int(f.Fd()))}
}

// pulse is the RunOptions.Progress callback. Throttled: phase changes
// always draw; recording ticks at most ~12/s.
func (p *progressRenderer) pulse(ev tape.ProgressEvent) {
	if !p.tty {
		return
	}
	line := renderProgress(ev)
	if line == p.line {
		return
	}
	if ev.Phase == tape.ProgressRecording && time.Since(p.last) < 80*time.Millisecond {
		return
	}
	p.line = line
	p.last = time.Now()
	p.draw()
}

func (p *progressRenderer) draw() {
	_, _ = fmt.Fprintf(p.w, "\r\x1b[2K%s", p.line)
}

// done clears the line so the final stdout/stderr output prints clean.
func (p *progressRenderer) done() {
	if !p.tty || p.line == "" {
		return
	}
	p.line = ""
	_, _ = fmt.Fprint(p.w, "\r\x1b[2K")
}

// warnWriter returns the writer warnings should stream through: it
// clears the progress line, lets the warning print whole, and redraws —
// so a mid-recording warning never lands glued to the pulse.
func (p *progressRenderer) warnWriter() io.Writer {
	if !p.tty {
		return p.w
	}
	return progressWarnWriter{p}
}

type progressWarnWriter struct{ p *progressRenderer }

func (w progressWarnWriter) Write(b []byte) (int, error) {
	_, _ = fmt.Fprint(w.p.w, "\r\x1b[2K")
	n, err := w.p.w.Write(b)
	if w.p.line != "" {
		w.p.draw()
	}
	return n, err
}

// renderProgress formats one pulse. Kept as a pure function so the
// styling pass (and its tests) can evolve it without touching the
// terminal mechanics.
func renderProgress(ev tape.ProgressEvent) string {
	switch ev.Phase {
	case tape.ProgressDeveloping:
		return fmt.Sprintf("foley: developing %s (%d frames)", ev.Output, ev.Frames)
	case tape.ProgressRecording:
	}
	if ev.Total > 0 {
		return fmt.Sprintf("foley: rec %.1fs/%.1fs · frame %d", ev.Now.Seconds(), ev.Total.Seconds(), ev.Frames)
	}
	return fmt.Sprintf("foley: rec %d:%02d · frame %d", int(ev.Now.Minutes()), int(ev.Now.Seconds())%60, ev.Frames)
}
