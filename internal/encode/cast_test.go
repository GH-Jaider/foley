package encode

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestWriteCast pins the emitter byte-for-byte: exact integer-micro
// timestamps, same-instant bursts merged (chunk boundaries are noise),
// and a multibyte rune torn across bursts re-joined instead of
// corrupted.
func TestWriteCast(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cast")
	events := []CastEvent{
		{At: 0, Data: []byte("hola")},
		{At: 0, Data: []byte(" mundo")},                      // same instant: merges
		{At: 1500 * time.Millisecond, Data: []byte("a\xc3")}, // "añ" torn mid-rune...
		{At: 2 * time.Second, Data: []byte("\xb1b")},         // ...re-joined here
	}
	if err := WriteCast(path, 80, 24, events); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path) //nolint:gosec // TempDir path
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version": 2, "width": 80, "height": 24}` + "\n" +
		`[0.000000, "o", "hola mundo"]` + "\n" +
		`[1.500000, "o", "a"]` + "\n" +
		`[2.000000, "o", "ñb"]` + "\n"
	if string(raw) != want {
		t.Fatalf("cast file:\n%s\nwant:\n%s", raw, want)
	}
}

// TestIncompleteTail pins the carry detector across rune widths.
func TestIncompleteTail(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"abc", 0},
		{"a\xc3", 1},         // start of a 2-byte rune
		{"a\xe2", 1},         // start of a 3-byte rune
		{"a\xe2\x82", 2},     // two of three
		{"a\xf0\x9f\x98", 3}, // three of four (emoji arriving)
		{"añ", 0},            // complete
		{"a€", 0},            // complete 3-byte
		{"\x80", 0},          // lone continuation: not carriable
		{"", 0},
	}
	for _, c := range cases {
		if got := incompleteTail([]byte(c.in)); got != c.want {
			t.Fatalf("incompleteTail(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
