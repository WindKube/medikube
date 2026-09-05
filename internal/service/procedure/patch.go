package procedure

import (
	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
)

type Patch struct {
	Name        *string
	Type        *clinical.ProcedureType
	Code        *string
	Description *string
	OccurredOn  *domain.Date
	Status      *clinical.OrderStatus
	Outcome     *clinical.ProcedureOutcome
	Setting     *clinical.ProcedureSetting

	Complications   *string
	DurationMin     *int
	Anesthesia      *clinical.Anesthesia
	AnesthesiaNotes *string

	Practitioner *string
	Facility     *string

	Notes *string
}

func (p Patch) applyTo(e clinical.Procedure) clinical.Procedure {
	assign(&e.Name, p.Name)
	assign(&e.Type, p.Type)
	assign(&e.Code, p.Code)
	assign(&e.Description, p.Description)
	assign(&e.OccurredOn, p.OccurredOn)
	assign(&e.Status, p.Status)
	assign(&e.Outcome, p.Outcome)
	assign(&e.Setting, p.Setting)
	assign(&e.Complications, p.Complications)
	assign(&e.DurationMin, p.DurationMin)
	assign(&e.Anesthesia, p.Anesthesia)
	assign(&e.AnesthesiaNotes, p.AnesthesiaNotes)
	assign(&e.PractitionerID, p.Practitioner)
	assign(&e.FacilityID, p.Facility)
	assign(&e.Notes, p.Notes)

	return e
}

func assign[T any](field *T, supplied *T) {
	if supplied != nil {
		*field = *supplied
	}
}
