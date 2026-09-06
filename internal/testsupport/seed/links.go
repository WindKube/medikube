package seed

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	coursemedicationstore "medikube/internal/store/coursemedication"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// The US6 linking fixtures: one medication attached to account A's allergy
// and condition each, one medication in each of a symptom's two independent
// roles (FR-032), and one treatment_medications join row so FR-060/FR-061's
// effective-value fallback has a seeded row to render.
const (
	AllergyLinkedMedicationID    = "mkmedamara00001"
	ConditionLinkedMedicationID  = "mkmedamara00002"
	SymptomTreatedByMedicationID = "mkmedamara00007"
	SymptomCausedByMedicationID  = AllergyLinkedMedicationID
	CourseMedicationTreatmentID  = "mktrtamara00002"
	CourseMedicationMedicationID = AllergyLinkedMedicationID
	// CourseMedicationLinkID is the join row's own id: findOrNew needs one to
	// address the same row on a second run, and treatment_medications has no
	// other unique-by-convention key this package can build a store.Query with
	// (internal/store's own filter DSL boundary keeps that query type-safe
	// building confined to internal/store, research D-26).
	CourseMedicationLinkID = "mktrtmedamara01"
)

// linksFieldMedications and the symptom role fields are built from the
// medications collection's own name, not spelled a second time (research
// D-05), the same way migration 1756400800 that adds them derives its own
// field-name vars.
var (
	linksFieldMedications            = kind.Medication.Collection()
	symptomFieldTreatedByMedications = "treated_by_" + kind.Medication.Collection()
	symptomFieldCausedByMedications  = "caused_by_" + kind.Medication.Collection()
)

const (
	courseMedicationFieldTreatment  = "treatment"
	courseMedicationFieldMedication = "medication"
	courseMedicationFieldDosage     = "dosage"
	courseMedicationFieldFrequency  = "frequency"
	courseMedicationFieldTiming     = "timing"
)

// applyLinks sets the medication-relation fields US6 adds to allergies,
// conditions and symptoms, and seeds one treatment_medications row. It runs
// after applyMedications, applyAllergies, applyConditions, applySymptoms and
// applyTreatments, whose rows it names by id rather than reseeds.
func applyLinks(app core.App) error {
	if err := applyAllergyMedicationLink(app); err != nil {
		return err
	}

	if err := applyConditionMedicationLink(app); err != nil {
		return err
	}

	if err := applySymptomMedicationLinks(app); err != nil {
		return err
	}

	return applyCourseMedicationLink(app)
}

func applyAllergyMedicationLink(app core.App) error {
	name := kind.Allergy.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if columnsErr := requireColumns(collection, linksFieldMedications); columnsErr != nil {
		return columnsErr
	}

	record, err := app.FindRecordById(collection, CriticalAllergyID)
	if err != nil {
		return fmt.Errorf("finding %s %s: %w", name, CriticalAllergyID, err)
	}

	record.Set(linksFieldMedications, []string{AllergyLinkedMedicationID})

	if saveErr := app.Save(record); saveErr != nil {
		return fmt.Errorf("linking %s.%s on %s: %w", name, linksFieldMedications, CriticalAllergyID, saveErr)
	}

	return nil
}

func applyConditionMedicationLink(app core.App) error {
	name := kind.Condition.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if columnsErr := requireColumns(collection, linksFieldMedications); columnsErr != nil {
		return columnsErr
	}

	record, err := app.FindRecordById(collection, ResolvedConditionID)
	if err != nil {
		return fmt.Errorf("finding %s %s: %w", name, ResolvedConditionID, err)
	}

	record.Set(linksFieldMedications, []string{ConditionLinkedMedicationID})

	if saveErr := app.Save(record); saveErr != nil {
		return fmt.Errorf("linking %s.%s on %s: %w", name, linksFieldMedications, ResolvedConditionID, saveErr)
	}

	return nil
}

func applySymptomMedicationLinks(app core.App) error {
	name := kind.Symptom.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if columnsErr := requireColumns(collection,
		symptomFieldTreatedByMedications, symptomFieldCausedByMedications,
	); columnsErr != nil {
		return columnsErr
	}

	treated, err := app.FindRecordById(collection, SymptomHeadacheOne)
	if err != nil {
		return fmt.Errorf("finding %s %s: %w", name, SymptomHeadacheOne, err)
	}

	treated.Set(symptomFieldTreatedByMedications, []string{SymptomTreatedByMedicationID})

	if saveErr := app.Save(treated); saveErr != nil {
		return fmt.Errorf("linking %s.%s on %s: %w", name, symptomFieldTreatedByMedications, SymptomHeadacheOne, saveErr)
	}

	caused, err := app.FindRecordById(collection, SymptomHeadacheTwo)
	if err != nil {
		return fmt.Errorf("finding %s %s: %w", name, SymptomHeadacheTwo, err)
	}

	caused.Set(symptomFieldCausedByMedications, []string{SymptomCausedByMedicationID})

	if saveErr := app.Save(caused); saveErr != nil {
		return fmt.Errorf("linking %s.%s on %s: %w", name, symptomFieldCausedByMedications, SymptomHeadacheTwo, saveErr)
	}

	return nil
}

func applyCourseMedicationLink(app core.App) error {
	name := coursemedicationstore.Collection

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if columnsErr := requireColumns(collection,
		courseMedicationFieldTreatment, courseMedicationFieldMedication,
		courseMedicationFieldDosage, courseMedicationFieldFrequency, courseMedicationFieldTiming,
	); columnsErr != nil {
		return columnsErr
	}

	link := clinical.CourseMedication{
		ID:           CourseMedicationLinkID,
		TreatmentID:  CourseMedicationTreatmentID,
		MedicationID: CourseMedicationMedicationID,
		Dosage:       "10 mg",
		Frequency:    "once daily",
		Timing:       "morning",
	}

	if validateErr := link.Validate(); validateErr != nil {
		return fmt.Errorf("seeding the treatment-medication link: %w", validateErr)
	}

	record, err := findOrNew(app, collection, link.ID)
	if err != nil {
		return err
	}

	record.Set(courseMedicationFieldTreatment, link.TreatmentID)
	record.Set(courseMedicationFieldMedication, link.MedicationID)
	record.Set(courseMedicationFieldDosage, link.Dosage)
	record.Set(courseMedicationFieldFrequency, link.Frequency)
	record.Set(courseMedicationFieldTiming, link.Timing)

	if saveErr := app.Save(record); saveErr != nil {
		return fmt.Errorf("seeding the treatment-medication link: %w", saveErr)
	}

	return nil
}
