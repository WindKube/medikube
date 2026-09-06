package records

import "medikube/internal/domain/kind"

// StatusView is one row of contracts/pages.md §3.5's catalogue: a narrowing
// of one kind's own list that answers "what is currently true for a person"
// (FR-078). It is declared once, here, and read by two consumers that must
// never drift apart: the kind's own FilterSpec (already declared on its
// registration) and the page route's SmokeVariants (T183a) — a status view
// added to this table with no corresponding page route fails the build
// (internal/httproute/registry_test.go), which is the whole point.
type StatusView struct {
	// Name is the catalogue entry's own identity, used in test failure
	// messages and nowhere else.
	Name string

	Kind kind.Kind

	// Query is the exact query string (no leading "?") the status view adds
	// to the kind's own list, verbatim from contracts/pages.md §3.5.
	Query string
}

// StatusViews is the seven-entry catalogue, in contracts/pages.md §3.5's own
// order. Name is built from the kind's own Segment() rather than spelled out,
// so the plural is pinned in exactly one place (internal/domain/kind/kind.go,
// research D-05, T046).
var StatusViews = []StatusView{
	{Name: kind.Condition.Segment() + "-active", Kind: kind.Condition, Query: "active=true"},
	{Name: kind.Medication.Segment() + "-active", Kind: kind.Medication, Query: "active=true"},
	{Name: kind.Procedure.Segment() + "-scheduled", Kind: kind.Procedure, Query: "scheduled=true"},
	{Name: kind.Injury.Segment() + "-unresolved", Kind: kind.Injury, Query: "unresolved=true"},
	{Name: kind.Allergy.Segment() + "-critical", Kind: kind.Allergy, Query: "critical=true"},
	{Name: kind.Equipment.Segment() + "-service-due", Kind: kind.Equipment, Query: "service_due_within_days=30"},
	{Name: kind.Insurance.Segment() + "-expiring", Kind: kind.Insurance, Query: "expiring_within_days=60"},
}

// SmokeURL is the concrete URL the browser gate visits for this status view,
// against the given patient — no unbound parameter, exactly as
// internal/httproute's SmokeVariants requires.
func (v StatusView) SmokeURL(patientID string) string {
	return "/" + v.Kind.Segment() + "?" + v.Query + "&patient=" + patientID
}

// StatusViewFor answers the catalogue entry for a kind, if any. Most kinds
// have none, and that is not a defect: FR-078 names six status views across
// seven kinds' own lists, not fourteen.
func StatusViewFor(k kind.Kind) (StatusView, bool) {
	for _, view := range StatusViews {
		if view.Kind == k {
			return view, true
		}
	}

	return StatusView{}, false
}
