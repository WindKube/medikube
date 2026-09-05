package facility

import (
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

// The facilities collection's columns. They mirror
// internal/store/migrations/1756200100_facilities.go, and the two lists
// staying in step is what this package's mapping and query surface depend on.
const (
	collectionName = "facilities"

	fieldOwner        = "owner"
	fieldKind         = "kind"
	fieldName         = "name"
	fieldBrand        = "brand"
	fieldStreet       = "street"
	fieldCity         = "city"
	fieldRegion       = "region"
	fieldPostalCode   = "postal_code"
	fieldCountry      = "country"
	fieldPhone        = "phone"
	fieldFax          = "fax"
	fieldEmail        = "email"
	fieldWebsite      = "website"
	fieldPortalURL    = "portal_url"
	fieldHours        = "hours"
	fieldOpen24h      = "open_24h"
	fieldDriveThrough = "drive_through"
	fieldServices     = "services"
	fieldNotes        = "notes"
	fieldCreated      = "created"
	fieldUpdated      = "updated"
)

// The practitioners and medications columns this package reads to answer
// Usage, and unsets on delete. They belong to collections this package does
// not own, but the names are storage knowledge and this is the one place
// facility touches them.
const (
	practitionersCollection   = "practitioners"
	practitionerFieldFacility = "facility"

	medicationFieldPharmacy = "pharmacy"
)

// medicationsCollection spells kind.Medication's own collection name rather
// than the literal, per research D-05 (internal/architecture's
// TestNoFileOutsideTheKindTableSpellsAKindSegmentOrCollection): medication is
// a dispatched record kind, and every spelling of its collection outside the
// kind table itself has to go through it.
var medicationsCollection = kind.Medication.Collection()

// asciiLower matches internal/store's own fold: SQLite's LIKE and LOWER() fold
// ASCII case and nothing else, so the Go twin of a LOWER(column) expression
// has to fold the same alphabet or a search here and the index it runs
// against silently disagree.
func asciiLower(value string) string {
	folded := []byte(value)
	for i, b := range folded {
		if b >= 'A' && b <= 'Z' {
			folded[i] = b + ('a' - 'A')
		}
	}

	return string(folded)
}

// facilitySchema is the facility list's query surface: kind and name are
// orderable (contracts/facilities.md's one published ordering, "kind, name,
// id"), name and brand are searchable (FR-036's `?q=`), and owner scopes
// every query.
//
// It is built with internal/store's own Schema rather than a package-private
// reimplementation: that package already owns the SQL-quoting, the keyset
// cursor and the boundary logic every other resource's query surface is held
// to, and a second copy of that machinery here is exactly the drift plan.md's
// "one seam" is meant to prevent.
func facilitySchema() store.Schema {
	return store.NewSchema(collectionName,
		store.Column{Name: fieldOwner},
		store.Column{
			Name: fieldKind,
			Value: func(record *core.Record) string {
				return record.GetString(fieldKind)
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
		store.Column{
			Name:       fieldBrand,
			Expr:       "LOWER([[" + fieldBrand + "]])",
			Searchable: true,
			FilterOnly: true,
			Value: func(record *core.Record) string {
				return asciiLower(record.GetString(fieldBrand))
			},
		},
		store.Column{Name: fieldCreated},
		store.Column{Name: fieldUpdated},
	)
}
