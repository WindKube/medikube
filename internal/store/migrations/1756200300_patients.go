package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/person"
)

// PhotoMaxBytes, PhotoMimeTypes and PhotoThumbs are the schema's baseline —
// SHARED-DESIGN §1.2's 15 MiB, research D-18 — written once here. The
// boot-time settings writer applies the operator's MEDIKUBE_FILES_PHOTO_*
// configuration on every start, the same split usersAuthTokenDuration uses for
// the session TTL.
const (
	PhotoMaxBytes int64 = 15 << 20 // 15 MiB, PocketBase's own default is 5.
)

var (
	photoMimeTypes = []string{"image/jpeg", "image/png", "image/webp"}
	photoThumbs    = []string{"100x100t", "400x400f"}
)

// The thirteen columns of data-model §3, in the order that document lists
// them.
const (
	patientFieldOwner               = "owner"
	patientFieldFirstName           = "first_name"
	patientFieldLastName            = "last_name"
	patientFieldBirthDate           = "birth_date"
	patientFieldSex                 = "sex"
	patientFieldBloodType           = "blood_type"
	patientFieldHeightCM            = "height_cm"
	patientFieldWeightKG            = "weight_kg"
	patientFieldAddress             = "address"
	patientFieldRelationshipToOwner = "relationship_to_owner"
	patientFieldPrimaryPractitioner = "primary_practitioner"
	patientFieldIsSelfRecord        = "is_self_record"
	patientFieldPhoto               = "photo"
)

const (
	patientFirstNameMax = 100
	patientLastNameMax  = 100
	patientAddressMax   = 500
)

const patientsCollection = "patients"

func init() {
	register(patientsUp, patientsDown)
}

func patientsUp(app core.App) error {
	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", usersCollection, err)
	}

	practitioners, err := app.FindCollectionByNameOrId(practitionersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", practitionersCollection, err)
	}

	collection := core.NewBaseCollection(patientsCollection)
	lockRules(collection)

	// FR-002: the authorization anchor and the cascade parent. Absent from
	// every request DTO, so a request cannot nominate its own owner.
	collection.Fields.Add(&core.RelationField{
		Name:          patientFieldOwner,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  users.Id,
	})

	// Collection-optional, DTO-required (research D-09, CT-2): only a
	// server-provisioned self-record may carry an empty value here, and
	// person.Patient.Validate is what refuses an empty one from anywhere else.
	collection.Fields.Add(&core.TextField{Name: patientFieldFirstName, Max: patientFirstNameMax})
	collection.Fields.Add(&core.TextField{Name: patientFieldLastName, Max: patientLastNameMax})
	collection.Fields.Add(&core.DateField{Name: patientFieldBirthDate})

	collection.Fields.Add(&core.SelectField{
		Name:      patientFieldSex,
		MaxSelect: 1,
		Values:    enumValues(person.Sexes()),
	})
	collection.Fields.Add(&core.SelectField{
		Name:      patientFieldBloodType,
		MaxSelect: 1,
		Values:    enumValues(person.BloodTypes()),
	})

	// No Min/Max here: canonical SI, but zero means "not set" (research D-20)
	// and a column-level Min would refuse the very value that means absence.
	// person.Patient.Validate enforces the 30..272 / 0.5..450 ranges once a
	// value is actually set.
	collection.Fields.Add(&core.NumberField{Name: patientFieldHeightCM})
	collection.Fields.Add(&core.NumberField{Name: patientFieldWeightKG})

	collection.Fields.Add(&core.TextField{Name: patientFieldAddress, Max: patientAddressMax})
	collection.Fields.Add(&core.SelectField{
		Name:      patientFieldRelationshipToOwner,
		MaxSelect: 1,
		Values:    enumValues(person.RelationshipsToOwner()),
	})

	// FR-001, FR-040. Deleting the practitioner unsets this rather than the
	// patient.
	collection.Fields.Add(&core.RelationField{
		Name:          patientFieldPrimaryPractitioner,
		Required:      false,
		CascadeDelete: false,
		MaxSelect:     1,
		CollectionId:  practitioners.Id,
	})

	// FR-004. Server-set; the partial unique index below is what enforces "at
	// most one true row per owner" rather than a runtime check alone.
	collection.Fields.Add(&core.BoolField{
		Name:     patientFieldIsSelfRecord,
		Required: false,
	})

	// FR-008, FR-009, FR-044. Protected is not optional: constitution
	// Principle VII refuses to start an instance with an unprotected file
	// field, and no PocketBase file token or link carrying its own credential
	// may reach a patient's photograph.
	collection.Fields.Add(&core.FileField{
		Name:      patientFieldPhoto,
		MaxSelect: 1,
		Protected: true,
		MimeTypes: photoMimeTypes,
		MaxSize:   PhotoMaxBytes,
		Thumbs:    photoThumbs,
	})

	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	// FR-004: at most one self-record per owner. A partial index rather than a
	// runtime check alone, so a race between two requests cannot create two.
	collection.AddIndex("idx_patients_self", true,
		patientFieldOwner, patientFieldIsSelfRecord+" = 1")
	// The list ordering and its cursor tiebreaker (research D-25).
	collection.AddIndex("idx_patients_owner_name", false,
		patientFieldOwner+", "+patientFieldLastName+", "+patientFieldFirstName+", id", "")
	// FR-040's usage count.
	collection.AddIndex("idx_patients_primary_pr", false, patientFieldPrimaryPractitioner, "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", patientsCollection, err)
	}

	return nil
}

func patientsDown(app core.App) error {
	return deleteCollection(app, patientsCollection)
}
