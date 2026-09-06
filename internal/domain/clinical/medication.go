package clinical

import (
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// Medication is one course of therapy as the person recorded it. It never
// crosses the wire — a DTO always mediates — and it never reaches a template
// unrendered, so it carries no tags, no JSON names and no display strings.
//
// Most of this struct is patient data. The name of a drug discloses a
// condition, the reason it is taken names one outright, and the notes hold
// whatever the person chose to write down. That is why MarshalZerologObject
// below is an allowlist of two identifiers rather than a convenience.
type Medication struct {
	ID string

	// The authorization anchor and the cascade parent (phase 002 research
	// D-13). Server-set on create and absent from the patch DTO entirely, so a
	// request can neither nominate nor change it.
	PatientID string

	// The prescriber and the pharmacy. Both optional and both auto-unset
	// rather than cascading when the practitioner or facility they name is
	// deleted (data-model §6): a medication survives the deletion of who
	// prescribed it or where it was filled.
	PractitionerID string
	PharmacyID     string

	Name            string
	AlternativeName string
	Type            MedicationType
	Dosage          string
	Frequency       string
	Route           MedicationRoute
	Indication      string

	// The absent date is the zero value, which is what an optional date column
	// holds. There is no pointer here because domain.Date already distinguishes
	// "not recorded" from every real calendar day.
	StartedOn domain.Date
	EndedOn   domain.Date

	Status      TherapyStatus
	SideEffects string
	Notes       string

	// Tags is data-model §0.8's universal field: any number of the owning
	// account's tags, applied with replace-set semantics (FR-064).
	Tags []string

	CreatedAt time.Time
	UpdatedAt time.Time

	// Version is the ETag source, derived from UpdatedAt by the store and never
	// a column of its own (research D-24). It is carried on the entity because
	// the handler that renders the ETag and the repository that checks
	// If-Match both need the value the read produced.
	Version string
}

// IsCurrent is data-model §2's derived is_current: the list narrows by it and
// the detail view heads with it. A method rather than a field, because it is a
// function of Status and a stored copy is a second truth to keep in step.
func (m Medication) IsCurrent() bool { return m.Status == TherapyStatusActive }

// MarshalZerologObject emits the two identifiers and nothing else.
//
// This is what makes FR-038 structural. Logging a medication is a reasonable
// thing for a handler or a hook to do, and the only reason it cannot leak a
// drug name, a dose, a reason or a note is that this method never had a line
// that could. Adding one here is the whole failure — there is no second gate.
func (m Medication) MarshalZerologObject(e *zerolog.Event) {
	e.Str("medication_id", m.ID).Str("patient_id", m.PatientID)
}
