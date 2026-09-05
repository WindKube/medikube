package records

import (
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/person"
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
	fieldLabels[FieldFamilyRelationship] = "Relationship"
	fieldLabels[FieldSex] = "Sex"
	fieldLabels[FieldBirthYear] = "Year of birth"
	fieldLabels[FieldDeathYear] = "Year of death"
	fieldLabels[FieldIsDeceased] = "Deceased"
	fieldLabels[FieldConditions] = "Conditions"
}

var familyRelationshipLabels = map[clinical.FamilyRelationship]string{
	clinical.FamilyRelationshipMother:      "Mother",
	clinical.FamilyRelationshipFather:      "Father",
	clinical.FamilyRelationshipSister:      "Sister",
	clinical.FamilyRelationshipBrother:     "Brother",
	clinical.FamilyRelationshipDaughter:    "Daughter",
	clinical.FamilyRelationshipSon:         "Son",
	clinical.FamilyRelationshipGrandmother: "Grandmother",
	clinical.FamilyRelationshipGrandfather: "Grandfather",
	clinical.FamilyRelationshipAunt:        "Aunt",
	clinical.FamilyRelationshipUncle:       "Uncle",
	clinical.FamilyRelationshipCousin:      "Cousin",
	clinical.FamilyRelationshipNiece:       "Niece",
	clinical.FamilyRelationshipNephew:      "Nephew",
	clinical.FamilyRelationshipHalfSibling: "Half-sibling",
	clinical.FamilyRelationshipOther:       "Other",
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

var sexLabels = map[person.Sex]string{
	person.SexFemale:      "Female",
	person.SexMale:        "Male",
	person.SexIntersex:    "Intersex",
	person.SexUnspecified: "Unspecified",
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
}

func (p FamilyMemberFormProps) Label() string {
	if p.New {
		return "Record a relative"
	}

	return "Edit relative"
}

func (p FamilyMemberFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}

	return "Save changes"
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
		{Field: FieldFamilyRelationship, Value: v.Relationship},
		{Field: FieldSex, Value: v.Sex},
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
