package records

import (
	"medikube/internal/domain/clinical"
)

const (
	FieldAllergen = "allergen"
	FieldReaction = "reaction"
	FieldSeverity = "severity"
	FieldOnsetOn  = "onset_on"
)

const (
	AllergyFormLabelCreate = "Record an allergy"
	AllergyFormLabelEdit   = "Edit allergy"
)

var severityLabels = map[clinical.Severity]string{
	clinical.SeverityMild:            "Mild",
	clinical.SeverityModerate:        "Moderate",
	clinical.SeveritySevere:          "Severe",
	clinical.SeverityLifeThreatening: "Life-threatening",
}

func SeverityLabel(value clinical.Severity) string {
	return label(string(value), severityLabels[value])
}

var conditionStatusLabels = map[clinical.ConditionStatus]string{
	clinical.ConditionStatusActive:   "Active",
	clinical.ConditionStatusHealing:  "Healing",
	clinical.ConditionStatusInactive: "Inactive",
	clinical.ConditionStatusResolved: "Resolved",
	clinical.ConditionStatusChronic:  "Chronic",
}

func ConditionStatusLabel(value clinical.ConditionStatus) string {
	return label(string(value), conditionStatusLabels[value])
}

func SeverityOptions(selected clinical.Severity) []Option {
	published := clinical.Severities()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: SeverityLabel(value), Selected: value == selected})
	}

	return options
}

func ConditionStatusOptions(selected clinical.ConditionStatus) []Option {
	published := clinical.ConditionStatuses()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: ConditionStatusLabel(value), Selected: value == selected})
	}

	return options
}

// AllergyLinks are the URLs one allergy's views address.
type AllergyLinks struct {
	Detail string
	Edit   string
	Record string
}

// AllergyView is one allergy as its views render it.
type AllergyView struct {
	ID        string
	PatientID string

	Allergen      string
	Reaction      string
	Severity      string
	SeverityValue string
	Status        string
	StatusValue   string
	OnsetOn       string
	Notes         string
	Critical      bool

	Created     Timestamp
	LastChanged Timestamp
	Version     string

	Links AllergyLinks
}

func NewAllergyView(allergy clinical.Allergy, links AllergyLinks) AllergyView {
	return AllergyView{
		ID:            allergy.ID,
		PatientID:     allergy.PatientID,
		Allergen:      allergy.Allergen,
		Reaction:      allergy.Reaction,
		Severity:      SeverityLabel(allergy.Severity),
		SeverityValue: string(allergy.Severity),
		Status:        ConditionStatusLabel(allergy.Status),
		StatusValue:   string(allergy.Status),
		OnsetOn:       allergy.OnsetOn.String(),
		Notes:         allergy.Notes,
		Critical:      allergy.Critical(),
		Created:       NewTimestamp(allergy.CreatedAt),
		LastChanged:   NewTimestamp(allergy.UpdatedAt),
		Version:       allergy.Version,
		Links:         links,
	}
}

func (a AllergyView) SeverityOptions() []Option {
	return SeverityOptions(clinical.Severity(a.SeverityValue))
}

func (a AllergyView) StatusOptions() []Option {
	return ConditionStatusOptions(clinical.ConditionStatus(a.StatusValue))
}

func (a AllergyView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: FieldReaction, Value: a.Reaction, Multiline: true},
		{Field: FieldSeverity, Value: a.Severity},
		{Field: FieldStatus, Value: a.Status},
		{Field: FieldOnsetOn, Value: a.OnsetOn, Datetime: a.OnsetOn},
		{Field: FieldNotes, Value: a.Notes, Multiline: true},
		{Field: FieldCreated, Value: a.Created.Human, Datetime: a.Created.Machine},
		{Field: FieldLastChanged, Value: a.LastChanged.Human, Datetime: a.LastChanged.Machine},
	}

	entries := make([]DetailEntry, 0, len(candidates))

	for _, entry := range candidates {
		if entry.Value == "" {
			continue
		}

		entry.Label = allergyFieldLabel(entry.Field)
		entries = append(entries, entry)
	}

	return entries
}

var allergyFieldLabels = map[string]string{
	FieldAllergen:    "Allergen",
	FieldReaction:    "Reaction",
	FieldSeverity:    "Severity",
	FieldStatus:      "State",
	FieldOnsetOn:     "First noticed",
	FieldNotes:       "Notes",
	FieldCreated:     "Recorded",
	FieldLastChanged: "Last changed",
}

func allergyFieldLabel(field string) string {
	if l, known := allergyFieldLabels[field]; known {
		return l
	}

	return field
}

func (a AllergyView) Value(field string) string {
	switch field {
	case FieldAllergen:
		return a.Allergen
	case FieldReaction:
		return a.Reaction
	case FieldSeverity:
		return a.SeverityValue
	case FieldStatus:
		return a.StatusValue
	case FieldOnsetOn:
		return a.OnsetOn
	case FieldNotes:
		return a.Notes
	default:
		return ""
	}
}

// AllergyListProps is one page of the list.
type AllergyListProps struct {
	Allergies []AllergyView

	CreateHref   string
	PreviousHref string
	NextHref     string
}

type AllergyDetailProps struct {
	Allergy     AllergyView
	Medications MedicationLinksEditorProps
}

type AllergyFormProps struct {
	FormID     string
	New        bool
	OnSubmit   string
	CancelHref string

	Allergy AllergyView
	Errors  FieldErrors
	Notice  string
}

func (p AllergyFormProps) Label() string {
	if p.New {
		return AllergyFormLabelCreate
	}

	return AllergyFormLabelEdit
}

func (p AllergyFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}

	return "Save changes"
}
