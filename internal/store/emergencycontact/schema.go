package emergencycontact

import (
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

var collectionName = kind.EmergencyContact.Collection()

const (
	fieldPatient      = "patient"
	fieldName         = "name"
	fieldRelationship = "relationship"
	fieldPhone        = "phone"
	fieldPhoneAlt     = "phone_alt"
	fieldEmail        = "email"
	fieldAddress      = "address"
	fieldIsPrimary    = "is_primary"
	fieldIsActive     = "is_active"
	fieldNotes        = "notes"
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

// contactSchema is FR-051's fixed ordering: is_active DESC, is_primary DESC,
// LOWER(name) ASC — id DESC is the repository's own tiebreaker.
func contactSchema() store.Schema {
	return store.NewSchema(kind.EmergencyContact.Collection(),
		store.Column{Name: fieldPatient},
		store.Column{
			Name: fieldIsActive,
			Value: func(record *core.Record) string {
				return boolString(record.GetBool(fieldIsActive))
			},
		},
		store.Column{
			Name: fieldIsPrimary,
			Value: func(record *core.Record) string {
				return boolString(record.GetBool(fieldIsPrimary))
			},
		},
		store.Column{
			Name:       fieldName,
			Expr:       "LOWER([[" + fieldName + "]])",
			Searchable: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(fieldName))
			},
		},
		store.Column{Name: fieldCreated},
		store.Column{Name: fieldUpdated},
	)
}

func boolString(value bool) string {
	if value {
		return "1"
	}

	return "0"
}
