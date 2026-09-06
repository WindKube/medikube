package records

import (
	"strconv"

	"medikube/internal/domain/clinical"
	viewtags "medikube/internal/web/views/tags"
)

// EncounterFormLabelCreate and EncounterFormLabelEdit are message ids (D-06).
const (
	EncounterFormLabelCreate = "page.encounter.record"
	EncounterFormLabelEdit   = "page.encounter.edit"
)

var encounterFields = []string{
	FieldReason, FieldOccurredOn, FieldVisitType, FieldPriority,
	FieldAssessment, FieldPlan, FieldFollowUp, FieldDurationMin,
	FieldPractitioner, FieldFacility, FieldCondition, FieldNotes,
}

func EncounterFields() []string { return append([]string(nil), encounterFields...) }

// EncounterLinks are the URLs one encounter's views address.
type EncounterLinks struct {
	Detail string
	Edit   string
	Record string
}

// EncounterView is one encounter as its views render it.
type EncounterView struct {
	ID        string
	PatientID string

	Reason       string
	OccurredOn   string
	VisitType    string
	VisitTypeVal string
	Priority     string
	PriorityVal  string
	Assessment   string
	Plan         string
	FollowUp     string
	DurationMin  int
	Practitioner string
	Facility     string
	Condition    string
	Notes        string

	Created     Timestamp
	LastChanged Timestamp

	Version string
	Links   EncounterLinks
}

func NewEncounterView(e clinical.Encounter, links EncounterLinks) EncounterView {
	return EncounterView{
		ID:           e.ID,
		PatientID:    e.PatientID,
		Reason:       e.Reason,
		OccurredOn:   e.OccurredOn.String(),
		VisitType:    VisitTypeLabel(e.VisitType),
		VisitTypeVal: string(e.VisitType),
		Priority:     VisitPriorityLabel(e.Priority),
		PriorityVal:  string(e.Priority),
		Assessment:   e.Assessment,
		Plan:         e.Plan,
		FollowUp:     e.FollowUp,
		DurationMin:  e.DurationMin,
		Practitioner: e.PractitionerID,
		Facility:     e.FacilityID,
		Condition:    e.ConditionID,
		Notes:        e.Notes,
		Created:      NewTimestamp(e.CreatedAt),
		LastChanged:  NewTimestamp(e.UpdatedAt),
		Version:      e.Version,
		Links:        links,
	}
}

// Entries is FR-024 applied to encounters: assessment and plan render under
// their own labels, never as "diagnosis" (FR-023).
func (e EncounterView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: FieldOccurredOn, Value: e.OccurredOn, Datetime: e.OccurredOn},
		{Field: FieldVisitType, Value: e.VisitType, Translate: true},
		{Field: FieldPriority, Value: e.Priority, Translate: true},
		{Field: FieldAssessment, Value: e.Assessment, Multiline: true},
		{Field: FieldPlan, Value: e.Plan, Multiline: true},
		{Field: FieldFollowUp, Value: e.FollowUp, Multiline: true},
		{Field: FieldDurationMin, Value: durationString(e.DurationMin)},
		{Field: FieldPractitioner, Value: e.Practitioner},
		{Field: FieldFacility, Value: e.Facility},
		{Field: FieldCondition, Value: e.Condition},
		{Field: FieldNotes, Value: e.Notes, Multiline: true},
		{Field: FieldCreated, Value: e.Created.Human, Datetime: e.Created.Machine},
		{Field: FieldLastChanged, Value: e.LastChanged.Human, Datetime: e.LastChanged.Machine},
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

func (e EncounterView) Value(field string) string {
	switch field {
	case FieldReason:
		return e.Reason
	case FieldOccurredOn:
		return e.OccurredOn
	case FieldVisitType:
		return e.VisitTypeVal
	case FieldPriority:
		return e.PriorityVal
	case FieldAssessment:
		return e.Assessment
	case FieldPlan:
		return e.Plan
	case FieldFollowUp:
		return e.FollowUp
	case FieldDurationMin:
		return durationString(e.DurationMin)
	case FieldNotes:
		return e.Notes
	default:
		return ""
	}
}

func (e EncounterView) VisitTypeOptions() []Option {
	published := clinical.VisitTypes()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{
			Value:    string(value),
			Label:    VisitTypeLabel(value),
			Selected: value == clinical.VisitType(e.VisitTypeVal),
		})
	}

	return options
}

func (e EncounterView) PriorityOptions() []Option {
	published := clinical.VisitPriorities()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{
			Value:    string(value),
			Label:    VisitPriorityLabel(value),
			Selected: value == clinical.VisitPriority(e.PriorityVal),
		})
	}

	return options
}

// EncounterListProps is one page of the list.
type EncounterListProps struct {
	Encounters   []EncounterView
	CreateHref   string
	PreviousHref string
	NextHref     string
}

type EncounterDetailProps struct {
	Encounter EncounterView
}

type EncounterFormProps struct {
	FormID     string
	New        bool
	OnSubmit   string
	CancelHref string

	Encounter EncounterView
	Errors    FieldErrors
	Notice    string

	Tags viewtags.FieldProps
}

func (p EncounterFormProps) Label() string {
	if p.New {
		return EncounterFormLabelCreate
	}
	return EncounterFormLabelEdit
}

func (p EncounterFormProps) SubmitLabel() string {
	if p.New {
		return "action.record_it"
	}
	return "action.save_changes"
}

func encounterDeleteExpression(e EncounterView) string {
	return "@delete(" + jsLiteral(e.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

// The display spellings of encounter's two published vocabularies.
// Values are message ids (D-06), resolved at render time.
var (
	visitTypeLabels = map[clinical.VisitType]string{
		clinical.VisitTypeOffice:     "enum.visit_type.office",
		clinical.VisitTypeTelehealth: "enum.visit_type.telehealth",
		clinical.VisitTypeUrgentCare: "enum.visit_type.urgent_care",
		clinical.VisitTypeEmergency:  "enum.visit_type.emergency",
		clinical.VisitTypeInpatient:  "enum.visit_type.inpatient",
		clinical.VisitTypeFollowUp:   "enum.visit_type.follow_up",
		clinical.VisitTypeAnnual:     "enum.visit_type.annual",
		clinical.VisitTypeOther:      "enum.visit_type.other",
	}

	visitPriorityLabels = map[clinical.VisitPriority]string{
		clinical.VisitPriorityRoutine:   "enum.visit_priority.routine",
		clinical.VisitPriorityUrgent:    "enum.visit_priority.urgent",
		clinical.VisitPriorityEmergency: "enum.visit_priority.emergency",
	}
)

// VisitTypeLabel and VisitPriorityLabel answer with the stored spelling for a
// value they do not know, and with nothing at all for the absent value —
// mirroring medication.go's MedicationTypeLabel.
func VisitTypeLabel(value clinical.VisitType) string {
	return label(string(value), visitTypeLabels[value])
}

func VisitPriorityLabel(value clinical.VisitPriority) string {
	return label(string(value), visitPriorityLabels[value])
}

func durationString(minutes int) string {
	if minutes == 0 {
		return ""
	}
	return strconv.Itoa(minutes)
}
