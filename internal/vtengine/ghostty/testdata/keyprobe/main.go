// keyprobe is a test fixture: it puts its tty in raw mode (like any real
// TUI), enables the kitty keyboard protocol (push flags=1), announces
// READY, then reports the first burst of bytes it reads from the tty as a
// hex line and exits. It lets the input e2e test prove that foley's
// engine tracks the application's keyboard mode and encodes keys
// accordingly.
package main

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

func main() {
	if _, err := term.MakeRaw(int(os.Stdin.Fd())); err != nil {
		fmt.Fprintf(os.Stderr, "keyprobe: MakeRaw: %v\n", err)
		os.Exit(1)
	}
	_, _ = os.Stdout.WriteString("\x1b[>1u") // push kitty keyboard flags
	_, _ = os.Stdout.WriteString("READY\r\n")
	buf := make([]byte, 256)
	n, err := os.Stdin.Read(buf)
	if n > 0 {
		fmt.Printf("HEX:%x\r\n", buf[:n])
	}
	if err != nil {
		os.Exit(1)
	}
}
