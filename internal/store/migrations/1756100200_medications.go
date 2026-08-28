package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// data-model §2's column bounds, the same numbers clinical.Medication.Validate
// enforces. assertions_test.go walks both and refuses a column the entity would
// not have refused.
const (
	medicationNameMin            = 1
	medicationNameMax            = 200
	medicationAlternativeNameMax = 200
	medicationDosageMax          = 200
	medicationFrequencyMax       = 100
	medicationIndicationMax      = 300
	medicationSideEffectsMax     = 1000
	medicationNotesMax           = 5000
)

// The thirteen columns of data-model §2, plus the two autodate columns §1.0
// puts on every collection. PocketBase's NewBaseCollection supplies `id` and
// nothing else — `created` and `updated` are not automatic, and
// idx_medications_owner_upd indexes a column that would not exist without them.
const (
	medicationFieldOwner           = "owner"
	medicationFieldName            = "name"
	medicationFieldAlternativeName = "alternative_name"
	medicationFieldType            = "type"
	medicationFieldDosage          = "dosage"
	medicationFieldFrequency       = "frequency"
	medicationFieldRoute           = "route"
	medicationFieldIndication      = "indication"
	medicationFieldStartedOn       = "started_on"
	medicationFieldEndedOn         = "ended_on"
	medicationFieldStatus          = "status"
	medicationFieldSideEffects     = "side_effects"
	medicationFieldNotes           = "notes"
)

const (
	fieldCreated = "created"
	fieldUpdated = "updated"
)

func init() {
	register(medicationsUp, medicationsDown)
}

func medicationsUp(app core.App) error {
	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", usersCollection, err)
	}

	collection := core.NewBaseCollection(kind.Medication.Collection())
	lockRules(collection)

	// The authorization anchor, and the one cascade this phase has. Required
	// and CascadeDelete are both load-bearing and both one character from
	// silently wrong: without CascadeDelete a closed account leaves its
	// medications behind (FR-014, SC-012); with Required but without
	// CascadeDelete, deleting the account fails outright when PocketBase tries
	// to empty the reference (core/record_model.go:1619).
	collection.Fields.Add(&core.RelationField{
		Name:          medicationFieldOwner,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  users.Id,
	})
	collection.Fields.Add(&core.TextField{
		Name:     medicationFieldName,
		Required: true,
		Min:      medicationNameMin,
		Max:      medicationNameMax,
	})
	collection.Fields.Add(&core.TextField{
		Name: medicationFieldAlternativeName,
		Max:  medicationAlternativeNameMax,
	})
	collection.Fields.Add(&core.SelectField{
		Name:      medicationFieldType,
		MaxSelect: 1,
		Values:    enumValues(clinical.MedicationTypes()),
	})
	collection.Fields.Add(&core.TextField{
		Name: medicationFieldDosage,
		Max:  medicationDosageMax,
	})
	collection.Fields.Add(&core.TextField{
		Name: medicationFieldFrequency,
		Max:  medicationFrequencyMax,
	})
	collection.Fields.Add(&core.SelectField{
		Name:      medicationFieldRoute,
		MaxSelect: 1,
		Values:    enumValues(clinical.MedicationRoutes()),
	})
	collection.Fields.Add(&core.TextField{
		Name: medicationFieldIndication,
		Max:  medicationIndicationMax,
	})
	collection.Fields.Add(&core.DateField{Name: medicationFieldStartedOn})
	collection.Fields.Add(&core.DateField{Name: medicationFieldEndedOn})

	// data-model §2 gives status a default of `active`. PocketBase v0.40.1 has
	// no per-field default, so the default lives in the domain
	// (clinical.TherapyStatusActive, applied on create) and the required column
	// is the safety net that catches a create path which forgot it.
	collection.Fields.Add(&core.SelectField{
		Name:      medicationFieldStatus,
		Required:  true,
		MaxSelect: 1,
		Values:    enumValues(clinical.TherapyStatuses()),
	})
	collection.Fields.Add(&core.TextField{
		Name: medicationFieldSideEffects,
		Max:  medicationSideEffectsMax,
	})
	collection.Fields.Add(&core.TextField{
		Name: medicationFieldNotes,
		Max:  medicationNotesMax,
	})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	// Each of the three orderings FR-022 publishes gets one composite index, and
	// each ends in id because the keyset cursor's tiebreaker is always the id
	// (research D-25). Without it, two medications started on the same day page
	// unstably, which is what FR-023 forbids. The index names are composed from
	// the kind's collection rather than written out, because the spelling lives
	// in exactly one table (research D-05).
	name := kind.Medication.Collection()
	collection.AddIndex("idx_"+name+"_owner", false, medicationFieldOwner, "")
	collection.AddIndex("idx_"+name+"_owner_start", false,
		medicationFieldOwner+", "+medicationFieldStartedOn+" DESC, id DESC", "")
	collection.AddIndex("idx_"+name+"_owner_name", false,
		medicationFieldOwner+", LOWER("+medicationFieldName+"), id DESC", "")
	collection.AddIndex("idx_"+name+"_owner_upd", false,
		medicationFieldOwner+", "+fieldUpdated+" DESC, id DESC", "")
	collection.AddIndex("idx_"+name+"_owner_status", false,
		medicationFieldOwner+", "+medicationFieldStatus, "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func medicationsDown(app core.App) error {
	return deleteCollection(app, kind.Medication.Collection())
}
