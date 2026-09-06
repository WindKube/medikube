package records

import "medikube/internal/web"

// CourseMedicationEffectiveView is FR-060's `{value, source}` pair rendered
// for one field: Value is already the display string (a date formatted, a
// plain string as-is) and Source is one of "course", "medication" or "none" —
// api.CourseMedicationEffective's own vocabulary, carried through unchanged
// rather than re-decided here.
type CourseMedicationEffectiveView struct {
	Value  string
	Source string
}

// courseMedicationSourceLabel turns the wire vocabulary into the message id
// for the word a reader sees next to the value; the templ that prints it
// resolves it with i18n.T at render.
func courseMedicationSourceLabel(source string) string {
	switch source {
	case "course":
		return "field.course_medication.source_course"
	case "medication":
		return "field.course_medication.source_medication"
	default:
		return "field.course_medication.source_none"
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

func (p CourseMedicationFormProps) medicationSignal() string { return signalBase(p.ID) + "_medication" }
func (p CourseMedicationFormProps) dosageSignal() string     { return signalBase(p.ID) + "_dosage" }
func (p CourseMedicationFormProps) frequencySignal() string  { return signalBase(p.ID) + "_frequency" }
func (p CourseMedicationFormProps) startedOnSignal() string  { return signalBase(p.ID) + "_started_on" }
func (p CourseMedicationFormProps) endedOnSignal() string    { return signalBase(p.ID) + "_ended_on" }

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
		", {headers: {'If-Match': " + jsLiteral(web.ETag(p.Etag)) + "}, payload: " + payload +
		"}).then(() => window.location.reload())"

	return medication + " ? (" + call + ") : ''"
}

// Fields is every effective field this row carries, labelled for rendering.
// A field whose value is empty is left out entirely (FR-024's convention,
// the same one DetailEntry follows for a plain field).
func (r CourseMedicationRow) Fields() []courseMedicationField {
	candidates := []courseMedicationField{
		{Label: "field.course_medication.dosage", Effective: r.Dosage},
		{Label: "field.course_medication.frequency", Effective: r.Frequency},
		{Label: "field.course_medication.duration", Effective: r.Duration},
		{Label: "field.course_medication.timing", Effective: r.Timing},
		{Label: "field.course_medication.prescriber", Effective: r.Prescriber},
		{Label: "field.course_medication.pharmacy", Effective: r.Pharmacy},
		{Label: "field.course_medication.started", Effective: r.StartedOn},
		{Label: "field.course_medication.ended", Effective: r.EndedOn},
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
