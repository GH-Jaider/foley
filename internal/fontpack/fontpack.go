package fontpack

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
)

// Pinned font files and their canonical hashes. scripts/fonts.sh fetches
// exactly these; Load refuses anything else — byte-identical rendering
// depends on it. JetBrains Mono v2.304 (OFL), Noto Color Emoji v2.047
// (OFL), plus the name catalog: Fira Code 5.2 (OFL), IBM Plex Mono
// v6.0.0 (OFL), Source Code Pro (OFL), Hack v3.003 (MIT+Bitstream),
// Ubuntu Mono (UFL).
func pinFor(name string) string {
	switch name {
	case "JetBrainsMono-Regular.ttf":
		return "a0bf60ef0f83c5ed4d7a75d45838548b1f6873372dfac88f71804491898d138f"
	case "JetBrainsMono-Bold.ttf":
		return "5590990c82e097397517f275f430af4546e1c45cff408bde4255dad142479dcb"
	case "JetBrainsMono-Italic.ttf":
		return "9d0a1f7a708e6af183f1193b7e81d40da294f5c67682c085d8401c60aac8ded4"
	case "JetBrainsMono-BoldItalic.ttf":
		return "4039d5ce0ed225bf9c8b2c8c6436290ae2f356b7e90d70fa666227238324aa3b"
	case "NotoColorEmoji.ttf":
		return "39ee3c587e10e89669b9ff32703261d10d5f9c4dd5ad147b6b5a1c5200591817"
	case "FiraCode-Regular.ttf":
		return "28c3ae21a853f1d74673384c7a0d620abb0e877b8c6cd8b64173a95512476824"
	case "FiraCode-Bold.ttf":
		return "37a609b7e27516ce0cf55cb7550edd1a1cbd8cd5bc028415a1d520c426c10357"
	case "IBMPlexMono-Regular.ttf":
		return "a3c50f7c0e063998cfaeae56c6169ece9e0feaffaef425aa038f85d037fb4b9b"
	case "IBMPlexMono-Bold.ttf":
		return "5474dd5d5c3dc6c027cac93fe7e5a736e7b33adb4717093a1e23b36aab4606e9"
	case "IBMPlexMono-Italic.ttf":
		return "d70bd62fa6b97d19853c0cf823667f99f7ff023d915052248e68635179c8fa83"
	case "IBMPlexMono-BoldItalic.ttf":
		return "3d0c0888a9c3a98b39fc5aace9c20b149c793063cd9e9e0634f561e55186c4bf"
	case "SourceCodePro-Regular.ttf":
		return "74bd80d3e42a08517cd7e1108ba3d86f2da29ac0f3065be95e0357956ab9db37"
	case "SourceCodePro-Bold.ttf":
		return "b2095e0d657e6d28dc32444a9dacabab0c9241d0bf39d96371756cc9bdbc3a5f"
	case "SourceCodePro-It.ttf":
		return "9c9e0f4d016210a3c5bdfba5262637c5b26ddff4ccc382ebbc781de5961d0042"
	case "SourceCodePro-BoldIt.ttf":
		return "1b49d9304012bf8db9e5dd4104183d5c122c445d0570a2259125f71977595b90"
	case "Hack-Regular.ttf":
		return "15f55cc0c85a2988d2b4b3a8cdb5d77fdfbaf319e1bb5309d725db9818fb7125"
	case "Hack-Bold.ttf":
		return "5bbf531eff7f8a0c2559c9a0656718e2828a012a9b1f60b5f54006d59a4de8d4"
	case "Hack-Italic.ttf":
		return "096fb67a2b85f3c866e9cb3e965b27c2c10b977315f4d3d7f095674be35091c1"
	case "Hack-BoldItalic.ttf":
		return "64f74a079700b7dfe128551a1e28875d5ba980971e55f5e0f0596e37bdc6a6bc"
	case "UbuntuMono-Regular.ttf":
		return "b35dd9d2131d5d83a9b87fe9ad22c6288fa3d17688d43302c14da29812417d63"
	case "UbuntuMono-Bold.ttf":
		return "11f15c3a6bbd998a8695fdefb3475931c3789aa035d7546f2efe78e83b352f6b"
	case "UbuntuMono-Italic.ttf":
		return "960b2bc286c2ff7d49073303858c65e1fc9013c17a971b61123b02c39454ef75"
	case "UbuntuMono-BoldItalic.ttf":
		return "bd255784bb87b5c41513a12a86f0f9cf061bce4e8256d3bfe7234611002e8f48"
	default:
		return ""
	}
}

// Pack holds the raw bytes of the pinned fonts. The rasterizer parses
// them with its text stack; fontpack stays parser-agnostic on purpose.
type Pack struct {
	Text           []byte
	TextBold       []byte
	TextItalic     []byte
	TextBoldItalic []byte
	Emoji          []byte
}

// source returns the filesystem the pinned files live in: the fonts
// EMBEDDED in the binary when dir is empty (a build with -tags
// embedfonts — the release binary is self-contained), otherwise the
// directory on disk. Either way the hash pin is verified identically,
// so byte-for-byte rendering is the same from either source.
func source(dir string) (fs.FS, error) {
	if dir == "" {
		return embeddedFonts()
	}
	return os.DirFS(dir), nil
}

