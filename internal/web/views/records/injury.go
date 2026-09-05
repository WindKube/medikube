package records

import (
	"strings"

	"medikube/internal/domain/clinical"
)

// The names the domain attaches its refusals to and the wire DTO publishes,
// mirroring medication.go's FieldX constants.
const (
	InjuryFieldName          = "name"
	InjuryFieldType          = "type"
	InjuryFieldBodyPart      = "body_part"
	InjuryFieldLaterality    = "laterality"
	InjuryFieldOccurredOn    = "occurred_on"
	InjuryFieldMechanism     = "mechanism"
	InjuryFieldSeverity      = "severity"
	InjuryFieldStatus        = "status"
	InjuryFieldRecoveryNotes = "recovery_notes"
)

const (
	InjuryFieldCreated     = "created"
	InjuryFieldLastChanged = "last_changed"
)

const (
	InjuryFormLabelCreate = "Record an injury"
	InjuryFormLabelEdit   = "Edit injury"
)

// injuryFields is data-model §4.9's column order.
var injuryFields = []string{
	InjuryFieldName,
	InjuryFieldType,
	InjuryFieldBodyPart,
	InjuryFieldLaterality,
	InjuryFieldOccurredOn,
	InjuryFieldMechanism,
	InjuryFieldSeverity,
	InjuryFieldStatus,
	InjuryFieldRecoveryNotes,
}

// InjuryFields is what the form offers, cloned so a caller that sorted it for
// one display could not reorder every form.
func InjuryFields() []string { return append([]string(nil), injuryFields...) }

var injuryFieldLabels = map[string]string{
	InjuryFieldName:          "Name",
	InjuryFieldType:          "Kind of injury",
	InjuryFieldBodyPart:      "Part of the body",
	InjuryFieldLaterality:    "Side",
	InjuryFieldOccurredOn:    "Happened on",
	InjuryFieldMechanism:     "How it happened",
	InjuryFieldSeverity:      "Severity",
	InjuryFieldStatus:        "State",
	InjuryFieldRecoveryNotes: "Recovery notes",
	InjuryFieldCreated:       "Recorded",
	InjuryFieldLastChanged:   "Last changed",
}

// InjuryFieldLabel answers with the field's own name when there is no label,
// the same fallback FieldLabel uses.
func InjuryFieldLabel(field string) string {
	if label, known := injuryFieldLabels[field]; known {
		return label
	}
	return field
}

var (
	injuryTypeLabels = map[clinical.InjuryType]string{
		clinical.InjuryTypeSprain:      "Sprain",
		clinical.InjuryTypeStrain:      "Strain",
		clinical.InjuryTypeFracture:    "Fracture",
		clinical.InjuryTypeDislocation: "Dislocation",
		clinical.InjuryTypeLaceration:  "Laceration",
		clinical.InjuryTypeContusion:   "Contusion (bruise)",
		clinical.InjuryTypeBurn:        "Burn",
		clinical.InjuryTypeConcussion:  "Concussion",
		clinical.InjuryTypePuncture:    "Puncture",
		clinical.InjuryTypeAbrasion:    "Abrasion (scrape)",
		clinical.InjuryTypeOther:       "Other",
	}

	lateralityLabels = map[clinical.Laterality]string{
		clinical.LateralityLeft:          "Left",
		clinical.LateralityRight:         "Right",
		clinical.LateralityBilateral:     "Both sides",
		clinical.LateralityNotApplicable: "Not applicable",
	}
)

func InjuryTypeLabel(value clinical.InjuryType) string {
	return label(string(value), injuryTypeLabels[value])
}

func LateralityLabel(value clinical.Laterality) string {
	return label(string(value), lateralityLabels[value])
}

// InjuryTypeOptions and the three below it walk the domain's own published
// slices, mirroring MedicationTypeOptions.
func InjuryTypeOptions(selected clinical.InjuryType) []Option {
	published := clinical.InjuryTypes()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{
			Value:    string(value),
			Label:    InjuryTypeLabel(value),
			Selected: value == selected,
		})
	}

	return options
}

func LateralityOptions(selected clinical.Laterality) []Option {
	published := clinical.Lateralities()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{
			Value:    string(value),
			Label:    LateralityLabel(value),
			Selected: value == selected,
		})
	}

	return options
}

// InjuryLinks are the URLs one injury's views address, mirroring
// MedicationLinks.
type InjuryLinks struct {
	Detail string
	Edit   string
	Record string
}

// InjuryView is one injury as its views render it.
type InjuryView struct {
	ID string

	PatientID string

	Name            string
	Type            string
	TypeValue       string
	BodyPart        string
	Laterality      string
	LateralityValue string
	OccurredOn      string
	Mechanism       string
	Severity        string
	SeverityValue   string
	Status          string
	StatusValue     string
	RecoveryNotes   string
	MedicationIDs   []string

	Created     Timestamp
	LastChanged Timestamp

	Version string

	Links InjuryLinks
}

