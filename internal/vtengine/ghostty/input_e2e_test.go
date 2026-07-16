//go:build ghosttyvt

package ghostty_test

import (
	"encoding/hex"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GH-Jaider/foley/internal/ptyx"
	"github.com/GH-Jaider/foley/internal/testassets"
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/ghostty"
	"github.com/GH-Jaider/foley/key"
)

// TestInputEndToEnd proves the full input path with a real process on a
// real pty: the fixture enables the kitty keyboard protocol, the engine
// observes it through the pty byte stream, and EncodeKey therefore
// produces CSI-u — which the fixture receives and reports back in hex.
func TestInputEndToEnd(t *testing.T) {
	const probe = "testdata/bin/keyprobe"
	_, err := os.Stat(probe)
	testassets.Require(t, err, "make fixtures")

	p, err := ptyx.Start(ptyx.Options{
		Command: []string{probe},
		Size:    ptyx.Winsize{Cols: 80, Rows: 24, XPix: 640, YPix: 384},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	e, err := ghostty.New(vtengine.Options{
		Geometry:  vtengine.Geometry{Cols: 80, Rows: 24, CellW: 8, CellH: 16},
		Responses: p, // full duplex: engine replies flow back to the app
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()

	var raw strings.Builder
	pump := func(until func() bool, what string) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for !until() {
			select {
			case c, ok := <-p.Chunks():
				if !ok {
					if !until() {
						t.Fatalf("pty closed while waiting for %s; raw output: %q", what, raw.String())
					}
					return
				}
				raw.Write(c.Data)
				if _, err := e.Write(c.Data); err != nil {
					t.Fatal(err)
				}
			case <-deadline:
				t.Fatalf("timeout waiting for %s; raw output: %q", what, raw.String())
			}
		}
	}

	// 1. The app starts and enables the kitty keyboard protocol.
	pump(func() bool { return strings.Contains(raw.String(), "READY") }, "READY")

	// 2. The engine saw the flag push: Escape must encode as CSI-u.
	esc, err := e.EncodeKey(vtengine.KeyEvent{Key: key.Named(key.NameEscape)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Write(esc); err != nil {
		t.Fatal(err)
	}

	// 3. The app reports exactly what it received, in hex.
	pump(func() bool { return strings.Contains(raw.String(), "HEX:") }, "HEX")
	m := regexp.MustCompile(`HEX:([0-9a-f]+)`).FindStringSubmatch(raw.String())
	if m == nil {
		t.Fatalf("no HEX line in %q", raw.String())
	}
	got, err := hex.DecodeString(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\x1b\[27(;\d+)?u$`).Match(got) {
		t.Fatalf("app received %q — expected CSI-u Escape (engine must track the flag push)", got)
	}
}
