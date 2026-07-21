package tape

import "fmt"

// Lint returns the compatibility warnings a Run of the tape would emit
// before touching a pty — staged/divergent settings and chord
// degradations — so tapes can be checked (CI, migrations) without
// recording anything. Runtime findings (a restless app) exist only in
// Run's Report. Mode and ModifyOtherKeys gate warnings exactly as in
// Run; the other RunOptions fields are irrelevant here.
func Lint(t *Tape, opts RunOptions) []string {
	var msgs []string
	warn := func(format string, args ...any) {
		msgs = append(msgs, fmt.Sprintf(format, args...))
	}
	warnStaged(t, opts.Mode, warn)
	if !opts.ModifyOtherKeys {
		warnDegradedChords(t, warn)
	}
	// A custom prompt's wait coordination belongs in the
	// spotting session too: validate must say what record will do.
	if settings, err := effectiveSettings(t, opts); err == nil {
		_ = promptWaitPattern(t, settings, warn)
	}
	return msgs
}
