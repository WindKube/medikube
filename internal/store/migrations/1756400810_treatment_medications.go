package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
)

// TreatmentMedicationsCollection is data-model §5.2, the one payload-carrying
// join US6 adds. It is not a kind.Kind: a course-medication attachment is not
// a record kind of its own, it is the relationship between a treatment and a
// medication (research D-05's reasoning, applied to a join rather than an
// anchor).
// Its value is derived from the medications collection's own name rather
// than spelled whole (research D-05): the join is "treatment_" plus it.
var TreatmentMedicationsCollection = "treatment_" + kind.Medication.Collection()

const (
	treatmentMedicationFieldTreatment  = "treatment"
	treatmentMedicationFieldMedication = "medication"
	treatmentMedicationFieldDosage     = "dosage"
	treatmentMedicationFieldFrequency  = "frequency"
	treatmentMedicationFieldDuration   = "duration"
	treatmentMedicationFieldTiming     = "timing"
	treatmentMedicationFieldPrescriber = "prescriber"
	treatmentMedicationFieldPharmacy   = "pharmacy"
	treatmentMedicationFieldStartedOn  = "started_on"
	treatmentMedicationFieldEndedOn    = "ended_on"
)

const (
	treatmentMedicationDosageMax    = 200
	treatmentMedicationFrequencyMax = 100
	treatmentMedicationDurationMax  = 100
	treatmentMedicationTimingMax    = 300
)

// uniqTreatmentMedicationIndex is FR-061: at most one attachment of a given
// medication to a given course of treatment.
const uniqTreatmentMedicationIndex = "uniq_treatment_medication"

func init() {
	register(treatmentMedicationsUp, treatmentMedicationsDown)
}

func treatmentMedicationsUp(app core.App) error {
	treatments, err := app.FindCollectionByNameOrId(kind.Treatment.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Treatment.Collection(), err)
	}

	medications, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Medication.Collection(), err)
	}

	practitioners, err := app.FindCollectionByNameOrId(practitionersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", practitionersCollection, err)
	}

	facilities, err := app.FindCollectionByNameOrId(facilitiesCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", facilitiesCollection, err)
	}

	collection := core.NewBaseCollection(TreatmentMedicationsCollection)
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name: treatmentMedicationFieldTreatment, Required: true, CascadeDelete: true,
		MaxSelect: 1, CollectionId: treatments.Id,
	})
	collection.Fields.Add(&core.RelationField{
		Name: treatmentMedicationFieldMedication, Required: true, CascadeDelete: true,
		MaxSelect: 1, CollectionId: medications.Id,
	})
	collection.Fields.Add(&core.TextField{Name: treatmentMedicationFieldDosage, Max: treatmentMedicationDosageMax})
	collection.Fields.Add(&core.TextField{Name: treatmentMedicationFieldFrequency, Max: treatmentMedicationFrequencyMax})
	collection.Fields.Add(&core.TextField{Name: treatmentMedicationFieldDuration, Max: treatmentMedicationDurationMax})
	collection.Fields.Add(&core.TextField{Name: treatmentMedicationFieldTiming, Max: treatmentMedicationTimingMax})
	collection.Fields.Add(&core.RelationField{
		Name: treatmentMedicationFieldPrescriber, MaxSelect: 1, CollectionId: practitioners.Id,
	})
	collection.Fields.Add(&core.RelationField{
		Name: treatmentMedicationFieldPharmacy, MaxSelect: 1, CollectionId: facilities.Id,
	})
	collection.Fields.Add(&core.DateField{Name: treatmentMedicationFieldStartedOn})
	collection.Fields.Add(&core.DateField{Name: treatmentMedicationFieldEndedOn})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	collection.AddIndex(uniqTreatmentMedicationIndex, true,
		treatmentMedicationFieldTreatment+", "+treatmentMedicationFieldMedication, "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", TreatmentMedicationsCollection, err)
	}

	return nil
}

func treatmentMedicationsDown(app core.App) error {
	return deleteCollection(app, TreatmentMedicationsCollection)
}
