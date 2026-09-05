package api

import (
	"fmt"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/service/equipment"
	"medikube/internal/web"
)

// The wire spellings of equipment's own members, not already published by
// dto_medication.go's Member* constants.
const (
	EquipmentFieldManufacturer = "manufacturer"
	EquipmentFieldModel        = "model"
	EquipmentFieldSerial       = "serial"
	EquipmentFieldPrescribedOn = "prescribed_on"
	EquipmentFieldServicedOn   = "serviced_on"
	EquipmentFieldServiceDueOn = "service_due_on"
	EquipmentFieldInstructions = "instructions"
	EquipmentFieldSupplier     = "supplier"
)

// equipmentMembers is data-model §4.11's column order.
var equipmentMembers = []string{
	MemberPatient,
	MemberName,
	MemberType,
	EquipmentFieldManufacturer,
	EquipmentFieldModel,
	EquipmentFieldSerial,
	EquipmentFieldPrescribedOn,
	EquipmentFieldServicedOn,
	EquipmentFieldServiceDueOn,
	EquipmentFieldInstructions,
	MemberStatus,
	EquipmentFieldSupplier,
	MemberPractitioner,
	MemberNotes,
}

// EquipmentSummary is what the list operation returns. Basis is FR-049's
// per-row overdue/due_soon distinction, present only when the list was
// narrowed by ?service_due_within_days=.
type EquipmentSummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Status       string  `json:"status,omitempty"`
	ServiceDueOn *string `json:"service_due_on"`
	UpdatedAt    string  `json:"updated_at"`

	Basis []string `json:"basis,omitempty"`
}

// Equipment is what the detail operations return: every recorded field of
// FR-048 plus the timestamps of FR-020.
type Equipment struct {
	EquipmentSummary

	Patient      string `json:"patient"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
	Serial       string `json:"serial,omitempty"`

	PrescribedOn *string `json:"prescribed_on"`
	ServicedOn   *string `json:"serviced_on"`

	Instructions string `json:"instructions,omitempty"`
	Supplier     string `json:"supplier,omitempty"`
	Practitioner string `json:"practitioner,omitempty"`

	Notes     string   `json:"notes,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	CreatedAt string   `json:"created_at"`
}

// EquipmentCreate is the create body (FR-048): patient, name and type are
// required; everything else is optional at creation.
type EquipmentCreate struct {
	Patient      string   `json:"patient"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Manufacturer string   `json:"manufacturer,omitempty"`
	Model        string   `json:"model,omitempty"`
	Serial       string   `json:"serial,omitempty"`
	PrescribedOn *string  `json:"prescribed_on,omitempty"`
	ServicedOn   *string  `json:"serviced_on,omitempty"`
	ServiceDueOn *string  `json:"service_due_on,omitempty"`
	Instructions string   `json:"instructions,omitempty"`
	Status       string   `json:"status,omitempty"`
	Supplier     *string  `json:"supplier,omitempty"`
	Practitioner *string  `json:"practitioner,omitempty"`
	Notes        string   `json:"notes,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable: a create always supplies its tags,
// even when that is none.
func (c *EquipmentCreate) TagIDs() (ids []string, supplied bool) { return c.Tags, true }

// EquipmentPatch is the partial update.
type EquipmentPatch struct {
	Name         *string `json:"name,omitempty"`
	Type         *string `json:"type,omitempty"`
	Manufacturer *string `json:"manufacturer,omitempty"`
	Model        *string `json:"model,omitempty"`
	Serial       *string `json:"serial,omitempty"`

	PrescribedOn web.Optional[string] `json:"prescribed_on,omitzero"`
	ServicedOn   web.Optional[string] `json:"serviced_on,omitzero"`
	ServiceDueOn web.Optional[string] `json:"service_due_on,omitzero"`

	Instructions *string `json:"instructions,omitempty"`
	Status       *string `json:"status,omitempty"`
	Supplier     *string `json:"supplier,omitempty"`
	Practitioner *string `json:"practitioner,omitempty"`
	Notes        *string `json:"notes,omitempty"`

	// Tags is replace-set (FR-064, FR-065): a nil pointer leaves the
	// applied tags alone, a non-nil one — including an empty array —
	// replaces the whole set.
	Tags *[]string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable.
func (p *EquipmentPatch) TagIDs() (ids []string, supplied bool) {
	if p.Tags == nil {
		return nil, false
	}

	return *p.Tags, true
}

// EquipmentCodec is the DTO boundary for equipment.
type EquipmentCodec struct{}

var _ equipment.Codec = EquipmentCodec{}

func EquipmentSchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(EquipmentSummary) },
		NewDetail:  func() any { return new(Equipment) },
		NewCreate:  func() any { return new(EquipmentCreate) },
		NewPatch:   func() any { return new(EquipmentPatch) },
	}
}

