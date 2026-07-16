package raster_test

import (
	"os"
	"testing"

	"github.com/go-text/typesetting/font"

	"github.com/GH-Jaider/foley/internal/testassets"
)

func TestEmojiCBDTExposed(t *testing.T) {
	f, err := os.Open("../fontpack/fonts/NotoColorEmoji.ttf")
	testassets.Require(t, err, "make fonts")
	defer func() { _ = f.Close() }()
	face, err := font.ParseTTF(f)
	if err != nil {
		t.Fatal(err)
	}
	gid, ok := face.NominalGlyph(0x1F680) // 🚀
	if !ok {
		t.Fatal("cmap missing 🚀")
	}
	data := face.GlyphData(gid)
	t.Logf("GlyphData(🚀) = %T", data)
	if bm, ok := data.(font.GlyphBitmap); ok {
		t.Logf("bitmap: format=%v %dx%d bytes=%d", bm.Format, bm.Width, bm.Height, len(bm.Data))
	} else {
		t.Fatalf("no bitmap glyph data: %T", data)
	}
}
