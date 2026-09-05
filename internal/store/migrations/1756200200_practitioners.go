package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/directory"
)

// data-model §2's column bounds, the same numbers directory.Practitioner.Validate
// enforces.
const (
	practitionerNameMax  = 200
	practitionerPhoneMax = 40
	practitionerNotesMax = 5000
)

const (
	practitionerFieldOwner     = "owner"
	practitionerFieldName      = "name"
	practitionerFieldSpecialty = "specialty"
	practitionerFieldFacility  = "facility"
	practitionerFieldPhone     = "phone"
	practitionerFieldEmail     = "email"
	practitionerFieldWebsite   = "website"
	practitionerFieldNotes     = "notes"
)

const practitionersCollection = "practitioners"

func init() {
	register(practitionersUp, practitionersDown)
}

func practitionersUp(app core.App) error {
	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", usersCollection, err)
	}

	facilities, err := app.FindCollectionByNameOrId(facilitiesCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", facilitiesCollection, err)
	}

	collection := core.NewBaseCollection(practitionersCollection)
	lockRules(collection)

	// FR-037: the directory is the account's own, and closing the account
	// destroys it.
	collection.Fields.Add(&core.RelationField{
		Name:          practitionerFieldOwner,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  users.Id,
	})
	collection.Fields.Add(&core.TextField{
		Name:     practitionerFieldName,
		Required: true,
		Min:      1,
		Max:      practitionerNameMax,
	})
	// Stored as the empty string when unset, never NULL (research D-25) — the
	// (owner, LOWER(name), specialty) uniqueness index below depends on that.
	collection.Fields.Add(&core.SelectField{
		Name:      practitionerFieldSpecialty,
		MaxSelect: 1,
		Values:    enumValues(directory.Specialties()),
	})
	// Deleting the facility unsets this rather than the practitioner: a
	// clinician's directory entry outlives the place they used to practise at.
	collection.Fields.Add(&core.RelationField{
		Name:          practitionerFieldFacility,
		Required:      false,
		CascadeDelete: false,
		MaxSelect:     1,
		CollectionId:  facilities.Id,
	})
	collection.Fields.Add(&core.TextField{Name: practitionerFieldPhone, Max: practitionerPhoneMax})
	collection.Fields.Add(&core.EmailField{Name: practitionerFieldEmail})
	collection.Fields.Add(&core.URLField{Name: practitionerFieldWebsite})
	collection.Fields.Add(&core.TextField{Name: practitionerFieldNotes, Max: practitionerNotesMax})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	// FR-038: the same clinician under the same specialty cannot be recorded
	// twice for one account. LOWER(name) so "Dr Amara" and "dr amara" collide.
	collection.AddIndex("idx_practitioners_owner_name_specialty", true,
		practitionerFieldOwner+", LOWER("+practitionerFieldName+"), "+practitionerFieldSpecialty, "")
	collection.AddIndex("idx_practitioners_owner", false, practitionerFieldOwner, "")
	collection.AddIndex("idx_practitioners_owner_spec", false,
		practitionerFieldOwner+", "+practitionerFieldSpecialty, "")
	collection.AddIndex("idx_practitioners_facility", false, practitionerFieldFacility, "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", practitionersCollection, err)
	}

	return nil
}

func practitionersDown(app core.App) error {
	return deleteCollection(app, practitionersCollection)
}
