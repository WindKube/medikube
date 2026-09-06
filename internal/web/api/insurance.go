package api

import (
	"fmt"
	"slices"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/service/insurance"
	"medikube/internal/web"
)

// The wire spellings of insurance's own members, not already published by
// dto_medication.go's Member* constants (research D-11: two kinds are allowed
// to agree on a spelling without sharing a constant, but a spelling that is
// this kind's own gets its own name).
const (
	InsuranceFieldCompany       = "company"
	InsuranceFieldPlanName      = "plan_name"
	InsuranceFieldEmployerGroup = "employer_group"
	InsuranceFieldMemberName    = "member_name"
	InsuranceFieldMemberID      = "member_id"
	InsuranceFieldGroupNumber   = "group_number"
	InsuranceFieldHolderName    = "holder_name"
	InsuranceFieldRelationship  = "relationship_to_holder"
	InsuranceFieldEffectiveOn   = "effective_on"
	InsuranceFieldExpiresOn     = "expires_on"
	InsuranceFieldIsPrimary     = "is_primary"
	InsuranceFieldCoverage      = "coverage"
	InsuranceFieldContact       = "contact"
)

// insuranceMembers is data-model §4.10's column order, walked the same way
// medicationMembers is: the order Insurance.Validate checks in and the order a
// response sorts its refusals into (FR-027).
var insuranceMembers = []string{
	MemberPatient,
	MemberType,
	InsuranceFieldCompany,
	InsuranceFieldPlanName,
	InsuranceFieldEmployerGroup,
	InsuranceFieldMemberName,
	InsuranceFieldMemberID,
	InsuranceFieldGroupNumber,
	InsuranceFieldHolderName,
	InsuranceFieldRelationship,
	InsuranceFieldEffectiveOn,
	InsuranceFieldExpiresOn,
	MemberStatus,
	InsuranceFieldIsPrimary,
	InsuranceFieldCoverage,
	InsuranceFieldContact,
	MemberNotes,
}

// CoverageDTO is data-model §6.2 on the wire: money as decimal strings, never
// as float64 (clinical.Money's own reasoning applies here word for word).
type CoverageDTO struct {
	Deductible      *string  `json:"deductible,omitempty"`
	OOPMax          *string  `json:"oop_max,omitempty"`
	CopayPrimary    *string  `json:"copay_primary,omitempty"`
	CopaySpecialist *string  `json:"copay_specialist,omitempty"`
	CopayER         *string  `json:"copay_er,omitempty"`
	CoinsurancePct  *float64 `json:"coinsurance_pct,omitempty"`
	Currency        string   `json:"currency,omitempty"`
}

// ContactDTO is data-model §6.3 on the wire.
type ContactDTO struct {
	Phone       string `json:"phone,omitempty"`
	ClaimsPhone string `json:"claims_phone,omitempty"`
	Website     string `json:"website,omitempty"`
	PortalURL   string `json:"portal_url,omitempty"`
	Address     string `json:"address,omitempty"`
}

// DisplacedDTO is FR-045's report of the policy a new primary unset.
type DisplacedDTO struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// InsuranceSummary is what the list operation returns. Basis is FR-046's
// per-row reason, present only when the list was narrowed by
// ?expiring_within_days=.
type InsuranceSummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	Company   string  `json:"company"`
	Type      string  `json:"type"`
	Status    string  `json:"status,omitempty"`
	IsPrimary bool    `json:"is_primary"`
	ExpiresOn *string `json:"expires_on"`
	UpdatedAt string  `json:"updated_at"`

	Basis []string `json:"basis,omitempty"`
}

