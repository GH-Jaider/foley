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
			t.Fatalf("timeout esperando chunks (got %d)", len(out))
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
	// El pty aplica ONLCR: \n sale como \r\n.
	if !strings.Contains(got, "hola foley\r\n") {
		t.Fatalf("output = %q", got)
	}
	if len(chunks) == 0 || chunks[0].Time.IsZero() {
		t.Fatal("chunks sin timestamp")
	}
	if err := p.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestTimestampsAdvanceAcrossBursts(t *testing.T) {
	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/sh", "-c", "printf a; sleep 0.15; printf b"},
		Size:    ptyx.Winsize{Cols: 80, Rows: 24},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	chunks := collect(t, p, 5*time.Second)
	if joined(chunks) != "ab" {
		t.Fatalf("output = %q", joined(chunks))
	}
	if len(chunks) < 2 {
		t.Fatalf("esperaba ≥2 chunks (ráfagas separadas), got %d", len(chunks))
	}
	gap := chunks[len(chunks)-1].Time.Sub(chunks[0].Time)
	if gap < 100*time.Millisecond {
		t.Fatalf("timestamps no reflejan la pausa: gap=%v", gap)
	}
}

func TestWriteReachesChild(t *testing.T) {
	p, err := ptyx.Start(ptyx.Options{
		Command: []string{"/bin/sh", "-c", `stty -echo; read x; printf "got:%s" "$x"`},
		Size:    ptyx.Winsize{Cols: 80, Rows: 24},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	time.Sleep(100 * time.Millisecond) // deja que stty/read arranquen
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
		for range p.Chunks() { //nolint:revive // drenar hasta cierre
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
		t.Fatal("el read-loop no se destrabó tras Close")
	}
	if _, err := p.Write([]byte("x")); err == nil {
		t.Fatal("Write tras Close debe fallar")
	}
}
