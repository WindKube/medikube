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
	Condition    *string

	Notes *string

	// Tags is data-model §0.8's universal field, replace-set (FR-064,
	// FR-065): nil leaves the applied tags alone, non-nil (including empty)
	// replaces the whole set.
	Tags *[]string
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
	assign(&e.ConditionID, p.Condition)
	assign(&e.Notes, p.Notes)

	if p.Tags != nil {
		e.Tags = *p.Tags
	}

	return e
}

func assign[T any](field *T, supplied *T) {
	if supplied != nil {
		*field = *supplied
	}
}
