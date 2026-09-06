package records

import (
	viewtags "medikube/internal/web/views/tags"
	"strings"

	"medikube/internal/domain/clinical"
)

const (
	TreatmentFormLabelCreate = "Record a treatment"
	TreatmentFormLabelEdit   = "Edit treatment"
)

type TreatmentLinks struct {
	Detail string
	Edit   string
	Record string
}

type TreatmentView struct {
	ID        string
	PatientID string

	Name            string
	Type            string
	Setting         string
	SettingVal      string
	Description     string
	StartedOn       string
	EndedOn         string
	Frequency       string
	Dosage          string
	ExpectedOutcome string
	Status          string
	StatusVal       string
	Practitioner    string
	Facility        string
	Condition       string
	Encounters      []string
	Equipment       []string
	Notes           string

	Created     Timestamp
	LastChanged Timestamp

	Version string
	Links   TreatmentLinks
}

func NewTreatmentView(t clinical.Treatment, links TreatmentLinks) TreatmentView {
	return TreatmentView{
		ID:              t.ID,
		PatientID:       t.PatientID,
		Name:            t.Name,
		Type:            t.Type,
		Setting:         string(t.Setting),
		SettingVal:      string(t.Setting),
		Description:     t.Description,
		StartedOn:       t.StartedOn.String(),
		EndedOn:         t.EndedOn.String(),
		Frequency:       t.Frequency,
		Dosage:          t.Dosage,
		ExpectedOutcome: t.ExpectedOutcome,
		Status:          string(t.Status),
		StatusVal:       string(t.Status),
		Practitioner:    t.PractitionerID,
		Facility:        t.FacilityID,
		Condition:       t.ConditionID,
		Encounters:      t.Encounters,
		Equipment:       t.Equipment,
		Notes:           t.Notes,
		Created:         NewTimestamp(t.CreatedAt),
		LastChanged:     NewTimestamp(t.UpdatedAt),
		Version:         t.Version,
		Links:           links,
	}
}

func (t TreatmentView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: FieldType, Value: t.Type},
		{Field: FieldSetting, Value: t.Setting},
		{Field: FieldDescription, Value: t.Description, Multiline: true},
		{Field: FieldStartedOn, Value: t.StartedOn, Datetime: t.StartedOn},
		{Field: FieldEndedOn, Value: t.EndedOn, Datetime: t.EndedOn},
		{Field: FieldFrequency, Value: t.Frequency},
		{Field: FieldDosage, Value: t.Dosage},
		{Field: FieldExpectedOutcome, Value: t.ExpectedOutcome, Multiline: true},
		{Field: FieldStatus, Value: t.Status},
		{Field: FieldPractitioner, Value: t.Practitioner},
		{Field: FieldFacility, Value: t.Facility},
		{Field: FieldCondition, Value: t.Condition},
		{Field: FieldEncounters, Value: strings.Join(t.Encounters, ", ")},
		{Field: FieldEquipment, Value: strings.Join(t.Equipment, ", ")},
		{Field: FieldNotes, Value: t.Notes, Multiline: true},
		{Field: FieldCreated, Value: t.Created.Human, Datetime: t.Created.Machine},
		{Field: FieldLastChanged, Value: t.LastChanged.Human, Datetime: t.LastChanged.Machine},
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

func (t TreatmentView) Value(field string) string {
	switch field {
	case FieldName:
		return t.Name
	case FieldType:
		return t.Type
	case FieldSetting:
		return t.SettingVal
	case FieldDescription:
		return t.Description
	case FieldStartedOn:
		return t.StartedOn
	case FieldEndedOn:
		return t.EndedOn
	case FieldFrequency:
		return t.Frequency
	case FieldDosage:
		return t.Dosage
	case FieldExpectedOutcome:
		return t.ExpectedOutcome
	case FieldStatus:
		return t.StatusVal
	case FieldNotes:
		return t.Notes
	default:
		return ""
	}
}

func (t TreatmentView) SettingOptions() []Option {
	return enumOptions(clinical.TreatmentSettings(), clinical.TreatmentSetting(t.SettingVal))
}
func (t TreatmentView) StatusOptions() []Option {
	return enumOptions(clinical.TherapyStatuses(), clinical.TherapyStatus(t.StatusVal))
}

type TreatmentListProps struct {
	Treatments   []TreatmentView
	CreateHref   string
	PreviousHref string
	NextHref     string
}

type TreatmentDetailProps struct {
	Treatment      TreatmentView
	ReferenceCount int
}

type TreatmentFormProps struct {
	FormID     string
	New        bool
	OnSubmit   string
	CancelHref string

	Treatment TreatmentView
	Errors    FieldErrors
	Notice    string

	Tags viewtags.FieldProps
}

func (p TreatmentFormProps) Label() string {
	if p.New {
		return TreatmentFormLabelCreate
	}
	return TreatmentFormLabelEdit
}

func (p TreatmentFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}
	return "Save changes"
}

func treatmentDeleteExpression(t TreatmentView) string {
	return "@delete(" + jsLiteral(t.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}
