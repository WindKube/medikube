package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

const (
	contactFieldPatient      = "patient"
	contactFieldName         = "name"
	contactFieldRelationship = "relationship"
	contactFieldPhone        = "phone"
	contactFieldPhoneAlt     = "phone_alt"
	contactFieldEmail        = "email"
	contactFieldAddress      = "address"
	contactFieldIsPrimary    = "is_primary"
	contactFieldIsActive     = "is_active"
	contactFieldNotes        = "notes"
	contactFieldTags         = "tags"
)

const (
	contactNameMin    = 2
	contactNameMax    = 100
	contactPhoneMax   = 40
	contactAddressMax = 500
	contactNotesMax   = 5000
)

const contactPrimaryIndex = "uniq_contacts_primary"

func init() {
	register(emergencyContactsUp, emergencyContactsDown)
}

// emergencyContactsUp is data-model §4.12: no primary date, and a partial
// unique index enforcing at most one primary contact per patient — the
// storage-level backstop behind the service's own transactional displacement
// (research D-16).
func emergencyContactsUp(app core.App) error {
	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	tags, err := app.FindCollectionByNameOrId(TagsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", TagsCollection, err)
	}

	collection := core.NewBaseCollection(kind.EmergencyContact.Collection())
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name:          contactFieldPatient,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})
	collection.Fields.Add(&core.TextField{
		Name:     contactFieldName,
		Required: true,
		Min:      contactNameMin,
		Max:      contactNameMax,
	})
	collection.Fields.Add(&core.SelectField{
		Name: contactFieldRelationship, Required: true, MaxSelect: 1,
		Values: enumValues(clinical.ContactRelationships()),
	})
	collection.Fields.Add(&core.TextField{
		Name:     contactFieldPhone,
		Required: true,
		Max:      contactPhoneMax,
	})
	collection.Fields.Add(&core.TextField{Name: contactFieldPhoneAlt, Max: contactPhoneMax})
	collection.Fields.Add(&core.EmailField{Name: contactFieldEmail})
	collection.Fields.Add(&core.TextField{Name: contactFieldAddress, Max: contactAddressMax})
	collection.Fields.Add(&core.BoolField{Name: contactFieldIsPrimary})
	collection.Fields.Add(&core.BoolField{Name: contactFieldIsActive})
	collection.Fields.Add(&core.TextField{Name: contactFieldNotes, Max: contactNotesMax})
	collection.Fields.Add(&core.RelationField{
		Name:         contactFieldTags,
		MaxSelect:    unlimitedTags,
		CollectionId: tags.Id,
	})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	name := kind.EmergencyContact.Collection()
	collection.AddIndex("idx_"+name+"_patient_sort", false,
		contactFieldPatient+", "+contactFieldIsActive+" DESC, "+contactFieldIsPrimary+" DESC, LOWER("+contactFieldName+"), id DESC", "")
	collection.AddIndex(contactPrimaryIndex, true,
		contactFieldPatient, contactFieldIsPrimary+" = 1")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func emergencyContactsDown(app core.App) error {
	return deleteCollection(app, kind.EmergencyContact.Collection())
}
