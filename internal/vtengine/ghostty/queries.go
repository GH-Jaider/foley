//go:build ghosttyvt

package ghostty

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/GH-Jaider/foley/internal/vtengine"
)

// This file answers the terminal queries the pinned lib leaves silent
// (ADR-025): XTWINOPS geometry reports (CSI 11/13/14/15/16/18/19 t),
// XTGETTCAP (DCS + q), DECRQSS (DCS $ q) and the color-scheme report
// (CSI ? 996 n). Modern TUIs fire them at startup and wait out reply
// timeouts; in deterministic mode that wait is pty silence, which reads
// as "scene over". The lib cannot answer them — pixel geometry is the
// EMBEDDER's knowledge (FontSize → cell) — so the binding does, from
// the same Geometry the raster derived. The scanner never consumes a
// byte: the lib still sees the entire stream verbatim; answers are
// interleaved in stream order via the same Responses writer the lib's
// WRITE_PTY callback uses.

type qfState uint8

const (
	qfGround        qfState = iota
	qfEsc                   // ESC seen
	qfCSI                   // ESC [ — collecting parameters
	qfDCS                   // ESC P — deciding which DCS query this is
	qfDCSPlus               // ESC P +  (XTGETTCAP if q follows)
	qfDCSDollar             // ESC P $  (DECRQSS if q follows)
	qfDCSPayload            // collecting the query payload
	qfDCSPayloadEsc         // in payload, ESC seen (ST closes the query)
	qfSkipDCS               // some other DCS: wait for its terminator
	qfSkipEsc               // in skip, ESC seen
)

// query buffer caps: CSI params beyond any geometry report, and a
// payload bound so a hostile/binary stream cannot grow state.
const (
	qfMaxParams = 16
	qfMaxTCap   = 512
)

// queryEvent is one completed query the lib will not answer.
type queryEvent struct {
	// kind: 't' = XTWINOPS report, 'n' = private CSI n report (996),
	// 'q' = XTGETTCAP, 'r' = DECRQSS.
	kind   byte
	params string // CSI parameter bytes, or the raw DCS payload
}

// queryFilter is a bounded scanner over the app's output stream. Its
// zero value is ready. State survives Write boundaries: a query split
// across chunks still completes.
type queryFilter struct {
	state    qfState
	buf      []byte
	overflow bool
	private  bool // CSI had a leading '?' (private marker)
	dcsKind  byte // which DCS query the payload belongs to
}

// scan consumes bytes until one query completes (returning how many
// bytes were consumed and the event) or the buffer ends (event nil).
// The caller feeds the lib exactly the consumed bytes before answering,
// so responses keep stream order.
func (f *queryFilter) scan(p []byte) (int, *queryEvent) {
	for i := 0; i < len(p); i++ {
		b := p[i]
	again:
		switch f.state {
		case qfGround:
			if b == 0x1b {
				f.state = qfEsc
			}
		case qfEsc:
			switch b {
			case '[':
				f.state = qfCSI
				f.buf = f.buf[:0]
				f.overflow = false
				f.private = false
			case 'P':
				f.state = qfDCS
			case 0x1b:
				// stay: ESC ESC re-arms
			default:
				f.state = qfGround
			}
		case qfCSI:
			switch {
			case b >= '0' && b <= '9' || b == ';':
				if len(f.buf) < qfMaxParams {
					f.buf = append(f.buf, b)
				} else {
					f.overflow = true
				}
			case b == '?' && len(f.buf) == 0 && !f.private:
				f.private = true
			case b >= 0x40 && b <= 0x7e:
				// final byte — the query families the lib skips:
				// XTWINOPS reports and the private color-scheme query.
				params := string(f.buf)
				f.state = qfGround
				if f.overflow {
					break
				}
				if b == 't' && !f.private && winopsKnown(params) {
					return i + 1, &queryEvent{kind: 't', params: params}
				}
				if b == 'n' && f.private && params == "996" {
					return i + 1, &queryEvent{kind: 'n', params: params}
				}
			case b == 0x1b:
				f.state = qfEsc
			default:
				// intermediate bytes (>, SP, $, ...) make it not-ours;
				// ride out the sequence to its final byte.
				f.overflow = true
			}
		case qfDCS:
			switch b {
			case '+':
				f.state = qfDCSPlus
			case '$':
				f.state = qfDCSDollar
			case 0x1b:
				f.state = qfEsc
			default:
				f.state = qfSkipDCS
			}
		case qfDCSPlus, qfDCSDollar:
			switch b {
			case 'q':
				if f.state == qfDCSPlus {
					f.dcsKind = 'q' // XTGETTCAP
				} else {
					f.dcsKind = 'r' // DECRQSS
				}
				f.state = qfDCSPayload
				f.buf = f.buf[:0]
				f.overflow = false
			case 0x1b:
				f.state = qfEsc
			default:
				f.state = qfSkipDCS
			}
		case qfDCSPayload:
			if b == 0x1b {
				f.state = qfDCSPayloadEsc
				break
			}
			if len(f.buf) < qfMaxTCap {
				f.buf = append(f.buf, b)
			} else {
				f.overflow = true
			}
		case qfDCSPayloadEsc:
			if b == '\\' && !f.overflow {
				payload := string(f.buf)
				f.state = qfGround
				return i + 1, &queryEvent{kind: f.dcsKind, params: payload}
			}
			// ESC canceled the DCS: reprocess as a fresh escape.
			f.state = qfEsc
			goto again
		case qfSkipDCS:
			if b == 0x1b {
				f.state = qfSkipEsc
			}
		case qfSkipEsc:
			if b == '\\' {
				f.state = qfGround
				break
			}
			f.state = qfEsc
			goto again
		}
	}
	return len(p), nil
}