// Insurance is what the detail operations return: every recorded field of
// FR-043/044/045 plus the timestamps of FR-020.
type Insurance struct {
	InsuranceSummary

	Patient       string `json:"patient"`
	PlanName      string `json:"plan_name,omitempty"`
	EmployerGroup string `json:"employer_group,omitempty"`
	MemberName    string `json:"member_name,omitempty"`
	MemberID      string `json:"member_id,omitempty"`
	GroupNumber   string `json:"group_number,omitempty"`
	HolderName    string `json:"holder_name,omitempty"`
	Relationship  string `json:"relationship_to_holder,omitempty"`
	EffectiveOn   string `json:"effective_on"`

	Coverage *CoverageDTO `json:"coverage,omitempty"`
	Contact  *ContactDTO  `json:"contact,omitempty"`

	Notes     string   `json:"notes,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	CreatedAt string   `json:"created_at"`

	// Displaced is FR-045's result: set only on the write that caused a
	// displacement, nil on every plain read.
	Displaced *DisplacedDTO `json:"displaced,omitempty"`
}

// GetTags implements records.Tagged so the search index stays in step with
// this record's tags on write (T164-T177 follow-up).
func (i *Insurance) GetTags() []string { return i.Tags }

// InsuranceCreate is the create body (FR-043): patient, type, company, member
// name, member id and the effective date are required; everything else is
// optional at creation.
type InsuranceCreate struct {
	Patient       string  `json:"patient"`
	Type          string  `json:"type"`
	Company       string  `json:"company"`
	PlanName      string  `json:"plan_name,omitempty"`
	EmployerGroup string  `json:"employer_group,omitempty"`
	MemberName    string  `json:"member_name"`
	MemberID      string  `json:"member_id"`
	GroupNumber   string  `json:"group_number,omitempty"`
	HolderName    string  `json:"holder_name,omitempty"`
	Relationship  string  `json:"relationship_to_holder,omitempty"`
	EffectiveOn   string  `json:"effective_on"`
	ExpiresOn     *string `json:"expires_on,omitempty"`
	Status        string  `json:"status,omitempty"`
	IsPrimary     bool    `json:"is_primary,omitempty"`

	Coverage *CoverageDTO `json:"coverage,omitempty"`
	Contact  *ContactDTO  `json:"contact,omitempty"`

	Notes string   `json:"notes,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable: a create always supplies its tags,
// even when that is none.
func (c *InsuranceCreate) TagIDs() (ids []string, supplied bool) { return c.Tags, true }

// InsurancePatch is the partial update. Coverage and Contact replace the whole
// nested object when supplied: FR-045's own transaction is what a partial
// field-by-field merge of a JSON object would need to reason about twice,
// once here and once in the store, for a shape this small.
type InsurancePatch struct {
	Type          *string `json:"type,omitempty"`
	Company       *string `json:"company,omitempty"`
	PlanName      *string `json:"plan_name,omitempty"`
	EmployerGroup *string `json:"employer_group,omitempty"`
	MemberName    *string `json:"member_name,omitempty"`
	MemberID      *string `json:"member_id,omitempty"`
	GroupNumber   *string `json:"group_number,omitempty"`
	HolderName    *string `json:"holder_name,omitempty"`
	Relationship  *string `json:"relationship_to_holder,omitempty"`

	EffectiveOn web.Optional[string] `json:"effective_on,omitzero"`
	ExpiresOn   web.Optional[string] `json:"expires_on,omitzero"`

	Status    *string `json:"status,omitempty"`
	IsPrimary *bool   `json:"is_primary,omitempty"`

	Coverage *CoverageDTO `json:"coverage,omitempty"`
	Contact  *ContactDTO  `json:"contact,omitempty"`

	Notes *string `json:"notes,omitempty"`

	// Tags is replace-set (FR-064, FR-065): a nil pointer leaves the
	// applied tags alone, a non-nil one — including an empty array —
	// replaces the whole set.
	Tags *[]string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable.
func (p *InsurancePatch) TagIDs() (ids []string, supplied bool) {
	if p.Tags == nil {
		return nil, false
	}

	return *p.Tags, true
}

// InsuranceCodec is the DTO boundary for insurance.
type InsuranceCodec struct{}

var _ insurance.Codec = InsuranceCodec{}

func InsuranceSchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(InsuranceSummary) },
		NewDetail:  func() any { return new(Insurance) },
		NewCreate:  func() any { return new(InsuranceCreate) },
		NewPatch:   func() any { return new(InsurancePatch) },
	}
}

