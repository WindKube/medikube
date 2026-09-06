package records

import (
	"medikube/internal/domain/clinical"
	viewtags "medikube/internal/web/views/tags"
)

// The wire spellings equipment adds beyond medication's own FieldName,
// FieldType, FieldStatus and FieldNotes.
const (
	FieldManufacturer = "manufacturer"
	FieldModel        = "model"
	FieldSerial       = "serial"
	FieldPrescribedOn = "prescribed_on"
	FieldServicedOn   = "serviced_on"
	FieldServiceDueOn = "service_due_on"
	FieldInstructions = "instructions"
	FieldSupplier     = "supplier"
)

var equipmentFields = []string{
	FieldName,
	FieldType,
	FieldManufacturer,
	FieldModel,
	FieldSerial,
	FieldPrescribedOn,
	FieldServicedOn,
	FieldServiceDueOn,
	FieldInstructions,
	FieldStatus,
	FieldNotes,
}

// EquipmentFields is what the form offers, cloned so a caller cannot reorder
// the published one.
func EquipmentFields() []string { return append([]string(nil), equipmentFields...) }

func init() {
	fieldLabels[FieldManufacturer] = "Manufacturer"
	fieldLabels[FieldModel] = "Model"
	fieldLabels[FieldSerial] = "Serial number"
	fieldLabels[FieldPrescribedOn] = "Prescribed"
	fieldLabels[FieldServicedOn] = "Last serviced"
	fieldLabels[FieldServiceDueOn] = "Service due"
	fieldLabels[FieldInstructions] = "Instructions"
	fieldLabels[FieldSupplier] = "Supplier"
}

var equipmentTypeLabels = map[clinical.EquipmentType]string{
	clinical.EquipmentTypeCPAP:         "CPAP machine",
	clinical.EquipmentTypeNebulizer:    "Nebulizer",
	clinical.EquipmentTypeWheelchair:   "Wheelchair",
	clinical.EquipmentTypeWalker:       "Walker",
	clinical.EquipmentTypeGlucoseMeter: "Glucose meter",
	clinical.EquipmentTypeBPMonitor:    "Blood pressure monitor",
	clinical.EquipmentTypeOximeter:     "Pulse oximeter",
	clinical.EquipmentTypeOxygen:       "Oxygen equipment",
	clinical.EquipmentTypeHearingAid:   "Hearing aid",
	clinical.EquipmentTypeProsthetic:   "Prosthetic",
	clinical.EquipmentTypeOrthotic:     "Orthotic",
	clinical.EquipmentTypeOther:        "Other",
}

func EquipmentTypeLabel(value clinical.EquipmentType) string {
	return label(string(value), equipmentTypeLabels[value])
}

func EquipmentTypeOptions(selected clinical.EquipmentType) []Option {
	published := clinical.EquipmentTypes()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: EquipmentTypeLabel(value), Selected: value == selected})
	}

	return options
}

// EquipmentLinks are the URLs one piece of equipment's views address.
type EquipmentLinks struct {
	Detail string
	Edit   string
	Record string
}

// EquipmentView is one piece of equipment as its views render it.
type EquipmentView struct {
	ID string

	PatientID string

	Name         string
	Type         string
	TypeValue    string
	Manufacturer string
	Model        string
	Serial       string
	PrescribedOn string
	ServicedOn   string
	ServiceDueOn string
	Instructions string
	Status       string
	StatusValue  string
	Notes        string

	// Basis is FR-049's per-row overdue/due_soon distinction, empty unless the
	// list this row came from was narrowed by ?service_due_within_days=.
	Basis []string

	Created     Timestamp
	LastChanged Timestamp

	Version string

	Links EquipmentLinks
}

func NewEquipmentView(entity clinical.Equipment, basis []string, links EquipmentLinks) EquipmentView {
	return EquipmentView{
		ID:           entity.ID,
		PatientID:    entity.PatientID,
		Name:         entity.Name,
		Type:         EquipmentTypeLabel(entity.Type),
		TypeValue:    string(entity.Type),
		Manufacturer: entity.Manufacturer,
		Model:        entity.Model,
		Serial:       entity.Serial,
		PrescribedOn: entity.PrescribedOn.String(),
		ServicedOn:   entity.ServicedOn.String(),
		ServiceDueOn: entity.ServiceDueOn.String(),
		Instructions: entity.Instructions,
		Status:       TherapyStatusLabel(entity.Status),
		StatusValue:  string(entity.Status),
		Notes:        entity.Notes,
		Basis:        basis,
		Created:      NewTimestamp(entity.CreatedAt),
		LastChanged:  NewTimestamp(entity.UpdatedAt),
		Version:      entity.Version,
		Links:        links,
	}
}

func (v EquipmentView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: FieldType, Value: v.Type},
		{Field: FieldManufacturer, Value: v.Manufacturer},
		{Field: FieldModel, Value: v.Model},
		{Field: FieldSerial, Value: v.Serial},
		{Field: FieldPrescribedOn, Value: v.PrescribedOn, Datetime: v.PrescribedOn},
		{Field: FieldServicedOn, Value: v.ServicedOn, Datetime: v.ServicedOn},
		{Field: FieldServiceDueOn, Value: v.ServiceDueOn, Datetime: v.ServiceDueOn},
		{Field: FieldInstructions, Value: v.Instructions, Multiline: true},
		{Field: FieldStatus, Value: v.Status},
		{Field: FieldNotes, Value: v.Notes, Multiline: true},
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

// EquipmentListProps is one page of the list.
type EquipmentListProps struct {
	Equipment []EquipmentView

	CreateHref   string
	PreviousHref string
	NextHref     string
}

// EquipmentDetailProps is one piece of equipment.
type EquipmentDetailProps struct {
	Equipment EquipmentView
}

// EquipmentFormProps is the create form and the edit form.
type EquipmentFormProps struct {
	FormID string
	New    bool

	OnSubmit   string
	CancelHref string

	Equipment EquipmentView
	Errors    FieldErrors

	Notice string

	Tags viewtags.FieldProps
}

func (p EquipmentFormProps) Label() string {
	if p.New {
		return "Record equipment"
	}

	return "Edit equipment"
}

func (p EquipmentFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}

	return "Save changes"
}

func (v EquipmentView) TypeOptions() []Option {
	return EquipmentTypeOptions(clinical.EquipmentType(v.TypeValue))
}

func (v EquipmentView) StatusOptions() []Option {
	return TherapyStatusOptions(clinical.TherapyStatus(v.StatusValue))
}

func (v EquipmentView) Value(field string) string {
	switch field {
	case FieldName:
		return v.Name
	case FieldType:
		return v.TypeValue
	case FieldManufacturer:
		return v.Manufacturer
	case FieldModel:
		return v.Model
	case FieldSerial:
		return v.Serial
	case FieldPrescribedOn:
		return v.PrescribedOn
	case FieldServicedOn:
		return v.ServicedOn
	case FieldServiceDueOn:
		return v.ServiceDueOn
	case FieldInstructions:
		return v.Instructions
	case FieldStatus:
		return v.StatusValue
	case FieldNotes:
		return v.Notes
	default:
		return ""
	}
}

func equipmentDeleteExpression(v EquipmentView) string {
	return "@delete(" + jsLiteral(v.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}