// readPinned reads one pinned file from fsys and verifies its hash —
// never a silent fallback to system fonts.
func readPinned(fsys fs.FS, name string) ([]byte, error) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("fontpack: %s: %w", name, err)
	}
	sum := sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); got != pinFor(name) {
		return nil, fmt.Errorf("fontpack: %s: hash %s does not match the pin", name, got[:16])
	}
	return b, nil
}

// Load reads and hash-verifies the pinned fonts from dir (empty dir =
// the embedded set). Any missing or tampered file is an error — never a
// silent fallback to system fonts.
func Load(dir string) (*Pack, error) {
	fsys, err := source(dir)
	if err != nil {
		return nil, err
	}
	var p Pack
	for _, f := range []struct {
		dst  *[]byte
		name string
	}{
		{&p.Text, "JetBrainsMono-Regular.ttf"},
		{&p.TextBold, "JetBrainsMono-Bold.ttf"},
		{&p.TextItalic, "JetBrainsMono-Italic.ttf"},
		{&p.TextBoldItalic, "JetBrainsMono-BoldItalic.ttf"},
		{&p.Emoji, "NotoColorEmoji.ttf"},
	} {
		if *f.dst, err = readPinned(fsys, f.name); err != nil {
			return nil, err
		}
	}
	return &p, nil
}

// LoadFile reads a USER font file. No hash pin: the file is
// the user's own input — their repo pins it — and determinism becomes
// parametrized: same tape + same font bytes → same frames. Whether the
// bytes parse as a font is the raster's to report, path included.
func LoadFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the tape-declared font path, CWD-relative by doctrine
	if err != nil {
		return nil, fmt.Errorf("fontpack: user font: %w", err)
	}
	return b, nil
}

// DefaultFamily is the pinned family every recording uses unless a
// tape asks otherwise — asking for it BY NAME is a no-op, not an error.
const DefaultFamily = "JetBrains Mono"

// familyFiles maps the four style slots (regular, bold, italic,
// bold-italic) to pinned file names. Families without true italics
// (Fira Code) alias those slots to their uprights: the weight stays,
// the slant degrades — the grid metrics never change.
type familyFiles [4]string

// families is the name catalog: `Set FontFamily "Fira Code"`
// resolves HERE — hash-pinned files fetched by scripts/fonts.sh — and
// never against the system font database. Keys are canonical names.
func families() map[string]familyFiles {
	return map[string]familyFiles{
		DefaultFamily: {
			"JetBrainsMono-Regular.ttf", "JetBrainsMono-Bold.ttf",
			"JetBrainsMono-Italic.ttf", "JetBrainsMono-BoldItalic.ttf",
		},
		"Fira Code": {
			"FiraCode-Regular.ttf", "FiraCode-Bold.ttf",
			"FiraCode-Regular.ttf", "FiraCode-Bold.ttf",
		},
		"IBM Plex Mono": {
			"IBMPlexMono-Regular.ttf", "IBMPlexMono-Bold.ttf",
			"IBMPlexMono-Italic.ttf", "IBMPlexMono-BoldItalic.ttf",
		},
		"Source Code Pro": {
			"SourceCodePro-Regular.ttf", "SourceCodePro-Bold.ttf",
			"SourceCodePro-It.ttf", "SourceCodePro-BoldIt.ttf",
		},
		"Hack": {
			"Hack-Regular.ttf", "Hack-Bold.ttf",
			"Hack-Italic.ttf", "Hack-BoldItalic.ttf",
		},
		"Ubuntu Mono": {
			"UbuntuMono-Regular.ttf", "UbuntuMono-Bold.ttf",
			"UbuntuMono-Italic.ttf", "UbuntuMono-BoldItalic.ttf",
		},
	}
}

// Families lists the catalog names, sorted — `foley fonts` prints this.
func Families() []string {
	fam := families()
	names := make([]string, 0, len(fam))
	for n := range fam {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// normalizeFamily makes lookup case- and spacing-insensitive:
// "fira code" and "Fira  Code" both find the catalog entry.
func normalizeFamily(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}

// ErrUnknownFamily reports a name outside the catalog.
var ErrUnknownFamily = errors.New("fontpack: unknown font family")

// HasFamily reports whether a name resolves in the catalog (case- and
// spacing-insensitive) — validation without loading any bytes.
func HasFamily(name string) bool {
	want := normalizeFamily(name)
	for canonical := range families() {
		if normalizeFamily(canonical) == want {
			return true
		}
	}
	return false
}

// Family holds one catalog family's style bytes, hash-verified.
type Family struct {
	// Name is the canonical catalog name (case normalized).
	Name                              string
	Regular, Bold, Italic, BoldItalic []byte
}

// LoadFamily reads a catalog family from dir (empty dir = the embedded
// set), hash-verified like the pack. Unknown names return
// ErrUnknownFamily listing the catalog.
func LoadFamily(dir, name string) (*Family, error) {
	want := normalizeFamily(name)
	for canonical, files := range families() {
		if normalizeFamily(canonical) != want {
			continue
		}
		fsys, err := source(dir)
		if err != nil {
			return nil, err
		}
		f := &Family{Name: canonical}
		for i, dst := range []*[]byte{&f.Regular, &f.Bold, &f.Italic, &f.BoldItalic} {
			b, err := readPinned(fsys, files[i])
			if err != nil {
				return nil, err
			}
			*dst = b
		}
		return f, nil
	}
	return nil, fmt.Errorf("%w: %q (available: %s)", ErrUnknownFamily, name, strings.Join(Families(), ", "))
}
