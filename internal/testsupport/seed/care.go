package seed

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// The seeded rows US2's three kinds need for the smoke gate and for anybody
// exploring the demo account by hand. Each carries at least one row with every
// optional field empty, mirroring NameOnlyID's role for medications.
const (
	EncounterNameOnlyID = "mkencamara00001"
	ProcedureNameOnlyID = "mkprcamara00001"
	TreatmentNameOnlyID = "mktrtamara00001"
)

// Encounters is account A's four rows. Account B and C have none, for the
// same reason Medications leaves account C empty (research D-39).
func Encounters() []clinical.Encounter {
	return []clinical.Encounter{
		{
			ID: EncounterNameOnlyID, PatientID: accountAPatientSelfID,
			Reason: "Annual check-up", OccurredOn: date(2025, 1, 10),
		},
		{
			ID: "mkencamara00002", PatientID: accountAPatientSelfID,
			Reason: "Follow-up on blood pressure", OccurredOn: date(2025, 4, 2),
			VisitType: clinical.VisitTypeOffice, Priority: clinical.VisitPriorityRoutine,
			Assessment: "Blood pressure well controlled", Plan: "Continue current medication",
			FollowUp: "Review in six months", DurationMin: 20,
			ConditionID: ResolvedConditionID,
		},
		{
			ID: "mkencamara00003", PatientID: accountAPatientSelfID,
			Reason: "Sudden chest pain", OccurredOn: date(2025, 8, 14),
			VisitType: clinical.VisitTypeEmergency, Priority: clinical.VisitPriorityEmergency,
			Assessment: "Non-cardiac chest pain, likely musculoskeletal",
			Plan:       "Analgesia and rest", FollowUp: "Return if it does not improve",
			DurationMin: 90, Notes: "Discharged the same day",
		},
		{
			ID: "mkencamara00004", PatientID: accountAPatientSelfID,
			Reason: "Telehealth consultation", OccurredOn: date(2025, 9, 1),
			VisitType: clinical.VisitTypeTelehealth, Priority: clinical.VisitPriorityRoutine,
			DurationMin: 15,
		},
	}
}

// Procedures is account A's four rows, one of them scheduled in the future so
// FR-025's "scheduled" basis (procedure.BasisFor) has a seeded row to render.
func Procedures() []clinical.Procedure {
	return []clinical.Procedure{
		{
			ID: ProcedureNameOnlyID, PatientID: accountAPatientSelfID,
			Name: "Skin biopsy", OccurredOn: date(2025, 2, 5),
			Status: clinical.OrderStatusCompleted,
		},
		{
			ID: "mkprcamara00002", PatientID: accountAPatientSelfID,
			Name: "Colonoscopy", Type: clinical.ProcedureTypeDiagnostic,
			Code: "45378", OccurredOn: date(2025, 5, 20),
			Status: clinical.OrderStatusCompleted, Outcome: clinical.ProcedureOutcomeSuccessful,
			Setting: clinical.ProcedureSettingOutpatient, DurationMin: 45,
			Anesthesia: clinical.AnesthesiaSedation,
		},
		{
			ID: "mkprcamara00003", PatientID: accountAPatientSelfID,
			Name: "Knee arthroscopy", Type: clinical.ProcedureTypeSurgical,
			OccurredOn: date(2099, 3, 1),
			Status:     clinical.OrderStatusScheduled, Setting: clinical.ProcedureSettingInpatient,
			Anesthesia: clinical.AnesthesiaGeneral,
		},
		{
			ID: "mkprcamara00004", PatientID: accountAPatientSelfID,
			Name: "Wound suturing", Type: clinical.ProcedureTypeTherapeutic,
			OccurredOn: date(2025, 6, 11),
			Status:     clinical.OrderStatusCompleted, Outcome: clinical.ProcedureOutcomeComplications,
			Complications: "Minor wound infection, resolved with antibiotics",
			Anesthesia:    clinical.AnesthesiaLocal, Notes: "Follow-up dressing change at one week",
		},
	}
}

// Treatments is account A's three rows. mkencamara00002 and mkencamara00003
// are two of Encounters' own ids, so FR-028's encounters relation has a row
// that is not empty.
func Treatments() []clinical.Treatment {
	return []clinical.Treatment{
		{
			ID: TreatmentNameOnlyID, PatientID: accountAPatientSelfID,
			Name: "Physical therapy", StartedOn: date(2025, 3, 1),
		},
		{
			ID: "mktrtamara00002", PatientID: accountAPatientSelfID,
			Name: "Cardiac rehabilitation", Setting: clinical.TreatmentSettingOutpatient,
			Description: "Supervised exercise programme after chest pain workup",
			StartedOn:   date(2025, 8, 20), Frequency: "twice weekly",
			ExpectedOutcome: "Improved exercise tolerance", Status: clinical.TherapyStatusActive,
			Encounters: []string{"mkencamara00002", "mkencamara00003"},
		},
		{
			ID: "mktrtamara00003", PatientID: accountAPatientSelfID,
			Name: "Wound care", Setting: clinical.TreatmentSettingHome,
			StartedOn: date(2025, 6, 12), EndedOn: date(2025, 6, 26),
			Dosage: "Daily dressing change", Status: clinical.TherapyStatusCompleted,
		},
	}
}

const (
	columnReason          = "reason"
	columnVisitType       = "visit_type"
	columnPriority        = "priority"
	columnAssessment      = "assessment"
	columnPlan            = "plan"
	columnFollowUp        = "follow_up"
	columnDurationMin     = "duration_minutes"
	columnCode            = "code"
	columnDescription     = "description"
	columnOutcome         = "outcome"
	columnSetting         = "setting"
	columnComplications   = "complications"
	columnAnesthesia      = "anesthesia"
	columnAnesthesiaNotes = "anesthesia_notes"
	columnExpectedOutcome = "expected_outcome"
	columnCondition       = "condition"
)

