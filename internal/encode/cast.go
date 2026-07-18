package encode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"unicode/utf8"
)

// CastEvent is one observed pty output burst on the recording's
// timeline (virtual in deterministic mode, wall-elapsed in realtime).
type CastEvent struct {
	At   time.Duration
	Data []byte
}

// WriteCast writes an asciicast v2 file (asciinema's format): a JSON
// header line, then one `[time, "o", data]` event per output burst.
// Deterministic by construction: no wall-clock header timestamp, and
// event times are formatted as exact integer microseconds (%d.%06d) —
// no float formatting crosses the output boundary. Multibyte runes
// torn across bursts are re-joined by carrying the incomplete tail
// into the next event: JSON strings must be valid UTF-8, and a
// replacement character would corrupt the stream a player sees.
func WriteCast(path string, cols, rows int, events []CastEvent) error {
	var b bytes.Buffer
	fmt.Fprintf(&b, `{"version": 2, "width": %d, "height": %d}`+"\n", cols, rows)
	// Merge consecutive same-instant bursts FIRST: how the pty splits
	// bytes into chunks is wall-clock noise, but in deterministic mode
	// every byte of a settle shares its step's virtual instant — after
	// merging, chunk boundaries vanish and two identical runs write
	// byte-identical casts.
	merged := make([]CastEvent, 0, len(events))
	for _, ev := range events {
		if n := len(merged); n > 0 && merged[n-1].At == ev.At {
			joined := make([]byte, 0, len(merged[n-1].Data)+len(ev.Data))
			joined = append(joined, merged[n-1].Data...)
			joined = append(joined, ev.Data...)
			merged[n-1].Data = joined
			continue
		}
		merged = append(merged, ev)
	}
	events = merged
	var carry []byte
	lastAt := time.Duration(0)
	for _, ev := range events {
		lastAt = ev.At
		data := ev.Data
		if len(carry) > 0 {
			joined := make([]byte, 0, len(carry)+len(data))
			joined = append(joined, carry...)
			joined = append(joined, data...)
			data, carry = joined, nil
		}
		if n := incompleteTail(data); n > 0 {
			carry = append(carry, data[len(data)-n:]...)
			data = data[:len(data)-n]
		}
		if len(data) == 0 {
			continue
		}
		if err := writeCastEvent(&b, ev.At, data); err != nil {
			return err
		}
	}
	if len(carry) > 0 {
		// The recording ended mid-rune (or on genuinely invalid
		// bytes): emit what remains — Marshal substitutes U+FFFD,
		// which is the honest rendering of a torn tail at EOF.
		if err := writeCastEvent(&b, lastAt, carry); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil { //nolint:gosec // caller-requested artifact
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}

func writeCastEvent(b *bytes.Buffer, at time.Duration, data []byte) error {
	payload, err := json.Marshal(string(data))
	if err != nil {
		return fmt.Errorf("encode: cast event: %w", err)
	}
	us := at.Microseconds()
	fmt.Fprintf(b, "[%d.%06d, \"o\", %s]\n", us/1_000_000, us%1_000_000, payload)
	return nil
}

// incompleteTail reports how many trailing bytes of data begin a
// multibyte rune whose continuation bytes have not arrived yet. Zero
// for complete (or invalid — invalid bytes must not be carried
// forever) tails.
func incompleteTail(data []byte) int {
	n := len(data)
	for i := 1; i <= utf8.UTFMax && i <= n; i++ {
		c := data[n-i]
		if c < 0x80 {
			return 0 // ASCII: complete
		}
		if c >= 0xC0 { // a start byte i-1 continuations in
			exp := 0
			switch {
			case c >= 0xF0 && c <= 0xF7:
				exp = 4
			case c >= 0xE0:
				exp = 3
			case c >= 0xC2:
				exp = 2
			default:
				return 0 // invalid start: not carriable
			}
			if i < exp {
				return i // the rune is still arriving
			}
			return 0 // complete (utf8.Valid decides validity later)
		}
		// 0x80..0xBF: continuation byte — keep scanning back.
	}
	return 0
}
