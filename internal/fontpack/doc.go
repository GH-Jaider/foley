// Package fontpack owns the bundled, version-pinned fonts (text, color emoji,
// CJK fallback) that make renders byte-identical across machines. No system
// font is ever consulted; foley doctor verifies pack integrity by hash.
package fontpack
