package injury

import (
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

// The injuries collection's columns. They mirror
// internal/store/migrations/1756400200_injuries.go.
const (
	fieldPatient       = "patient"
	fieldPractitioner  = "practitioner"
	fieldName          = "name"
	fieldType          = "type"
	fieldBodyPart      = "body_part"
	fieldLaterality    = "laterality"
	fieldOccurredOn    = "occurred_on"
	fieldMechanism     = "mechanism"
	fieldSeverity      = "severity"
	fieldStatus        = "status"
	fieldRecoveryNotes = "recovery_notes"
	fieldMedications   = "medications"
	fieldCreated       = "created"
	fieldUpdated       = "updated"
)

func asciiLower(value string) string {
	folded := []byte(value)
	for i, b := range folded {
		if b >= 'A' && b <= 'Z' {
			folded[i] = b + ('a' - 'A')
		}
	}

	return string(folded)
}

// injurySchema is the injury list's query surface: name searchable,
// occurred_on and name orderable, status/severity/type/laterality narrowable.
func injurySchema() store.Schema {
	return store.NewSchema(kind.Injury.Collection(),
		store.Column{Name: fieldPatient},
		store.Column{
			Name:       fieldName,
			Expr:       "LOWER([[" + fieldName + "]])",
			Searchable: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(fieldName))
			},
		},
		store.Column{Name: fieldType},
		store.Column{Name: fieldLaterality},
		store.Column{Name: fieldSeverity},
		store.Column{Name: fieldStatus},
		store.Column{Name: fieldOccurredOn, AbsentLast: true},
		store.Column{Name: fieldCreated},
		store.Column{Name: fieldUpdated},
	)
}
