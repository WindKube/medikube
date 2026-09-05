package clinical

import (
	"strings"
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// Allergy is what a person reacts to, how badly, and whether it still applies
// (data-model §4.1, US1). It never crosses the wire unrendered.
type Allergy struct {
	ID        string
	PatientID string

	Allergen string
	Reaction string
	Severity Severity
	Status   ConditionStatus
	OnsetOn  domain.Date

	// MedicationIDs is the multi-relation touching every drug this allergy
	// concerns (FR-017): a drug-class allergy touches many rows.
	MedicationIDs []string

	Notes string

	// Tags is data-model §0.8's universal field: any number of the owning
	// account's tags, applied with replace-set semantics (FR-064).
	Tags []string

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string
}

// Critical is FR-018's derivation: never stored, so tightening or loosening
// the rule changes no row.
func (a Allergy) Critical() bool {
	severe := a.Severity == SeveritySevere || a.Severity == SeverityLifeThreatening
	current := a.Status == ConditionStatusActive || a.Status == ConditionStatusChronic

	return severe && current
}

// MarshalZerologObject is an allowlist of two identifiers. The allergen, the
// reaction and the notes are all patient data.
func (a Allergy) MarshalZerologObject(e *zerolog.Event) {
	e.Str("allergy_id", a.ID).Str("patient_id", a.PatientID)
}

const (
	maxAllergen = 200
	maxReaction = 500
)

// Validate reports every offending field at once (FR-016, FR-013).
func (a Allergy) Validate() error {
	var invalid domain.ValidationError

	if allergen := strings.TrimSpace(a.Allergen); allergen == "" {
		invalid.Add("allergen", domain.CodeRequired, "what the person reacts to is required")
	} else {
		checkLength(&invalid, "allergen", "the allergen", allergen, maxAllergen)
	}

	checkLength(&invalid, "reaction", "the reaction", a.Reaction, maxReaction)

	switch {
	case a.Severity == "":
		invalid.Add("severity", domain.CodeRequired, "how severe the reaction is required")
	case !a.Severity.Valid():
		invalid.Add("severity", domain.CodeInvalidValue, "not one of the severities MediKube accepts")
	}

	switch {
	case a.Status == "":
		invalid.Add("status", domain.CodeRequired, "a state is required")
	case !a.Status.Valid():
		invalid.Add("status", domain.CodeInvalidValue, "not one of the states MediKube accepts")
	}

	checkLength(&invalid, "notes", "the notes", a.Notes, maxNotes)

	return invalid.OrNil()
}
