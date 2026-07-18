package tape

import "time"

// ResolveThemeForTest and WarnDegradedChordsForTest expose internals to
// the external test package (the exported surface stays Parse/Run only).
//
//nolint:gochecknoglobals // canonical export_test seam, test binary only
var (
	ResolveThemeForTest       = resolveTheme
	WarnDegradedChordsForTest = warnDegradedChords
)

// EffectiveSettingsForTest exposes the dress/Set layering resolver to
// the external test package: the seam that pins precedence, the -dress
// override, `none` stripping and Run's no-mutation contract.
func EffectiveSettingsForTest(t *Tape, opts RunOptions) (Settings, error) {
	return effectiveSettings(t, opts)
}

// DeclaredTotalForTest exposes the progress total: the pin that keeps
// it an exact mirror of how Run actually spends virtual time.
func DeclaredTotalForTest(t *Tape, settings Settings) time.Duration {
	return declaredTotal(t, settings)
}
