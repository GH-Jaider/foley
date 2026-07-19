package ptyx

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

// ErrClosed is returned by operations on a closed Proc.
var ErrClosed = errors.New("ptyx: proc is closed")

// Winsize is the pty size in cells and pixels. Pixel dimensions are NOT
// optional for foley's use case: kitty-graphics-aware TUIs read them
// (TIOCGWINSZ) to compute the cell size, and zeros make them misbehave or
// disable graphics. The driver always passes Cols*CellW × Rows*CellH.
type Winsize struct {
	Cols, Rows int
	XPix, YPix int
}

// Options configures a demo process.
type Options struct {
	// Command is the argv to run; Command[0] resolves via PATH.
	Command []string

	// Dir is the working directory (empty = inherit).
	Dir string

	// Env is the exact environment for the child. nil inherits the parent
	// environment. The driver passes a sanitized environment carrying
	// foley's declared identity (ADR-021: TERM=xterm-ghostty plus its
	// pinned terminfo); this plumbing-level fallback only guards a bare
	// ptyx caller, with the one entry every host resolves.
	Env []string

	Size Winsize
}

// Chunk is one pty read: an owned copy of the bytes plus the arrival
// timestamp the driver uses for the realtime timeline and settle.
type Chunk struct {
	Data []byte
	Time time.Time
}

// Proc is a command running on a real pty.
type Proc struct {
	cmd    *exec.Cmd
	master *os.File
	chunks chan Chunk
	done   chan struct{} // closed by Close; unblocks a stalled chunk send

	closeOnce sync.Once
	waitOnce  sync.Once
	waitErr   error
	closed    bool
	mu        sync.Mutex
}

// Start launches the command on a fresh pty and begins the read loop.
func Start(opts Options) (*Proc, error) {
	if len(opts.Command) == 0 {
		return nil, errors.New("ptyx: empty command")
	}
	//nolint:gosec // ptyx's whole purpose is running the user's demo command.
	cmd := exec.Command(opts.Command[0], opts.Command[1:]...)
	cmd.Dir = opts.Dir
	env := opts.Env
	if env == nil {
		env = os.Environ()
	}
	if !hasEnv(env, "TERM") {
		env = append(env, "TERM=xterm-256color")
	}
	cmd.Env = env

	master, err := pty.StartWithSize(cmd, winsizeC(opts.Size))
	if err != nil {
		return nil, fmt.Errorf("ptyx: start %q: %w", opts.Command[0], err)
	}

	p := &Proc{
		cmd:    cmd,
		master: master,
		chunks: make(chan Chunk, 64),
		done:   make(chan struct{}),
	}
	go p.readLoop()
	return p, nil
}

func hasEnv(env []string, name string) bool {
	prefix := name + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}
	return false
}

func (p *Proc) readLoop() {
	// Reap the child once output ends, even if the caller never calls
	// Wait or Close (waitOnce makes this race-free with user Waits).
	defer func() { _ = p.Wait() }()
	defer close(p.chunks)
	buf := make([]byte, 32*1024)
	for {
		n, err := p.master.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			select {
			case p.chunks <- Chunk{Data: data, Time: time.Now()}:
			case <-p.done:
				// Consumer is gone and Close was called: stop instead of
				// blocking forever on a full channel.
				return
			}
		}
		if err != nil {
			// EOF, or EIO once the child side is gone: normal shutdown.
			return
		}
	}
}

// Chunks streams the application's output. The channel closes when the
// child exits (or the Proc is closed).
func (p *Proc) Chunks() <-chan Chunk { return p.chunks }

// Write sends input bytes (encoded keys, terminal query responses) to the
// application.
func (p *Proc) Write(b []byte) (int, error) {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return 0, ErrClosed
	}
	return p.master.Write(b)
}

// Resize changes the pty size; the child receives SIGWINCH.
func (p *Proc) Resize(w Winsize) error {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return ErrClosed
	}
	return pty.Setsize(p.master, winsizeC(w))
}

// winsizeC converts to pty.Winsize clamping into uint16 range.
func winsizeC(w Winsize) *pty.Winsize {
	return &pty.Winsize{
		Cols: clampU16(w.Cols), Rows: clampU16(w.Rows),
		X: clampU16(w.XPix), Y: clampU16(w.YPix),
	}
}

func clampU16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(v)
}

// Wait blocks until the child exits and returns its error (nil on clean
// exit). Safe to call multiple times.
func (p *Proc) Wait() error {
	p.waitOnce.Do(func() { p.waitErr = p.cmd.Wait() })
	return p.waitErr
}

// Close terminates the child if needed and releases the pty. Idempotent.
func (p *Proc) Close() error {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
		close(p.done)
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		_ = p.master.Close()
		_ = p.Wait() // reap; error irrelevant after a kill
	})
	return nil
}

var _ io.Writer = (*Proc)(nil)
