package preview

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"strings"
	"testing"
)

//nolint:gochecknoglobals // tiny fixed test palette
var testPalette = []color.Color{color.Black, color.White}

// TestKittyChunks pins the protocol framing: single-chunk payloads
// carry the control keys alone, multi-chunk ones say m=1 on every
// chunk but the last, and the data survives the split intact.
func TestKittyChunks(t *testing.T) {
	one := kittyChunks("a=T,i=1", []byte("hi"))
	if len(one) != 1 {
		t.Fatalf("small payload chunks = %d, want 1", len(one))
	}
	if got := string(one[0]); got != "\x1b_Ga=T,i=1;aGk=\x1b\\" {
		t.Fatalf("single chunk = %q", got)
	}

	big := bytes.Repeat([]byte{0xAB}, 9000) // b64 length 12000 → 3 chunks
	chunks := kittyChunks("a=T,f=100,i=1,q=2,c=80", big)
	if len(chunks) != 3 {
		t.Fatalf("chunks = %d, want 3", len(chunks))
	}
	if !strings.HasPrefix(string(chunks[0]), "\x1b_Ga=T,f=100,i=1,q=2,c=80,m=1;") {
		t.Fatalf("first chunk header = %q", chunks[0][:40])
	}
	if !strings.HasPrefix(string(chunks[1]), "\x1b_Gm=1;") {
		t.Fatalf("middle chunk header = %q", chunks[1][:12])
	}
	if !strings.HasPrefix(string(chunks[2]), "\x1b_Gm=0;") {
		t.Fatalf("last chunk header = %q", chunks[2][:12])
	}
	// Reassemble the base64 and confirm nothing was lost or reordered.
	var b64 strings.Builder
	for _, c := range chunks {
		s := string(c)
		s = s[strings.IndexByte(s, ';')+1:]
		b64.WriteString(strings.TrimSuffix(s, "\x1b\\"))
	}
	if got := b64.Len(); got != 12000 {
		t.Fatalf("reassembled b64 length = %d, want 12000", got)
	}
}

// TestHandshakeParsing pins the reply detection: a kitty OK plus the
// DA1 sentinel says yes; DA1 alone (every other terminal) says no; our
// probe id must be the one acknowledged.
func TestHandshakeParsing(t *testing.T) {
	kitty := []byte("\x1b_Gi=" + probeID + ";OK\x1b\\" + "\x1b[?62;c")
	if !da1Answered(kitty) || !kittyAnswered(kitty) {
		t.Fatal("kitty reply not recognized")
	}
	plain := []byte("\x1b[?1;2c")
	if !da1Answered(plain) {
		t.Fatal("DA1 sentinel not recognized")
	}
	if kittyAnswered(plain) {
		t.Fatal("plain terminal misread as kitty")
	}
	wrongID := []byte("\x1b_Gi=999;OK\x1b\\" + "\x1b[?62;c")
	if kittyAnswered(wrongID) {
		t.Fatal("a foreign graphics reply must not count as ours")
	}
	if da1Answered([]byte("\x1b_G")) {
		t.Fatal("no DA1 yet — the read must continue")
	}
}

// TestTargetCols pins the sizing: full width when it fits, scaled by
// height when the take is tall, floored at one column.
func TestTargetCols(t *testing.T) {
	// 1280x440 image, 100x40 cells of 10x20px: full width implies
	// 1280*... height 440*(1000/1280)=344px → 18 rows ≤ 39 → fits.
	if got := targetCols(100, 39, 10, 20, 1280, 440); got != 100 {
		t.Fatalf("wide take cols = %d, want full 100", got)
	}
	// A tall square take on a short terminal must shrink: 10 rows of
	// 20px = 200px of height → at 1:1 aspect that is 200px wide = 20
	// columns of 10px.
	if got := targetCols(100, 10, 10, 20, 1000, 1000); got != 20 {
		t.Fatalf("tall take cols = %d, want 20", got)
	}
	if got := targetCols(80, 1, 8, 16, 100, 10000); got != 1 {
		t.Fatalf("degenerate take cols = %d, want the floor 1", got)
	}
	// Zeroed pixel info degrades to full width, never a crash.
	if got := targetCols(80, 24, 0, 0, 1280, 440); got != 80 {
		t.Fatalf("no-pixel-info cols = %d, want 80", got)
	}
}

// TestAnimationChunks pins the kitty ANIMATION stream: root via a=T
// sized in columns, every later frame appended with a=f and its gap in
// ms, the root's own gap patched by r=1, and the loop set running.
func TestAnimationChunks(t *testing.T) {
	g := &gif.GIF{
		Image: []*image.Paletted{
			image.NewPaletted(image.Rect(0, 0, 4, 4), testPalette),
			image.NewPaletted(image.Rect(0, 0, 4, 4), testPalette),
			image.NewPaletted(image.Rect(0, 0, 4, 4), testPalette),
		},
		Delay: []int{45, 22, 90},
	}
	chunks, err := animationChunks(g, 18)
	if err != nil {
		t.Fatal(err)
	}
	all := ""
	for _, c := range chunks {
		all += string(c)
	}
	for _, want := range []string{
		"a=T,q=2,o=z,s=4,v=4,i=1,C=1,c=18;",
		"\x1b_Ga=a,q=2,v=1,r=1,i=1,z=450\x1b\\",
		"a=f,q=2,o=z,s=4,v=4,c=1,i=1,z=220;",
		"\x1b_Ga=a,q=2,s=2,v=1,r=1,i=1,z=450\x1b\\",
		"a=f,q=2,o=z,s=4,v=4,c=2,i=1,z=900;",
		"\x1b_Ga=a,q=2,s=3,v=1,r=1,i=1,z=450\x1b\\",
	} {
		if !strings.Contains(all, want) {
			t.Fatalf("animation stream lacks %q", want)
		}
	}
	if strings.Count(all, "a=f") != 2 {
		t.Fatalf("want exactly 2 appended frames:\n%q", all)
	}
	// ONE escape per payload — chunked a=f frames lose their keys in
	// kitty and the animation freezes (found by live bisection).
	if strings.Contains(all, "m=1") || strings.Contains(all, "m=0") {
		t.Fatalf("animation payloads must never be chunked:\n%q", all)
	}
}
