package api

import (
	"fmt"
	"slices"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/service/treatment"
	"medikube/internal/web"
)

const (
	TreatmentMemberName            = "name"
	TreatmentMemberType            = "type"
	TreatmentMemberSetting         = "setting"
	TreatmentMemberDescription     = "description"
	TreatmentMemberStartedOn       = "started_on"
	TreatmentMemberEndedOn         = "ended_on"
	TreatmentMemberFrequency       = "frequency"
	TreatmentMemberDosage          = "dosage"
	TreatmentMemberExpectedOutcome = "expected_outcome"
	TreatmentMemberStatus          = "status"
	TreatmentMemberPractitioner    = "practitioner"
	TreatmentMemberFacility        = "facility"
	TreatmentMemberCondition       = "condition"
	TreatmentMemberNotes           = "notes"
)

// Named after the collections they relate to, per data-model §4.5 (FR-028),
// so declared from the kind table rather than spelled a second time here.
var (
	TreatmentMemberEncounters = kind.Encounter.Collection()
	TreatmentMemberEquipment  = kind.Equipment.Collection()
)

var treatmentMembers = []string{
	TreatmentMemberName, TreatmentMemberType, TreatmentMemberSetting, TreatmentMemberDescription,
	TreatmentMemberStartedOn, TreatmentMemberEndedOn, TreatmentMemberFrequency, TreatmentMemberDosage,
	TreatmentMemberExpectedOutcome, TreatmentMemberStatus, TreatmentMemberPractitioner,
	TreatmentMemberFacility, TreatmentMemberCondition, TreatmentMemberEncounters, TreatmentMemberEquipment,
	TreatmentMemberNotes,
}

type TreatmentSummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	Name      string  `json:"name"`
	Status    string  `json:"status,omitempty"`
	StartedOn *string `json:"started_on"`
	UpdatedAt string  `json:"updated_at"`
}

type Treatment struct {
	TreatmentSummary

	Patient string `json:"patient"`

	Type            string  `json:"type,omitempty"`
	Setting         string  `json:"setting,omitempty"`
	Description     string  `json:"description,omitempty"`
	EndedOn         *string `json:"ended_on"`
	Frequency       string  `json:"frequency,omitempty"`
	Dosage          string  `json:"dosage,omitempty"`
	ExpectedOutcome string  `json:"expected_outcome,omitempty"`

	Practitioner string   `json:"practitioner,omitempty"`
	Facility     string   `json:"facility,omitempty"`
	Condition    string   `json:"condition,omitempty"`
	Encounters   []string `json:"encounters"`
	Equipment    []string `json:"equipment"`

	Notes     string   `json:"notes,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	CreatedAt string   `json:"created_at"`
}

// GetTags implements records.Tagged so the search index stays in step with
// this record's tags on write (T164-T177 follow-up).
func (t *Treatment) GetTags() []string { return t.Tags }

type TreatmentCreate struct {
	Patient         string  `json:"patient"`
	Name            string  `json:"name"`
	Type            string  `json:"type,omitempty"`
	Setting         string  `json:"setting,omitempty"`
	Description     string  `json:"description,omitempty"`
	StartedOn       *string `json:"started_on,omitempty"`
	EndedOn         *string `json:"ended_on,omitempty"`
	Frequency       string  `json:"frequency,omitempty"`
	Dosage          string  `json:"dosage,omitempty"`
	ExpectedOutcome string  `json:"expected_outcome,omitempty"`
	Status          string  `json:"status,omitempty"`
	Notes           string  `json:"notes,omitempty"`

	Practitioner *string  `json:"practitioner,omitempty"`
	Facility     *string  `json:"facility,omitempty"`
	Condition    *string  `json:"condition,omitempty"`
	Encounters   []string `json:"encounters,omitempty"`
	Equipment    []string `json:"equipment,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable: a create always supplies its tags,
// even when that is none.
func (c *TreatmentCreate) TagIDs() (ids []string, supplied bool) { return c.Tags, true }

type TreatmentPatch struct {
	Name            *string              `json:"name,omitempty"`
	Type            *string              `json:"type,omitempty"`
	Setting         *string              `json:"setting,omitempty"`
	Description     *string              `json:"description,omitempty"`
	StartedOn       web.Optional[string] `json:"started_on,omitzero"`
	EndedOn         web.Optional[string] `json:"ended_on,omitzero"`
	Frequency       *string              `json:"frequency,omitempty"`
	Dosage          *string              `json:"dosage,omitempty"`
	ExpectedOutcome *string              `json:"expected_outcome,omitempty"`
	Status          *string              `json:"status,omitempty"`
	Notes           *string              `json:"notes,omitempty"`

	Practitioner *string   `json:"practitioner,omitempty"`
	Facility     *string   `json:"facility,omitempty"`
	Condition    *string   `json:"condition,omitempty"`
	Encounters   *[]string `json:"encounters,omitempty"`
	Equipment    *[]string `json:"equipment,omitempty"`

	// Tags is replace-set (FR-064, FR-065): a nil pointer leaves the
	// applied tags alone, a non-nil one — including an empty array —
	// replaces the whole set.
	Tags *[]string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable.
func (p *TreatmentPatch) TagIDs() (ids []string, supplied bool) {
	if p.Tags == nil {
		return nil, false
	}

	return *p.Tags, true
}

type TreatmentCodec struct{}

var _ treatment.Codec = TreatmentCodec{}

func TreatmentSchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(TreatmentSummary) },
		NewDetail:  func() any { return new(Treatment) },
		NewCreate:  func() any { return new(TreatmentCreate) },
		NewPatch:   func() any { return new(TreatmentPatch) },
	}
}

