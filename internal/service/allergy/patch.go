package allergy

import (
	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
)

// Patch is a change to an allergy: every field optional.
type Patch struct {
	Allergen *string
	Reaction *string
	Severity *clinical.Severity
	Status   *clinical.ConditionStatus
	OnsetOn  *domain.Date
	Notes    *string

	// MedicationIDs replaces the whole set when supplied (FR-056 replace-set
	// semantics). nil leaves it alone; a non-nil, empty slice clears it.
	MedicationIDs *[]string
}

func (p Patch) applyTo(entity clinical.Allergy) clinical.Allergy {
	assign(&entity.Allergen, p.Allergen)
	assign(&entity.Reaction, p.Reaction)
	assign(&entity.Severity, p.Severity)
	assign(&entity.Status, p.Status)
	assign(&entity.OnsetOn, p.OnsetOn)
	assign(&entity.Notes, p.Notes)

	if p.MedicationIDs != nil {
		entity.MedicationIDs = *p.MedicationIDs
	}

	return entity
}

func assign[T any](field *T, supplied *T) {
	if supplied != nil {
		*field = *supplied
	}
}