// winopsKnown reports whether a CSI …t parameter is a report this
// terminal answers. Window-manipulation ops stay silent — there is no
// window.
func winopsKnown(params string) bool {
	switch params {
	case "11", "13", "14", "15", "16", "18", "19":
		return true
	}
	return false
}

// terminalName is the identity foley declares to the app (TERM and the
// pinned terminfo entry, ADR-021); XTGETTCAP must tell the same story.
const terminalName = "xterm-ghostty"

// tcapValue serves XTGETTCAP from the PINNED terminfo entry
// (internal/terminfo/xterm-ghostty.terminfo — the drift test pins each
// value to that source). String values carry real escape bytes, the
// way tigetstr would return them; boolean flags answer with the name
// alone. Everything else gets an immediate negative.
func tcapValue(name string) (value string, isBool, known bool) {
	switch name {
	case "TN": // terminal name (XTGETTCAP's own convention)
		return terminalName, false, true
	case "colors", "Co": // Co is the termcap alias of colors#256
		return "256", false, true
	case "Tc", "Su": // truecolor + styled underlines, flags in the pin
		return "", true, true
	case "Smulx":
		return "\x1b[4:%p1%dm", false, true
	case "Ms":
		return "\x1b]52;%p1%s;%p2%s\a", false, true
	case "setrgbf":
		return "\x1b[38:2:%p1%d:%p2%d:%p3%dm", false, true
	case "setrgbb":
		return "\x1b[48:2:%p1%d:%p2%d:%p3%dm", false, true
	}
	return "", false, false
}

// answerQuery writes the response for one completed query. Geometry
// answers use LOGICAL pixels — the same space as the pty winsize, so
// both interrogation paths agree.
func (e *Engine) answerQuery(ev *queryEvent) {
	w := e.opts.Responses
	if w == nil {
		return
	}
	switch ev.kind {
	case 't':
		g := e.geo
		switch ev.params {
		case "11": // window state: there is a "window" and it is shown
			_, _ = w.Write([]byte("\x1b[1t"))
		case "13": // window position: the canvas origin
			_, _ = w.Write([]byte("\x1b[3;0;0t"))
		case "14": // text area in pixels: CSI 4 ; height ; width t
			_, _ = fmt.Fprintf(w, "\x1b[4;%d;%dt", g.Rows*g.CellH, g.Cols*g.CellW)
		case "15": // screen size in pixels — the take IS the screen
			_, _ = fmt.Fprintf(w, "\x1b[5;%d;%dt", g.Rows*g.CellH, g.Cols*g.CellW)
		case "16": // cell size in pixels: CSI 6 ; height ; width t
			_, _ = fmt.Fprintf(w, "\x1b[6;%d;%dt", g.CellH, g.CellW)
		case "18": // text area in cells: CSI 8 ; rows ; cols t
			_, _ = fmt.Fprintf(w, "\x1b[8;%d;%dt", g.Rows, g.Cols)
		case "19": // screen size in cells — same story as 15
			_, _ = fmt.Fprintf(w, "\x1b[9;%d;%dt", g.Rows, g.Cols)
		}
	case 'n': // CSI ? 996 n — color scheme, from the LIVE background
		bg, ok := e.effectiveBG()
		if !ok {
			return
		}
		scheme := 1 // dark
		if luma(bg) >= 128 {
			scheme = 2 // light
		}
		_, _ = fmt.Fprintf(w, "\x1b[?997;%dn", scheme)
	case 'q':
		_, _ = w.Write(xtgettcapResponse(ev.params))
	case 'r':
		// DECRQSS for a setting nobody here introspects: the immediate
		// "invalid" (xterm convention: 0 = invalid, 1 = valid) ends the
		// app's reply timeout without inventing state.
		_, _ = w.Write([]byte("\x1bP0$r\x1b\\"))
	}
}

// luma is the BT.709 luminance of a color, 0–255: the dark/light line
// for the color-scheme report sits at 128.
func luma(c vtengine.RGB) int {
	return (2126*int(c.R) + 7152*int(c.G) + 722*int(c.B)) / 10000
}

// xtgettcapResponse builds the XTGETTCAP reply: DCS 1 + r name=value ST
// carrying every known capability (hex-encoded like the request), or
// DCS 0 + r ST when nothing matched. An immediate "no" ends the app's
// reply timeout just as well as a "yes".
func xtgettcapResponse(payload string) []byte {
	var known []string
	for _, h := range strings.Split(payload, ";") {
		name, err := hex.DecodeString(h)
		if err != nil {
			continue
		}
		value, isBool, ok := tcapValue(string(name))
		if !ok {
			continue
		}
		if isBool {
			known = append(known, h)
			continue
		}
		known = append(known, h+"="+hex.EncodeToString([]byte(value)))
	}
	if len(known) == 0 {
		return []byte("\x1bP0+r\x1b\\")
	}
	return []byte("\x1bP1+r" + strings.Join(known, ";") + "\x1b\\")
}
