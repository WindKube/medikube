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

	// MedicationsLabel is the kind's own plural, read off kind.Medication
	// rather than spelled here (research D-05): a literal would be the second
	// place this word could drift from the route table's.
	MedicationsLabel string

	MedicationsHref string
	SettingsHref    string
}
