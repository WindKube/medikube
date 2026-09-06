package immunization

import (
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

// The immunizations collection's columns. They mirror
// internal/store/migrations/1756400100_immunizations.go.
const (
	fieldPatient      = "patient"
	fieldPractitioner = "practitioner"
	fieldFacility     = "facility"
	fieldVaccineName  = "vaccine_name"
	fieldTradeName    = "trade_name"
	fieldAdministered = "administered_on"
	fieldDoseNumber   = "dose_number"
	fieldLotNumber    = "lot_number"
	fieldManufacturer = "manufacturer"
	fieldSite         = "site"
	fieldRoute        = "route"
	fieldExpiresOn    = "expires_on"
	fieldTags         = "tags"
	fieldCreated      = "created"
	fieldUpdated      = "updated"
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

// immunizationSchema is the immunization list's query surface: vaccine name
// and trade name searchable (FR-069/records-clinical §1), administered_on and
// vaccine_name orderable.
func immunizationSchema() store.Schema {
	return store.NewSchema(kind.Immunization.Collection(),
		store.Column{Name: fieldPatient},
		store.Column{
			Name:       fieldVaccineName,
			Expr:       "LOWER([[" + fieldVaccineName + "]])",
			Searchable: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(fieldVaccineName))
			},
		},
		store.Column{
			Name:       fieldTradeName,
			Expr:       "LOWER([[" + fieldTradeName + "]])",
			Searchable: true,
			FilterOnly: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(fieldTradeName))
			},
		},
		store.Column{Name: fieldAdministered, AbsentLast: true},
		// FilterOnly: `?tags=` narrows, but a multi-select relation's JSON
		// column is never an ordering (research D-05's cursor-disclosure
		// rule, the same reason alternative_name is FilterOnly above).
		store.Column{Name: fieldTags, FilterOnly: true},
		store.Column{Name: fieldCreated},
		store.Column{Name: fieldUpdated},
	)
}
