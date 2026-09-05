package seed

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	"medikube/internal/domain"
	"medikube/internal/domain/directory"
	"medikube/internal/domain/person"
)

// The phase 002 identifiers, data-model §9's cast. Account C carries none: an
// account with no patients is what proves FR-005's automatic self-record
// provisioning is the application's doing and not the seed's.
const (
	AccountAPatientSelfID   = "mkpatamara00001"
	AccountAPatientChildID  = "mkpatamara00002"
	AccountAPatientParentID = "mkpatamara00003"
	AccountBPatientSelfID   = "mkpatboris00001"

	AccountAPractitionerID = "mkprcamara00001"
	AccountBPractitionerID = "mkprcboris00001"

	AccountAFacilityPracticeID = "mkfacamara00001"
	AccountAFacilityPharmacyID = "mkfacamara00002"
)

// The columns phase 002's three new collections and users.active_patient add.
const (
	columnKind                = "kind"
	columnFirstName           = "first_name"
	columnLastName            = "last_name"
	columnBirthDate           = "birth_date"
	columnSex                 = "sex"
	columnBloodType           = "blood_type"
	columnRelationshipToOwner = "relationship_to_owner"
	columnPrimaryPractitioner = "primary_practitioner"
	columnIsSelfRecord        = "is_self_record"
	columnPhoto               = "photo"
	columnSpecialty           = "specialty"
	columnFacility            = "facility"
	columnPhone               = "phone"
	columnActivePatient       = "active_patient"
)

const (
	facilitiesCollection    = "facilities"
	practitionersCollection = "practitioners"
	patientsCollection      = "patients"
)

// selfPhotoPNG is a one-pixel, transparent PNG — the smallest byte sequence
// PocketBase's own content sniffing accepts as image/png. What data-model §9
// asks for is that Account A's self-record *carries* a photo; nothing asks
// what the photo shows, so it is the smallest file that satisfies the field's
// MimeTypes validator rather than an image of anything.
const selfPhotoPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

// Facilities is data-model §9's two rows: one practice, one pharmacy, both
// Account A's. Account B's directory stays empty — SC-013 requires the smoke
// gate to pass on empty screens, and an always-populated seed would never
// exercise that (research D-35).
func Facilities() []directory.Facility {
	return []directory.Facility{
		{
			ID: AccountAFacilityPracticeID, OwnerID: AccountAID,
			Kind: directory.FacilityKindPractice, Name: "Riverside Family Practice",
			City: "Lagos", Country: "Nigeria", Phone: "+234 1 555 0101",
		},
		{
			ID: AccountAFacilityPharmacyID, OwnerID: AccountAID,
			Kind: directory.FacilityKindPharmacy, Name: "Riverside Pharmacy",
			Brand: "Boots", City: "Lagos", Country: "Nigeria",
		},
	}
}

// Practitioners is data-model §9's two rows: Account A's, with a specialty and
// a facility; Account B's, with neither, addressed by every stranger test that
// treats Account A's ids as Account B's own.
func Practitioners() []directory.Practitioner {
	return []directory.Practitioner{
		{
			ID: AccountAPractitionerID, OwnerID: AccountAID,
			Name: "Dr. Ngozi Adeyemi", Specialty: directory.SpecialtyFamilyMedicine,
			FacilityID: AccountAFacilityPracticeID,
		},
		{
			ID: AccountBPractitionerID, OwnerID: AccountBID,
			Name: "Dr. Petra Novakova",
		},
	}
}

// Patients is data-model §9's four rows: Account A's self-record (with a
// photo), her child and her parent; Account B's self-record only, the
// isolation counterparty every stranger test addresses Account A's ids as.
//
// Medications on the self-record and the parent are Medications' own rows
// (mkmedamara*), still attributed by `owner` — phase 002 does not repoint
// medications; research D-13's migration is a later phase's.
func Patients() []person.Patient {
	return []person.Patient{
		{
			ID: AccountAPatientSelfID, OwnerID: AccountAID,
			FirstName: "Amara", LastName: "Okonkwo",
			BirthDate:             mustDate(1988, time.April, 12),
			Sex:                   person.SexFemale,
			BloodType:             person.BloodTypeOPos,
			RelationshipToOwner:   person.RelationshipSelf,
			PrimaryPractitionerID: AccountAPractitionerID,
			IsSelfRecord:          true,
		},
		{
			ID: AccountAPatientChildID, OwnerID: AccountAID,
			FirstName: "Chiamaka", LastName: "Okonkwo",
			BirthDate:           mustDate(2015, time.September, 3),
			Sex:                 person.SexFemale,
			RelationshipToOwner: person.RelationshipChild,
		},
		{
			ID: AccountAPatientParentID, OwnerID: AccountAID,
			FirstName: "Emeka", LastName: "Okonkwo",
			BirthDate:           mustDate(1955, time.January, 20),
			Sex:                 person.SexMale,
			BloodType:           person.BloodTypeAPos,
			RelationshipToOwner: person.RelationshipParent,
		},
		{
			ID: AccountBPatientSelfID, OwnerID: AccountBID,
			FirstName: "Boris", LastName: "Novak",
			BirthDate:           mustDate(1990, time.July, 22),
			Sex:                 person.SexMale,
			RelationshipToOwner: person.RelationshipSelf,
			IsSelfRecord:        true,
		},
	}
}

