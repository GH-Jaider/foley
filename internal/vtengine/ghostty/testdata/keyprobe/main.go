// keyprobe is a test fixture: it puts its tty in raw mode (like any real
// TUI), enables the kitty keyboard protocol (push flags=1), announces
// READY, then reads until it has a complete CSI-u sequence (terminated by
// 'u') — tolerating kernel-split reads — reports it as a hex line and
// exits. It lets the input e2e test prove that foley's engine tracks the
// application's keyboard mode and encodes keys accordingly.
package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

func main() {
	if _, err := term.MakeRaw(int(os.Stdin.Fd())); err != nil {
		fmt.Fprintf(os.Stderr, "keyprobe: MakeRaw: %v\n", err)
		os.Exit(1)
	}
	_, _ = os.Stdout.WriteString("\x1b[>1u") // push kitty keyboard flags
	_, _ = os.Stdout.WriteString("READY\r\n")

	var acc []byte
	buf := make([]byte, 256)
	for reads := 0; reads < 8; reads++ {
		if len(acc) > 0 {
			// Partial sequence in hand: bound the wait for the rest. The
			// bytes are already in flight through a local pty, so this is
			// pure headroom — sized for heavily loaded CI runners.
			_ = os.Stdin.SetReadDeadline(time.Now().Add(2 * time.Second))
		}
		n, err := os.Stdin.Read(buf)
		acc = append(acc, buf[:n]...)
		if bytes.ContainsRune(acc, 'u') || err != nil {
			break
		}
	}
	if len(acc) > 0 {
		fmt.Printf("HEX:%x\r\n", acc)
	}
}
