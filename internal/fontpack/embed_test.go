//go:build embedfonts

package fontpack_test

import (
	"bytes"
	"testing"

	"github.com/GH-Jaider/foley/internal/fontpack"
	"github.com/GH-Jaider/foley/internal/testassets"
)

// TestEmbeddedMatchesDisk is the whole safety of the embed: a binary
// built with -tags embedfonts must render byte-for-byte identically to
// one reading the fonts off disk. Load("") serves the embedded set;
// Load("fonts") the directory — every style slot must be equal.
func TestEmbeddedMatchesDisk(t *testing.T) {
	if !fontpack.Embedded {
		t.Fatal("built with -tags embedfonts but Embedded is false")
	}
	embedded, err := fontpack.Load("")
	if err != nil {
		t.Fatalf("Load(\"\") from the embedded set: %v", err)
	}
	disk, err := fontpack.Load("fonts")
	testassets.Require(t, err, "make fonts")

	for _, s := range []struct {
		name     string
		emb, dsk []byte
	}{
		{"Text", embedded.Text, disk.Text},
		{"TextBold", embedded.TextBold, disk.TextBold},
		{"TextItalic", embedded.TextItalic, disk.TextItalic},
		{"TextBoldItalic", embedded.TextBoldItalic, disk.TextBoldItalic},
		{"Emoji", embedded.Emoji, disk.Emoji},
	} {
		if !bytes.Equal(s.emb, s.dsk) {
			t.Fatalf("%s: embedded bytes differ from disk (%d vs %d)", s.name, len(s.emb), len(s.dsk))
		}
	}
}

// TestEmbeddedFamiliesMatchDisk holds the same equality for the name
// catalog: LoadFamily("") and LoadFamily("fonts") agree on every style.
func TestEmbeddedFamiliesMatchDisk(t *testing.T) {
	for _, name := range fontpack.Families() {
		emb, err := fontpack.LoadFamily("", name)
		if err != nil {
			t.Fatalf("%s embedded: %v", name, err)
		}
		dsk, err := fontpack.LoadFamily("fonts", name)
		testassets.Require(t, err, "make fonts")
		if !bytes.Equal(emb.Regular, dsk.Regular) || !bytes.Equal(emb.Bold, dsk.Bold) ||
			!bytes.Equal(emb.Italic, dsk.Italic) || !bytes.Equal(emb.BoldItalic, dsk.BoldItalic) {
			t.Fatalf("%s: embedded family differs from disk", name)
		}
	}
}
