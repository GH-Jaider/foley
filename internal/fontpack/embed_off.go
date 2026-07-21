//go:build !embedfonts

package fontpack

import (
	"errors"
	"io/fs"
)

// The default build does NOT bake the fonts in: the pinned files are
// gitignored (fetched by scripts/fonts.sh), so a go:embed of them would
// break `go build` for anyone who has not run `make fonts`. Fonts stay
// on disk, resolved via FontsDir / $FOLEY_FONTS. The release build adds
// -tags embedfonts (see embed_on.go) to ship a self-contained binary.

// Embedded reports that this binary carries the pinned fonts. Off here:
// callers fall back to a fonts directory.
const Embedded = false

// errNoEmbed explains an empty dir when nothing is embedded — the
// caller asked for the baked-in fonts a plain build does not have.
var errNoEmbed = errors.New("fontpack: this binary has no embedded fonts — set FontsDir or $FOLEY_FONTS to the pinned fonts directory (or build with -tags embedfonts)")

func embeddedFonts() (fs.FS, error) {
	return nil, errNoEmbed
}
