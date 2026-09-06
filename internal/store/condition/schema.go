package condition

import (
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

var collectionName = kind.Condition.Collection()

const (
	fieldPatient      = "patient"
	fieldDiagnosis    = "diagnosis"
	fieldStatus       = "status"
	fieldSeverity     = "severity"
	fieldOnsetOn      = "onset_on"
	fieldResolvedOn   = "resolved_on"
	fieldICD10Code    = "icd10_code"
	fieldSNOMEDCode   = "snomed_code"
	fieldPractitioner = "practitioner"
	fieldNotes        = "notes"
	fieldTags         = "tags"
	fieldCreated      = "created"
	fieldUpdated      = "updated"
)

// fieldMedications is migration 17's addition (data-model §4.2, FR-021),
// derived rather than spelled a second time; see allergy's own schema.go.
var fieldMedications = kind.Medication.Collection()

func asciiLower(value string) string {
	folded := []byte(value)
	for i, b := range folded {
		if b >= 'A' && b <= 'Z' {
			folded[i] = b + ('a' - 'A')
		}
	}

	return string(folded)
}

func conditionSchema() store.Schema {
	return store.NewSchema(kind.Condition.Collection(),
		store.Column{Name: fieldPatient},
		store.Column{
			Name:       fieldDiagnosis,
			Expr:       "LOWER([[" + fieldDiagnosis + "]])",
			Searchable: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(fieldDiagnosis))
			},
		},
		store.Column{Name: fieldStatus},
		store.Column{Name: fieldSeverity},
		store.Column{Name: fieldOnsetOn, AbsentLast: true},
		// FilterOnly: `?tags=` narrows, but a MaxSelect:0 relation's JSON
		// column is never an ordering (research D-05's cursor-disclosure
		// rule).
		store.Column{Name: fieldTags, FilterOnly: true},
		store.Column{Name: fieldCreated},
		store.Column{Name: fieldUpdated},
	)
}
