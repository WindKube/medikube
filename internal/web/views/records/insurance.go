package records

import (
	"medikube/internal/domain/clinical"
	viewtags "medikube/internal/web/views/tags"
)

// The wire spellings insurance adds beyond medication's own FieldName,
// FieldType, FieldStatus and FieldNotes (already declared in medication.go,
// this package's first kind).
const (
	FieldCompany              = "company"
	FieldPlanName             = "plan_name"
	FieldEmployerGroup        = "employer_group"
	FieldMemberName           = "member_name"
	FieldMemberID             = "member_id"
	FieldGroupNumber          = "group_number"
	FieldHolderName           = "holder_name"
	FieldRelationshipToHolder = "relationship_to_holder"
	FieldEffectiveOn          = "effective_on"
	FieldExpiresOn            = "expires_on"
)

var insuranceFields = []string{
	FieldType,
	FieldCompany,
	FieldPlanName,
	FieldEmployerGroup,
	FieldMemberName,
	FieldMemberID,
	FieldGroupNumber,
	FieldHolderName,
	FieldRelationshipToHolder,
	FieldEffectiveOn,
	FieldExpiresOn,
	FieldStatus,
	FieldIsPrimary,
	FieldNotes,
}

// InsuranceFields is what the form offers, cloned so a caller cannot reorder
// the published one.
func InsuranceFields() []string { return append([]string(nil), insuranceFields...) }

func init() {
	fieldLabels[FieldCompany] = "field.insurance.company"
	fieldLabels[FieldPlanName] = "field.insurance.plan_name"
	fieldLabels[FieldEmployerGroup] = "field.insurance.employer_group"
	fieldLabels[FieldMemberName] = "field.insurance.member_name"
	fieldLabels[FieldMemberID] = "field.insurance.member_id"
	fieldLabels[FieldGroupNumber] = "field.insurance.group_number"
	fieldLabels[FieldHolderName] = "field.insurance.holder_name"
	fieldLabels[FieldRelationshipToHolder] = "field.insurance.relationship_to_holder"
	fieldLabels[FieldEffectiveOn] = "field.insurance.effective_on"
	fieldLabels[FieldExpiresOn] = "field.insurance.expires_on"
	fieldLabels[FieldIsPrimary] = "field.insurance.is_primary"
}

// The values below are message ids (D-06), resolved at render time.
var insuranceTypeLabels = map[clinical.InsuranceType]string{
	clinical.InsuranceTypeMedical:      "enum.insurance_type.medical",
	clinical.InsuranceTypeDental:       "enum.insurance_type.dental",
	clinical.InsuranceTypeVision:       "enum.insurance_type.vision",
	clinical.InsuranceTypePrescription: "enum.insurance_type.prescription",
	clinical.InsuranceTypeOther:        "enum.insurance_type.other",
}

// insuranceStatusLabels' Active/Inactive reuse condition_status's vocabulary
// (identical English text); Expired/Pending are insurance-specific.
var insuranceStatusLabels = map[clinical.InsuranceStatus]string{
	clinical.InsuranceStatusActive:   "enum.condition_status.active",
	clinical.InsuranceStatusInactive: "enum.condition_status.inactive",
	clinical.InsuranceStatusExpired:  "enum.insurance_status.expired",
	clinical.InsuranceStatusPending:  "enum.insurance_status.pending",
}

var holderRelationshipLabels = map[clinical.HolderRelationship]string{
	clinical.HolderRelationshipSelf:      "enum.holder_relationship.self",
	clinical.HolderRelationshipSpouse:    "enum.holder_relationship.spouse",
	clinical.HolderRelationshipChild:     "enum.holder_relationship.child",
	clinical.HolderRelationshipDependent: "enum.holder_relationship.dependent",
	clinical.HolderRelationshipOther:     "enum.holder_relationship.other",
}

func InsuranceTypeLabel(value clinical.InsuranceType) string {
	return label(string(value), insuranceTypeLabels[value])
}

func InsuranceStatusLabel(value clinical.InsuranceStatus) string {
	return label(string(value), insuranceStatusLabels[value])
}

func HolderRelationshipLabel(value clinical.HolderRelationship) string {
	return label(string(value), holderRelationshipLabels[value])
}

func InsuranceTypeOptions(selected clinical.InsuranceType) []Option {
	published := clinical.InsuranceTypes()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: InsuranceTypeLabel(value), Selected: value == selected})
	}

	return options
}

func InsuranceStatusOptions(selected clinical.InsuranceStatus) []Option {
	published := clinical.InsuranceStatuses()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: InsuranceStatusLabel(value), Selected: value == selected})
	}

	return options
}

func HolderRelationshipOptions(selected clinical.HolderRelationship) []Option {
	published := clinical.HolderRelationships()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: HolderRelationshipLabel(value), Selected: value == selected})
	}

	return options
}

// InsuranceLinks are the URLs one policy's views address.
type InsuranceLinks struct {
	Detail string
	Edit   string
	Record string
}

// InsuranceView is one policy as its views render it.
type InsuranceView struct {
	ID string

	PatientID string

	Company           string
	Type              string
	TypeValue         string
	PlanName          string
	EmployerGroup     string
	MemberName        string
	MemberID          string
	GroupNumber       string
	HolderName        string
	Relationship      string
	RelationshipValue string
	EffectiveOn       string
	ExpiresOn         string
	Status            string
	StatusValue       string
	IsPrimary         bool
	Notes             string

	// Basis is FR-046's per-row reason, empty unless the list this row came
	// from was narrowed by ?expiring_within_days=.
	Basis []string

	Created     Timestamp
	LastChanged Timestamp

	Version string

	Links InsuranceLinks
}

