package api

import (
	"fmt"
	"slices"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/service/procedure"
	"medikube/internal/web"
)

const (
	ProcedureMemberName            = "name"
	ProcedureMemberType            = "type"
	ProcedureMemberCode            = "code"
	ProcedureMemberDescription     = "description"
	ProcedureMemberOccurredOn      = "occurred_on"
	ProcedureMemberStatus          = "status"
	ProcedureMemberOutcome         = "outcome"
	ProcedureMemberSetting         = "setting"
	ProcedureMemberComplications   = "complications"
	ProcedureMemberDurationMin     = "duration_minutes"
	ProcedureMemberAnesthesia      = "anesthesia"
	ProcedureMemberAnesthesiaNotes = "anesthesia_notes"
	ProcedureMemberPractitioner    = "practitioner"
	ProcedureMemberFacility        = "facility"
	ProcedureMemberCondition       = "condition"
	ProcedureMemberNotes           = "notes"
)

var procedureMembers = []string{
	ProcedureMemberName, ProcedureMemberType, ProcedureMemberCode, ProcedureMemberDescription,
	ProcedureMemberOccurredOn, ProcedureMemberStatus, ProcedureMemberOutcome, ProcedureMemberSetting,
	ProcedureMemberComplications, ProcedureMemberDurationMin, ProcedureMemberAnesthesia,
	ProcedureMemberAnesthesiaNotes, ProcedureMemberPractitioner, ProcedureMemberFacility, ProcedureMemberCondition,
	ProcedureMemberNotes,
}

type ProcedureSummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	Name       string  `json:"name"`
	Status     string  `json:"status"`
	OccurredOn *string `json:"occurred_on"`
	Outcome    string  `json:"outcome,omitempty"`
	UpdatedAt  string  `json:"updated_at"`
}

type Procedure struct {
	ProcedureSummary

	Patient string `json:"patient"`

	Type            string `json:"type,omitempty"`
	Code            string `json:"code,omitempty"`
	Description     string `json:"description,omitempty"`
	Setting         string `json:"setting,omitempty"`
	Complications   string `json:"complications,omitempty"`
	DurationMin     int    `json:"duration_minutes,omitempty"`
	Anesthesia      string `json:"anesthesia,omitempty"`
	AnesthesiaNotes string `json:"anesthesia_notes,omitempty"`

	Practitioner string `json:"practitioner,omitempty"`
	Facility     string `json:"facility,omitempty"`
	Condition    string `json:"condition,omitempty"`

	Notes     string `json:"notes,omitempty"`
	CreatedAt string `json:"created_at"`
}

type ProcedureCreate struct {
	Patient         string  `json:"patient"`
	Name            string  `json:"name"`
	Type            string  `json:"type,omitempty"`
	Code            string  `json:"code,omitempty"`
	Description     string  `json:"description,omitempty"`
	OccurredOn      *string `json:"occurred_on,omitempty"`
	Status          string  `json:"status,omitempty"`
	Outcome         string  `json:"outcome,omitempty"`
	Setting         string  `json:"setting,omitempty"`
	Complications   string  `json:"complications,omitempty"`
	DurationMin     int     `json:"duration_minutes,omitempty"`
	Anesthesia      string  `json:"anesthesia,omitempty"`
	AnesthesiaNotes string  `json:"anesthesia_notes,omitempty"`
	Notes           string  `json:"notes,omitempty"`

	Practitioner *string `json:"practitioner,omitempty"`
	Facility     *string `json:"facility,omitempty"`
	Condition    *string `json:"condition,omitempty"`
}

type ProcedurePatch struct {
	Name            *string              `json:"name,omitempty"`
	Type            *string              `json:"type,omitempty"`
	Code            *string              `json:"code,omitempty"`
	Description     *string              `json:"description,omitempty"`
	OccurredOn      web.Optional[string] `json:"occurred_on,omitzero"`
	Status          *string              `json:"status,omitempty"`
	Outcome         *string              `json:"outcome,omitempty"`
	Setting         *string              `json:"setting,omitempty"`
	Complications   *string              `json:"complications,omitempty"`
	DurationMin     *int                 `json:"duration_minutes,omitempty"`
	Anesthesia      *string              `json:"anesthesia,omitempty"`
	AnesthesiaNotes *string              `json:"anesthesia_notes,omitempty"`
	Notes           *string              `json:"notes,omitempty"`

	Practitioner *string `json:"practitioner,omitempty"`
	Facility     *string `json:"facility,omitempty"`
	Condition    *string `json:"condition,omitempty"`
}

type ProcedureCodec struct{}

var _ procedure.Codec = ProcedureCodec{}

func ProcedureSchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(ProcedureSummary) },
		NewDetail:  func() any { return new(Procedure) },
		NewCreate:  func() any { return new(ProcedureCreate) },
		NewPatch:   func() any { return new(ProcedurePatch) },
	}
}

