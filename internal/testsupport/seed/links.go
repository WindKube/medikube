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

	if err := requireColumns(collection, linksFieldMedications); err != nil {
		return err
	}

	record, err := app.FindRecordById(collection, CriticalAllergyID)
	if err != nil {
		return fmt.Errorf("finding %s %s: %w", name, CriticalAllergyID, err)
	}

	record.Set(linksFieldMedications, []string{AllergyLinkedMedicationID})

	if err := app.Save(record); err != nil {
		return fmt.Errorf("linking %s.%s on %s: %w", name, linksFieldMedications, CriticalAllergyID, err)
	}

	return nil
}

func applyConditionMedicationLink(app core.App) error {
	name := kind.Condition.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection, linksFieldMedications); err != nil {
		return err
	}

	record, err := app.FindRecordById(collection, ResolvedConditionID)
	if err != nil {
		return fmt.Errorf("finding %s %s: %w", name, ResolvedConditionID, err)
	}

	record.Set(linksFieldMedications, []string{ConditionLinkedMedicationID})

	if err := app.Save(record); err != nil {
		return fmt.Errorf("linking %s.%s on %s: %w", name, linksFieldMedications, ResolvedConditionID, err)
	}

	return nil
}

func applySymptomMedicationLinks(app core.App) error {
	name := kind.Symptom.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection, symptomFieldTreatedByMedications, symptomFieldCausedByMedications); err != nil {
		return err
	}

	treated, err := app.FindRecordById(collection, SymptomHeadacheOne)
	if err != nil {
		return fmt.Errorf("finding %s %s: %w", name, SymptomHeadacheOne, err)
	}

	treated.Set(symptomFieldTreatedByMedications, []string{SymptomTreatedByMedicationID})

	if err := app.Save(treated); err != nil {
		return fmt.Errorf("linking %s.%s on %s: %w", name, symptomFieldTreatedByMedications, SymptomHeadacheOne, err)
	}

	caused, err := app.FindRecordById(collection, SymptomHeadacheTwo)
	if err != nil {
		return fmt.Errorf("finding %s %s: %w", name, SymptomHeadacheTwo, err)
	}

	caused.Set(symptomFieldCausedByMedications, []string{SymptomCausedByMedicationID})

	if err := app.Save(caused); err != nil {
		return fmt.Errorf("linking %s.%s on %s: %w", name, symptomFieldCausedByMedications, SymptomHeadacheTwo, err)
	}

	return nil
}

func applyCourseMedicationLink(app core.App) error {
	name := coursemedicationstore.Collection

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection,
		courseMedicationFieldTreatment, courseMedicationFieldMedication,
		courseMedicationFieldDosage, courseMedicationFieldFrequency, courseMedicationFieldTiming,
	); err != nil {
		return err
	}

	link := clinical.CourseMedication{
		TreatmentID:  CourseMedicationTreatmentID,
		MedicationID: CourseMedicationMedicationID,
		Dosage:       "10 mg",
		Frequency:    "once daily",
		Timing:       "morning",
	}

	if err := link.Validate(); err != nil {
		return fmt.Errorf("seeding the treatment-medication link: %w", err)
	}

	built, err := app.FindFirstRecordByFilter(collection,
		courseMedicationFieldTreatment+" = {:treatment} && "+courseMedicationFieldMedication+" = {:medication}",
		map[string]any{"treatment": link.TreatmentID, "medication": link.MedicationID},
	)

	record := built
	if err != nil {
		record = core.NewRecord(collection)
	}

	record.Set(courseMedicationFieldTreatment, link.TreatmentID)
	record.Set(courseMedicationFieldMedication, link.MedicationID)
	record.Set(courseMedicationFieldDosage, link.Dosage)
	record.Set(courseMedicationFieldFrequency, link.Frequency)
	record.Set(courseMedicationFieldTiming, link.Timing)

	if err := app.Save(record); err != nil {
		return fmt.Errorf("seeding the treatment-medication link: %w", err)
	}

	return nil
}
