// Package search renders US8's one page (contracts/pages.md §3): one search
// over a named person's whole chart, grouped by kind, each group paged on
// its own. It is deliberately not part of internal/records — search is not a
// registered kind, and the view model below is built straight from
// internal/service/search's own Result rather than from a hydrated record.
package search

// Item is one matched row, as the page renders it.
type Item struct {
	Title      string
	OccurredOn string
	Href       string
	Tags       []string
}

// Group is one kind's page of results.
type Group struct {
	Kind         string
	Label        string
	Items        []Item
	LoadMoreHref string
}

// Chip is one active narrowing, removable by following its href.
type Chip struct {
	Label string
	Href  string
}

// Props is the whole page. Exactly one of NoPatient, NoTerm or Groups/empty
// reason is the state actually rendered — the three named in
// contracts/pages.md §5 and US8 scenario 2.
type Props struct {
	FormAction string
	PatientID  string
	// Query re-populates the search box with what was typed, and is folded
	// into every chip/load-more/clear href so a narrowing action continues
	// the same search (contracts/search.md §1's `q` travels with every one
	// of those, the same way it travelled in the request that produced this
	// page). FR-075/research D-12's prohibition is on the API's own
	// `criteria` object — the JSON response and every log line, span,
	// metric and audit entry never carry it (SearchLimit test and the
	// phileak suite's API leg both police that) — not on a same-account
	// page continuing to search for what its own address bar already reads.
	Query string

	// NoPatient is FR-070/US8-3: no person named, so nothing is searched and
	// nothing is guessed.
	NoPatient bool
	// NoTerm is the seeded, pre-search state (contracts/pages.md §5): a
	// person is named but no term has been typed yet.
	NoTerm bool

	Chips  []Chip
	Groups []Group

	// EmptyReason is "", "no_matches" or "no_records" — the two states US8
	// scenario 2 requires to read differently. Meaningful only when NoPatient
	// and NoTerm are both false.
	EmptyReason string
	ClearHref   string
}
