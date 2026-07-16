package tape

// ResolveThemeForTest and WarnDegradedChordsForTest expose internals to
// the external test package (the exported surface stays Parse/Run only).
//
//nolint:gochecknoglobals // canonical export_test seam, test binary only
var (
	ResolveThemeForTest       = resolveTheme
	WarnDegradedChordsForTest = warnDegradedChords
)
