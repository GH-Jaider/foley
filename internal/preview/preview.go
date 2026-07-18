// Package preview replays a closed recording's frames in the user's
// OWN terminal via the kitty graphics protocol (ADR-020) — the tool
// that exists because VHS could not speak kitty graphics, speaking it
// back. Support is decided by asking the terminal itself, never by
// sniffing environment variables.
package preview

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/GH-Jaider/foley/internal/encode"
)

// probeID marks our handshake query so the reply cannot be confused
// with any other pending terminal traffic.
const probeID = "31337"

// probe is a 1x1 RGB query (a=q: validate, do not display). Terminals
// that speak the protocol answer `_Gi=<id>;OK`; the rest ignore APC
// entirely. The DA1 request after it is the sentinel: every terminal
// answers DA1, so its reply closes the read without a fixed sleep.
const probe = "\x1b_Gi=" + probeID + ",s=1,v=1,a=q,f=24;AAAA\x1b\\" + "\x1b[c"

// Supported reports whether the tty speaks kitty graphics — by
// handshake, in raw mode, bounded by timeout. Any failure along the
// way (no raw mode, no reply) is a plain "no": the caller falls back,
// it never breaks. The timeout runs on unix.Poll over the raw fd —
// NOT SetReadDeadline: /dev/tty is not kqueue-pollable on darwin, so
// the runtime refuses deadlines there (ErrNoDeadline) and the probe
// found Supported silently always-false on macOS.
func Supported(tty *os.File, timeout time.Duration) bool {
	st, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return false
	}
	defer func() { _ = term.Restore(int(tty.Fd()), st) }()
	if _, err := tty.WriteString(probe); err != nil {
		return false
	}
	deadline := time.Now().Add(timeout)
	var buf []byte
	tmp := make([]byte, 256)
	for {
		remain := time.Until(deadline)
		if remain <= 0 {
			break
		}
		ready, err := pollIn(int(tty.Fd()), remain)
		if err != nil || !ready {
			break
		}
		n, err := tty.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if da1Answered(buf) || err != nil {
			break
		}
	}
	return kittyAnswered(buf)
}

// da1Answered spots a DA1 reply: CSI ? ... c.
func da1Answered(buf []byte) bool {
	i := bytes.Index(buf, []byte("\x1b[?"))
	return i >= 0 && bytes.IndexByte(buf[i:], 'c') > 0
}

// kittyAnswered spots our probe's acknowledgment.
func kittyAnswered(buf []byte) bool {
	return bytes.Contains(buf, []byte("_Gi="+probeID+";OK"))
}

// Play replays a closed recording (its frames directory) on the tty:
// every frame at its exact recorded duration, drawn at a stable
// position, sized to fit the terminal. The image is transmitted under
// one id and replaced per frame — the retransmit-same-id idiom every
// kitty-graphics terminal animates without flicker. Ctrl-C works: the
// terminal stays in cooked mode during playback (signals live), and
// ctx cancellation strikes the set early.
func Play(ctx context.Context, tty *os.File, framesDir string) error {
	frames, err := encode.Manifest(framesDir)
	if err != nil {
		return err
	}
	cols, rows, err := term.GetSize(int(tty.Fd()))
	if err != nil {
		return fmt.Errorf("preview: terminal size: %w", err)
	}
	cellW, cellH := cellPixels(tty)
	first, err := os.Open(frames[0].Path) //nolint:gosec // manifest paths inside the caller-owned frames dir
	if err != nil {
		return fmt.Errorf("preview: %w", err)
	}
	cfg, err := png.DecodeConfig(first)
	_ = first.Close()
	if err != nil {
		return fmt.Errorf("preview: %s: %w", frames[0].Path, err)
	}
	// One spare row keeps the prompt line below the image on screen.
	c := targetCols(cols, rows-1, cellW, cellH, cfg.Width, cfg.Height)

	out := bytes.Buffer{}
	write := func() error {
		_, werr := tty.Write(out.Bytes())
		out.Reset()
		return werr
	}
	defer func() {
		// Strike the set even on early exit: free the image and give
		// the shell a clean line.
		_, _ = tty.WriteString("\x1b_Ga=d,d=I,i=1,q=2\x1b\\\r\n")
	}()
	out.WriteString("\x1b7") // remember where the take plays
	for _, f := range frames {
		data, err := os.ReadFile(f.Path) //nolint:gosec // manifest paths inside the caller-owned frames dir
		if err != nil {
			return fmt.Errorf("preview: %w", err)
		}
		out.WriteString("\x1b8")
		ctrl := fmt.Sprintf("a=T,f=100,i=1,q=2,c=%d", c)
		for _, chunk := range kittyChunks(ctrl, data) {
			out.Write(chunk)
		}
		if err := write(); err != nil {
			return fmt.Errorf("preview: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(f.Dur):
		}
	}
	return nil
}

// kittyChunks frames a payload for the graphics protocol: base64,
// split at the spec's 4096 encoded bytes per escape; the first chunk
// carries the control keys, every chunk but the last says m=1.
func kittyChunks(ctrl string, payload []byte) [][]byte {
	b64 := base64.StdEncoding.EncodeToString(payload)
	var out [][]byte
	for first := true; first || len(b64) > 0; first = false {
		n := len(b64)
		if n > 4096 {
			n = 4096
		}
		part := b64[:n]
		b64 = b64[n:]
		last := len(b64) == 0
		keys := ""
		switch {
		case first && last:
			keys = ctrl
		case first:
			keys = ctrl + ",m=1"
		case last:
			keys = "m=0"
		default:
			keys = "m=1"
		}
		out = append(out, []byte("\x1b_G"+keys+";"+part+"\x1b\\"))
	}
	return out
}

// targetCols picks the widest column count whose implied height still
// fits the available rows — full width when it fits, scaled down when
// the take is tall. Never below 1.
func targetCols(termCols, availRows, cellW, cellH, imgW, imgH int) int {
	if termCols < 1 {
		termCols = 1
	}
	if availRows < 1 || cellW < 1 || cellH < 1 || imgW < 1 || imgH < 1 {
		return termCols
	}
	byHeight := imgW * availRows * cellH / (imgH * cellW)
	c := termCols
	if byHeight < c {
		c = byHeight
	}
	if c < 1 {
		c = 1
	}
	return c
}
