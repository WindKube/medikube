package records

import (
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/person"
	viewtags "medikube/internal/web/views/tags"
)

// The wire spellings family history adds beyond medication's own FieldName.
const (
	FieldFamilyRelationship = "relationship"
	FieldSex                = "sex"
	FieldBirthYear          = "birth_year"
	FieldDeathYear          = "death_year"
	FieldIsDeceased         = "is_deceased"
	FieldConditions         = "conditions"
)

var familyMemberFields = []string{
	FieldName,
	FieldFamilyRelationship,
	FieldSex,
	FieldBirthYear,
	FieldDeathYear,
	FieldIsDeceased,
}

// FamilyMemberFields is what the form offers, cloned so a caller cannot
// reorder the published one.
func FamilyMemberFields() []string { return append([]string(nil), familyMemberFields...) }

func init() {
	fieldLabels[FieldFamilyRelationship] = "field.relationship"
	fieldLabels[FieldSex] = "field.family_member.sex"
	fieldLabels[FieldBirthYear] = "field.family_member.birth_year"
	fieldLabels[FieldDeathYear] = "field.family_member.death_year"
	fieldLabels[FieldIsDeceased] = "field.family_member.is_deceased"
	fieldLabels[FieldConditions] = "field.family_member.conditions"
}

// The values below are message ids (D-06), resolved at render time.
var familyRelationshipLabels = map[clinical.FamilyRelationship]string{
	clinical.FamilyRelationshipMother:      "enum.family_relationship.mother",
	clinical.FamilyRelationshipFather:      "enum.family_relationship.father",
	clinical.FamilyRelationshipSister:      "enum.family_relationship.sister",
	clinical.FamilyRelationshipBrother:     "enum.family_relationship.brother",
	clinical.FamilyRelationshipDaughter:    "enum.family_relationship.daughter",
	clinical.FamilyRelationshipSon:         "enum.family_relationship.son",
	clinical.FamilyRelationshipGrandmother: "enum.family_relationship.grandmother",
	clinical.FamilyRelationshipGrandfather: "enum.family_relationship.grandfather",
	clinical.FamilyRelationshipAunt:        "enum.family_relationship.aunt",
	clinical.FamilyRelationshipUncle:       "enum.family_relationship.uncle",
	clinical.FamilyRelationshipCousin:      "enum.family_relationship.cousin",
	clinical.FamilyRelationshipNiece:       "enum.family_relationship.niece",
	clinical.FamilyRelationshipNephew:      "enum.family_relationship.nephew",
	clinical.FamilyRelationshipHalfSibling: "enum.family_relationship.half_sibling",
	clinical.FamilyRelationshipOther:       "enum.family_relationship.other",
}

func FamilyRelationshipLabel(value clinical.FamilyRelationship) string {
	return label(string(value), familyRelationshipLabels[value])
}

func FamilyRelationshipOptions(selected clinical.FamilyRelationship) []Option {
	published := clinical.FamilyRelationships()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: FamilyRelationshipLabel(value), Selected: value == selected})
	}

	return options
}

// The values below are message ids (D-06), resolved at render time.
var sexLabels = map[person.Sex]string{
	person.SexFemale:      "enum.sex.female",
	person.SexMale:        "enum.sex.male",
	person.SexIntersex:    "enum.sex.intersex",
	person.SexUnspecified: "enum.sex.unspecified",
}

func SexLabel(value person.Sex) string {
	return label(string(value), sexLabels[value])
}

func SexOptions(selected person.Sex) []Option {
	published := person.Sexes()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: SexLabel(value), Selected: value == selected})
	}

	return options
}

// FamilyConditionView is one condition a relative had, as the detail view
// renders it.
type FamilyConditionView struct {
	Name         string
	ICD10Code    string
	DiagnosedAge string
	Severity     string
	Status       string
	Notes        string
}

// FamilyMemberLinks are the URLs one relative's views address.
type FamilyMemberLinks struct {
	Detail string
	Edit   string
	Record string
}