// InsuranceSearchFields reads the search_index columns off the wire DTO: the
// insurer, and the plan name and notes.
func InsuranceSearchFields(body any) (title, text string) {
	found, ok := body.(*Insurance)
	if !ok {
		return "", ""
	}

	return found.Company, found.PlanName + " " + found.Notes
}

// InsuranceBasis reads back the per-row reason InsuranceCodec.Summary already
// computed (FR-046), so the registry's own Basis consumer has something real
// to answer with once the generic handler starts calling it (US9).
func InsuranceBasis(body any, _ records.Criteria) []string {
	summary, ok := body.(*InsuranceSummary)
	if !ok {
		return nil
	}

	return summary.Basis
}

func (InsuranceCodec) Summary(entity clinical.Insurance, basis []string) any {
	return &InsuranceSummary{
		ID:        entity.ID,
		Kind:      kind.Insurance.Enum(),
		Company:   entity.Company,
		Type:      string(entity.Type),
		Status:    string(entity.Status),
		IsPrimary: entity.IsPrimary,
		ExpiresOn: wireDate(entity.ExpiresOn),
		UpdatedAt: wireInstant(entity.UpdatedAt),
		Basis:     basis,
	}
}

func (c InsuranceCodec) Detail(entity clinical.Insurance, displaced *insurance.Displaced) any {
	summary, ok := c.Summary(entity, nil).(*InsuranceSummary)
	if !ok {
		return &Insurance{}
	}

	var displacedDTO *DisplacedDTO
	if displaced != nil {
		displacedDTO = &DisplacedDTO{ID: displaced.ID, Kind: kind.Insurance.Enum()}
	}

	return &Insurance{
		InsuranceSummary: *summary,
		Patient:          entity.PatientID,
		PlanName:         entity.PlanName,
		EmployerGroup:    entity.EmployerGroup,
		MemberName:       entity.MemberName,
		MemberID:         entity.MemberID,
		GroupNumber:      entity.GroupNumber,
		HolderName:       entity.HolderName,
		Relationship:     string(entity.Relationship),
		EffectiveOn:      entity.EffectiveOn.String(),
		Coverage:         wireCoverage(entity.Coverage),
		Contact:          wireContact(entity.Contact),
		Notes:            entity.Notes,
		Tags:             entity.Tags,
		CreatedAt:        wireInstant(entity.CreatedAt),
		Displaced:        displacedDTO,
	}
}

