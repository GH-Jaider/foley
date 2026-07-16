//go:build !ghosttyvt

package foley

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/GH-Jaider/foley/internal/testassets"
	"github.com/GH-Jaider/foley/internal/vtengine"
)

// TestNewWithoutEngineTagFailsLoudly pins the untagged behavior: a build
// without the engine must say exactly why instead of failing somewhere
// deep. Under the ghosttyvt tag New succeeds, so this file is excluded.
func TestNewWithoutEngineTagFailsLoudly(t *testing.T) {
	_, err := New(Options{
		Command:  []string{"/bin/sh", "-c", "true"},
		FontsDir: filepath.Join("internal", "fontpack", "fonts"),
	})
	testassets.Require(t, fontsMissing(err), "make fonts")
	if !errors.Is(err, vtengine.ErrUnknownEngine) {
		t.Fatalf("err = %v, want ErrUnknownEngine (untagged build)", err)
	}
}