// FamilyMemberView is one relative as its views render it.
type FamilyMemberView struct {
	ID string

	PatientID string

	Name              string
	Relationship      string
	RelationshipValue string
	Sex               string
	SexValue          string
	BirthYear         string
	DeathYear         string
	IsDeceased        bool
	Conditions        []FamilyConditionView

	Created     Timestamp
	LastChanged Timestamp

	Version string

	Links FamilyMemberLinks
}

func NewFamilyMemberView(entity clinical.FamilyMember, links FamilyMemberLinks) FamilyMemberView {
	return FamilyMemberView{
		ID:                entity.ID,
		PatientID:         entity.PatientID,
		Name:              entity.Name,
		Relationship:      FamilyRelationshipLabel(entity.Relationship),
		RelationshipValue: string(entity.Relationship),
		Sex:               SexLabel(entity.Sex),
		SexValue:          string(entity.Sex),
		BirthYear:         yearString(entity.BirthYear),
		DeathYear:         yearString(entity.DeathYear),
		IsDeceased:        entity.IsDeceased,
		Conditions:        familyConditionViews(entity.Conditions),
		Created:           NewTimestamp(entity.CreatedAt),
		LastChanged:       NewTimestamp(entity.UpdatedAt),
		Version:           entity.Version,
		Links:             links,
	}
}

func familyConditionViews(conditions []clinical.FamilyCondition) []FamilyConditionView {
	views := make([]FamilyConditionView, 0, len(conditions))

	for _, condition := range conditions {
		views = append(views, FamilyConditionView{
			Name:         condition.Name,
			ICD10Code:    condition.ICD10Code,
			DiagnosedAge: intString(condition.DiagnosedAge),
			Severity:     SeverityLabel(condition.Severity),
			Status:       ConditionStatusLabel(condition.Status),
			Notes:        condition.Notes,
		})
	}

	return views
}

func yearString(year *int) string {
	return intString(year)
}

func intString(value *int) string {
	if value == nil {
		return ""
	}

	return itoa(*value)
}

// FamilyMemberListProps is one page of the list.
type FamilyMemberListProps struct {
	FamilyMembers []FamilyMemberView

	CreateHref   string
	PreviousHref string
	NextHref     string
}

// FamilyMemberDetailProps is one relative.
type FamilyMemberDetailProps struct {
	FamilyMember FamilyMemberView
}

// FamilyMemberFormProps is the create form and the edit form.
type FamilyMemberFormProps struct {
	FormID string
	New    bool

	OnSubmit   string
	CancelHref string

	FamilyMember FamilyMemberView
	Errors       FieldErrors

	Notice string

	Tags viewtags.FieldProps
}

func (p FamilyMemberFormProps) Label() string {
	if p.New {
		return "page.family_member.record"
	}

	return "page.family_member.edit"
}

func (p FamilyMemberFormProps) SubmitLabel() string {
	if p.New {
		return "action.record_it"
	}

	return "action.save_changes"
}

func (v FamilyMemberView) RelationshipOptions() []Option {
	return FamilyRelationshipOptions(clinical.FamilyRelationship(v.RelationshipValue))
}

func (v FamilyMemberView) SexOptions() []Option {
	return SexOptions(person.Sex(v.SexValue))
}

func (v FamilyMemberView) Value(field string) string {
	switch field {
	case FieldName:
		return v.Name
	case FieldFamilyRelationship:
		return v.RelationshipValue
	case FieldSex:
		return v.SexValue
	case FieldBirthYear:
		return v.BirthYear
	case FieldDeathYear:
		return v.DeathYear
	default:
		return ""
	}
}

func (v FamilyMemberView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: FieldFamilyRelationship, Value: v.Relationship, Translate: true},
		{Field: FieldSex, Value: v.Sex, Translate: true},
		{Field: FieldBirthYear, Value: v.BirthYear},
		{Field: FieldDeathYear, Value: v.DeathYear},
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

func familyMemberDeleteExpression(v FamilyMemberView) string {
	return "@delete(" + jsLiteral(v.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}
