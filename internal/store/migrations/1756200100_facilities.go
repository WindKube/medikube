package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/directory"
)

// data-model §1's column bounds, the same numbers directory.Facility.Validate
// enforces.
const (
	facilityNameMax       = 200
	facilityBrandMax      = 120
	facilityStreetMax     = 200
	facilityCityMax       = 120
	facilityRegionMax     = 120
	facilityPostalCodeMax = 20
	facilityCountryMax    = 80
	facilityPhoneMax      = 40
	facilityFaxMax        = 40
	facilityHoursMax      = 300
	facilityServicesMax   = 500
	facilityNotesMax      = 5000
)

// The eighteen columns of data-model §1, in the order that document lists
// them.
const (
	facilityFieldOwner        = "owner"
	facilityFieldKind         = "kind"
	facilityFieldName         = "name"
	facilityFieldBrand        = "brand"
	facilityFieldStreet       = "street"
	facilityFieldCity         = "city"
	facilityFieldRegion       = "region"
	facilityFieldPostalCode   = "postal_code"
	facilityFieldCountry      = "country"
	facilityFieldPhone        = "phone"
	facilityFieldFax          = "fax"
	facilityFieldEmail        = "email"
	facilityFieldWebsite      = "website"
	facilityFieldPortalURL    = "portal_url"
	facilityFieldHours        = "hours"
	facilityFieldOpen24h      = "open_24h"
	facilityFieldDriveThrough = "drive_through"
	facilityFieldServices     = "services"
	facilityFieldNotes        = "notes"
)

const facilitiesCollection = "facilities"

func init() {
	register(facilitiesUp, facilitiesDown)
}

func facilitiesUp(app core.App) error {
	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", usersCollection, err)
	}

	collection := core.NewBaseCollection(facilitiesCollection)
	lockRules(collection)

	// FR-037: the directory is the account's own, and closing the account
	// destroys it.
	collection.Fields.Add(&core.RelationField{
		Name:          facilityFieldOwner,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  users.Id,
	})
	collection.Fields.Add(&core.SelectField{
		Name:      facilityFieldKind,
		Required:  true,
		MaxSelect: 1,
		Values:    enumValues(directory.FacilityKinds()),
	})
	collection.Fields.Add(&core.TextField{
		Name:     facilityFieldName,
		Required: true,
		Min:      1,
		Max:      facilityNameMax,
	})
	collection.Fields.Add(&core.TextField{Name: facilityFieldBrand, Max: facilityBrandMax})
	collection.Fields.Add(&core.TextField{Name: facilityFieldStreet, Max: facilityStreetMax})
	collection.Fields.Add(&core.TextField{Name: facilityFieldCity, Max: facilityCityMax})
	collection.Fields.Add(&core.TextField{Name: facilityFieldRegion, Max: facilityRegionMax})
	collection.Fields.Add(&core.TextField{Name: facilityFieldPostalCode, Max: facilityPostalCodeMax})
	collection.Fields.Add(&core.TextField{Name: facilityFieldCountry, Max: facilityCountryMax})
	collection.Fields.Add(&core.TextField{Name: facilityFieldPhone, Max: facilityPhoneMax})
	collection.Fields.Add(&core.TextField{Name: facilityFieldFax, Max: facilityFaxMax})
	collection.Fields.Add(&core.EmailField{Name: facilityFieldEmail})
	collection.Fields.Add(&core.URLField{Name: facilityFieldWebsite})
	collection.Fields.Add(&core.URLField{Name: facilityFieldPortalURL})
	collection.Fields.Add(&core.TextField{Name: facilityFieldHours, Max: facilityHoursMax})
	collection.Fields.Add(&core.BoolField{Name: facilityFieldOpen24h})
	collection.Fields.Add(&core.BoolField{Name: facilityFieldDriveThrough})
	collection.Fields.Add(&core.TextField{Name: facilityFieldServices, Max: facilityServicesMax})
	collection.Fields.Add(&core.TextField{Name: facilityFieldNotes, Max: facilityNotesMax})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	// Deliberately no unique index on name: FR-035 and US5-3, two branches of
	// one chain are two rows sharing a name.
	collection.AddIndex("idx_facilities_owner", false, facilityFieldOwner, "")
	collection.AddIndex("idx_facilities_owner_kind", false,
		facilityFieldOwner+", "+facilityFieldKind, "")
	collection.AddIndex("idx_facilities_owner_name", false,
		facilityFieldOwner+", "+facilityFieldName, "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", facilitiesCollection, err)
	}

	return nil
}

func facilitiesDown(app core.App) error {
	return deleteCollection(app, facilitiesCollection)
}
