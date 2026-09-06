package overview

// Props is what the overview page needs. It stays this small deliberately:
// FR-050 asks for a short summary and two links, not a dashboard, and phase
// 003's kinds add rows to MedicationCount's family rather than members here.
type Props struct {
	// MedicationCount is the account's own medication count, resolved through
	// the same authorization checkpoint every record read passes — the same
	// counter contracts/account.md's getMe answers with, so the number here
	// and the number the API reports cannot differ.
	MedicationCount int

	// MedicationsLabel is the kind's own translated display name (nav.medications),
	// used by the "go to" link and the zero-count sentence — the count
	// sentence itself wraps kind.medication's own plural form instead, so
	// Polish's few/many/other agree with the number (D-06, FR-008).
	MedicationsLabel string

	MedicationsHref string
	SettingsHref    string
}
