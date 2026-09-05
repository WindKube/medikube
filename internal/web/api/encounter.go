package api

import (
	"fmt"
	"slices"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/service/encounter"
	"medikube/internal/web"
)

const (
	EncounterMemberReason       = "reason"
	EncounterMemberOccurredOn   = "occurred_on"
	EncounterMemberVisitType    = "visit_type"
	EncounterMemberPriority     = "priority"
	EncounterMemberAssessment   = "assessment"
	EncounterMemberPlan         = "plan"
	EncounterMemberFollowUp     = "follow_up"
	EncounterMemberDurationMin  = "duration_minutes"
	EncounterMemberPractitioner = "practitioner"
	EncounterMemberFacility     = "facility"
	EncounterMemberCondition    = "condition"
	EncounterMemberNotes        = "notes"
)

var encounterMembers = []string{
	EncounterMemberReason, EncounterMemberOccurredOn, EncounterMemberVisitType,
	EncounterMemberPriority, EncounterMemberAssessment, EncounterMemberPlan,
	EncounterMemberFollowUp, EncounterMemberDurationMin,
	EncounterMemberPractitioner, EncounterMemberFacility, EncounterMemberCondition, EncounterMemberNotes,
}

// EncounterSummary is what the list operations return.
type EncounterSummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	Reason     string  `json:"reason"`
	OccurredOn *string `json:"occurred_on"`
	VisitType  string  `json:"visit_type,omitempty"`
	Priority   string  `json:"priority,omitempty"`
	UpdatedAt  string  `json:"updated_at"`
}

// Encounter is what the detail operations return.
type Encounter struct {
	EncounterSummary

	Patient string `json:"patient"`

	// Assessment and Plan are kept separate from Reason and are never mapped
	// to or from a diagnosed condition (FR-023).
	Assessment  string `json:"assessment,omitempty"`
	Plan        string `json:"plan,omitempty"`
	FollowUp    string `json:"follow_up,omitempty"`
	DurationMin int    `json:"duration_minutes,omitempty"`

	Practitioner string `json:"practitioner,omitempty"`
	Facility     string `json:"facility,omitempty"`
	Condition    string `json:"condition,omitempty"`

	Notes     string `json:"notes,omitempty"`
	CreatedAt string `json:"created_at"`
}

type EncounterCreate struct {
	Patient     string  `json:"patient"`
	Reason      string  `json:"reason"`
	OccurredOn  *string `json:"occurred_on,omitempty"`
	VisitType   string  `json:"visit_type,omitempty"`
	Priority    string  `json:"priority,omitempty"`
	Assessment  string  `json:"assessment,omitempty"`
	Plan        string  `json:"plan,omitempty"`
	FollowUp    string  `json:"follow_up,omitempty"`
	DurationMin int     `json:"duration_minutes,omitempty"`
	Notes       string  `json:"notes,omitempty"`

	Practitioner *string `json:"practitioner,omitempty"`
	Facility     *string `json:"facility,omitempty"`
	Condition    *string `json:"condition,omitempty"`
}

type EncounterPatch struct {
	Reason      *string              `json:"reason,omitempty"`
	OccurredOn  web.Optional[string] `json:"occurred_on,omitzero"`
	VisitType   *string              `json:"visit_type,omitempty"`
	Priority    *string              `json:"priority,omitempty"`
	Assessment  *string              `json:"assessment,omitempty"`
	Plan        *string              `json:"plan,omitempty"`
	FollowUp    *string              `json:"follow_up,omitempty"`
	DurationMin *int                 `json:"duration_minutes,omitempty"`
	Notes       *string              `json:"notes,omitempty"`

	Practitioner *string `json:"practitioner,omitempty"`
	Facility     *string `json:"facility,omitempty"`
	Condition    *string `json:"condition,omitempty"`
}

type EncounterCodec struct{}

var _ encounter.Codec = EncounterCodec{}

func EncounterSchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(EncounterSummary) },
		NewDetail:  func() any { return new(Encounter) },
		NewCreate:  func() any { return new(EncounterCreate) },
		NewPatch:   func() any { return new(EncounterPatch) },
	}
}

func EncounterSearchFields(body any) (title, text string) {
	e, ok := body.(*Encounter)
	if !ok {
		return "", ""
	}

	return e.Reason, e.Assessment + " " + e.Plan + " " + e.FollowUp + " " + e.Notes
}

func EncounterBasis(any, records.Criteria) []string { return nil }

func (EncounterCodec) Summary(e clinical.Encounter) any {
	return &EncounterSummary{
		ID:         e.ID,
		Kind:       kind.Encounter.Enum(),
		Reason:     e.Reason,
		OccurredOn: wireDate(e.OccurredOn),
		VisitType:  string(e.VisitType),
		Priority:   string(e.Priority),
		UpdatedAt:  wireInstant(e.UpdatedAt),
	}
}

func (c EncounterCodec) Detail(e clinical.Encounter) any {
	summary, ok := c.Summary(e).(*EncounterSummary)
	if !ok {
		return &Encounter{}
	}

	return &Encounter{
		EncounterSummary: *summary,
		Patient:          e.PatientID,
		Assessment:       e.Assessment,
		Plan:             e.Plan,
		FollowUp:         e.FollowUp,
		DurationMin:      e.DurationMin,
		Practitioner:     e.PractitionerID,
		Facility:         e.FacilityID,
		Condition:        e.ConditionID,
		Notes:            e.Notes,
		CreatedAt:        wireInstant(e.CreatedAt),
	}
}

func (EncounterCodec) Draft(body any) (clinical.Encounter, error) {
	create, ok := body.(*EncounterCreate)
	if !ok {
		return clinical.Encounter{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	occurredOn := readDate(&invalid, EncounterMemberOccurredOn, create.OccurredOn)

	if err := orderedEncounterRefusal(&invalid); err != nil {
		return clinical.Encounter{}, err
	}

	return clinical.Encounter{
		PatientID:      create.Patient,
		Reason:         create.Reason,
		OccurredOn:     occurredOn,
		VisitType:      clinical.VisitType(create.VisitType),
		Priority:       clinical.VisitPriority(create.Priority),
		Assessment:     create.Assessment,
		Plan:           create.Plan,
		FollowUp:       create.FollowUp,
		DurationMin:    create.DurationMin,
		Notes:          create.Notes,
		PractitionerID: deref(create.Practitioner),
		FacilityID:     deref(create.Facility),
		ConditionID:    deref(create.Condition),
	}, nil
}

func (EncounterCodec) Patch(body any) (encounter.Patch, error) {
	incoming, ok := body.(*EncounterPatch)
	if !ok {
		return encounter.Patch{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	patch := encounter.Patch{
		Reason:       incoming.Reason,
		OccurredOn:   readOptionalDate(&invalid, EncounterMemberOccurredOn, incoming.OccurredOn),
		VisitType:    convert[clinical.VisitType](incoming.VisitType),
		Priority:     convert[clinical.VisitPriority](incoming.Priority),
		Assessment:   incoming.Assessment,
		Plan:         incoming.Plan,
		FollowUp:     incoming.FollowUp,
		DurationMin:  incoming.DurationMin,
		Notes:        incoming.Notes,
		Practitioner: incoming.Practitioner,
		Facility:     incoming.Facility,
		Condition:    incoming.Condition,
	}

	if err := orderedEncounterRefusal(&invalid); err != nil {
		return encounter.Patch{}, err
	}

	return patch, nil
}

func orderedEncounterRefusal(invalid *domain.ValidationError) error {
	if invalid.Empty() {
		return nil
	}

	slices.SortStableFunc(invalid.Fields, func(left, right domain.FieldError) int {
		return slices.Index(encounterMembers, left.Field) - slices.Index(encounterMembers, right.Field)
	})

	return invalid.OrNil()
}
