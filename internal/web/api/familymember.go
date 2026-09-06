package api

import (
	"fmt"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/domain/person"
	"medikube/internal/records"
	"medikube/internal/service/familymember"
	"medikube/internal/web"
)

// The wire spellings of family history's own members.
const (
	FamilyMemberFieldRelationship = "relationship"
	FamilyMemberFieldSex          = "sex"
	FamilyMemberFieldBirthYear    = "birth_year"
	FamilyMemberFieldDeathYear    = "death_year"
	FamilyMemberFieldIsDeceased   = "is_deceased"
	FamilyMemberFieldConditions   = "conditions"
)

// FamilyCondition is data-model §6.1's wire shape.
type FamilyCondition struct {
	Name         string `json:"name"`
	ICD10Code    string `json:"icd10_code,omitempty"`
	DiagnosedAge *int   `json:"diagnosed_age,omitempty"`
	Severity     string `json:"severity,omitempty"`
	Status       string `json:"status,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

// FamilyMemberSummary is what the list operation returns.
type FamilyMemberSummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	Name         string `json:"name"`
	Relationship string `json:"relationship"`
	UpdatedAt    string `json:"updated_at"`
}

// FamilyMember is what the detail operations return: every recorded field of
// FR-052/FR-053.
type FamilyMember struct {
	FamilyMemberSummary

	Patient    string            `json:"patient"`
	Sex        string            `json:"sex,omitempty"`
	BirthYear  *int              `json:"birth_year,omitempty"`
	DeathYear  *int              `json:"death_year,omitempty"`
	IsDeceased bool              `json:"is_deceased,omitempty"`
	Conditions []FamilyCondition `json:"conditions"`
	Tags       []string          `json:"tags,omitempty"`

	CreatedAt string `json:"created_at"`
}

// GetTags implements records.Tagged so the search index stays in step with
// this record's tags on write (T164-T177 follow-up).
func (f *FamilyMember) GetTags() []string { return f.Tags }

// FamilyMemberCreate is the create body: patient, name and relationship are
// required; everything else is optional at creation.
type FamilyMemberCreate struct {
	Patient      string            `json:"patient"`
	Name         string            `json:"name"`
	Relationship string            `json:"relationship"`
	Sex          string            `json:"sex,omitempty"`
	BirthYear    *int              `json:"birth_year,omitempty"`
	DeathYear    *int              `json:"death_year,omitempty"`
	IsDeceased   bool              `json:"is_deceased,omitempty"`
	Conditions   []FamilyCondition `json:"conditions,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable: a create always supplies its tags,
// even when that is none.
func (c *FamilyMemberCreate) TagIDs() (ids []string, supplied bool) { return c.Tags, true }

// FamilyMemberPatch is the partial update.
type FamilyMemberPatch struct {
	Name         *string `json:"name,omitempty"`
	Relationship *string `json:"relationship,omitempty"`
	Sex          *string `json:"sex,omitempty"`

	BirthYear web.Optional[int] `json:"birth_year,omitzero"`
	DeathYear web.Optional[int] `json:"death_year,omitzero"`

	IsDeceased *bool              `json:"is_deceased,omitempty"`
	Conditions *[]FamilyCondition `json:"conditions,omitempty"`

	// Tags is replace-set (FR-064, FR-065): a nil pointer leaves the
	// applied tags alone, a non-nil one — including an empty array —
	// replaces the whole set.
	Tags *[]string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable.
func (p *FamilyMemberPatch) TagIDs() (ids []string, supplied bool) {
	if p.Tags == nil {
		return nil, false
	}

	return *p.Tags, true
}

// FamilyMemberCodec is the DTO boundary for family history.
type FamilyMemberCodec struct{}

var _ familymember.Codec = FamilyMemberCodec{}

func FamilyMemberSchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(FamilyMemberSummary) },
		NewDetail:  func() any { return new(FamilyMember) },
		NewCreate:  func() any { return new(FamilyMemberCreate) },
		NewPatch:   func() any { return new(FamilyMemberPatch) },
	}
}

