package key

// Mod is a bitmask of key modifiers.
type Mod uint8

// Modifier bits.
const (
	ModShift Mod = 1 << iota
	ModCtrl
	ModAlt
	ModSuper
)

// Name identifies a named (non-rune) key. The full VHS-compatible name
// table lands in M4; this is the shared vocabulary needed by the engine
// contract.
type Name uint16

// Named keys.
const (
	NameNone Name = iota
	NameEnter
	NameEscape
	NameBackspace
	NameTab
	NameSpace
	NameDelete
	NameInsert
	NameUp
	NameDown
	NameLeft
	NameRight
	NameHome
	NameEnd
	NamePageUp
	NamePageDown
)

// Key is one logical key: either a printable rune (Rune != 0, Name ==
// NameNone) or a named key (Name != NameNone), plus modifiers. The zero
// value is invalid.
type Key struct {
	Rune rune
	Name Name
	Mods Mod
}

// RuneKey returns a printable-rune key.
func RuneKey(r rune) Key { return Key{Rune: r} }

// Named returns a named key.
func Named(n Name) Key { return Key{Name: n} }

// With returns a copy of k with the given modifiers added.
func (k Key) With(m Mod) Key {
	k.Mods |= m
	return k
}

// IsZero reports whether k is the invalid zero value.
func (k Key) IsZero() bool { return k.Rune == 0 && k.Name == NameNone && k.Mods == 0 }