func NewInsuranceView(entity clinical.Insurance, basis []string, links InsuranceLinks) InsuranceView {
	return InsuranceView{
		ID:                entity.ID,
		PatientID:         entity.PatientID,
		Company:           entity.Company,
		Type:              InsuranceTypeLabel(entity.Type),
		TypeValue:         string(entity.Type),
		PlanName:          entity.PlanName,
		EmployerGroup:     entity.EmployerGroup,
		MemberName:        entity.MemberName,
		MemberID:          entity.MemberID,
		GroupNumber:       entity.GroupNumber,
		HolderName:        entity.HolderName,
		Relationship:      HolderRelationshipLabel(entity.Relationship),
		RelationshipValue: string(entity.Relationship),
		EffectiveOn:       entity.EffectiveOn.String(),
		ExpiresOn:         entity.ExpiresOn.String(),
		Status:            InsuranceStatusLabel(entity.Status),
		StatusValue:       string(entity.Status),
		IsPrimary:         entity.IsPrimary,
		Notes:             entity.Notes,
		Basis:             basis,
		Created:           NewTimestamp(entity.CreatedAt),
		LastChanged:       NewTimestamp(entity.UpdatedAt),
		Version:           entity.Version,
		Links:             links,
	}
}

func (v InsuranceView) Entries() []DetailEntry {
	primary := ""
	if v.IsPrimary {
		primary = "enum.bool.yes"
	}

	candidates := []DetailEntry{
		{Field: FieldPlanName, Value: v.PlanName},
		{Field: FieldEmployerGroup, Value: v.EmployerGroup},
		{Field: FieldMemberName, Value: v.MemberName},
		{Field: FieldMemberID, Value: v.MemberID},
		{Field: FieldGroupNumber, Value: v.GroupNumber},
		{Field: FieldHolderName, Value: v.HolderName},
		{Field: FieldRelationshipToHolder, Value: v.Relationship, Translate: true},
		{Field: FieldEffectiveOn, Value: v.EffectiveOn, Datetime: v.EffectiveOn},
		{Field: FieldExpiresOn, Value: v.ExpiresOn, Datetime: v.ExpiresOn},
		{Field: FieldStatus, Value: v.Status, Translate: true},
		{Field: FieldIsPrimary, Value: primary, Translate: true},
		{Field: FieldNotes, Value: v.Notes, Multiline: true},
		{Field: FieldCreated, Value: v.Created.Human, Datetime: v.Created.Machine},
		{Field: FieldLastChanged, Value: v.LastChanged.Human, Datetime: v.LastChanged.Machine},
	}

	entries := make([]DetailEntry, 0, len(candidates))

	for _, entry := range candidates {
		if entry.Value == "" {
			continue
		}

		entry.Label = FieldLabel(entry.Field)
		entries = append(entries, entry)
	}

	return entries
}

// InsuranceListProps is one page of the list.
type InsuranceListProps struct {
	Policies []InsuranceView

	CreateHref   string
	PreviousHref string
	NextHref     string
}

// InsuranceDetailProps is one policy.
type InsuranceDetailProps struct {
	Insurance InsuranceView
}

// InsuranceFormProps is the create form and the edit form.
type InsuranceFormProps struct {
	FormID string
	New    bool

	OnSubmit   string
	CancelHref string

	Insurance InsuranceView
	Errors    FieldErrors

	Notice string

	Tags viewtags.FieldProps
}

func (p InsuranceFormProps) Label() string {
	if p.New {
		return "page.insurance.record"
	}

	return "page.insurance.edit"
}

func (p InsuranceFormProps) SubmitLabel() string {
	if p.New {
		return "action.record_it"
	}

	return "action.save_changes"
}

func (v InsuranceView) TypeOptions() []Option {
	return InsuranceTypeOptions(clinical.InsuranceType(v.TypeValue))
}

func (v InsuranceView) StatusOptions() []Option {
	return InsuranceStatusOptions(clinical.InsuranceStatus(v.StatusValue))
}

func (v InsuranceView) RelationshipOptions() []Option {
	return HolderRelationshipOptions(clinical.HolderRelationship(v.RelationshipValue))
}

func (v InsuranceView) Value(field string) string {
	switch field {
	case FieldCompany:
		return v.Company
	case FieldType:
		return v.TypeValue
	case FieldPlanName:
		return v.PlanName
	case FieldEmployerGroup:
		return v.EmployerGroup
	case FieldMemberName:
		return v.MemberName
	case FieldMemberID:
		return v.MemberID
	case FieldGroupNumber:
		return v.GroupNumber
	case FieldHolderName:
		return v.HolderName
	case FieldRelationshipToHolder:
		return v.RelationshipValue
	case FieldEffectiveOn:
		return v.EffectiveOn
	case FieldExpiresOn:
		return v.ExpiresOn
	case FieldStatus:
		return v.StatusValue
	case FieldNotes:
		return v.Notes
	default:
		return ""
	}
}

func insuranceDeleteExpression(v InsuranceView) string {
	return "@delete(" + jsLiteral(v.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}