// FamilyMemberSearchFields reads the search_index columns off the wire DTO:
// the relative's own name, and nothing else — sex, years and conditions are
// not free text a search term matches against.
func FamilyMemberSearchFields(body any) (title, text string) {
	found, ok := body.(*FamilyMember)
	if !ok {
		return "", ""
	}

	return found.Name, ""
}

// FamilyMemberBasis narrows nothing beyond the published query parameters.
func FamilyMemberBasis(any, records.Criteria) []string { return nil }

func (FamilyMemberCodec) Summary(entity clinical.FamilyMember) any {
	return &FamilyMemberSummary{
		ID:           entity.ID,
		Kind:         kind.FamilyMember.Enum(),
		Name:         entity.Name,
		Relationship: string(entity.Relationship),
		UpdatedAt:    wireInstant(entity.UpdatedAt),
	}
}

func (c FamilyMemberCodec) Detail(entity clinical.FamilyMember) any {
	summary, ok := c.Summary(entity).(*FamilyMemberSummary)
	if !ok {
		return &FamilyMember{}
	}

	return &FamilyMember{
		FamilyMemberSummary: *summary,
		Patient:             entity.PatientID,
		Sex:                 string(entity.Sex),
		BirthYear:           entity.BirthYear,
		DeathYear:           entity.DeathYear,
		IsDeceased:          entity.IsDeceased,
		Conditions:          wireFamilyConditions(entity.Conditions),
		Tags:                entity.Tags,
		CreatedAt:           wireInstant(entity.CreatedAt),
	}
}

func (FamilyMemberCodec) Draft(body any) (clinical.FamilyMember, error) {
	create, ok := body.(*FamilyMemberCreate)
	if !ok {
		return clinical.FamilyMember{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	return clinical.FamilyMember{
		PatientID:    create.Patient,
		Name:         create.Name,
		Relationship: clinical.FamilyRelationship(create.Relationship),
		Sex:          person.Sex(create.Sex),
		BirthYear:    create.BirthYear,
		DeathYear:    create.DeathYear,
		IsDeceased:   create.IsDeceased,
		Conditions:   domainFamilyConditions(create.Conditions),
		Tags:         create.Tags,
	}, nil
}

func (FamilyMemberCodec) Patch(body any) (familymember.Patch, error) {
	incoming, ok := body.(*FamilyMemberPatch)
	if !ok {
		return familymember.Patch{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	patch := familymember.Patch{
		Name:         incoming.Name,
		Relationship: convert[clinical.FamilyRelationship](incoming.Relationship),
		Sex:          convert[person.Sex](incoming.Sex),
		BirthYear:    readOptionalIntPtr(incoming.BirthYear),
		DeathYear:    readOptionalIntPtr(incoming.DeathYear),
		IsDeceased:   incoming.IsDeceased,
		Tags:         incoming.Tags,
	}

	if incoming.Conditions != nil {
		converted := domainFamilyConditions(*incoming.Conditions)
		patch.Conditions = &converted
	}

	return patch, nil
}

func wireFamilyConditions(conditions []clinical.FamilyCondition) []FamilyCondition {
	wire := make([]FamilyCondition, 0, len(conditions))

	for _, condition := range conditions {
		wire = append(wire, FamilyCondition{
			Name:         condition.Name,
			ICD10Code:    condition.ICD10Code,
			DiagnosedAge: condition.DiagnosedAge,
			Severity:     string(condition.Severity),
			Status:       string(condition.Status),
			Notes:        condition.Notes,
		})
	}

	return wire
}

func domainFamilyConditions(conditions []FamilyCondition) []clinical.FamilyCondition {
	converted := make([]clinical.FamilyCondition, 0, len(conditions))

	for _, condition := range conditions {
		converted = append(converted, clinical.FamilyCondition{
			Name:         condition.Name,
			ICD10Code:    condition.ICD10Code,
			DiagnosedAge: condition.DiagnosedAge,
			Severity:     clinical.Severity(condition.Severity),
			Status:       clinical.ConditionStatus(condition.Status),
			Notes:        condition.Notes,
		})
	}

	return converted
}
