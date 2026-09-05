package seed

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// The US1 clinical-record fixtures: one critical allergy, one resolved
// condition, and one primary emergency contact, all account A's.
const (
	CriticalAllergyID   = "mkalgamara00001"
	ResolvedConditionID = "mkcndamara00001"
	PrimaryContactID    = "mkcntamara00001"
)

const (
	columnAllergen     = "allergen"
	columnReaction     = "reaction"
	columnOnsetOn      = "onset_on"
	columnDiagnosis    = "diagnosis"
	columnResolvedOn   = "resolved_on"
	columnICD10Code    = "icd10_code"
	columnRelationship = "relationship"
	columnIsActive     = "is_active"
)

// Allergies is account A's one row: severe and active, so it is FR-018's
// critical case (data-model §4.1).
func Allergies() []clinical.Allergy {
	return []clinical.Allergy{
		{
			ID: CriticalAllergyID, PatientID: accountAPatientSelfID,
			Allergen: "Penicillin", Reaction: "anaphylaxis",
			Severity: clinical.SeverityLifeThreatening, Status: clinical.ConditionStatusActive,
			OnsetOn: date(2018, 4, 12),
		},
	}
}

// Conditions is account A's one row: resolved, so onset and resolution both
// render (data-model §4.2).
func Conditions() []clinical.Condition {
	return []clinical.Condition{
		{
			ID: ResolvedConditionID, PatientID: accountAPatientSelfID,
			Diagnosis: "Bacterial pneumonia", Status: clinical.ConditionStatusResolved,
			Severity: clinical.SeverityModerate,
			OnsetOn:  date(2023, 1, 5), ResolvedOn: date(2023, 1, 26),
			ICD10Code: "J15.9",
		},
	}
}

// EmergencyContacts is account A's one row: the primary contact, so a fresh
// account always has exactly one (research D-16).
func EmergencyContacts() []clinical.EmergencyContact {
	return []clinical.EmergencyContact{
		{
			ID: PrimaryContactID, PatientID: accountAPatientSelfID,
			Name: "Ngozi Okonkwo", Relationship: clinical.ContactRelationshipSpouse,
			Phone: "+1-555-0100", IsPrimary: true, IsActive: true,
		},
	}
}

func applyAllergies(app core.App) error {
	name := kind.Allergy.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection,
		columnPatient, columnAllergen, columnReaction, columnSeverity, columnStatus, columnOnsetOn, columnNotes,
	); err != nil {
		return err
	}

	for _, allergy := range Allergies() {
		if err := allergy.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", allergy.ID, err)
		}

		record, err := findOrNew(app, collection, allergy.ID)
		if err != nil {
			return err
		}

		record.Set(columnPatient, allergy.PatientID)
		record.Set(columnAllergen, allergy.Allergen)
		record.Set(columnReaction, allergy.Reaction)
		record.Set(columnSeverity, string(allergy.Severity))
		record.Set(columnStatus, string(allergy.Status))
		record.Set(columnOnsetOn, allergy.OnsetOn.UTC())
		record.Set(columnNotes, allergy.Notes)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", allergy.ID, err)
		}

		if err := IndexRecord(app, kind.Allergy, allergy.ID, allergy.PatientID,
			allergy.Allergen, allergy.Reaction, allergy.OnsetOn); err != nil {
			return err
		}
	}

	return nil
}

func applyConditions(app core.App) error {
	name := kind.Condition.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection,
		columnPatient, columnDiagnosis, columnStatus, columnSeverity,
		columnOnsetOn, columnResolvedOn, columnICD10Code, columnNotes,
	); err != nil {
		return err
	}

	for _, condition := range Conditions() {
		if err := condition.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", condition.ID, err)
		}

		record, err := findOrNew(app, collection, condition.ID)
		if err != nil {
			return err
		}

		record.Set(columnPatient, condition.PatientID)
		record.Set(columnDiagnosis, condition.Diagnosis)
		record.Set(columnStatus, string(condition.Status))
		record.Set(columnSeverity, string(condition.Severity))
		record.Set(columnOnsetOn, condition.OnsetOn.UTC())
		record.Set(columnResolvedOn, condition.ResolvedOn.UTC())
		record.Set(columnICD10Code, condition.ICD10Code)
		record.Set(columnNotes, condition.Notes)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", condition.ID, err)
		}

		if err := IndexRecord(app, kind.Condition, condition.ID, condition.PatientID,
			condition.Diagnosis, condition.ICD10Code, condition.OnsetOn); err != nil {
			return err
		}
	}

	return nil
}

func applyEmergencyContacts(app core.App) error {
	name := kind.EmergencyContact.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection,
		columnPatient, columnName, columnRelationship, columnPhone, columnIsPrimary, columnIsActive, columnNotes,
	); err != nil {
		return err
	}

	for _, contact := range EmergencyContacts() {
		if err := contact.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", contact.ID, err)
		}

		record, err := findOrNew(app, collection, contact.ID)
		if err != nil {
			return err
		}

		record.Set(columnPatient, contact.PatientID)
		record.Set(columnName, contact.Name)
		record.Set(columnRelationship, string(contact.Relationship))
		record.Set(columnPhone, contact.Phone)
		record.Set(columnIsPrimary, contact.IsPrimary)
		record.Set(columnIsActive, contact.IsActive)
		record.Set(columnNotes, contact.Notes)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", contact.ID, err)
		}

		// Emergency contacts have no primary date column (FR-051's ordering is
		// name/flags, not a date), so occurredOn is the zero Date — the same
		// "unset" IndexRecord already treats null.
		if err := IndexRecord(app, kind.EmergencyContact, contact.ID, contact.PatientID,
			contact.Name, string(contact.Relationship), domain.Date{}); err != nil {
			return err
		}
	}

	return nil
}
