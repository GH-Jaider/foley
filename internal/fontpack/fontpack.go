package fontpack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// Pinned font files and their canonical hashes. scripts/fonts.sh fetches
// exactly these; Load refuses anything else — byte-identical rendering
// depends on it. JetBrains Mono v2.304 (OFL), Noto Color Emoji v2.047
// (OFL).
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

// Load reads and hash-verifies the pinned fonts from dir. Any missing or
// tampered file is an error — never a silent fallback to system fonts.
func Load(dir string) (*Pack, error) {
	read := func(name string) ([]byte, error) {
		b, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // paths come from the pin table
		if err != nil {
			return nil, fmt.Errorf("fontpack: %s: %w (corre scripts/fonts.sh)", name, err)
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != pinFor(name) {
			return nil, fmt.Errorf("fontpack: %s: hash %s no coincide con el pin", name, got[:16])
		}
		return b, nil
	}
	var p Pack
	var err error
	if p.Text, err = read("JetBrainsMono-Regular.ttf"); err != nil {
		return nil, err
	}
	if p.TextBold, err = read("JetBrainsMono-Bold.ttf"); err != nil {
		return nil, err
	}
	if p.TextItalic, err = read("JetBrainsMono-Italic.ttf"); err != nil {
		return nil, err
	}
	if p.TextBoldItalic, err = read("JetBrainsMono-BoldItalic.ttf"); err != nil {
		return nil, err
	}
	if p.Emoji, err = read("NotoColorEmoji.ttf"); err != nil {
		return nil, err
	}
	return &p, nil
}
