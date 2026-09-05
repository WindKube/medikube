package treatment

import (
	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
)

type Patch struct {
	Name            *string
	Type            *string
	Setting         *clinical.TreatmentSetting
	Description     *string
	StartedOn       *domain.Date
	EndedOn         *domain.Date
	Frequency       *string
	Dosage          *string
	ExpectedOutcome *string
	Status          *clinical.TherapyStatus

	Practitioner *string
	Facility     *string

	// Encounters and Equipment replace the whole set when supplied (FR-056);
	// nil leaves the stored set alone.
	Encounters *[]string
	Equipment  *[]string

	Notes *string
}

func (p Patch) applyTo(e clinical.Treatment) clinical.Treatment {
	assign(&e.Name, p.Name)
	assign(&e.Type, p.Type)
	assign(&e.Setting, p.Setting)
	assign(&e.Description, p.Description)
	assign(&e.StartedOn, p.StartedOn)
	assign(&e.EndedOn, p.EndedOn)
	assign(&e.Frequency, p.Frequency)
	assign(&e.Dosage, p.Dosage)
	assign(&e.ExpectedOutcome, p.ExpectedOutcome)
	assign(&e.Status, p.Status)
	assign(&e.PractitionerID, p.Practitioner)
	assign(&e.FacilityID, p.Facility)
	assign(&e.Notes, p.Notes)

	if p.Encounters != nil {
		e.Encounters = *p.Encounters
	}

	if p.Equipment != nil {
		e.Equipment = *p.Equipment
	}

	return e
}

func assign[T any](field *T, supplied *T) {
	if supplied != nil {
		*field = *supplied
	}
}
