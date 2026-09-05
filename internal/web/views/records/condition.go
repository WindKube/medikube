package records

import "medikube/internal/domain/clinical"

const (
	FieldDiagnosis  = "diagnosis"
	FieldResolvedOn = "resolved_on"
	FieldICD10Code  = "icd10_code"
	FieldSNOMEDCode = "snomed_code"
)

const (
	ConditionFormLabelCreate = "Record a condition"
	ConditionFormLabelEdit   = "Edit condition"
)

type ConditionLinks struct {
	Detail string
	Edit   string
	Record string
}

type ConditionView struct {
	ID        string
	PatientID string

	Diagnosis     string
	Status        string
	StatusValue   string
	Severity      string
	SeverityValue string
	OnsetOn       string
	ResolvedOn    string
	ICD10Code     string
	SNOMEDCode    string
	Notes         string

	Created     Timestamp
	LastChanged Timestamp
	Version     string

	Links ConditionLinks
}

func NewConditionView(condition clinical.Condition, links ConditionLinks) ConditionView {
	return ConditionView{
		ID:            condition.ID,
		PatientID:     condition.PatientID,
		Diagnosis:     condition.Diagnosis,
		Status:        ConditionStatusLabel(condition.Status),
		StatusValue:   string(condition.Status),
		Severity:      SeverityLabel(condition.Severity),
		SeverityValue: string(condition.Severity),
		OnsetOn:       condition.OnsetOn.String(),
		ResolvedOn:    condition.ResolvedOn.String(),
		ICD10Code:     condition.ICD10Code,
		SNOMEDCode:    condition.SNOMEDCode,
		Notes:         condition.Notes,
		Created:       NewTimestamp(condition.CreatedAt),
		LastChanged:   NewTimestamp(condition.UpdatedAt),
		Version:       condition.Version,
		Links:         links,
	}
}

func (c ConditionView) SeverityOptions() []Option {
	return SeverityOptions(clinical.Severity(c.SeverityValue))
}

func (c ConditionView) StatusOptions() []Option {
	return ConditionStatusOptions(clinical.ConditionStatus(c.StatusValue))
}

var conditionFieldLabels = map[string]string{
	FieldDiagnosis:   "Diagnosis",
	FieldStatus:      "State",
	FieldSeverity:    "Severity",
	FieldOnsetOn:     "Onset",
	FieldResolvedOn:  "Resolved",
	FieldICD10Code:   "ICD-10 code",
	FieldSNOMEDCode:  "SNOMED code",
	FieldNotes:       "Notes",
	FieldCreated:     "Recorded",
	FieldLastChanged: "Last changed",
}

func conditionFieldLabel(field string) string {
	if l, known := conditionFieldLabels[field]; known {
		return l
	}

	return field
}

func (c ConditionView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: FieldStatus, Value: c.Status},
		{Field: FieldSeverity, Value: c.Severity},
		{Field: FieldOnsetOn, Value: c.OnsetOn, Datetime: c.OnsetOn},
		{Field: FieldResolvedOn, Value: c.ResolvedOn, Datetime: c.ResolvedOn},
		{Field: FieldICD10Code, Value: c.ICD10Code},
		{Field: FieldSNOMEDCode, Value: c.SNOMEDCode},
		{Field: FieldNotes, Value: c.Notes, Multiline: true},
		{Field: FieldCreated, Value: c.Created.Human, Datetime: c.Created.Machine},
		{Field: FieldLastChanged, Value: c.LastChanged.Human, Datetime: c.LastChanged.Machine},
	}

	entries := make([]DetailEntry, 0, len(candidates))

	for _, entry := range candidates {
		if entry.Value == "" {
			continue
		}

		entry.Label = conditionFieldLabel(entry.Field)
		entries = append(entries, entry)
	}

	return entries
}

func (c ConditionView) Value(field string) string {
	switch field {
	case FieldDiagnosis:
		return c.Diagnosis
	case FieldStatus:
		return c.StatusValue
	case FieldSeverity:
		return c.SeverityValue
	case FieldOnsetOn:
		return c.OnsetOn
	case FieldResolvedOn:
		return c.ResolvedOn
	case FieldICD10Code:
		return c.ICD10Code
	case FieldSNOMEDCode:
		return c.SNOMEDCode
	case FieldNotes:
		return c.Notes
	default:
		return ""
	}
}

type ConditionListProps struct {
	Conditions []ConditionView

	CreateHref   string
	PreviousHref string
	NextHref     string
}

type ConditionDetailProps struct {
	Condition ConditionView
}

type ConditionFormProps struct {
	FormID     string
	New        bool
	OnSubmit   string
	CancelHref string

	Condition ConditionView
	Errors    FieldErrors
	Notice    string
}

func (p ConditionFormProps) Label() string {
	if p.New {
		return ConditionFormLabelCreate
	}

	return ConditionFormLabelEdit
}

func (p ConditionFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}

	return "Save changes"
}
