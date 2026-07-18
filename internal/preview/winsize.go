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

// waitReadable waits until the fd has readable data or the budget runs
// out. select(2), NOT poll and NOT runtime deadlines: /dev/tty rejects
// Go's deadlines on darwin, and darwin's poll(2) on the /dev/tty ALIAS
// reports false readiness (found live: a silent terminal left the
// handshake blocked in read(2) forever). select is the one primitive
// that answers honestly for that device.
func waitReadable(fd int, budget time.Duration) (bool, error) {
	if budget < time.Millisecond {
		budget = time.Millisecond
	}
	for {
		var set unix.FdSet
		set.Set(fd)
		tv := unix.NsecToTimeval(budget.Nanoseconds())
		n, err := unix.Select(fd+1, &set, nil, nil, &tv)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return false, err
		}
		return n > 0, nil
	}
}
