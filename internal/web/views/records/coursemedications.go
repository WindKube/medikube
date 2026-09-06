package records

// CourseMedicationEffectiveView is FR-060's `{value, source}` pair rendered
// for one field: Value is already the display string (a date formatted, a
// plain string as-is) and Source is one of "course", "medication" or "none" —
// api.CourseMedicationEffective's own vocabulary, carried through unchanged
// rather than re-decided here.
type CourseMedicationEffectiveView struct {
	Value  string
	Source string
}

// courseMedicationSourceLabel turns the wire vocabulary into the word a
// reader sees next to the value.
func courseMedicationSourceLabel(source string) string {
	switch source {
	case "course":
		return "this course"
	case "medication":
		return "the medication"
	default:
		return "not set"
	}
}

// CourseMedicationRow is one medication attached to a treatment (FR-060,
// FR-061): the medication it resolves against and every effective field with
// its provenance.
type CourseMedicationRow struct {
	MedicationID   string
	MedicationName string
	MedicationHref string
	RemoveOn       string

	Dosage     CourseMedicationEffectiveView
	Frequency  CourseMedicationEffectiveView
	Duration   CourseMedicationEffectiveView
	Timing     CourseMedicationEffectiveView
	Prescriber CourseMedicationEffectiveView
	Pharmacy   CourseMedicationEffectiveView
	StartedOn  CourseMedicationEffectiveView
	EndedOn    CourseMedicationEffectiveView
}

// CourseMedicationFormProps is FR-060/FR-061's upsert form: pick one of the
// patient's own medications not already attached, and optionally override
// dosage, frequency, when it started and when it ended — every other field
// falls back to the medication's own value by being left out of the PUT body
// entirely (contracts/treatment-medications.md §2).
type CourseMedicationFormProps struct {
	ID         string
	UpsertBase string
	Etag       string
	Options    []MedicationLinkOption
}

func (p CourseMedicationFormProps) medicationSignal() string { return p.ID + "_medication" }
func (p CourseMedicationFormProps) dosageSignal() string     { return p.ID + "_dosage" }
func (p CourseMedicationFormProps) frequencySignal() string  { return p.ID + "_frequency" }
func (p CourseMedicationFormProps) startedOnSignal() string  { return p.ID + "_started_on" }
func (p CourseMedicationFormProps) endedOnSignal() string    { return p.ID + "_ended_on" }

// submitExpr PUTs whichever medication is picked with whichever overrides
// were typed, each falling back to null (and so, server-side, to the
// medication's own value) when left blank.
func (p CourseMedicationFormProps) submitExpr() string {
	medication := "$" + p.medicationSignal()

	payload := jsObject(
		jsField{"dosage", "($" + p.dosageSignal() + " || null)"},
		jsField{"frequency", "($" + p.frequencySignal() + " || null)"},
		jsField{"started_on", "($" + p.startedOnSignal() + " || null)"},
		jsField{"ended_on", "($" + p.endedOnSignal() + " || null)"},
	)

	call := "@put(" + jsLiteral(p.UpsertBase) + " + " + medication +
		", {headers: {'If-Match': " + jsLiteral(p.Etag) + "}, payload: " + payload +
		"}).then(() => window.location.reload())"

	return medication + " ? (" + call + ") : ''"
}

// Fields is every effective field this row carries, labelled for rendering.
// A field whose value is empty is left out entirely (FR-024's convention,
// the same one DetailEntry follows for a plain field).
func (r CourseMedicationRow) Fields() []courseMedicationField {
	candidates := []courseMedicationField{
		{Label: "Dosage", Effective: r.Dosage},
		{Label: "Frequency", Effective: r.Frequency},
		{Label: "Duration", Effective: r.Duration},
		{Label: "Timing", Effective: r.Timing},
		{Label: "Prescriber", Effective: r.Prescriber},
		{Label: "Pharmacy", Effective: r.Pharmacy},
		{Label: "Started", Effective: r.StartedOn},
		{Label: "Ended", Effective: r.EndedOn},
	}

	fields := make([]courseMedicationField, 0, len(candidates))

	for _, field := range candidates {
		if field.Effective.Value == "" {
			continue
		}

		fields = append(fields, field)
	}

	return fields
}

type courseMedicationField struct {
	Label     string
	Effective CourseMedicationEffectiveView
}
