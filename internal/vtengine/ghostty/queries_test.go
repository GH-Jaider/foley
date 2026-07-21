//go:build ghosttyvt

package ghostty

// The query filter's edges: sequences split across Write boundaries,
// response ordering against lib-answered queries, and the streams that
// must NOT trigger an answer. The happy paths live in enginetest
// (geometry_query_responses); these are the scanner's own corners.

import (
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/GH-Jaider/foley/internal/vtengine"
)

func newQueryEngine(t *testing.T) (*Engine, *bytes.Buffer) {
	t.Helper()
	var resp bytes.Buffer
	e, err := New(vtengine.Options{
		Geometry:  vtengine.Geometry{Cols: 20, Rows: 4, CellW: 8, CellH: 16},
		Responses: &resp,
		Colors: &vtengine.Colors{
			FG: vtengine.RGB{R: 0xcd, G: 0xd6, B: 0xf4},
			BG: vtengine.RGB{R: 0x1e, G: 0x1e, B: 0x2e}, // dark seed
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e, &resp
}

func write(t *testing.T, e *Engine, s string) {
	t.Helper()
	if _, err := e.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
}

func TestQuerySplitAcrossWrites(t *testing.T) {
	e, resp := newQueryEngine(t)
	// Byte-by-byte delivery: pty chunking must never lose a query.
	for _, b := range []byte("\x1b[14t") {
		write(t, e, string(b))
	}
	if got := resp.String(); got != "\x1b[4;64;160t" {
		t.Fatalf("split 14t = %q, want %q", got, "\x1b[4;64;160t")
	}
	resp.Reset()
	for _, chunk := range []string{"\x1bP+", "q54", "4e\x1b", "\\"} {
		write(t, e, chunk)
	}
	if got := resp.String(); !strings.HasPrefix(got, "\x1bP1+r544e=") {
		t.Fatalf("split XTGETTCAP = %q, want DCS 1+r 544e=…", got)
	}
}

func TestQueryResponseOrdering(t *testing.T) {
	e, resp := newQueryEngine(t)
	// One chunk, two queries: ours first, the lib's second. Answers
	// must come back in stream order — the lib is fed segment by
	// segment around each matched query.
	write(t, e, "\x1b[14t\x1b[6n")
	want := "\x1b[4;64;160t\x1b[1;1R"
	if got := resp.String(); got != want {
		t.Fatalf("ordering = %q, want %q", got, want)
	}
}

func TestQueryNonMatchesStaySilent(t *testing.T) {
	e, resp := newQueryEngine(t)
	for _, s := range []string{
		"plain 14t text",       // no escape at all
		"\x1b[14;2t",           // parameterized variant is not the query
		"\x1b[38;5;141m",       // SGR that merely contains the digits
		"\x1b[?14t",            // private-marked: not ours
		"\x1bPqABC\x1b\\",      // sixel-shaped DCS is skipped whole
		"\x1bP+wnope\x1b\\",    // DCS +w is not XTGETTCAP
		"\x1b]11;not-a-q\x07",  // OSC payload, BEL-terminated
		"\x1b_Gi=9,a=q;AA\x1b", // APC left dangling
	} {
		resp.Reset()
		write(t, e, s)
		// The lib may answer its own queries here (none of these are);
		// ours must not fire.
		if got := resp.String(); strings.Contains(got, "t") &&
			strings.HasPrefix(got, "\x1b[4;") {
			t.Fatalf("%q drew a geometry answer: %q", s, got)
		}
		if got := resp.String(); strings.Contains(got, "+r") {
			t.Fatalf("%q drew an XTGETTCAP answer: %q", s, got)
		}
	}
	// After all that noise the scanner still answers cleanly.
	resp.Reset()
	write(t, e, "\x1b[18t")
	if got := resp.String(); got != "\x1b[8;4;20t" {
		t.Fatalf("18t after noise = %q, want %q", got, "\x1b[8;4;20t")
	}
}

func TestQueryXTGetTCapMixedBatch(t *testing.T) {
	e, resp := newQueryEngine(t)
	// TN plus a served value (Ms) plus an unknown cap in one request:
	// the reply carries the known pairs and stays positive.
	write(t, e, "\x1bP+q544e;4d73;6e6f7065\x1b\\")
	want := "\x1bP1+r544e=787465726d2d67686f73747479" +
		";4d73=" + hex.EncodeToString([]byte("\x1b]52;%p1%s;%p2%s\a")) + "\x1b\\"
	if got := resp.String(); got != want {
		t.Fatalf("mixed batch = %q, want %q", got, want)
	}
	resp.Reset()
	write(t, e, "\x1bP+qzz;;\x1b\\") // undecodable junk: immediate negative
	if got := resp.String(); got != "\x1bP0+r\x1b\\" {
		t.Fatalf("junk batch = %q, want %q", got, "\x1bP0+r\x1b\\")
	}
	// Boolean flags (Tc) answer with the bare hex name — how neovim
	// detects truecolor and styled underlines.
	resp.Reset()
	write(t, e, "\x1bP+q5463\x1b\\")
	if got := resp.String(); got != "\x1bP1+r5463\x1b\\" {
		t.Fatalf("Tc flag = %q, want bare-name positive", got)
	}
}

// TestQueryTCapMatchesPinnedTerminfo pins every served XTGETTCAP value
// to the terminfo SOURCE the engine pin generates (ADR-021): if the pin
// moves and a capability changes, this fails before a recorded demo
// tells a different story than TERM does.
func TestQueryTCapMatchesPinnedTerminfo(t *testing.T) {
	src, err := os.ReadFile("../../terminfo/xterm-ghostty.terminfo")
	if err != nil {
		t.Fatalf("pinned terminfo source: %v", err)
	}
	// Terminfo source escaping: \x1b is \E, BEL is \007.
	toSource := func(s string) string {
		s = strings.ReplaceAll(s, "\x1b", `\E`)
		return strings.ReplaceAll(s, "\a", `\007`)
	}
	for name, want := range map[string]string{
		"Smulx":   "\x1b[4:%p1%dm",
		"Ms":      "\x1b]52;%p1%s;%p2%s\a",
		"setrgbf": "\x1b[38:2:%p1%d:%p2%d:%p3%dm",
		"setrgbb": "\x1b[48:2:%p1%d:%p2%d:%p3%dm",
	} {
		value, isBool, known := tcapValue(name)
		if !known || isBool || value != want {
			t.Fatalf("tcapValue(%s) = (%q,%v,%v), want served string %q", name, value, isBool, known, want)
		}
		if frag := name + "=" + toSource(want); !strings.Contains(string(src), frag) {
			t.Fatalf("pinned terminfo lost %q — the served table drifted from the pin", frag)
		}
	}
	for _, flag := range []string{"Tc", "Su"} {
		if _, isBool, known := tcapValue(flag); !known || !isBool {
			t.Fatalf("tcapValue(%s): want a served boolean flag", flag)
		}
		if !strings.Contains(string(src), flag+",") {
			t.Fatalf("pinned terminfo lost flag %q", flag)
		}
	}
	if !strings.Contains(string(src), "colors#256") {
		t.Fatal("pinned terminfo lost colors#256")
	}
}

func TestQueryWinopsFamily(t *testing.T) {
	e, resp := newQueryEngine(t)
	for _, c := range []struct{ query, want string }{
		{"\x1b[11t", "\x1b[1t"},
		{"\x1b[13t", "\x1b[3;0;0t"},
		{"\x1b[15t", "\x1b[5;64;160t"},
		{"\x1b[19t", "\x1b[9;4;20t"},
		{"\x1b[21t", "\x1b]l\x1b\\"}, // title report: the lib's own answer
	} {
		resp.Reset()
		write(t, e, c.query)
		if got := resp.String(); got != c.want {
			t.Fatalf("%q = %q, want %q", c.query, got, c.want)
		}
	}
}

func TestQueryDECRQSSNegative(t *testing.T) {
	e, resp := newQueryEngine(t)
	// vim's startup probes: SGR and cursor-style status strings. The
	// immediate xterm-convention "invalid" ends the wait.
	for _, q := range []string{"\x1bP$qm\x1b\\", "\x1bP$q q\x1b\\", "\x1bP$qr\x1b\\"} {
		resp.Reset()
		write(t, e, q)
		if got := resp.String(); got != "\x1bP0$r\x1b\\" {
			t.Fatalf("DECRQSS %q = %q, want %q", q, got, "\x1bP0$r\x1b\\")
		}
	}
}

func TestQueryColorScheme(t *testing.T) {
	e, resp := newQueryEngine(t) // dark theme seed (catppuccin-ish BG)
	write(t, e, "\x1b[?996n")
	if got := resp.String(); got != "\x1b[?997;1n" {
		t.Fatalf("dark scheme = %q, want %q", got, "\x1b[?997;1n")
	}
	// The report follows the LIVE background: an OSC 11 override to a
	// light color flips the answer.
	resp.Reset()
	write(t, e, "\x1b]11;rgb:fa/fa/fa\x07\x1b[?996n")
	if got := resp.String(); got != "\x1b[?997;2n" {
		t.Fatalf("after OSC 11 light = %q, want %q", got, "\x1b[?997;2n")
	}
}

func TestQueryStreamStillReachesGrid(t *testing.T) {
	e, _ := newQueryEngine(t)
	// The filter must never eat bytes: text around a query lands on
	// the grid intact.
	write(t, e, "ab\x1b[16tcd")
	var f vtengine.Frame
	if err := e.Snapshot(&f); err != nil {
		t.Fatal(err)
	}
	got := string(f.Cells[0].Runes) + string(f.Cells[1].Runes) +
		string(f.Cells[2].Runes) + string(f.Cells[3].Runes)
	if got != "abcd" {
		t.Fatalf("grid row = %q, want %q", got, "abcd")
	}
}