func ProcedureSearchFields(body any) (title, text string) {
	p, ok := body.(*Procedure)
	if !ok {
		return "", ""
	}

	return p.Name, p.Description + " " + p.Complications + " " + p.Notes
}

// ProcedureBasis is FR-026's row-level distinction, read off the DTO's own
// Status member.
func ProcedureBasis(body any, _ records.Criteria) []string {
	p, ok := body.(*Procedure)
	if !ok {
		return nil
	}

	return procedure.BasisFor(p.Status)
}

func (ProcedureCodec) Summary(p clinical.Procedure) any {
	return &ProcedureSummary{
		ID:         p.ID,
		Kind:       kind.Procedure.Enum(),
		Name:       p.Name,
		Status:     string(p.Status),
		OccurredOn: wireDate(p.OccurredOn),
		Outcome:    string(p.Outcome),
		UpdatedAt:  wireInstant(p.UpdatedAt),
	}
}

func (c ProcedureCodec) Detail(p clinical.Procedure) any {
	summary, ok := c.Summary(p).(*ProcedureSummary)
	if !ok {
		return &Procedure{}
	}

	return &Procedure{
		ProcedureSummary: *summary,
		Patient:          p.PatientID,
		Type:             string(p.Type),
		Code:             p.Code,
		Description:      p.Description,
		Setting:          string(p.Setting),
		Complications:    p.Complications,
		DurationMin:      p.DurationMin,
		Anesthesia:       string(p.Anesthesia),
		AnesthesiaNotes:  p.AnesthesiaNotes,
		Practitioner:     p.PractitionerID,
		Facility:         p.FacilityID,
		Condition:        p.ConditionID,
		Notes:            p.Notes,
		CreatedAt:        wireInstant(p.CreatedAt),
	}
}

func (ProcedureCodec) Draft(body any) (clinical.Procedure, error) {
	create, ok := body.(*ProcedureCreate)
	if !ok {
		return clinical.Procedure{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	occurredOn := readDate(&invalid, ProcedureMemberOccurredOn, create.OccurredOn)

	if err := orderedProcedureRefusal(&invalid); err != nil {
		return clinical.Procedure{}, err
	}

	return clinical.Procedure{
		PatientID:       create.Patient,
		Name:            create.Name,
		Type:            clinical.ProcedureType(create.Type),
		Code:            create.Code,
		Description:     create.Description,
		OccurredOn:      occurredOn,
		Status:          clinical.OrderStatus(create.Status),
		Outcome:         clinical.ProcedureOutcome(create.Outcome),
		Setting:         clinical.ProcedureSetting(create.Setting),
		Complications:   create.Complications,
		DurationMin:     create.DurationMin,
		Anesthesia:      clinical.Anesthesia(create.Anesthesia),
		AnesthesiaNotes: create.AnesthesiaNotes,
		Notes:           create.Notes,
		PractitionerID:  deref(create.Practitioner),
		FacilityID:      deref(create.Facility),
		ConditionID:     deref(create.Condition),
	}, nil
}

func (ProcedureCodec) Patch(body any) (procedure.Patch, error) {
	incoming, ok := body.(*ProcedurePatch)
	if !ok {
		return procedure.Patch{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	patch := procedure.Patch{
		Name:            incoming.Name,
		Type:            convert[clinical.ProcedureType](incoming.Type),
		Code:            incoming.Code,
		Description:     incoming.Description,
		OccurredOn:      readOptionalDate(&invalid, ProcedureMemberOccurredOn, incoming.OccurredOn),
		Status:          convert[clinical.OrderStatus](incoming.Status),
		Outcome:         convert[clinical.ProcedureOutcome](incoming.Outcome),
		Setting:         convert[clinical.ProcedureSetting](incoming.Setting),
		Complications:   incoming.Complications,
		DurationMin:     incoming.DurationMin,
		Anesthesia:      convert[clinical.Anesthesia](incoming.Anesthesia),
		AnesthesiaNotes: incoming.AnesthesiaNotes,
		Notes:           incoming.Notes,
		Practitioner:    incoming.Practitioner,
		Facility:        incoming.Facility,
		Condition:       incoming.Condition,
	}

	if err := orderedProcedureRefusal(&invalid); err != nil {
		return procedure.Patch{}, err
	}

	return patch, nil
}

func orderedProcedureRefusal(invalid *domain.ValidationError) error {
	if invalid.Empty() {
		return nil
	}

	slices.SortStableFunc(invalid.Fields, func(left, right domain.FieldError) int {
		return slices.Index(procedureMembers, left.Field) - slices.Index(procedureMembers, right.Field)
	})

	return invalid.OrNil()
}
