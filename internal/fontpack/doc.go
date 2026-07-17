// Package fontpack owns the bundled, version-pinned fonts (text, color emoji,
// CJK fallback) that make renders byte-identical across machines, plus the
// name catalog (ADR-015): the families `Set FontFamily` may name, all
// hash-pinned, and user font FILES loaded unpinned (the user's repo pins
// those). No system font is ever consulted; foley doctor verifies pack
// integrity by hash.
package fontpack
