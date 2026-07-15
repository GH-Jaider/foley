// Package ghostty implements the vtengine contract on libghostty-vt via cgo.
// This package is the project's cgo quarantine (ADR-009): the pinned static
// library, its C headers and every cgo directive live here and nowhere else.
// Cross-compilation uses zig cc; the pinned build artifact is cached per
// libghostty commit.
package ghostty
