// Package vtengine defines the embedded terminal engine contract: grid state,
// styles, cursor, kitty graphics storage and keyboard-protocol-aware input
// encoding. It is the "nothing hardcoded" boundary of the project:
// the driver and rasterizer depend on this contract, never on a concrete
// engine. Its factory is the only code allowed to import implementations.
package vtengine
