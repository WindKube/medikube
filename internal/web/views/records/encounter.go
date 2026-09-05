package records

import (
	"strconv"

	"medikube/internal/domain/clinical"
)

const (
	EncounterFormLabelCreate = "Record an encounter"
	EncounterFormLabelEdit   = "Edit encounter"
)

var encounterFields = []string{
	FieldReason, FieldOccurredOn, FieldVisitType, FieldPriority,
	FieldAssessment, FieldPlan, FieldFollowUp, FieldDurationMin,
	FieldPractitioner, FieldFacility, FieldNotes,
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
		VisitType:    string(e.VisitType),
		VisitTypeVal: string(e.VisitType),
		Priority:     string(e.Priority),
		PriorityVal:  string(e.Priority),
		Assessment:   e.Assessment,
		Plan:         e.Plan,
		FollowUp:     e.FollowUp,
		DurationMin:  e.DurationMin,
		Practitioner: e.PractitionerID,
		Facility:     e.FacilityID,
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
		{Field: FieldVisitType, Value: e.VisitType},
		{Field: FieldPriority, Value: e.Priority},
		{Field: FieldAssessment, Value: e.Assessment, Multiline: true},
		{Field: FieldPlan, Value: e.Plan, Multiline: true},
		{Field: FieldFollowUp, Value: e.FollowUp, Multiline: true},
		{Field: FieldDurationMin, Value: durationString(e.DurationMin)},
		{Field: FieldPractitioner, Value: e.Practitioner},
		{Field: FieldFacility, Value: e.Facility},
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
	return enumOptions(clinical.VisitTypes(), clinical.VisitType(e.VisitTypeVal))
}
func (e EncounterView) PriorityOptions() []Option {
	return enumOptions(clinical.VisitPriorities(), clinical.VisitPriority(e.PriorityVal))
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
}

func (p EncounterFormProps) Label() string {
	if p.New {
		return EncounterFormLabelCreate
	}
	return EncounterFormLabelEdit
}

func (p EncounterFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}
	return "Save changes"
}

func encounterDeleteExpression(e EncounterView) string {
	return "@delete(" + jsLiteral(e.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}

// enumOptions walks a published vocabulary generically, so a form cannot
// offer a value the domain refuses or withhold one it accepts.
func enumOptions[T ~string](published []T, selected T) []Option {
	options := make([]Option, 0, len(published))
	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: string(value), Selected: value == selected})
	}
	return options
}

func durationString(minutes int) string {
	if minutes == 0 {
		return ""
	}
	return strconv.Itoa(minutes)
}
