package ptyx_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/ptyx"
)

func collect(t *testing.T, p *ptyx.Proc, timeout time.Duration) []ptyx.Chunk {
	t.Helper()
	var out []ptyx.Chunk
	deadline := time.After(timeout)
	for {
		select {
		case c, ok := <-p.Chunks():
			if !ok {
				return out
			}
			out = append(out, c)
		case <-deadline:
			t.Fatalf("timeout waiting for chunks (got %d)", len(out))
		}
	}
}

func joined(chunks []ptyx.Chunk) string {
	var b bytes.Buffer
	for _, c := range chunks {
		b.Write(c.Data)
	}
	return b.String()
}

func TestEchoThroughPty(t *testing.T) {
	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/echo", "hola foley"},
		Size:    ptyx.Winsize{Cols: 80, Rows: 24, XPix: 640, YPix: 384},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	chunks := collect(t, p, 5*time.Second)
	got := joined(chunks)
	// The pty applies ONLCR: \n comes out as \r\n.
	if !strings.Contains(got, "hola foley\r\n") {
		t.Fatalf("output = %q", got)
	}
	if len(chunks) == 0 || chunks[0].Time.IsZero() {
		t.Fatal("chunks missing timestamps")
	}
	if err := p.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestTimestampsAdvanceAcrossBursts(t *testing.T) {
	// The second burst is gated on OUR input, so two separate chunks are
	// guaranteed by construction — no scheduler luck involved (slow CI
	// runners deschedule readers long enough to merge sleep-based bursts).
	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/sh", "-c", "stty -echo; printf a; read x; printf b"},
		Size:    ptyx.Winsize{Cols: 80, Rows: 24},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	first := awaitOutput(t, p, "a", 5*time.Second)
	pause := 120 * time.Millisecond
	time.Sleep(pause)
	if _, err := p.Write([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	second := awaitOutput(t, p, "b", 5*time.Second)
	if gap := second.Sub(first); gap < pause {
		t.Fatalf("timestamps do not reflect the pause: gap=%v", gap)
	}
}

// awaitOutput consumes chunks until the wanted text shows up, returning
// the timestamp of the chunk that completed it.
func awaitOutput(t *testing.T, p *ptyx.Proc, want string, timeout time.Duration) time.Time {
	t.Helper()
	var acc bytes.Buffer
	deadline := time.After(timeout)
	for {
		select {
		case c, ok := <-p.Chunks():
			if !ok {
				t.Fatalf("pty closed waiting for %q; got %q", want, acc.String())
			}
			acc.Write(c.Data)
			if strings.Contains(acc.String(), want) {
				return c.Time
			}
		case <-deadline:
			t.Fatalf("timeout waiting for %q; got %q", want, acc.String())
		}
	}
}

func TestWriteReachesChild(t *testing.T) {
	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/sh", "-c", `stty -echo; printf R; read x; printf "got:%s" "$x"`},
		Size:    ptyx.Winsize{Cols: 80, Rows: 24},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	// Readiness is signaled by the child itself — no sleep guessing.
	awaitOutput(t, p, "R", 5*time.Second)
	if _, err := p.Write([]byte("ping\r")); err != nil {
		t.Fatal(err)
	}
	got := joined(collect(t, p, 5*time.Second))
	if !strings.Contains(got, "got:ping") {
		t.Fatalf("output = %q", got)
	}
}

func TestSizeAndTermVisibleToChild(t *testing.T) {
	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/sh", "-c", `printf "%s %s $TERM" $(stty size)`},
		Size:    ptyx.Winsize{Cols: 120, Rows: 30, XPix: 960, YPix: 600},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	got := joined(collect(t, p, 5*time.Second))
	if !strings.Contains(got, "30 120") {
		t.Fatalf("stty size = %q, want 30 120", got)
	}
	if !strings.Contains(got, "xterm-256color") {
		t.Fatalf("TERM = %q, want xterm-256color", got)
	}
}

func TestCloseIsIdempotentAndUnblocks(t *testing.T) {
	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/sh", "-c", "sleep 60"},
		Size:    ptyx.Winsize{Cols: 80, Rows: 24},
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		for range p.Chunks() { //nolint:revive // drain until closed
		}
		close(done)
	}()
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("read loop did not unblock after Close")
	}
	if _, err := p.Write([]byte("x")); err == nil {
		t.Fatal("Write after Close must fail")
	}
}