func (InsuranceCodec) Draft(body any) (clinical.Insurance, error) {
	create, ok := body.(*InsuranceCreate)
	if !ok {
		return clinical.Insurance{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	effectiveOn := readDate(&invalid, InsuranceFieldEffectiveOn, &create.EffectiveOn)
	expiresOn := readDate(&invalid, InsuranceFieldExpiresOn, create.ExpiresOn)
	coverage := readCoverage(&invalid, create.Coverage)

	if err := orderedRefusal2(&invalid, insuranceMembers); err != nil {
		return clinical.Insurance{}, err
	}

	return clinical.Insurance{
		PatientID:     create.Patient,
		Type:          clinical.InsuranceType(create.Type),
		Company:       create.Company,
		PlanName:      create.PlanName,
		EmployerGroup: create.EmployerGroup,
		MemberName:    create.MemberName,
		MemberID:      create.MemberID,
		GroupNumber:   create.GroupNumber,
		HolderName:    create.HolderName,
		Relationship:  clinical.HolderRelationship(create.Relationship),
		EffectiveOn:   effectiveOn,
		ExpiresOn:     expiresOn,
		Status:        clinical.InsuranceStatus(create.Status),
		IsPrimary:     create.IsPrimary,
		Coverage:      coverage,
		Contact:       readContact(create.Contact),
		Notes:         create.Notes,
		Tags:          create.Tags,
	}, nil
}

func (InsuranceCodec) Patch(body any) (insurance.Patch, error) {
	incoming, ok := body.(*InsurancePatch)
	if !ok {
		return insurance.Patch{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	patch := insurance.Patch{
		Type:          convert[clinical.InsuranceType](incoming.Type),
		Company:       incoming.Company,
		PlanName:      incoming.PlanName,
		EmployerGroup: incoming.EmployerGroup,
		MemberName:    incoming.MemberName,
		MemberID:      incoming.MemberID,
		GroupNumber:   incoming.GroupNumber,
		HolderName:    incoming.HolderName,
		Relationship:  convert[clinical.HolderRelationship](incoming.Relationship),
		EffectiveOn:   readOptionalDate(&invalid, InsuranceFieldEffectiveOn, incoming.EffectiveOn),
		ExpiresOn:     readOptionalDate(&invalid, InsuranceFieldExpiresOn, incoming.ExpiresOn),
		Status:        convert[clinical.InsuranceStatus](incoming.Status),
		IsPrimary:     incoming.IsPrimary,
		Notes:         incoming.Notes,
		Tags:          incoming.Tags,
	}

	if incoming.Coverage != nil {
		coverage := readCoverage(&invalid, incoming.Coverage)
		patch.Coverage = &coverage
	}

	if incoming.Contact != nil {
		contact := readContact(incoming.Contact)
		patch.Contact = &contact
	}

	if err := orderedRefusal2(&invalid, insuranceMembers); err != nil {
		return insurance.Patch{}, err
	}

	return patch, nil
}

func readCoverage(invalid *domain.ValidationError, dto *CoverageDTO) clinical.Coverage {
	if dto == nil {
		return clinical.Coverage{}
	}

	coverage := clinical.Coverage{CoinsurancePct: dto.CoinsurancePct, Currency: dto.Currency}

	coverage.Deductible = parseMoney(invalid, "deductible", dto.Deductible)
	coverage.OOPMax = parseMoney(invalid, "oop_max", dto.OOPMax)
	coverage.CopayPrimary = parseMoney(invalid, "copay_primary", dto.CopayPrimary)
	coverage.CopaySpecialist = parseMoney(invalid, "copay_specialist", dto.CopaySpecialist)
	coverage.CopayER = parseMoney(invalid, "copay_er", dto.CopayER)

	return coverage
}

func parseMoney(invalid *domain.ValidationError, member string, raw *string) *clinical.Money {
	if raw == nil {
		return nil
	}

	money, err := clinical.ParseMoney(*raw)
	if err != nil {
		invalid.Add(InsuranceFieldCoverage+"."+member, domain.CodeInvalidValue, "an amount is a non-negative decimal with at most two fractional digits")

		return nil
	}

	return &money
}

func readContact(dto *ContactDTO) clinical.Contact {
	if dto == nil {
		return clinical.Contact{}
	}

	return clinical.Contact(*dto)
}

func wireCoverage(coverage clinical.Coverage) *CoverageDTO {
	if coverage.IsZero() {
		return nil
	}

	return &CoverageDTO{
		Deductible:      moneyPtr(coverage.Deductible),
		OOPMax:          moneyPtr(coverage.OOPMax),
		CopayPrimary:    moneyPtr(coverage.CopayPrimary),
		CopaySpecialist: moneyPtr(coverage.CopaySpecialist),
		CopayER:         moneyPtr(coverage.CopayER),
		CoinsurancePct:  coverage.CoinsurancePct,
		Currency:        coverage.Currency,
	}
}

func moneyPtr(m *clinical.Money) *string {
	if m == nil {
		return nil
	}

	rendered := m.String()

	return &rendered
}

func wireContact(contact clinical.Contact) *ContactDTO {
	if contact.IsZero() {
		return nil
	}

	dto := ContactDTO(contact)

	return &dto
}

// orderedRefusal2 is orderedRefusal against a caller-supplied column order,
// for the kinds that are not medication's own.
func orderedRefusal2(invalid *domain.ValidationError, order []string) error {
	if invalid.Empty() {
		return nil
	}

	slices.SortStableFunc(invalid.Fields, func(left, right domain.FieldError) int {
		return slices.Index(order, left.Field) - slices.Index(order, right.Field)
	})

	return invalid.OrNil()
}
