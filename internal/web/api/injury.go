package api

import (
	"fmt"
	"slices"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/kinds"
	"medikube/internal/service/injury"
	"medikube/internal/web"
)

// The wire spellings of every injury member, mirroring dto_medication.go's
// MemberX constants.
const (
	InjuryMemberID            = "id"
	InjuryMemberKind          = "kind"
	InjuryMemberPatient       = "patient"
	InjuryMemberPractitioner  = "practitioner"
	InjuryMemberName          = "name"
	InjuryMemberType          = "type"
	InjuryMemberBodyPart      = "body_part"
	InjuryMemberLaterality    = "laterality"
	InjuryMemberOccurredOn    = "occurred_on"
	InjuryMemberMechanism     = "mechanism"
	InjuryMemberSeverity      = "severity"
	InjuryMemberStatus        = "status"
	InjuryMemberRecoveryNotes = "recovery_notes"
	InjuryMemberMedications   = "medication_ids"
	InjuryMemberCreatedAt     = "created_at"
	InjuryMemberUpdatedAt     = "updated_at"
)

// injuryMembers is data-model §4.9's column order: the order
// clinical.Injury.Validate checks the rules in and the order refusals are
// sorted into before they leave.
var injuryMembers = []string{
	InjuryMemberPatient,
	InjuryMemberName,
	InjuryMemberType,
	InjuryMemberBodyPart,
	InjuryMemberLaterality,
	InjuryMemberOccurredOn,
	InjuryMemberMechanism,
	InjuryMemberSeverity,
	InjuryMemberStatus,
	InjuryMemberRecoveryNotes,
	InjuryMemberPractitioner,
	InjuryMemberMedications,
}

// InjurySummary is what the list operations return.
type InjurySummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	Name       string   `json:"name"`
	Type       string   `json:"type,omitempty"`
	Severity   string   `json:"severity,omitempty"`
	Status     string   `json:"status"`
	OccurredOn *string  `json:"occurred_on"`
	UpdatedAt  string   `json:"updated_at"`
	Basis      []string `json:"basis"`
}

func (s *InjurySummary) SetBasis(basis []string) { s.Basis = basis }

