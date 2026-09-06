package clinical

import (
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// Injury is harm to a part of a person's body (FR-040). Like every other
// entity in this package, it never crosses the wire unrendered, and
// MarshalZerologObject is an allowlist of two identifiers.
//
// MedicationIDs is the one of FR-042's four link fields this phase stores as a
// real relation: conditions, procedures and treatments are kinds sibling
// phases of 003 add on branches this one does not have, so linking to them is
// deferred to the migration that adds them (data-model.md §8's own pattern
// for cross-kind links, applied one phase level up).
type Injury struct {
	ID string

	PatientID string

	PractitionerID string

	Name          string
	Type          InjuryType
	BodyPart      string
	Laterality    Laterality
	OccurredOn    domain.Date
	Mechanism     string
	Severity      Severity
	Status        ConditionStatus
	RecoveryNotes string

	MedicationIDs []string

	// Tags is data-model §0.8's universal field: any number of the owning
	// account's tags, applied with replace-set semantics (FR-064).
	Tags []string

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string
}

func (i Injury) MarshalZerologObject(e *zerolog.Event) {
	e.Str("injury_id", i.ID).Str("patient_id", i.PatientID)
}
