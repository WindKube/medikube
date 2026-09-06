package familymember

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/domain/person"
	"medikube/internal/store"
)

const (
	fieldPatient      = "patient"
	fieldName         = "name"
	fieldRelationship = "relationship"
	fieldSex          = "sex"
	fieldBirthYear    = "birth_year"
	fieldDeathYear    = "death_year"
	fieldIsDeceased   = "is_deceased"
	fieldConditions   = "conditions"
	fieldCreated      = "created"
	fieldUpdated      = "updated"
)

// The published column names, exported for the repository.
const (
	ColumnID           = "id"
	ColumnPatient      = fieldPatient
	ColumnName         = fieldName
	ColumnRelationship = fieldRelationship
)

// ErrUnexpectedCollection is a record handed to the wrong mapper.
var ErrUnexpectedCollection = errors.New("familymember: the record is not from the family_members collection")

// Schema is this kind's query surface.
func Schema() store.Schema {
	return store.NewSchema(kind.FamilyMember.Collection(),
		store.Column{Name: fieldPatient},
		store.Column{
			Name:       fieldName,
			Expr:       "LOWER(" + quote(fieldName) + ")",
			Searchable: true,
			Value:      func(record *core.Record) string { return asciiLower(record.GetString(fieldName)) },
		},
		store.Column{Name: fieldRelationship},
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
	if record.Collection().Name != kind.FamilyMember.Collection() {
		return ErrUnexpectedCollection
	}

	return nil
}

// wireFamilyCondition is data-model §6.1's on-the-wire shape for the JSON
// column: the mapper and the API's DTO layer are allowed to agree on a shape
// without being the same type (research D-11, applied to storage).
type wireFamilyCondition struct {
	Name         string `json:"name"`
	ICD10Code    string `json:"icd10_code,omitempty"`
	DiagnosedAge *int   `json:"diagnosed_age,omitempty"`
	Severity     string `json:"severity,omitempty"`
	Status       string `json:"status,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

// FromRecord reads a stored row into the entity. It does not validate: a row
// already stored is a fact whatever the current rules say about it.
func FromRecord(record *core.Record) (clinical.FamilyMember, error) {
	if err := expectCollection(record); err != nil {
		return clinical.FamilyMember{}, err
	}

	conditions, err := readConditions(record)
	if err != nil {
		return clinical.FamilyMember{}, err
	}

	return clinical.FamilyMember{
		ID:           record.Id,
		PatientID:    record.GetString(fieldPatient),
		Name:         record.GetString(fieldName),
		Relationship: clinical.FamilyRelationship(record.GetString(fieldRelationship)),
		Sex:          person.Sex(record.GetString(fieldSex)),
		BirthYear:    recordIntPtr(record, fieldBirthYear),
		DeathYear:    recordIntPtr(record, fieldDeathYear),
		IsDeceased:   record.GetBool(fieldIsDeceased),
		Conditions:   conditions,
		CreatedAt:    recordInstant(record, fieldCreated),
		UpdatedAt:    recordInstant(record, fieldUpdated),
		Version:      store.Version(record),
	}, nil
}

// ToRecord writes the entity's columns onto the record. It never writes id,
// created, updated or version — PocketBase owns all four.
func ToRecord(record *core.Record, entity clinical.FamilyMember) error {
	if err := expectCollection(record); err != nil {
		return err
	}

	record.Set(fieldPatient, entity.PatientID)
	record.Set(fieldName, entity.Name)
	record.Set(fieldRelationship, string(entity.Relationship))
	record.Set(fieldSex, string(entity.Sex))
	setIntPtr(record, fieldBirthYear, entity.BirthYear)
	setIntPtr(record, fieldDeathYear, entity.DeathYear)
	record.Set(fieldIsDeceased, entity.IsDeceased)

	if err := writeConditions(record, entity.Conditions); err != nil {
		return err
	}

	return nil
}

func readConditions(record *core.Record) ([]clinical.FamilyCondition, error) {
	raw := record.GetString(fieldConditions)
	if isEmptyJSON(raw) {
		return nil, nil
	}

	var wire []wireFamilyCondition
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		return nil, fmt.Errorf("familymember: %s holds a value that is not the conditions shape: %w", fieldConditions, err)
	}

	conditions := make([]clinical.FamilyCondition, 0, len(wire))
	for _, item := range wire {
		conditions = append(conditions, clinical.FamilyCondition{
			Name:         item.Name,
			ICD10Code:    item.ICD10Code,
			DiagnosedAge: item.DiagnosedAge,
			Severity:     clinical.Severity(item.Severity),
			Status:       clinical.ConditionStatus(item.Status),
			Notes:        item.Notes,
		})
	}

	return conditions, nil
}

// writeConditions always writes a JSON array, `[]` when there are none: the
// contract says conditions never marshals as null.
func writeConditions(record *core.Record, conditions []clinical.FamilyCondition) error {
	wire := make([]wireFamilyCondition, 0, len(conditions))
	for _, condition := range conditions {
		wire = append(wire, wireFamilyCondition{
			Name:         condition.Name,
			ICD10Code:    condition.ICD10Code,
			DiagnosedAge: condition.DiagnosedAge,
			Severity:     string(condition.Severity),
			Status:       string(condition.Status),
			Notes:        condition.Notes,
		})
	}

	encoded, err := json.Marshal(wire)
	if err != nil {
		return fmt.Errorf("familymember: encoding conditions: %w", err)
	}

	record.Set(fieldConditions, string(encoded))

	return nil
}

// isEmptyJSON matches every rendering PocketBase's JSONField gives an
// absent value.
func isEmptyJSON(raw string) bool {
	switch raw {
	case "", "null", `""`, "[]", "{}":
		return true
	default:
		return false
	}
}

// recordIntPtr and setIntPtr treat 0 as "not recorded" — the same convention
// internal/store/equipment/mapper.go documents for its own optional numeric
// columns. A birth or death year of exactly zero is not one FR-054's range
// (1850..2200) ever admits, so the convention loses nothing here.
func recordIntPtr(record *core.Record, field string) *int {
	n := record.GetInt(field)
	if n == 0 {
		return nil
	}

	return &n
}

func setIntPtr(record *core.Record, field string, value *int) {
	if value == nil {
		record.Set(field, 0)

		return
	}

	record.Set(field, *value)
}

func recordInstant(record *core.Record, field string) time.Time {
	return record.GetDateTime(field).Time().UTC().Truncate(time.Millisecond)
}
