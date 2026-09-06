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
	fieldLabels[FieldCompany] = "Insurer"
	fieldLabels[FieldPlanName] = "Plan name"
	fieldLabels[FieldEmployerGroup] = "Employer group"
	fieldLabels[FieldMemberName] = "Member name"
	fieldLabels[FieldMemberID] = "Member ID"
	fieldLabels[FieldGroupNumber] = "Group number"
	fieldLabels[FieldHolderName] = "Policy holder"
	fieldLabels[FieldRelationshipToHolder] = "Relationship to holder"
	fieldLabels[FieldEffectiveOn] = "Cover starts"
	fieldLabels[FieldExpiresOn] = "Cover ends"
	fieldLabels[FieldIsPrimary] = "Primary policy"
}

var insuranceTypeLabels = map[clinical.InsuranceType]string{
	clinical.InsuranceTypeMedical:      "Medical",
	clinical.InsuranceTypeDental:       "Dental",
	clinical.InsuranceTypeVision:       "Vision",
	clinical.InsuranceTypePrescription: "Prescription",
	clinical.InsuranceTypeOther:        "Other",
}

var insuranceStatusLabels = map[clinical.InsuranceStatus]string{
	clinical.InsuranceStatusActive:   "Active",
	clinical.InsuranceStatusInactive: "Inactive",
	clinical.InsuranceStatusExpired:  "Expired",
	clinical.InsuranceStatusPending:  "Pending",
}

var holderRelationshipLabels = map[clinical.HolderRelationship]string{
	clinical.HolderRelationshipSelf:      "Self",
	clinical.HolderRelationshipSpouse:    "Spouse",
	clinical.HolderRelationshipChild:     "Child",
	clinical.HolderRelationshipDependent: "Dependent",
	clinical.HolderRelationshipOther:     "Other",
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
		primary = "Yes"
	}

	candidates := []DetailEntry{
		{Field: FieldPlanName, Value: v.PlanName},
		{Field: FieldEmployerGroup, Value: v.EmployerGroup},
		{Field: FieldMemberName, Value: v.MemberName},
		{Field: FieldMemberID, Value: v.MemberID},
		{Field: FieldGroupNumber, Value: v.GroupNumber},
		{Field: FieldHolderName, Value: v.HolderName},
		{Field: FieldRelationshipToHolder, Value: v.Relationship},
		{Field: FieldEffectiveOn, Value: v.EffectiveOn, Datetime: v.EffectiveOn},
		{Field: FieldExpiresOn, Value: v.ExpiresOn, Datetime: v.ExpiresOn},
		{Field: FieldStatus, Value: v.Status},
		{Field: FieldIsPrimary, Value: primary},
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
		return "Record an insurance policy"
	}

	return "Edit insurance policy"
}

func (p InsuranceFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}

	return "Save changes"
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
