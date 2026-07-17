package fontpack_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/GH-Jaider/foley/internal/fontpack"
	"github.com/GH-Jaider/foley/internal/testassets"
)

func TestHasFamilyIsGenerous(t *testing.T) {
	for _, name := range []string{"Fira Code", "fira code", "FIRA  CODE", "JetBrains Mono", "ubuntu mono"} {
		if !fontpack.HasFamily(name) {
			t.Fatalf("HasFamily(%q) = false, want true", name)
		}
	}
	if fontpack.HasFamily("Comic Sans") {
		t.Fatal("Comic Sans must not be in the catalog")
	}
}

func TestFamiliesListsDefault(t *testing.T) {
	names := fontpack.Families()
	if len(names) < 6 {
		t.Fatalf("catalog = %v, want at least 6 families", names)
	}
	found := false
	for _, n := range names {
		if n == fontpack.DefaultFamily {
			found = true
		}
	}
	if !found {
		t.Fatalf("catalog %v lacks the default family", names)
	}
}

// TestLoadFamily pins the catalog contract: every family loads all four
// hash-verified styles, and italic-less families alias their uprights.
func TestLoadFamily(t *testing.T) {
	for _, name := range fontpack.Families() {
		f, err := fontpack.LoadFamily("fonts", name)
		testassets.Require(t, err, "make fonts")
		if len(f.Regular) == 0 || len(f.Bold) == 0 || len(f.Italic) == 0 || len(f.BoldItalic) == 0 {
			t.Fatalf("%s: a style slot is empty", name)
		}
	}
	f, err := fontpack.LoadFamily("fonts", "fira  CODE")
	testassets.Require(t, err, "make fonts")
	if f.Name != "Fira Code" {
		t.Fatalf("Name = %q, want the canonical Fira Code", f.Name)
	}
	if !bytes.Equal(f.Italic, f.Regular) || !bytes.Equal(f.BoldItalic, f.Bold) {
		t.Fatal("Fira Code has no italics — those slots must alias the uprights")
	}
}

func TestLoadFamilyUnknownIsLoud(t *testing.T) {
	_, err := fontpack.LoadFamily("fonts", "Comic Sans")
	if !errors.Is(err, fontpack.ErrUnknownFamily) {
		t.Fatalf("err = %v, want ErrUnknownFamily", err)
	}
	if !strings.Contains(err.Error(), "Fira Code") {
		t.Fatalf("the error must list the catalog, got %v", err)
	}
}
