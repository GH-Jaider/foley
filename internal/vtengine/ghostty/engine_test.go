//go:build ghosttyvt

package ghostty_test

import (
	"testing"

	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/enginetest"
	"github.com/GH-Jaider/foley/internal/vtengine/ghostty"
)

func TestGhosttyConformsBasic(t *testing.T) {
	enginetest.RunBasic(t, func(t *testing.T, opts vtengine.Options) vtengine.Engine {
		t.Helper()
		e, err := ghostty.New(opts)
		if err != nil {
			t.Fatalf("ghostty.New: %v", err)
		}
		return e
	})
}
