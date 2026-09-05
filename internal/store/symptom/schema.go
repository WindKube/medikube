package symptom

import (
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

// The columns a repository names when it builds a store.Query.
const (
	ColumnPatient   = fieldPatient
	ColumnName      = fieldName
	ColumnSeverity  = fieldSeverity
	ColumnStatus    = fieldStatus
	ColumnIsChronic = fieldIsChronic
)

// Schema is the symptom list's query surface.
func Schema() store.Schema {
	return store.NewSchema(kind.Symptom.Collection(),
		store.Column{Name: fieldPatient},
		store.Column{
			Name:       fieldName,
			Expr:       "LOWER(" + quoteColumn(fieldName) + ")",
			Searchable: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(fieldName))
			},
		},
		store.Column{Name: fieldSeverity},
		store.Column{Name: fieldStatus},
		store.Column{Name: fieldIsChronic},
		store.Column{Name: fieldOccurredAt, AbsentLast: true},
		// FilterOnly: `?tags=` narrows, but a MaxSelect:0 relation's JSON
		// column is never an ordering (research D-05's cursor-disclosure
		// rule).
		store.Column{Name: fieldTags, FilterOnly: true},
		store.Column{Name: fieldCreated},
		store.Column{Name: fieldUpdated},
	)
}

func quoteColumn(name string) string { return "[[" + name + "]]" }

// asciiLower is SQLite's LOWER(), not Go's — see internal/store/filter.go's
// own copy for why the two must agree byte for byte.
func asciiLower(value string) string {
	out := make([]byte, len(value))

	for i := range len(value) {
		b := value[i]
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}

		out[i] = b
	}

	return string(out)
}
