// Package preview replays a closed recording's frames in the user's
// OWN terminal via the kitty graphics protocol (ADR-020) — the tool
// that exists because VHS could not speak kitty graphics, speaking it
// back. Support is decided by asking the terminal itself, never by
// sniffing environment variables.
package preview

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/gif"
	"image/png"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
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
	fd := int(tty.Fd())
	st, err := term.MakeRaw(fd)
	if err != nil {
		return false
	}
	defer func() { _ = term.Restore(fd, st) }()
	// Belt and braces against darwin's /dev/tty quirks: the fd goes
	// nonblocking for the handshake, so even a lying readiness check
	// yields EAGAIN instead of a read(2) blocked forever.
	if err := unix.SetNonblock(fd, true); err != nil {
		return false
	}
	defer func() { _ = unix.SetNonblock(fd, false) }()
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
		ready, err := waitReadable(fd, remain)
		if err != nil {
			break
		}
		if !ready {
			continue
		}
		n, rerr := unix.Read(fd, tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if da1Answered(buf) {
			break
		}
		if rerr != nil && !errors.Is(rerr, unix.EAGAIN) {
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

// ShowGIF plays an animated GIF at the cursor via the kitty graphics
// ANIMATION protocol: root frame transmitted with a=T, the rest
// appended as a=f with their gaps, then set running in a loop. A
// terminal that speaks graphics but not animation simply shows the
// root frame — degradation is built into the protocol (unknown APC
// payloads are ignored). Frames are re-encoded as PNG (the only
// payload format every implementation takes).
func ShowGIF(tty *os.File, g *gif.GIF, cols int) error {
	chunks, err := animationChunks(g, cols)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	for _, c := range chunks {
		out.Write(c)
	}
	// C=1 keeps the cursor still, and WE move below the image —
	// implementations disagree on how far a=T advances the cursor
	// (found live: libghostty-vt left it at the top and the welcome
	// text printed OVER the logo). Explicit rows are identical
	// everywhere.
	b := g.Image[0].Bounds()
	cw, ch := cellPixels(tty)
	rows := (cols*b.Dy()*cw + b.Dx()*ch - 1) / (b.Dx() * ch)
	out.WriteString("\r" + strings.Repeat("\n", rows+1))
	if _, err := tty.Write(out.Bytes()); err != nil {
		return fmt.Errorf("preview: %w", err)
	}
	return nil
}

// animationChunks builds the escape stream for a looping GIF — pure,
// so the tests pin the protocol without a terminal. The shape mirrors
// kitten icat byte for byte, because three live findings say nothing
// else survives:
//
//   - every payload goes in ONE escape: the 4096-byte chunking the
//     spec recommends LOSES the z/c keys of a=f frames in kitty's
//     accumulation — frames land with gap 0, the animation scanner
//     skips them (`while (!current_frame->gap)`), and the image plays
//     once through loading and freezes on the last frame;
//   - frames are RGBA+zlib with explicit dims (no f key: 32 is the
//     default), each composed onto the PREVIOUS frame via c=N —
//     without c, consecutive frames PILE INTO SLOT 2 replacing each
//     other (probed loud: every a=f answered r=2);
//   - the controls ride a=a: r=1,z sets the root's gap, v=1 loops
//     forever (kitty stores max_loops = v-1; 0 = infinite), s=2 while
//     loading, s=3 to run — WITH q=2: terminals that answer the
//     graphics query but not the animation actions (Warp) would
//     otherwise error loudly into the shell (icat omits q there;
//     proven live that kitty animates either way).
func animationChunks(g *gif.GIF, cols int) ([][]byte, error) {
	if len(g.Image) == 0 {
		return nil, fmt.Errorf("preview: gif has no frames")
	}
	rootGap := 10 * g.Delay[0] // centiseconds → ms
	single := func(ctrl string, payload []byte) []byte {
		return []byte("\x1b_G" + ctrl + ";" + base64.StdEncoding.EncodeToString(payload) + "\x1b\\")
	}
	var out [][]byte
	for i, frame := range g.Image {
		b := frame.Bounds()
		raw := make([]byte, 0, b.Dx()*b.Dy()*4)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, gg, bl, a := frame.At(x, y).RGBA()
				raw = append(raw, byte(r>>8), byte(gg>>8), byte(bl>>8), byte(a>>8)) //nolint:gosec // 16-bit color channels shifted into bytes
			}
		}
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		if _, err := zw.Write(raw); err != nil {
			return nil, fmt.Errorf("preview: frame %d: %w", i, err)
		}
		if err := zw.Close(); err != nil {
			return nil, fmt.Errorf("preview: frame %d: %w", i, err)
		}
		if i == 0 {
			out = append(out,
				single(fmt.Sprintf("a=T,q=2,o=z,s=%d,v=%d,i=1,C=1,c=%d", b.Dx(), b.Dy(), cols), z.Bytes()),
				[]byte(fmt.Sprintf("\x1b_Ga=a,q=2,v=1,r=1,i=1,z=%d\x1b\\", rootGap)),
			)
			continue
		}
		out = append(out, single(
			fmt.Sprintf("a=f,q=2,o=z,s=%d,v=%d,c=%d,i=1,z=%d", b.Dx(), b.Dy(), i, 10*g.Delay[i]), z.Bytes()))
		if i == 1 {
			out = append(out, []byte(fmt.Sprintf("\x1b_Ga=a,q=2,s=2,v=1,r=1,i=1,z=%d\x1b\\", rootGap)))
		}
	}
	out = append(out, []byte(fmt.Sprintf("\x1b_Ga=a,q=2,s=3,v=1,r=1,i=1,z=%d\x1b\\", rootGap)))
	return out, nil
}
