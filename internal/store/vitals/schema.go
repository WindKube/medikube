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
		// FilterOnly: `?tags=` narrows, but a multi-select relation's JSON
		// column is never an ordering (research D-05's cursor-disclosure
		// rule).
		store.Column{Name: fieldTags, FilterOnly: true},
		store.Column{Name: fieldCreated},
		store.Column{Name: fieldUpdated},
	)
}