// Injury is what the detail operations return: every recorded field of
// data-model §4.9 plus the created and last-changed instants.
type Injury struct {
	InjurySummary

	Patient      string `json:"patient"`
	Practitioner string `json:"practitioner,omitempty"`

	BodyPart      string   `json:"body_part,omitempty"`
	Laterality    string   `json:"laterality,omitempty"`
	Mechanism     string   `json:"mechanism,omitempty"`
	RecoveryNotes string   `json:"recovery_notes,omitempty"`
	Medications   []string `json:"medication_ids,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	CreatedAt     string   `json:"created_at"`
}

// GetTags implements records.Tagged so the search index stays in step with
// this record's tags on write (T164-T177 follow-up).
func (i *Injury) GetTags() []string { return i.Tags }

// InjuryCreate is the create body. There is no `owner`, no `id` and no
// timestamps, and every field FR-040 through FR-042 name is optional beyond
// the patient and the name (clinical.Injury.Validate is what rejects a body
// this decoder let through).
type InjuryCreate struct {
	Patient       string   `json:"patient"`
	Name          string   `json:"name"`
	Type          string   `json:"type,omitempty"`
	BodyPart      string   `json:"body_part,omitempty"`
	Laterality    string   `json:"laterality,omitempty"`
	OccurredOn    *string  `json:"occurred_on,omitempty"`
	Mechanism     string   `json:"mechanism,omitempty"`
	Severity      string   `json:"severity,omitempty"`
	Status        string   `json:"status,omitempty"`
	RecoveryNotes string   `json:"recovery_notes,omitempty"`
	Practitioner  *string  `json:"practitioner,omitempty"`
	Medications   []string `json:"medication_ids,omitempty"`
	Tags          []string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable: a create always supplies its tags,
// even when that is none.
func (c *InjuryCreate) TagIDs() (ids []string, supplied bool) { return c.Tags, true }

// InjuryPatch is the partial update. Only supplied members change.
//
// OccurredOn goes through web.Optional the same way medication's two dates
// do. Medications stays a plain, non-pointer slice rather than
// web.Optional[[]string]: encoding/json already tells absent (nil) apart
// from supplied-including-empty (a non-nil, possibly zero-length slice)
// for a slice field, which is exactly service/injury.Patch.MedicationIDs's
// *[]string contract — nil leaves the set alone, non-nil replaces it whole,
// including clearing it with `[]`.
type InjuryPatch struct {
	Name          *string              `json:"name,omitempty"`
	Type          *string              `json:"type,omitempty"`
	BodyPart      *string              `json:"body_part,omitempty"`
	Laterality    *string              `json:"laterality,omitempty"`
	OccurredOn    web.Optional[string] `json:"occurred_on,omitzero"`
	Mechanism     *string              `json:"mechanism,omitempty"`
	Severity      *string              `json:"severity,omitempty"`
	Status        *string              `json:"status,omitempty"`
	RecoveryNotes *string              `json:"recovery_notes,omitempty"`

	Practitioner *string `json:"practitioner,omitempty"`

	Medications []string `json:"medication_ids,omitempty"`

	// Tags is replace-set (FR-064, FR-065): a nil pointer leaves the
	// applied tags alone, a non-nil one — including an empty array —
	// replaces the whole set.
	Tags *[]string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable.
func (p *InjuryPatch) TagIDs() (ids []string, supplied bool) {
	if p.Tags == nil {
		return nil, false
	}

	return *p.Tags, true
}

// InjuryCodec is the DTO boundary for injuries: the only place a
// clinical.Injury becomes a wire shape and the only place a wire shape
// becomes one.
type InjuryCodec struct{}

var _ kinds.InjuryCodec = InjuryCodec{}

// InjurySchema is the four constructors the registry publishes.
func InjurySchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(InjurySummary) },
		NewDetail:  func() any { return new(Injury) },
		NewCreate:  func() any { return new(InjuryCreate) },
		NewPatch:   func() any { return new(InjuryPatch) },
	}
}

// InjurySearchFields reads the two search_index columns off the wire DTO
// Record.Body carries: the name, and the body part, mechanism and recovery
// notes as the free-text detail (research D-11, mirroring
// MedicationSearchFields's name-plus-details precedent).
func InjurySearchFields(body any) (title, text string) {
	inj, ok := body.(*Injury)
	if !ok {
		return "", ""
	}

	return inj.Name, inj.BodyPart + " " + inj.Mechanism + " " + inj.RecoveryNotes
}

// InjuryBasis narrows unresolved rows: `?unresolved=true` groups two distinct
// statuses (active and healing), so a row's own status is the reason it
// qualifies (research D-05).
func InjuryBasis(body any, criteria records.Criteria) []string {
	if _, narrowed := criteria.Filters[injury.FilterUnresolved]; !narrowed {
		return nil
	}

	inj, ok := body.(*InjurySummary)
	if !ok || inj.Status == "" {
		return nil
	}

	return []string{inj.Status}
}

// Summary renders the list shape.
func (InjuryCodec) Summary(i clinical.Injury) any {
	return &InjurySummary{
		ID:         i.ID,
		Kind:       kind.Injury.Enum(),
		Name:       i.Name,
		Type:       string(i.Type),
		Severity:   string(i.Severity),
		Status:     string(i.Status),
		OccurredOn: wireDate(i.OccurredOn),
		UpdatedAt:  wireInstant(i.UpdatedAt),
	}
}

// Detail renders the full shape.
func (c InjuryCodec) Detail(i clinical.Injury) any {
	summary, ok := c.Summary(i).(*InjurySummary)
	if !ok {
		// Unreachable while Summary returns what it says it does.
		return &Injury{}
	}

	return &Injury{
		InjurySummary: *summary,
		Patient:       i.PatientID,
		Practitioner:  i.PractitionerID,
		BodyPart:      i.BodyPart,
		Laterality:    string(i.Laterality),
		Mechanism:     i.Mechanism,
		RecoveryNotes: i.RecoveryNotes,
		Medications:   i.MedicationIDs,
		Tags:          i.Tags,
		CreatedAt:     wireInstant(i.CreatedAt),
	}
}

// Draft reads a create body.
func (InjuryCodec) Draft(body any) (clinical.Injury, error) {
	create, ok := body.(*InjuryCreate)
	if !ok {
		return clinical.Injury{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	occurredOn := readDate(&invalid, InjuryMemberOccurredOn, create.OccurredOn)

	if err := orderedInjuryRefusal(&invalid); err != nil {
		return clinical.Injury{}, err
	}

	return clinical.Injury{
		PatientID:      create.Patient,
		Name:           create.Name,
		Type:           clinical.InjuryType(create.Type),
		BodyPart:       create.BodyPart,
		Laterality:     clinical.Laterality(create.Laterality),
		OccurredOn:     occurredOn,
		Mechanism:      create.Mechanism,
		Severity:       clinical.Severity(create.Severity),
		Status:         clinical.ConditionStatus(create.Status),
		RecoveryNotes:  create.RecoveryNotes,
		MedicationIDs:  create.Medications,
		PractitionerID: deref(create.Practitioner),
		Tags:           create.Tags,
	}, nil
}

// Patch reads an update body.
func (InjuryCodec) Patch(body any) (injury.Patch, error) {
	incoming, ok := body.(*InjuryPatch)
	if !ok {
		return injury.Patch{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	patch := injury.Patch{
		Name:          incoming.Name,
		Type:          convert[clinical.InjuryType](incoming.Type),
		BodyPart:      incoming.BodyPart,
		Laterality:    convert[clinical.Laterality](incoming.Laterality),
		OccurredOn:    readOptionalDate(&invalid, InjuryMemberOccurredOn, incoming.OccurredOn),
		Mechanism:     incoming.Mechanism,
		Severity:      convert[clinical.Severity](incoming.Severity),
		Status:        convert[clinical.ConditionStatus](incoming.Status),
		RecoveryNotes: incoming.RecoveryNotes,
		Practitioner:  incoming.Practitioner,
	}

	if incoming.Medications != nil {
		patch.MedicationIDs = &incoming.Medications
	}

	patch.Tags = incoming.Tags

	if err := orderedInjuryRefusal(&invalid); err != nil {
		return injury.Patch{}, err
	}

	return patch, nil
}

// orderedInjuryRefusal sorts the refusals into data-model §4.9's column
// order, the same discipline orderedRefusal applies for medication.
func orderedInjuryRefusal(invalid *domain.ValidationError) error {
	if invalid.Empty() {
		return nil
	}

	slices.SortStableFunc(invalid.Fields, func(left, right domain.FieldError) int {
		return slices.Index(injuryMembers, left.Field) - slices.Index(injuryMembers, right.Field)
	})

	return invalid.OrNil()
}