// NewInjuryView is the whole of the entity-to-page mapping.
func NewInjuryView(injury clinical.Injury, links InjuryLinks) InjuryView {
	return InjuryView{
		ID:              injury.ID,
		PatientID:       injury.PatientID,
		Name:            injury.Name,
		Type:            InjuryTypeLabel(injury.Type),
		TypeValue:       string(injury.Type),
		BodyPart:        injury.BodyPart,
		Laterality:      LateralityLabel(injury.Laterality),
		LateralityValue: string(injury.Laterality),
		OccurredOn:      injury.OccurredOn.String(),
		Mechanism:       injury.Mechanism,
		Severity:        SeverityLabel(injury.Severity),
		SeverityValue:   string(injury.Severity),
		Status:          ConditionStatusLabel(injury.Status),
		StatusValue:     string(injury.Status),
		RecoveryNotes:   injury.RecoveryNotes,
		MedicationIDs:   injury.MedicationIDs,
		Created:         NewTimestamp(injury.CreatedAt),
		LastChanged:     NewTimestamp(injury.UpdatedAt),
		Version:         injury.Version,
		Links:           links,
	}
}

// Medications renders the linked medication ids as one line, or the empty
// string when there are none — FR-024's "not recorded" applied to a relation
// rather than a scalar.
func (v InjuryView) Medications() string {
	return strings.Join(v.MedicationIDs, ", ")
}

// Entries is FR-024 made a property of the mapping, mirroring MedicationView.
func (v InjuryView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: InjuryFieldType, Value: v.Type},
		{Field: InjuryFieldBodyPart, Value: v.BodyPart},
		{Field: InjuryFieldLaterality, Value: v.Laterality},
		{Field: InjuryFieldOccurredOn, Value: v.OccurredOn, Datetime: v.OccurredOn},
		{Field: InjuryFieldMechanism, Value: v.Mechanism, Multiline: true},
		{Field: InjuryFieldSeverity, Value: v.Severity},
		{Field: InjuryFieldStatus, Value: v.Status},
		{Field: InjuryFieldRecoveryNotes, Value: v.RecoveryNotes, Multiline: true},
		{Field: InjuryFieldCreated, Value: v.Created.Human, Datetime: v.Created.Machine},
		{Field: InjuryFieldLastChanged, Value: v.LastChanged.Human, Datetime: v.LastChanged.Machine},
	}

	entries := make([]DetailEntry, 0, len(candidates))

	for _, entry := range candidates {
		if entry.Value == "" {
			continue
		}
		entry.Label = InjuryFieldLabel(entry.Field)
		entries = append(entries, entry)
	}

	return entries
}

// InjuryListProps is one page of the list.
type InjuryListProps struct {
	Injuries []InjuryView

	CreateHref   string
	PreviousHref string
	NextHref     string
}

// InjuryDetailProps is one injury and its delete confirmation.
type InjuryDetailProps struct {
	Injury InjuryView
}

// InjuryFormProps is the create form and the edit form.
type InjuryFormProps struct {
	FormID string
	New    bool

	OnSubmit   string
	CancelHref string

	Injury InjuryView
	Errors FieldErrors

	Notice string
}

func (p InjuryFormProps) Label() string {
	if p.New {
		return InjuryFormLabelCreate
	}
	return InjuryFormLabelEdit
}

func (v InjuryView) TypeOptions() []Option {
	return InjuryTypeOptions(clinical.InjuryType(v.TypeValue))
}

func (v InjuryView) LateralityOptions() []Option {
	return LateralityOptions(clinical.Laterality(v.LateralityValue))
}

func (v InjuryView) SeverityOptions() []Option {
	return SeverityOptions(clinical.Severity(v.SeverityValue))
}

func (v InjuryView) StatusOptions() []Option {
	return ConditionStatusOptions(clinical.ConditionStatus(v.StatusValue))
}

// Value is what a form control holds for one field.
func (v InjuryView) Value(field string) string {
	switch field {
	case InjuryFieldName:
		return v.Name
	case InjuryFieldType:
		return v.TypeValue
	case InjuryFieldBodyPart:
		return v.BodyPart
	case InjuryFieldLaterality:
		return v.LateralityValue
	case InjuryFieldOccurredOn:
		return v.OccurredOn
	case InjuryFieldMechanism:
		return v.Mechanism
	case InjuryFieldSeverity:
		return v.SeverityValue
	case InjuryFieldStatus:
		return v.StatusValue
	case InjuryFieldRecoveryNotes:
		return v.RecoveryNotes
	default:
		return ""
	}
}

func (p InjuryFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}
	return "Save changes"
}

func injuryDeleteExpression(injury InjuryView) string {
	return "@delete(" + jsLiteral(injury.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}
