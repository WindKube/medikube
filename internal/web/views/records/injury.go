package records

import (
	"strings"

	"medikube/internal/domain/clinical"
	viewtags "medikube/internal/web/views/tags"
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

// The values below are message ids (D-06), resolved at render time.
const (
	InjuryFormLabelCreate = "page.injury.record"
	InjuryFormLabelEdit   = "page.injury.edit"
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

// The values below are message ids (D-06), resolved at render time.
var injuryFieldLabels = map[string]string{
	InjuryFieldName:          "field.name",
	InjuryFieldType:          "field.injury.type",
	InjuryFieldBodyPart:      "field.injury.body_part",
	InjuryFieldLaterality:    "field.injury.laterality",
	InjuryFieldOccurredOn:    "field.injury.occurred_on",
	InjuryFieldMechanism:     "field.injury.mechanism",
	InjuryFieldSeverity:      "field.severity",
	InjuryFieldStatus:        "field.status",
	InjuryFieldRecoveryNotes: "field.injury.recovery_notes",
	InjuryFieldCreated:       "field.recorded",
	InjuryFieldLastChanged:   "field.last_changed",
}

// InjuryFieldLabel answers with the field's own name when there is no label,
// the same fallback FieldLabel uses.
func InjuryFieldLabel(field string) string {
	if label, known := injuryFieldLabels[field]; known {
		return label
	}
	return field
}

// The values below are message ids (D-06), resolved at render time.
var (
	injuryTypeLabels = map[clinical.InjuryType]string{
		clinical.InjuryTypeSprain:      "enum.injury_type.sprain",
		clinical.InjuryTypeStrain:      "enum.injury_type.strain",
		clinical.InjuryTypeFracture:    "enum.injury_type.fracture",
		clinical.InjuryTypeDislocation: "enum.injury_type.dislocation",
		clinical.InjuryTypeLaceration:  "enum.injury_type.laceration",
		clinical.InjuryTypeContusion:   "enum.injury_type.contusion",
		clinical.InjuryTypeBurn:        "enum.injury_type.burn",
		clinical.InjuryTypeConcussion:  "enum.injury_type.concussion",
		clinical.InjuryTypePuncture:    "enum.injury_type.puncture",
		clinical.InjuryTypeAbrasion:    "enum.injury_type.abrasion",
		clinical.InjuryTypeOther:       "enum.injury_type.other",
	}

	lateralityLabels = map[clinical.Laterality]string{
		clinical.LateralityLeft:          "enum.laterality.left",
		clinical.LateralityRight:         "enum.laterality.right",
		clinical.LateralityBilateral:     "enum.laterality.bilateral",
		clinical.LateralityNotApplicable: "enum.laterality.not_applicable",
	}
)

func InjuryTypeLabel(value clinical.InjuryType) string {
	return label(string(value), injuryTypeLabels[value])
}

func LateralityLabel(value clinical.Laterality) string {
	return label(string(value), lateralityLabels[value])
}

// SeverityLabel and ConditionStatusLabel are declared once, in allergy.go —
// injury shares both vocabularies rather than redeclaring their maps.

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

// SeverityOptions and ConditionStatusOptions are declared once, in
// allergy.go — injury shares both option builders rather than redeclaring
// them.

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
		{Field: InjuryFieldType, Value: v.Type, Translate: true},
		{Field: InjuryFieldBodyPart, Value: v.BodyPart},
		{Field: InjuryFieldLaterality, Value: v.Laterality, Translate: true},
		{Field: InjuryFieldOccurredOn, Value: v.OccurredOn, Datetime: v.OccurredOn},
		{Field: InjuryFieldMechanism, Value: v.Mechanism, Multiline: true},
		{Field: InjuryFieldSeverity, Value: v.Severity, Translate: true},
		{Field: InjuryFieldStatus, Value: v.Status, Translate: true},
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
	Injury      InjuryView
	Medications MedicationLinksEditorProps
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

	Tags viewtags.FieldProps
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
		return "action.record_it"
	}
	return "action.save_changes"
}

func injuryDeleteExpression(injury InjuryView) string {
	return "@delete(" + jsLiteral(injury.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}
