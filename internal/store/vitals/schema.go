package vitals

import (
	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

const ColumnPatient = fieldPatient

// Schema is the vitals list's query surface: dates only
// (contracts/records-clinical.md §1).
func Schema() store.Schema {
	return store.NewSchema(kind.Vitals.Collection(),
		store.Column{Name: fieldPatient},
		store.Column{Name: fieldRecordedAt, AbsentLast: true},
		store.Column{Name: fieldCreated},
		store.Column{Name: fieldUpdated},
	)
}
