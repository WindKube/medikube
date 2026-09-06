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

// MedicationLinkOption is one of a patient's own medications, offered by the
// editor's picker.
type MedicationLinkOption struct {
	ID   string
	Name string
}

// MedicationLinkRole is one medication-relation field the editor writes.
// Allergy, condition and injury each have exactly one, unlabelled; symptom has
// two (FR-032), each labelled so the role a medication is added under is
// never ambiguous.
type MedicationLinkRole struct {
	Field string
	Label string
	Items []LinkedRecordItem
	IDs   []string
}

// MedicationLinksEditorProps is FR-055's editor: usable from either end, this
// end's own multi-relation field(s), read and written through the record's
// own generic PATCH (replace-set, If-Match, FR-056).
//
// Every button computes its own complete payload server-side — the ids to
// keep, or the ids to keep plus whichever medication is picked — rather than
// tracking an array in a client-side signal, so there is nothing here to fall
// out of sync with what the record actually holds.
type MedicationLinksEditorProps struct {
	ID         string
	Title      string
	RecordHref string
	Options    []MedicationLinkOption
	Roles      []MedicationLinkRole
}

func (p MedicationLinksEditorProps) addMedicationSignal() string { return p.ID + "_add_medication" }
func (p MedicationLinksEditorProps) addRoleSignal() string       { return p.ID + "_add_role" }

// medicationRoleSignals declares $_add_role's initial value, defaulted to the
// first role — a symptom's select otherwise binds to an empty string until
// touched, and Add would then have no role to act on.
func medicationRoleSignals(p MedicationLinksEditorProps) string {
	if len(p.Roles) < 2 {
		return p.addRoleSignal() + ": ''"
	}

	return p.addRoleSignal() + ": " + jsLiteral(p.Roles[0].Field)
}

// removeExpr answers Remove: every id this role holds now, minus the one
// removed, PATCHed as that field alone — every other role's field is
// untouched by omission, since a PATCH body says nothing about a field it did
// not name.
func (p MedicationLinksEditorProps) removeExpr(role MedicationLinkRole, medicationID string) string {
	remaining := make([]string, 0, len(role.IDs))
	for _, id := range role.IDs {
		if id != medicationID {
			remaining = append(remaining, id)
		}
	}

	return patchExpr(p.RecordHref, jsObject(jsField{role.Field, jsArray(remaining)}))
}

// addExpr appends whichever medication $_add_medication holds to whichever
// role is picked — the record's only role for allergy/condition/injury, or
// whichever $_add_role names for symptom — leaving every other role's field
// out of the payload, same as removeExpr.
func (p MedicationLinksEditorProps) addExpr() string {
	medication := "$" + p.addMedicationSignal()

	if len(p.Roles) == 1 {
		role := p.Roles[0]
		payload := jsObject(jsField{role.Field, jsArrayAppend(role.IDs, medication)})

		return medication + " ? " + patchExpr(p.RecordHref, payload) + " : ''"
	}

	role := "$" + p.addRoleSignal()
	var branches string

	for i, candidate := range p.Roles {
		payload := jsObject(jsField{candidate.Field, jsArrayAppend(candidate.IDs, medication)})
		call := patchExpr(p.RecordHref, payload)

		if i > 0 {
			branches += " : "
		}
		branches += "(" + role + " === " + jsLiteral(candidate.Field) + ") ? " + call
	}

	return medication + " ? (" + branches + " : '') : ''"
}

type jsField struct {
	name, value string
}

func jsObject(fields ...jsField) string {
	out := "{"
	for i, field := range fields {
		if i > 0 {
			out += ", "
		}
		out += field.name + ": " + field.value
	}
	return out + "}"
}

func jsArray(values []string) string {
	out := "["
	for i, value := range values {
		if i > 0 {
			out += ", "
		}
		out += jsLiteral(value)
	}
	return out + "]"
}

// jsArrayAppend is jsArray with one more, non-literal element appended — a
// signal reference rather than a server-known string.
func jsArrayAppend(values []string, expr string) string {
	out := "["
	for _, value := range values {
		out += jsLiteral(value) + ", "
	}
	return out + expr + "]"
}

func patchExpr(href, payload string) string {
	return "@patch(" + jsLiteral(href) + ", {headers: {'If-Match': $_etag}, payload: " + payload + "})"
}

// RemovableLink is one back-reference to a medication, rendered on the
// medication's own page (FR-059's other end): openable, and removable through
// a PATCH of the record that names it — never of the medication itself,
// which does not own the relation.
type RemovableLink struct {
	Kind     string
	Summary  string
	Href     string
	RemoveOn string
}

type RemovableLinksProps struct {
	ID    string
	Title string
	Items []RemovableLink
}

// MedicationRemoveExpr builds a RemovableLink's RemoveOn: a PATCH of the
// linking record's own href, replacing its field with every id but the one
// being removed, then a reload so the medication's own page reflects it — the
// response patches the OTHER record's elements, none of which are on this
// page to receive it.
func MedicationRemoveExpr(href, etag, field string, ids []string, remove string) string {
	remaining := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != remove {
			remaining = append(remaining, id)
		}
	}

	return "@patch(" + jsLiteral(href) + ", {headers: {'If-Match': " + jsLiteral(etag) +
		"}, payload: " + jsObject(jsField{field, jsArray(remaining)}) + "}).then(() => window.location.reload())"
}

// CourseMedicationRemoveExpr is MedicationRemoveExpr's twin for the one
// payload-carrying join: DELETE and not PATCH, because the link itself is the
// record being removed rather than a member of somebody else's set.
func CourseMedicationRemoveExpr(href, etag string) string {
	return "@delete(" + jsLiteral(href) + ", {headers: {'If-Match': " + jsLiteral(etag) +
		"}}).then(() => window.location.reload())"
}