// EquipmentSearchFields reads the search_index columns off the wire DTO: the
// name, and the instructions and notes.
func EquipmentSearchFields(body any) (title, text string) {
	found, ok := body.(*Equipment)
	if !ok {
		return "", ""
	}

	return found.Name, found.Instructions + " " + found.Notes
}

// EquipmentBasis reads back the per-row reason EquipmentCodec.Summary already
// computed (FR-049).
func EquipmentBasis(body any, _ records.Criteria) []string {
	summary, ok := body.(*EquipmentSummary)
	if !ok {
		return nil
	}

	return summary.Basis
}

func (EquipmentCodec) Summary(entity clinical.Equipment, basis []string) any {
	return &EquipmentSummary{
		ID:           entity.ID,
		Kind:         kind.Equipment.Enum(),
		Name:         entity.Name,
		Type:         string(entity.Type),
		Status:       string(entity.Status),
		ServiceDueOn: wireDate(entity.ServiceDueOn),
		UpdatedAt:    wireInstant(entity.UpdatedAt),
		Basis:        basis,
	}
}

func (c EquipmentCodec) Detail(entity clinical.Equipment) any {
	summary, ok := c.Summary(entity, nil).(*EquipmentSummary)
	if !ok {
		return &Equipment{}
	}

	return &Equipment{
		EquipmentSummary: *summary,
		Patient:          entity.PatientID,
		Manufacturer:     entity.Manufacturer,
		Model:            entity.Model,
		Serial:           entity.Serial,
		PrescribedOn:     wireDate(entity.PrescribedOn),
		ServicedOn:       wireDate(entity.ServicedOn),
		Instructions:     entity.Instructions,
		Supplier:         entity.SupplierID,
		Practitioner:     entity.PractitionerID,
		Notes:            entity.Notes,
		Tags:             entity.Tags,
		CreatedAt:        wireInstant(entity.CreatedAt),
	}
}

func (EquipmentCodec) Draft(body any) (clinical.Equipment, error) {
	create, ok := body.(*EquipmentCreate)
	if !ok {
		return clinical.Equipment{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	prescribedOn := readDate(&invalid, EquipmentFieldPrescribedOn, create.PrescribedOn)
	servicedOn := readDate(&invalid, EquipmentFieldServicedOn, create.ServicedOn)
	serviceDueOn := readDate(&invalid, EquipmentFieldServiceDueOn, create.ServiceDueOn)

	if err := orderedRefusal2(&invalid, equipmentMembers); err != nil {
		return clinical.Equipment{}, err
	}

	return clinical.Equipment{
		PatientID:      create.Patient,
		Name:           create.Name,
		Type:           clinical.EquipmentType(create.Type),
		Manufacturer:   create.Manufacturer,
		Model:          create.Model,
		Serial:         create.Serial,
		PrescribedOn:   prescribedOn,
		ServicedOn:     servicedOn,
		ServiceDueOn:   serviceDueOn,
		Instructions:   create.Instructions,
		Status:         clinical.TherapyStatus(create.Status),
		SupplierID:     deref(create.Supplier),
		PractitionerID: deref(create.Practitioner),
		Notes:          create.Notes,
		Tags:           create.Tags,
	}, nil
}

func (EquipmentCodec) Patch(body any) (equipment.Patch, error) {
	incoming, ok := body.(*EquipmentPatch)
	if !ok {
		return equipment.Patch{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	patch := equipment.Patch{
		Name:         incoming.Name,
		Type:         convert[clinical.EquipmentType](incoming.Type),
		Manufacturer: incoming.Manufacturer,
		Model:        incoming.Model,
		Serial:       incoming.Serial,
		PrescribedOn: readOptionalDate(&invalid, EquipmentFieldPrescribedOn, incoming.PrescribedOn),
		ServicedOn:   readOptionalDate(&invalid, EquipmentFieldServicedOn, incoming.ServicedOn),
		ServiceDueOn: readOptionalDate(&invalid, EquipmentFieldServiceDueOn, incoming.ServiceDueOn),
		Instructions: incoming.Instructions,
		Status:       convert[clinical.TherapyStatus](incoming.Status),
		Notes:        incoming.Notes,
		Supplier:     incoming.Supplier,
		Practitioner: incoming.Practitioner,
		Tags:         incoming.Tags,
	}

	if err := orderedRefusal2(&invalid, equipmentMembers); err != nil {
		return equipment.Patch{}, err
	}

	return patch, nil
}
