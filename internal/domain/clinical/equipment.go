package clinical

import (
	"strings"
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

const (
	equipmentNameMin         = 2
	equipmentNameMax         = 200
	equipmentManufacturerMax = 200
	equipmentModelMax        = 100
	equipmentSerialMax       = 100
	equipmentInstructionsMax = 5000
)

// Equipment is one piece of medical equipment a person depends on
// (data-model §4.11).
type Equipment struct {
	ID        string
	PatientID string

	Name         string
	Type         EquipmentType
	Manufacturer string
	Model        string
	Serial       string

	PrescribedOn   Date
	ServicedOn     Date
	ServiceDueOn   Date
	Instructions   string
	Status         TherapyStatus
	SupplierID     string
	PractitionerID string

	Notes string

	// Tags is data-model §0.8's universal field: any number of the owning
	// account's tags, applied with replace-set semantics (FR-064).
	Tags []string

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string
}

// Validate enforces FR-048: name and type are required; service_due_on, when
// present, is not before serviced_on.
func (e Equipment) Validate() error {
	var invalid domain.ValidationError

	if name := strings.TrimSpace(e.Name); name == "" {
		invalid.Add("name", domain.CodeRequired, "a name is required")
	} else if utf8Len(name) < equipmentNameMin {
		invalid.Addf("name", domain.CodeTooShort, "the name accepts at least %d characters", equipmentNameMin)
	} else {
		checkLength(&invalid, "name", "the name", name, equipmentNameMax)
	}

	if e.Type == "" {
		invalid.Add("type", domain.CodeRequired, "a type is required")
	} else if !e.Type.Valid() {
		invalid.Add("type", domain.CodeInvalidValue, "not one of the kinds MediKube accepts")
	}

	checkLength(&invalid, "manufacturer", "the manufacturer", e.Manufacturer, equipmentManufacturerMax)
	checkLength(&invalid, "model", "the model", e.Model, equipmentModelMax)
	checkLength(&invalid, "serial", "the serial number", e.Serial, equipmentSerialMax)
	checkLength(&invalid, "instructions", "the instructions", e.Instructions, equipmentInstructionsMax)

	if err := Order(Ref{Field: "serviced_on", Value: e.ServicedOn}, Ref{Field: "service_due_on", Value: e.ServiceDueOn}); err != nil {
		invalid.Add(err.Field, err.Code, err.Message)
	}

	if e.Status != "" && !e.Status.Valid() {
		invalid.Add("status", domain.CodeInvalidValue, "not one of the states MediKube accepts")
	}

	checkLength(&invalid, "notes", "the notes", e.Notes, maxNotes)

	return invalid.OrNil()
}

// MarshalZerologObject emits the two identifiers and nothing else. The serial
// number is PHI-adjacent (data-model §4.11) and never reaches this method.
func (e Equipment) MarshalZerologObject(ev *zerolog.Event) {
	ev.Str("equipment_id", e.ID).Str("patient_id", e.PatientID)
}

func utf8Len(s string) int {
	count := 0
	for range s {
		count++
	}
	return count
}
