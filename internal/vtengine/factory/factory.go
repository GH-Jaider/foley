// Package factory constructs named engines. It lives one package below
// the contract on purpose: vtengine defines the types every engine
// imports, so a factory THERE would be an import cycle — here it can see
// both the contract and the implementations (still inside the depguard
// quarantine zone).
package factory

import (
	"fmt"

	"github.com/GH-Jaider/foley/internal/vtengine"
)

// New constructs a named engine. v1 ships "ghostty" (libghostty-vt,
// behind the ghosttyvt build tag); tests construct fakes directly.
func New(name string, opts vtengine.Options) (vtengine.Engine, error) {
	switch name {
	case "ghostty":
		return newGhostty(opts)
	default:
		return nil, fmt.Errorf("%w: %q", vtengine.ErrUnknownEngine, name)
	}
}
