package records

// LinkedRecordItem is one related record rendered by links.templ: enough to
// name what it is, what to call it, and where to open it (FR-059). It is
// deliberately not tied to any one kind's DTO — a treatment's condition, a
// medication's back-reference, an allergy's linked medication all render the
// same way.
type LinkedRecordItem struct {
	Kind    string
	Summary string
	Href    string
}
