//go:build !ghosttyvt

package factory

import (
	"fmt"

	"github.com/GH-Jaider/foley/internal/vtengine"
)

// newGhostty without the ghosttyvt build tag: the binary was built
// without the engine (no libghostty-vt.a linked). Fail loudly instead of
// pretending.
func newGhostty(vtengine.Options) (vtengine.Engine, error) {
	return nil, fmt.Errorf("%w: ghostty (binary built without the ghosttyvt tag)", vtengine.ErrUnknownEngine)
}
