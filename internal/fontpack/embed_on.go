//go:build embedfonts

package fontpack

import (
	"embed"
	"io/fs"
)

// The pinned fonts, baked into the binary. The release build runs
// `make fonts` first, then compiles with -tags embedfonts, so the
// files exist when go:embed reads them. Without the tag (the default
// build) this file is absent and the fonts stay on disk — go:embed
// never sees the gitignored directory, so a plain `go build` needs no
// fonts present. Same hashes are verified either way (fontpack.go), so
// embedded and on-disk render byte-for-byte identically.
//
//go:embed fonts/*.ttf
var embedded embed.FS

// Embedded reports that this binary carries the pinned fonts — so a
// recording needs no FontsDir, no $FOLEY_FONTS, no fonts directory.
const Embedded = true

// embeddedFonts is the pinned set as a flat fs.FS (the "fonts/" prefix
// stripped, so names match the on-disk layout).
func embeddedFonts() (fs.FS, error) {
	return fs.Sub(embedded, "fonts")
}
