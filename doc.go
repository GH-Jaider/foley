// Package foley renders scripted, VHS-style terminal demos without a
// terminal window and without screen capture: the demo app runs on a real
// pty, an embedded terminal engine (libghostty-vt) maintains the state, and
// foley rasterizes the frames itself — byte-identical output on macOS, Linux
// and CI, with zero permissions.
//
// This is the public, library-first API; the CLI in cmd/foley is one of its
// consumers. The API surface lands in milestone M8 (see docs/ROADMAP.md).
package foley
