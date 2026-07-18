package preview

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// cellPixels reads the terminal's cell size in pixels from the kernel
// winsize (kitty-family terminals fill Xpixel/Ypixel). Terminals that
// report zero get the classic 8x16 cell — only the ASPECT matters for
// sizing, and 1:2 is the terminal norm.
func cellPixels(tty *os.File) (w, h int) {
	ws, err := unix.IoctlGetWinsize(int(tty.Fd()), unix.TIOCGWINSZ)
	if err == nil && ws.Xpixel > 0 && ws.Ypixel > 0 && ws.Col > 0 && ws.Row > 0 {
		return int(ws.Xpixel) / int(ws.Col), int(ws.Ypixel) / int(ws.Row)
	}
	return 8, 16
}

// pollIn waits until the fd has readable data or the budget runs out.
// Raw poll(2), not runtime deadlines: /dev/tty rejects them on darwin.
func pollIn(fd int, budget time.Duration) (bool, error) {
	ms := int(budget.Milliseconds())
	if ms < 1 {
		ms = 1
	}
	for {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}} //nolint:gosec // tty fds are tiny
		n, err := unix.Poll(fds, ms)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return false, err
		}
		return n > 0, nil
	}
}
