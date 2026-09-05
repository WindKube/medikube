package records

import "medikube/internal/domain/clinical"

const (
	ProcedureFormLabelCreate = "Record a procedure"
	ProcedureFormLabelEdit   = "Edit procedure"
)

type ProcedureLinks struct {
	Detail string
	Edit   string
	Record string
}

type ProcedureView struct {
	ID        string
	PatientID string

	Name            string
	Type            string
	TypeVal         string
	Code            string
	Description     string
	OccurredOn      string
	Status          string
	StatusVal       string
	Outcome         string
	OutcomeVal      string
	Setting         string
	SettingVal      string
	Complications   string
	DurationMin     int
	Anesthesia      string
	AnesthesiaVal   string
	AnesthesiaNotes string
	Practitioner    string
	Facility        string
	Condition       string
	Notes           string

	Created     Timestamp
	LastChanged Timestamp

	Version string
	Links   ProcedureLinks
}

func NewProcedureView(p clinical.Procedure, links ProcedureLinks) ProcedureView {
	return ProcedureView{
		ID:              p.ID,
		PatientID:       p.PatientID,
		Name:            p.Name,
		Type:            string(p.Type),
		TypeVal:         string(p.Type),
		Code:            p.Code,
		Description:     p.Description,
		OccurredOn:      p.OccurredOn.String(),
		Status:          string(p.Status),
		StatusVal:       string(p.Status),
		Outcome:         string(p.Outcome),
		OutcomeVal:      string(p.Outcome),
		Setting:         string(p.Setting),
		SettingVal:      string(p.Setting),
		Complications:   p.Complications,
		DurationMin:     p.DurationMin,
		Anesthesia:      string(p.Anesthesia),
		AnesthesiaVal:   string(p.Anesthesia),
		AnesthesiaNotes: p.AnesthesiaNotes,
		Practitioner:    p.PractitionerID,
		Facility:        p.FacilityID,
		Condition:       p.ConditionID,
		Notes:           p.Notes,
		Created:         NewTimestamp(p.CreatedAt),
		LastChanged:     NewTimestamp(p.UpdatedAt),
		Version:         p.Version,
		Links:           links,
	}
}

func (p ProcedureView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: FieldType, Value: p.Type},
		{Field: FieldCode, Value: p.Code},
		{Field: FieldDescription, Value: p.Description, Multiline: true},
		{Field: FieldOccurredOn, Value: p.OccurredOn, Datetime: p.OccurredOn},
		{Field: FieldStatus, Value: p.Status},
		{Field: FieldOutcome, Value: p.Outcome},
		{Field: FieldSetting, Value: p.Setting},
		{Field: FieldComplications, Value: p.Complications, Multiline: true},
		{Field: FieldDurationMin, Value: durationString(p.DurationMin)},
		{Field: FieldAnesthesia, Value: p.Anesthesia},
		{Field: FieldAnesthesiaNote, Value: p.AnesthesiaNotes, Multiline: true},
		{Field: FieldPractitioner, Value: p.Practitioner},
		{Field: FieldFacility, Value: p.Facility},
		{Field: FieldCondition, Value: p.Condition},
		{Field: FieldNotes, Value: p.Notes, Multiline: true},
		{Field: FieldCreated, Value: p.Created.Human, Datetime: p.Created.Machine},
		{Field: FieldLastChanged, Value: p.LastChanged.Human, Datetime: p.LastChanged.Machine},
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

func (p ProcedureView) Value(field string) string {
	switch field {
	case FieldName:
		return p.Name
	case FieldType:
		return p.TypeVal
	case FieldCode:
		return p.Code
	case FieldDescription:
		return p.Description
	case FieldOccurredOn:
		return p.OccurredOn
	case FieldStatus:
		return p.StatusVal
	case FieldOutcome:
		return p.OutcomeVal
	case FieldSetting:
		return p.SettingVal
	case FieldComplications:
		return p.Complications
	case FieldDurationMin:
		return durationString(p.DurationMin)
	case FieldAnesthesia:
		return p.AnesthesiaVal
	case FieldAnesthesiaNote:
		return p.AnesthesiaNotes
	case FieldNotes:
		return p.Notes
	default:
		return ""
	}
}

func (p ProcedureView) TypeOptions() []Option {
	return enumOptions(clinical.ProcedureTypes(), clinical.ProcedureType(p.TypeVal))
}
func (p ProcedureView) StatusOptions() []Option {
	return enumOptions(clinical.OrderStatuses(), clinical.OrderStatus(p.StatusVal))
}
func (p ProcedureView) OutcomeOptions() []Option {
	return enumOptions(clinical.ProcedureOutcomes(), clinical.ProcedureOutcome(p.OutcomeVal))
}
func (p ProcedureView) SettingOptions() []Option {
	return enumOptions(clinical.ProcedureSettings(), clinical.ProcedureSetting(p.SettingVal))
}
func (p ProcedureView) AnesthesiaOptions() []Option {
	return enumOptions(clinical.Anesthesias(), clinical.Anesthesia(p.AnesthesiaVal))
}

type ProcedureListProps struct {
	Procedures   []ProcedureView
	CreateHref   string
	PreviousHref string
	NextHref     string
}

type ProcedureDetailProps struct {
	Procedure ProcedureView
}

type ProcedureFormProps struct {
	FormID     string
	New        bool
	OnSubmit   string
	CancelHref string

	Procedure ProcedureView
	Errors    FieldErrors
	Notice    string
}

func (p ProcedureFormProps) Label() string {
	if p.New {
		return ProcedureFormLabelCreate
	}
	return ProcedureFormLabelEdit
}

func (p ProcedureFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}
	return "Save changes"
}

func procedureDeleteExpression(p ProcedureView) string {
	return "@delete(" + jsLiteral(p.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}
