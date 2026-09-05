package api

import (
	"fmt"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/service/condition"
	"medikube/internal/web"
)

const (
	MemberDiagnosis  = "diagnosis"
	MemberResolvedOn = "resolved_on"
	MemberICD10Code  = "icd10_code"
	MemberSNOMEDCode = "snomed_code"
)

var conditionMembers = []string{
	MemberPatient,
	MemberDiagnosis,
	MemberStatus,
	MemberSeverity,
	MemberOnsetOn,
	MemberResolvedOn,
	MemberICD10Code,
	MemberSNOMEDCode,
	MemberNotes,
	MemberPractitioner,
}

type ConditionSummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	Diagnosis string  `json:"diagnosis"`
	Status    string  `json:"status"`
	Severity  string  `json:"severity,omitempty"`
	OnsetOn   *string `json:"onset_on"`
	UpdatedAt string  `json:"updated_at"`
}

type Condition struct {
	ConditionSummary

	Patient      string  `json:"patient"`
	ResolvedOn   *string `json:"resolved_on"`
	ICD10Code    string  `json:"icd10_code,omitempty"`
	SNOMEDCode   string  `json:"snomed_code,omitempty"`
	Practitioner string  `json:"practitioner,omitempty"`
	Notes        string  `json:"notes,omitempty"`

	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
}

type ConditionCreate struct {
	Patient      string   `json:"patient"`
	Diagnosis    string   `json:"diagnosis"`
	Status       string   `json:"status"`
	Severity     string   `json:"severity,omitempty"`
	OnsetOn      *string  `json:"onset_on,omitempty"`
	ResolvedOn   *string  `json:"resolved_on,omitempty"`
	ICD10Code    string   `json:"icd10_code,omitempty"`
	SNOMEDCode   string   `json:"snomed_code,omitempty"`
	Notes        string   `json:"notes,omitempty"`
	Practitioner *string  `json:"practitioner,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable: a create always supplies its tags,
// even when that is none.
func (c *ConditionCreate) TagIDs() (ids []string, supplied bool) { return c.Tags, true }

type ConditionPatch struct {
	Diagnosis  *string              `json:"diagnosis,omitempty"`
	Status     *string              `json:"status,omitempty"`
	Severity   *string              `json:"severity,omitempty"`
	OnsetOn    web.Optional[string] `json:"onset_on,omitzero"`
	ResolvedOn web.Optional[string] `json:"resolved_on,omitzero"`
	ICD10Code  *string              `json:"icd10_code,omitempty"`
	SNOMEDCode *string              `json:"snomed_code,omitempty"`
	Notes      *string              `json:"notes,omitempty"`

	Practitioner *string `json:"practitioner,omitempty"`

	// Tags is replace-set (FR-064, FR-065): a nil pointer leaves the
	// applied tags alone, a non-nil one — including an empty array —
	// replaces the whole set.
	Tags *[]string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable.
func (p *ConditionPatch) TagIDs() (ids []string, supplied bool) {
	if p.Tags == nil {
		return nil, false
	}

	return *p.Tags, true
}

type ConditionCodec struct{}

func ConditionSchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(ConditionSummary) },
		NewDetail:  func() any { return new(Condition) },
		NewCreate:  func() any { return new(ConditionCreate) },
		NewPatch:   func() any { return new(ConditionPatch) },
	}
}

func ConditionSearchFields(body any) (title, text string) {
	found, ok := body.(*Condition)
	if !ok {
		return "", ""
	}

	return found.Diagnosis, found.Notes
}

// ConditionBasis is FR-078's `?active=true` narrowing: active and chronic
// conditions share the "active" basis.
func ConditionBasis(body any, criteria records.Criteria) []string {
	found, ok := body.(*Condition)
	if !ok {
		return nil
	}

	if len(criteria.Filters[condition.FilterActive]) > 0 &&
		(found.Status == string(clinical.ConditionStatusActive) || found.Status == string(clinical.ConditionStatusChronic)) {
		return []string{"active"}
	}

	return nil
}

func (ConditionCodec) Summary(c clinical.Condition) any {
	return &ConditionSummary{
		ID:        c.ID,
		Kind:      kind.Condition.Enum(),
		Diagnosis: c.Diagnosis,
		Status:    string(c.Status),
		Severity:  string(c.Severity),
		OnsetOn:   wireDate(c.OnsetOn),
		UpdatedAt: wireInstant(c.UpdatedAt),
	}
}

func (c ConditionCodec) Detail(cond clinical.Condition) any {
	summary, _ := c.Summary(cond).(*ConditionSummary)

	return &Condition{
		ConditionSummary: *summary,
		Patient:          cond.PatientID,
		ResolvedOn:       wireDate(cond.ResolvedOn),
		ICD10Code:        cond.ICD10Code,
		SNOMEDCode:       cond.SNOMEDCode,
		Practitioner:     cond.PractitionerID,
		Notes:            cond.Notes,
		Tags:             nonNil(cond.Tags),
		CreatedAt:        wireInstant(cond.CreatedAt),
	}
}

func (ConditionCodec) Draft(body any) (clinical.Condition, error) {
	create, ok := body.(*ConditionCreate)
	if !ok {
		return clinical.Condition{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	onsetOn := readDate(&invalid, MemberOnsetOn, create.OnsetOn)
	resolvedOn := readDate(&invalid, MemberResolvedOn, create.ResolvedOn)

	if err := sortFieldOrder(&invalid, conditionMembers); err != nil {
		return clinical.Condition{}, err
	}

	return clinical.Condition{
		PatientID:      create.Patient,
		Diagnosis:      create.Diagnosis,
		Status:         clinical.ConditionStatus(create.Status),
		Severity:       clinical.Severity(create.Severity),
		OnsetOn:        onsetOn,
		ResolvedOn:     resolvedOn,
		ICD10Code:      create.ICD10Code,
		SNOMEDCode:     create.SNOMEDCode,
		Notes:          create.Notes,
		PractitionerID: deref(create.Practitioner),
		Tags:           create.Tags,
	}, nil
}

func (ConditionCodec) Patch(body any) (condition.Patch, error) {
	incoming, ok := body.(*ConditionPatch)
	if !ok {
		return condition.Patch{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	patch := condition.Patch{
		Diagnosis:    incoming.Diagnosis,
		Status:       convert[clinical.ConditionStatus](incoming.Status),
		Severity:     convert[clinical.Severity](incoming.Severity),
		OnsetOn:      readOptionalDate(&invalid, MemberOnsetOn, incoming.OnsetOn),
		ResolvedOn:   readOptionalDate(&invalid, MemberResolvedOn, incoming.ResolvedOn),
		ICD10Code:    incoming.ICD10Code,
		SNOMEDCode:   incoming.SNOMEDCode,
		Notes:        incoming.Notes,
		Practitioner: incoming.Practitioner,
		Tags:         incoming.Tags,
	}

	if err := sortFieldOrder(&invalid, conditionMembers); err != nil {
		return condition.Patch{}, err
	}

	return patch, nil
}
