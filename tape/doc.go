// Package tape parses the .tape DSL (VHS-compatible core commands) into a
// typed AST.
//
// The vendored VHS grammar (stringly-typed by design) is quarantined in
// tape/internal/vhsgrammar and converted to the typed AST in exactly one
// place; nothing stringly leaks past this package.
package tape
