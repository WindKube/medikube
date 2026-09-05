package encounter

import (
	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
)

type Patch struct {
	Reason      *string
	OccurredOn  *domain.Date
	VisitType   *clinical.VisitType
	Priority    *clinical.VisitPriority
	Assessment  *string
	Plan        *string
	FollowUp    *string
	DurationMin *int

	Practitioner *string
	Facility     *string

	Notes *string
}

func (p Patch) applyTo(e clinical.Encounter) clinical.Encounter {
	assign(&e.Reason, p.Reason)
	assign(&e.OccurredOn, p.OccurredOn)
	assign(&e.VisitType, p.VisitType)
	assign(&e.Priority, p.Priority)
	assign(&e.Assessment, p.Assessment)
	assign(&e.Plan, p.Plan)
	assign(&e.FollowUp, p.FollowUp)
	assign(&e.DurationMin, p.DurationMin)
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
