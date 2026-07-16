//go:build ghosttyvt

package factory

import (
	"github.com/GH-Jaider/foley/internal/vtengine"
	"github.com/GH-Jaider/foley/internal/vtengine/ghostty"
)

// newGhostty builds the real engine. Only this file sees the cgo
// quarantine; without the ghosttyvt tag the stub answers instead.
func newGhostty(opts vtengine.Options) (vtengine.Engine, error) {
	return ghostty.New(opts)
}
