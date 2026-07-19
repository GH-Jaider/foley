// Package terminfo carries the compiled terminfo entry behind foley's
// declared TERM (ADR-021): xterm-ghostty, emitted from the SAME
// libghostty pin the engine is built from — the entry describes, by
// construction, what the embedded engine actually implements. The world
// detects terminal capabilities by identity (TERM allowlists), not by
// probing, so declaring a 1990s TERM hid every capability the engine
// has; declaring ghostty's without shipping its entry would break every
// terminfo reader on hosts that never installed ghostty. This package
// closes the loop: the blob travels inside the binary and Dir
// materializes it where ncurses can find it. Regenerate after an engine
// pin bump with `make terminfo` (scripts/terminfo.sh); never edit the
// blobs by hand.
package terminfo

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// entry is the compiled terminfo blob for xterm-ghostty|ghostty|Ghostty
// (tic -x over xterm-ghostty.terminfo, kept next to it for review). The
// compiled format is the portable little-endian one every ncurses reads.
//
//go:embed xterm-ghostty
var entry []byte

// entryNames are the on-disk lookups ncurses tries: Linux files entries
// under the first LETTER ("x/"), macOS under its HEX code ("78/"); the
// alias gets its own pair. All four are the same blob — tic hardlinks
// aliases, this materialization copies.
//
//nolint:gochecknoglobals // immutable path table
var entryNames = []string{
	"x/xterm-ghostty", "78/xterm-ghostty",
	"g/ghostty", "67/ghostty",
}

// Dir materializes the pinned entry under the user cache directory and
// returns the directory TERMINFO should point at. Content-addressed and
// idempotent: a pin bump lands in a fresh directory, an intact cache is
// only read, a corrupted file is rewritten. Setting TERMINFO does not
// blind other terms — ncurses searches it FIRST and then its usual
// fallback list (an inner tmux still resolves tmux-256color from the
// system database).
func Dir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("terminfo: no user cache dir: %w", err)
	}
	return dirIn(filepath.Join(base, "foley", "terminfo"))
}

func dirIn(base string) (string, error) {
	sum := sha256.Sum256(entry)
	root := filepath.Join(base, hex.EncodeToString(sum[:6]))
	for _, rel := range entryNames {
		dst := filepath.Join(root, filepath.FromSlash(rel))
		//nolint:gosec // dst derives from the embedded blob's own hash under the cache dir
		if have, err := os.ReadFile(dst); err == nil && bytes.Equal(have, entry) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return "", fmt.Errorf("terminfo: %w", err)
		}
		// Write-then-rename: a concurrent render must never observe a
		// half-written entry (ncurses would misparse, not error). The
		// 0600 CreateTemp default is enough — every reader is the pty
		// child running as this same user.
		tmp, err := os.CreateTemp(filepath.Dir(dst), ".entry-*")
		if err != nil {
			return "", fmt.Errorf("terminfo: %w", err)
		}
		_, werr := tmp.Write(entry)
		cerr := tmp.Close()
		if werr == nil {
			werr = cerr
		}
		if werr == nil {
			werr = os.Rename(tmp.Name(), dst)
		}
		if werr != nil {
			_ = os.Remove(tmp.Name())
			return "", fmt.Errorf("terminfo: materialize %s: %w", rel, werr)
		}
	}
	return root, nil
}