// Named after the collections they relate to, per data-model §4.5 (FR-028),
// so declared from the kind table rather than spelled a second time here.
var (
	columnEncounters = kind.Encounter.Collection()
	columnEquipment  = kind.Equipment.Collection()
)

func applyEncounters(app core.App) error {
	name := kind.Encounter.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection,
		columnPatient, columnReason, columnOccurredOn, columnVisitType, columnPriority,
		columnAssessment, columnPlan, columnFollowUp, columnDurationMin,
		columnPractitioner, columnFacility, columnCondition, columnNotes,
	); err != nil {
		return err
	}

	for _, encounter := range Encounters() {
		if err := encounter.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", encounter.ID, err)
		}

		record, err := findOrNew(app, collection, encounter.ID)
		if err != nil {
			return err
		}

		record.Set(columnPatient, encounter.PatientID)
		record.Set(columnReason, encounter.Reason)
		record.Set(columnOccurredOn, encounter.OccurredOn.UTC())
		record.Set(columnVisitType, string(encounter.VisitType))
		record.Set(columnPriority, string(encounter.Priority))
		record.Set(columnAssessment, encounter.Assessment)
		record.Set(columnPlan, encounter.Plan)
		record.Set(columnFollowUp, encounter.FollowUp)
		record.Set(columnDurationMin, encounter.DurationMin)
		record.Set(columnPractitioner, encounter.PractitionerID)
		record.Set(columnFacility, encounter.FacilityID)
		record.Set(columnCondition, encounter.ConditionID)
		record.Set(columnNotes, encounter.Notes)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", encounter.ID, err)
		}

		if err := IndexRecord(app, kind.Encounter, encounter.ID, encounter.PatientID,
			encounter.Reason, encounter.Assessment, encounter.OccurredOn); err != nil {
			return err
		}
	}

	return nil
}

func applyProcedures(app core.App) error {
	name := kind.Procedure.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection,
		columnPatient, columnName, columnType, columnCode, columnDescription, columnOccurredOn,
		columnStatus, columnOutcome, columnSetting, columnComplications, columnDurationMin,
		columnAnesthesia, columnAnesthesiaNotes, columnPractitioner, columnFacility, columnCondition, columnNotes,
	); err != nil {
		return err
	}

	for _, procedure := range Procedures() {
		if err := procedure.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", procedure.ID, err)
		}

		record, err := findOrNew(app, collection, procedure.ID)
		if err != nil {
			return err
		}

		record.Set(columnPatient, procedure.PatientID)
		record.Set(columnName, procedure.Name)
		record.Set(columnType, string(procedure.Type))
		record.Set(columnCode, procedure.Code)
		record.Set(columnDescription, procedure.Description)
		record.Set(columnOccurredOn, procedure.OccurredOn.UTC())
		record.Set(columnStatus, string(procedure.Status))
		record.Set(columnOutcome, string(procedure.Outcome))
		record.Set(columnSetting, string(procedure.Setting))
		record.Set(columnComplications, procedure.Complications)
		record.Set(columnDurationMin, procedure.DurationMin)
		record.Set(columnAnesthesia, string(procedure.Anesthesia))
		record.Set(columnAnesthesiaNotes, procedure.AnesthesiaNotes)
		record.Set(columnPractitioner, procedure.PractitionerID)
		record.Set(columnFacility, procedure.FacilityID)
		record.Set(columnCondition, procedure.ConditionID)
		record.Set(columnNotes, procedure.Notes)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", procedure.ID, err)
		}

		if err := IndexRecord(app, kind.Procedure, procedure.ID, procedure.PatientID,
			procedure.Name, procedure.Description, procedure.OccurredOn); err != nil {
			return err
		}
	}

	return nil
}

func applyTreatments(app core.App) error {
	name := kind.Treatment.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection,
		columnPatient, columnName, columnType, columnSetting, columnDescription,
		columnStartedOn, columnEndedOn, columnFrequency, columnDosage, columnExpectedOutcome,
		columnStatus, columnPractitioner, columnFacility, columnCondition, columnEncounters, columnEquipment, columnNotes,
	); err != nil {
		return err
	}

	for _, treatment := range Treatments() {
		if err := treatment.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", treatment.ID, err)
		}

		record, err := findOrNew(app, collection, treatment.ID)
		if err != nil {
			return err
		}

		record.Set(columnPatient, treatment.PatientID)
		record.Set(columnName, treatment.Name)
		record.Set(columnType, treatment.Type)
		record.Set(columnSetting, string(treatment.Setting))
		record.Set(columnDescription, treatment.Description)
		record.Set(columnStartedOn, treatment.StartedOn.UTC())
		record.Set(columnEndedOn, treatment.EndedOn.UTC())
		record.Set(columnFrequency, treatment.Frequency)
		record.Set(columnDosage, treatment.Dosage)
		record.Set(columnExpectedOutcome, treatment.ExpectedOutcome)
		record.Set(columnStatus, string(treatment.Status))
		record.Set(columnPractitioner, treatment.PractitionerID)
		record.Set(columnFacility, treatment.FacilityID)
		record.Set(columnCondition, treatment.ConditionID)
		record.Set(columnEncounters, treatment.Encounters)
		record.Set(columnEquipment, treatment.Equipment)
		record.Set(columnNotes, treatment.Notes)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", treatment.ID, err)
		}

		if err := IndexRecord(app, kind.Treatment, treatment.ID, treatment.PatientID,
			treatment.Name, treatment.Description, treatment.StartedOn); err != nil {
			return err
		}
	}

	return nil
}
