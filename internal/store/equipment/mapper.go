package equipment

import (
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

// The stored column names, spelled once here and nowhere the migration did not
// already spell them (internal/store/migrations/1756400010_equipment.go).
const (
	fieldPatient      = "patient"
	fieldName         = "name"
	fieldType         = "type"
	fieldManufacturer = "manufacturer"
	fieldModel        = "model"
	fieldSerial       = "serial"
	fieldPrescribedOn = "prescribed_on"
	fieldServicedOn   = "serviced_on"
	fieldServiceDueOn = "service_due_on"
	fieldInstructions = "instructions"
	fieldStatus       = "status"
	fieldSupplier     = "supplier"
	fieldPractitioner = "practitioner"
	fieldNotes        = "notes"
	fieldTags         = "tags"
	fieldCreated      = "created"
	fieldUpdated      = "updated"
)

// The published column names, exported for the repository and for a future
// caller that builds a Query against this schema.
const (
	ColumnID           = "id"
	ColumnPatient      = fieldPatient
	ColumnName         = fieldName
	ColumnType         = fieldType
	ColumnStatus       = fieldStatus
	ColumnPrescribedOn = fieldPrescribedOn
	ColumnServiceDueOn = fieldServiceDueOn
)

// ErrUnexpectedCollection mirrors internal/store's own sentinel: a record
// handed to the wrong mapper.
var ErrUnexpectedCollection = errors.New("equipment: the record is not from the equipment collection")

// Schema is this kind's query surface.
func Schema() store.Schema {
	return store.NewSchema(kind.Equipment.Collection(),
		store.Column{Name: fieldPatient},
		store.Column{
			Name:       fieldName,
			Expr:       "LOWER(" + quote(fieldName) + ")",
			Searchable: true,
			Value:      func(record *core.Record) string { return asciiLower(record.GetString(fieldName)) },
		},
		store.Column{Name: fieldType},
		store.Column{Name: fieldStatus},
		store.Column{Name: fieldPrescribedOn, AbsentLast: true},
		store.Column{Name: fieldServiceDueOn},
		// FilterOnly: `?tags=` narrows, but a multi-select relation's JSON
		// column is never an ordering (research D-05's cursor-disclosure
		// rule).
		store.Column{Name: fieldTags, FilterOnly: true},
		store.Column{Name: fieldCreated},
		store.Column{Name: fieldUpdated},
	)
}

func quote(name string) string { return "[[" + name + "]]" }

func asciiLower(value string) string {
	folded := []byte(value)
	for i, b := range folded {
		if b >= 'A' && b <= 'Z' {
			folded[i] = b + ('a' - 'A')
		}
	}

	return string(folded)
}

func expectCollection(record *core.Record) error {
	if record.Collection().Name != kind.Equipment.Collection() {
		return ErrUnexpectedCollection
	}

	return nil
}

// FromRecord reads a stored row into the entity. It does not validate: a row
// already stored is a fact whatever the current rules say about it.
func FromRecord(record *core.Record) (clinical.Equipment, error) {
	if err := expectCollection(record); err != nil {
		return clinical.Equipment{}, err
	}

	prescribedOn, err := recordDate(record, fieldPrescribedOn)
	if err != nil {
		return clinical.Equipment{}, err
	}

	servicedOn, err := recordDate(record, fieldServicedOn)
	if err != nil {
		return clinical.Equipment{}, err
	}

	serviceDueOn, err := recordDate(record, fieldServiceDueOn)
	if err != nil {
		return clinical.Equipment{}, err
	}

	return clinical.Equipment{
		ID:             record.Id,
		PatientID:      record.GetString(fieldPatient),
		Name:           record.GetString(fieldName),
		Type:           clinical.EquipmentType(record.GetString(fieldType)),
		Manufacturer:   record.GetString(fieldManufacturer),
		Model:          record.GetString(fieldModel),
		Serial:         record.GetString(fieldSerial),
		PrescribedOn:   prescribedOn,
		ServicedOn:     servicedOn,
		ServiceDueOn:   serviceDueOn,
		Instructions:   record.GetString(fieldInstructions),
		Status:         clinical.TherapyStatus(record.GetString(fieldStatus)),
		SupplierID:     record.GetString(fieldSupplier),
		PractitionerID: record.GetString(fieldPractitioner),
		Notes:          record.GetString(fieldNotes),
		Tags:           record.GetStringSlice(fieldTags),
		CreatedAt:      recordInstant(record, fieldCreated),
		UpdatedAt:      recordInstant(record, fieldUpdated),
		Version:        store.Version(record),
	}, nil
}

// ToRecord writes the entity's columns onto the record. It never writes id,
// created, updated or version — PocketBase owns all four.
func ToRecord(record *core.Record, entity clinical.Equipment) error {
	if err := expectCollection(record); err != nil {
		return err
	}

	record.Set(fieldPatient, entity.PatientID)
	record.Set(fieldName, entity.Name)
	record.Set(fieldType, string(entity.Type))
	record.Set(fieldManufacturer, entity.Manufacturer)
	record.Set(fieldModel, entity.Model)
	record.Set(fieldSerial, entity.Serial)
	setDate(record, fieldPrescribedOn, entity.PrescribedOn)
	setDate(record, fieldServicedOn, entity.ServicedOn)
	setDate(record, fieldServiceDueOn, entity.ServiceDueOn)
	record.Set(fieldInstructions, entity.Instructions)
	record.Set(fieldStatus, string(entity.Status))
	record.Set(fieldSupplier, entity.SupplierID)
	record.Set(fieldPractitioner, entity.PractitionerID)
	record.Set(fieldNotes, entity.Notes)
	record.Set(fieldTags, entity.Tags)

	return nil
}

// recordDate mirrors internal/store's own unexported helper of the same name
// (mapping.go): an unset date column's zero types.DateTime must be recognised
// before it is converted, or "not recorded" becomes 1 January, year 1.
func recordDate(record *core.Record, field string) (domain.Date, error) {
	stored := record.GetDateTime(field)
	if stored.IsZero() {
		return domain.Date{}, nil
	}

	var date domain.Date
	if err := date.Scan(stored.Time().UTC()); err != nil {
		return domain.Date{}, fmt.Errorf("equipment: %s is not a calendar date: %w", field, err)
	}

	return date, nil
}

func setDate(record *core.Record, field string, date domain.Date) {
	record.Set(field, date.UTC())
}

func recordInstant(record *core.Record, field string) time.Time {
	return record.GetDateTime(field).Time().UTC().Truncate(time.Millisecond)
}
