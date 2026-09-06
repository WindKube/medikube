package api

import (
	"fmt"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/service/allergy"
	"medikube/internal/web"
)

const (
	MemberAllergen = "allergen"
	MemberReaction = "reaction"
	MemberSeverity = "severity"
	MemberOnsetOn  = "onset_on"
)

var allergyMembers = []string{
	MemberPatient,
	MemberAllergen,
	MemberReaction,
	MemberSeverity,
	MemberStatus,
	MemberOnsetOn,
	MemberNotes,
}

// AllergySummary is what the list operations return. Critical is FR-018's
// derivation, rendered rather than stored.
type AllergySummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	Allergen  string  `json:"allergen"`
	Severity  string  `json:"severity"`
	Status    string  `json:"status"`
	OnsetOn   *string `json:"onset_on"`
	Critical  bool    `json:"critical"`
	UpdatedAt string  `json:"updated_at"`
}

type Allergy struct {
	AllergySummary

	Patient  string `json:"patient"`
	Reaction string `json:"reaction,omitempty"`
	Notes    string `json:"notes,omitempty"`

	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
}

type AllergyCreate struct {
	Patient  string   `json:"patient"`
	Allergen string   `json:"allergen"`
	Reaction string   `json:"reaction,omitempty"`
	Severity string   `json:"severity"`
	Status   string   `json:"status,omitempty"`
	OnsetOn  *string  `json:"onset_on,omitempty"`
	Notes    string   `json:"notes,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable: a create always supplies its tags,
// even when that is none.
func (c *AllergyCreate) TagIDs() (ids []string, supplied bool) { return c.Tags, true }

type AllergyPatch struct {
	Allergen *string              `json:"allergen,omitempty"`
	Reaction *string              `json:"reaction,omitempty"`
	Severity *string              `json:"severity,omitempty"`
	Status   *string              `json:"status,omitempty"`
	OnsetOn  web.Optional[string] `json:"onset_on,omitzero"`
	Notes    *string              `json:"notes,omitempty"`

	// Tags is replace-set (FR-064, FR-065): a nil pointer leaves the
	// applied tags alone, a non-nil one — including an empty array —
	// replaces the whole set.
	Tags *[]string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable.
func (p *AllergyPatch) TagIDs() (ids []string, supplied bool) {
	if p.Tags == nil {
		return nil, false
	}

	return *p.Tags, true
}

// AllergyCodec is the DTO boundary for allergies.
type AllergyCodec struct{}

func AllergySchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(AllergySummary) },
		NewDetail:  func() any { return new(Allergy) },
		NewCreate:  func() any { return new(AllergyCreate) },
		NewPatch:   func() any { return new(AllergyPatch) },
	}
}

func AllergySearchFields(body any) (title, text string) {
	found, ok := body.(*Allergy)
	if !ok {
		return "", ""
	}

	return found.Allergen, found.Reaction + " " + found.Notes
}

// AllergyBasis is FR-018's critical narrowing (contracts/pages.md §3.5,
// `?critical=true`): a row's own basis, not a query-level fact.
func AllergyBasis(body any, _ records.Criteria) []string {
	found, ok := body.(*Allergy)
	if !ok || !found.Critical {
		return nil
	}

	return []string{"critical"}
}

func (AllergyCodec) Summary(a clinical.Allergy) any {
	return &AllergySummary{
		ID:        a.ID,
		Kind:      kind.Allergy.Enum(),
		Allergen:  a.Allergen,
		Severity:  string(a.Severity),
		Status:    string(a.Status),
		OnsetOn:   wireDate(a.OnsetOn),
		Critical:  a.Critical(),
		UpdatedAt: wireInstant(a.UpdatedAt),
	}
}

func (c AllergyCodec) Detail(a clinical.Allergy) any {
	summary, _ := c.Summary(a).(*AllergySummary)

	return &Allergy{
		AllergySummary: *summary,
		Patient:        a.PatientID,
		Reaction:       a.Reaction,
		Notes:          a.Notes,
		Tags:           nonNil(a.Tags),
		CreatedAt:      wireInstant(a.CreatedAt),
	}
}

func (AllergyCodec) Draft(body any) (clinical.Allergy, error) {
	create, ok := body.(*AllergyCreate)
	if !ok {
		return clinical.Allergy{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	onsetOn := readDate(&invalid, MemberOnsetOn, create.OnsetOn)

	if err := sortFieldOrder(&invalid, allergyMembers); err != nil {
		return clinical.Allergy{}, err
	}

	return clinical.Allergy{
		PatientID: create.Patient,
		Allergen:  create.Allergen,
		Reaction:  create.Reaction,
		Severity:  clinical.Severity(create.Severity),
		Status:    clinical.ConditionStatus(create.Status),
		OnsetOn:   onsetOn,
		Notes:     create.Notes,
		Tags:      create.Tags,
	}, nil
}

func (AllergyCodec) Patch(body any) (allergy.Patch, error) {
	incoming, ok := body.(*AllergyPatch)
	if !ok {
		return allergy.Patch{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	patch := allergy.Patch{
		Allergen: incoming.Allergen,
		Reaction: incoming.Reaction,
		Severity: convert[clinical.Severity](incoming.Severity),
		Status:   convert[clinical.ConditionStatus](incoming.Status),
		OnsetOn:  readOptionalDate(&invalid, MemberOnsetOn, incoming.OnsetOn),
		Notes:    incoming.Notes,
		Tags:     incoming.Tags,
	}

	if err := sortFieldOrder(&invalid, allergyMembers); err != nil {
		return allergy.Patch{}, err
	}

	return patch, nil
}