func TreatmentSearchFields(body any) (title, text string) {
	t, ok := body.(*Treatment)
	if !ok {
		return "", ""
	}

	return t.Name, t.Description + " " + t.ExpectedOutcome + " " + t.Notes
}

func TreatmentBasis(any, records.Criteria) []string { return nil }

func (TreatmentCodec) Summary(t clinical.Treatment) any {
	return &TreatmentSummary{
		ID:        t.ID,
		Kind:      kind.Treatment.Enum(),
		Name:      t.Name,
		Status:    string(t.Status),
		StartedOn: wireDate(t.StartedOn),
		UpdatedAt: wireInstant(t.UpdatedAt),
	}
}

func (c TreatmentCodec) Detail(t clinical.Treatment) any {
	summary, ok := c.Summary(t).(*TreatmentSummary)
	if !ok {
		return &Treatment{}
	}

	return &Treatment{
		TreatmentSummary: *summary,
		Patient:          t.PatientID,
		Type:             t.Type,
		Setting:          string(t.Setting),
		Description:      t.Description,
		EndedOn:          wireDate(t.EndedOn),
		Frequency:        t.Frequency,
		Dosage:           t.Dosage,
		ExpectedOutcome:  t.ExpectedOutcome,
		Practitioner:     t.PractitionerID,
		Facility:         t.FacilityID,
		Condition:        t.ConditionID,
		Encounters:       nonNil(t.Encounters),
		Equipment:        nonNil(t.Equipment),
		Notes:            t.Notes,
		Tags:             t.Tags,
		CreatedAt:        wireInstant(t.CreatedAt),
	}
}

func (TreatmentCodec) Draft(body any) (clinical.Treatment, error) {
	create, ok := body.(*TreatmentCreate)
	if !ok {
		return clinical.Treatment{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	startedOn := readDate(&invalid, TreatmentMemberStartedOn, create.StartedOn)
	endedOn := readDate(&invalid, TreatmentMemberEndedOn, create.EndedOn)

	if err := orderedTreatmentRefusal(&invalid); err != nil {
		return clinical.Treatment{}, err
	}

	return clinical.Treatment{
		PatientID:       create.Patient,
		Name:            create.Name,
		Type:            create.Type,
		Setting:         clinical.TreatmentSetting(create.Setting),
		Description:     create.Description,
		StartedOn:       startedOn,
		EndedOn:         endedOn,
		Frequency:       create.Frequency,
		Dosage:          create.Dosage,
		ExpectedOutcome: create.ExpectedOutcome,
		Status:          clinical.TherapyStatus(create.Status),
		Notes:           create.Notes,
		PractitionerID:  deref(create.Practitioner),
		FacilityID:      deref(create.Facility),
		ConditionID:     deref(create.Condition),
		Encounters:      create.Encounters,
		Equipment:       create.Equipment,
		Tags:            create.Tags,
	}, nil
}

func (TreatmentCodec) Patch(body any) (treatment.Patch, error) {
	incoming, ok := body.(*TreatmentPatch)
	if !ok {
		return treatment.Patch{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	patch := treatment.Patch{
		Name:            incoming.Name,
		Type:            incoming.Type,
		Setting:         convert[clinical.TreatmentSetting](incoming.Setting),
		Description:     incoming.Description,
		StartedOn:       readOptionalDate(&invalid, TreatmentMemberStartedOn, incoming.StartedOn),
		EndedOn:         readOptionalDate(&invalid, TreatmentMemberEndedOn, incoming.EndedOn),
		Frequency:       incoming.Frequency,
		Dosage:          incoming.Dosage,
		ExpectedOutcome: incoming.ExpectedOutcome,
		Status:          convert[clinical.TherapyStatus](incoming.Status),
		Notes:           incoming.Notes,
		Practitioner:    incoming.Practitioner,
		Facility:        incoming.Facility,
		Condition:       incoming.Condition,
		Encounters:      incoming.Encounters,
		Equipment:       incoming.Equipment,
		Tags:            incoming.Tags,
	}

	if err := orderedTreatmentRefusal(&invalid); err != nil {
		return treatment.Patch{}, err
	}

	return patch, nil
}

func orderedTreatmentRefusal(invalid *domain.ValidationError) error {
	if invalid.Empty() {
		return nil
	}

	slices.SortStableFunc(invalid.Fields, func(left, right domain.FieldError) int {
		return slices.Index(treatmentMembers, left.Field) - slices.Index(treatmentMembers, right.Field)
	})

	return invalid.OrNil()
}

// nonNil renders an unset multi-relation as `[]`, never `null` (mirrors
// FR-024's treatment of every optional list on the wire).
func nonNil(ids []string) []string {
	if ids == nil {
		return []string{}
	}

	return ids
}
