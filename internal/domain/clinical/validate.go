package clinical

import (
	"strings"
	"unicode/utf8"

	"medikube/internal/domain"
)

// CodeEndBeforeStart is FR-018's refusal, spelled as contracts/README.md's
// error envelope spells it. It lives beside the rule that raises it rather than
// in internal/domain, where only the codes more than one entity uses belong.
const CodeEndBeforeStart = "end_before_start"

// The documented maximum of every free-text field (data-model §2, FR-017).
// Counted in Unicode code points and not in bytes, so a name in a non-Latin
// script is not silently a third of the length a Latin one is allowed.
const (
	maxName            = 200
	maxAlternativeName = 200
	maxDosage          = 200
	maxFrequency       = 100
	maxIndication      = 300
	maxSideEffects     = 1000
	maxNotes           = 5000
)

// Validate reports every offending field at once (FR-027). The rules run in the
// order data-model §2 lists the columns, which is the order the form renders
// them, so the refusals arrive in the order the person reads them.
//
// FR-018's "must be a real calendar date" is not a rule here and cannot be: a
// domain.Date has no representation for 30 February, so the refusal happens
// when the submitted text is parsed at the edge and Validate is never handed a
// date the calendar does not have.
func (m Medication) Validate() error {
	var invalid domain.ValidationError

	// The name is trimmed before it is measured, so a field of spaces is not a
	// name however much whitespace it holds.
	if name := strings.TrimSpace(m.Name); name == "" {
		invalid.Add("name", domain.CodeRequired, "a name is required")
	} else {
		checkLength(&invalid, "name", "the name", name, maxName)
	}

	checkLength(&invalid, "alternative_name", "the alternative name", m.AlternativeName, maxAlternativeName)

	// The two optional selects distinguish absence from an unpublished value.
	// Absence is what an unfilled field carries; a value MediKube does not
	// publish is FR-016's refusal.
	if m.Type != "" && !m.Type.Valid() {
		invalid.Add("type", domain.CodeInvalidValue, "not one of the kinds MediKube accepts")
	}

	checkLength(&invalid, "dosage", "the dose", m.Dosage, maxDosage)
	checkLength(&invalid, "frequency", "how often it is taken", m.Frequency, maxFrequency)

	if m.Route != "" && !m.Route.Valid() {
		invalid.Add("route", domain.CodeInvalidValue, "not one of the routes MediKube accepts")
	}

	checkLength(&invalid, "indication", "the reason it is taken", m.Indication, maxIndication)

	// Equality is accepted: a single-day course is a real prescription. A
	// future start date is accepted too, which is why this entity has no clock.
	if !m.StartedOn.IsZero() && !m.EndedOn.IsZero() && m.EndedOn.Before(m.StartedOn) {
		invalid.Add("ended_on", CodeEndBeforeStart, "the end date is before the start date")
	}

	switch {
	case m.Status == "":
		invalid.Add("status", domain.CodeRequired, "a state is required")
	case !m.Status.Valid():
		invalid.Add("status", domain.CodeInvalidValue, "not one of the states MediKube accepts")
	}

	checkLength(&invalid, "side_effects", "the side effects", m.SideEffects, maxSideEffects)
	checkLength(&invalid, "notes", "the notes", m.Notes, maxNotes)

	return invalid.OrNil()
}

// The message names the field and its limit and never the text that broke it —
// that text is patient data and this message reaches the log (FR-017, VII).
func checkLength(invalid *domain.ValidationError, field, label, value string, limit int) {
	if utf8.RuneCountInString(value) > limit {
		invalid.Addf(field, domain.CodeTooLong, "%s accepts at most %d characters", label, limit)
	}
}

// requireText is checkLength plus the "required, trimmed" half phase 003's
// kinds share with Medication.Name: a field of spaces is not a value, and the
// length is measured on the trimmed text.
func requireText(invalid *domain.ValidationError, field, label, value string, minLen, maxLen int) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		invalid.Add(field, domain.CodeRequired, label+" is required")

		return
	}

	if utf8.RuneCountInString(trimmed) < minLen {
		invalid.Addf(field, domain.CodeTooShort, "%s needs at least %d characters", label, minLen)

		return
	}

	checkLength(invalid, field, label, trimmed, maxLen)
}
