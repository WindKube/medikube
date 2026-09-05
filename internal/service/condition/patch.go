package condition

import (
	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
)

// Patch is a change to a condition: every field optional.
type Patch struct {
	Diagnosis  *string
	Status     *clinical.ConditionStatus
	Severity   *clinical.Severity
	OnsetOn    *domain.Date
	ResolvedOn *domain.Date
	ICD10Code  *string
	SNOMEDCode *string
	Notes      *string

	Practitioner  *string
	MedicationIDs *[]string
}

func (p Patch) applyTo(entity clinical.Condition) clinical.Condition {
	assign(&entity.Diagnosis, p.Diagnosis)
	assign(&entity.Status, p.Status)
	assign(&entity.Severity, p.Severity)
	assign(&entity.OnsetOn, p.OnsetOn)
	assign(&entity.ResolvedOn, p.ResolvedOn)
	assign(&entity.ICD10Code, p.ICD10Code)
	assign(&entity.SNOMEDCode, p.SNOMEDCode)
	assign(&entity.Notes, p.Notes)
	assign(&entity.PractitionerID, p.Practitioner)

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