func mustDate(year int, month time.Month, day int) domain.Date {
	value, err := domain.NewDate(year, month, day)
	if err != nil {
		panic(fmt.Sprintf("seed: %04d-%02d-%02d is not a calendar date", year, month, day))
	}

	return value
}

func applyFacilities(app core.App) error {
	collection, err := app.FindCollectionByNameOrId(facilitiesCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", facilitiesCollection, err)
	}

	if err := requireColumns(collection, columnOwner, columnKind, columnName); err != nil {
		return err
	}

	for _, facility := range Facilities() {
		if err := facility.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", facility.ID, err)
		}

		record, err := findOrNew(app, collection, facility.ID)
		if err != nil {
			return err
		}

		record.Set(columnOwner, facility.OwnerID)
		record.Set(columnKind, string(facility.Kind))
		record.Set(columnName, facility.Name)
		record.Set("brand", facility.Brand)
		record.Set("city", facility.City)
		record.Set("country", facility.Country)
		record.Set(columnPhone, facility.Phone)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", facility.ID, err)
		}
	}

	return nil
}

func applyPractitioners(app core.App) error {
	collection, err := app.FindCollectionByNameOrId(practitionersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", practitionersCollection, err)
	}

	if err := requireColumns(collection, columnOwner, columnName, columnSpecialty, columnFacility); err != nil {
		return err
	}

	for _, practitioner := range Practitioners() {
		if err := practitioner.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", practitioner.ID, err)
		}

		record, err := findOrNew(app, collection, practitioner.ID)
		if err != nil {
			return err
		}

		record.Set(columnOwner, practitioner.OwnerID)
		record.Set(columnName, practitioner.Name)
		record.Set(columnSpecialty, string(practitioner.Specialty))
		record.Set(columnFacility, practitioner.FacilityID)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", practitioner.ID, err)
		}
	}

	return nil
}

func applyPatients(app core.App) error {
	collection, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	if requireErr := requireColumns(collection,
		columnOwner, columnFirstName, columnLastName, columnBirthDate, columnSex, columnBloodType,
		columnRelationshipToOwner, columnPrimaryPractitioner, columnIsSelfRecord, columnPhoto,
	); requireErr != nil {
		return requireErr
	}

	photo, err := selfPhoto()
	if err != nil {
		return fmt.Errorf("building the seeded self-record's photo: %w", err)
	}

	for _, patient := range Patients() {
		if err := patient.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", patient.ID, err)
		}

		record, err := findOrNew(app, collection, patient.ID)
		if err != nil {
			return err
		}

		record.Set(columnOwner, patient.OwnerID)
		record.Set(columnFirstName, patient.FirstName)
		record.Set(columnLastName, patient.LastName)
		record.Set(columnBirthDate, patient.BirthDate.UTC())
		record.Set(columnSex, string(patient.Sex))
		record.Set(columnBloodType, string(patient.BloodType))
		record.Set(columnRelationshipToOwner, string(patient.RelationshipToOwner))
		record.Set(columnPrimaryPractitioner, patient.PrimaryPractitionerID)
		record.Set(columnIsSelfRecord, patient.IsSelfRecord)

		// data-model §9: only Account A's self-record carries a photo.
		if patient.ID == AccountAPatientSelfID {
			record.Set(columnPhoto, photo)
		}

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", patient.ID, err)
		}
	}

	return nil
}

// applyActivePatients points each of Account A and B's active_patient at
// their own self-record. FR-013's pointer is a UI convenience nothing
// authorizes against (D-08), but a seed that left it unset would mean no
// account has a person in view at all, which every phase 003+ overview page
// depends on having.
func applyActivePatients(app core.App) error {
	collection, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", usersCollection, err)
	}

	if err := requireColumns(collection, columnActivePatient); err != nil {
		return err
	}

	for accountID, patientID := range map[string]string{
		AccountAID: AccountAPatientSelfID,
		AccountBID: AccountBPatientSelfID,
	} {
		record, err := app.FindRecordById(collection, accountID)
		if err != nil {
			return fmt.Errorf("finding %s: %w", accountID, err)
		}

		record.Set(columnActivePatient, patientID)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("pointing %s at its active patient: %w", accountID, err)
		}
	}

	return nil
}

func selfPhoto() (*filesystem.File, error) {
	raw, err := base64.StdEncoding.DecodeString(selfPhotoPNGBase64)
	if err != nil {
		return nil, fmt.Errorf("decoding the seeded photo: %w", err)
	}

	return filesystem.NewFileFromBytes(raw, "self.png")
}
