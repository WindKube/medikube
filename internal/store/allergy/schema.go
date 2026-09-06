package allergy

import (
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

// The allergies collection's columns, mirroring
// internal/store/migrations/<ts>_allergies.go.
var collectionName = kind.Allergy.Collection()

const (
	fieldPatient  = "patient"
	fieldAllergen = "allergen"
	fieldReaction = "reaction"
	fieldSeverity = "severity"
	fieldStatus   = "status"
	fieldOnsetOn  = "onset_on"
	fieldNotes    = "notes"
	fieldTags     = "tags"
	fieldCreated  = "created"
	fieldUpdated  = "updated"
)

// fieldMedications is migration 17's addition (data-model §4.1, FR-017). Its
// value is the medications collection's own name — the relation field is
// named after its target by data-model design — so it is derived rather than
// spelled a second time (research D-05).
var fieldMedications = kind.Medication.Collection()

// asciiLower matches internal/store's own fold: SQLite's LIKE and LOWER()
// fold ASCII case and nothing else.
func asciiLower(value string) string {
	folded := []byte(value)
	for i, b := range folded {
		if b >= 'A' && b <= 'Z' {
			folded[i] = b + ('a' - 'A')
		}
	}

	return string(folded)
}

// allergySchema is the allergy list's query surface: patient scopes every
// query, allergen is searchable and orderable, severity and status are
// narrowable, onset_on is the primary date (sorts undated last).
func allergySchema() store.Schema {
	return store.NewSchema(kind.Allergy.Collection(),
		store.Column{Name: fieldPatient},
		store.Column{
			Name:       fieldAllergen,
			Expr:       "LOWER([[" + fieldAllergen + "]])",
			Searchable: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(fieldAllergen))
			},
		},
		store.Column{Name: fieldSeverity},
		store.Column{Name: fieldStatus},
		store.Column{Name: fieldOnsetOn, AbsentLast: true},
		// FilterOnly: `?tags=` narrows, but a MaxSelect:0 relation's JSON
		// column is never an ordering (research D-05's cursor-disclosure
		// rule).
		store.Column{Name: fieldTags, FilterOnly: true},
		store.Column{Name: fieldCreated},
		store.Column{Name: fieldUpdated},
	)
}
