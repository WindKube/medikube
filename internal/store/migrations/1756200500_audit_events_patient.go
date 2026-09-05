package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/audit"
)

const auditFieldPatient = "patient"

const auditPatientTimeIndex = "idx_audit_patient_time"

func init() {
	register(auditEventsPatientUp, auditEventsPatientDown)
}

func auditEventsPatientUp(app core.App) error {
	audits, err := app.FindCollectionByNameOrId(auditEventsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", auditEventsCollection, err)
	}

	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	// FR-020/FR-045: the person a patient-scoped action concerned. Null for a
	// non-patient action; auto-unset when the patient is deleted, so a
	// historical entry survives without pointing at a ghost.
	audits.Fields.Add(&core.RelationField{
		Name:          auditFieldPatient,
		Required:      false,
		CascadeDelete: false,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})

	action, ok := audits.Fields.GetByName(auditFieldAction).(*core.SelectField)
	if !ok {
		return fmt.Errorf("%s.%s is not a select field", auditEventsCollection, auditFieldAction)
	}
	action.Values = enumValues(audit.Actions())

	targetKind, ok := audits.Fields.GetByName(auditFieldTargetKind).(*core.SelectField)
	if !ok {
		return fmt.Errorf("%s.%s is not a select field", auditEventsCollection, auditFieldTargetKind)
	}
	targetKind.Values = enumValues(audit.TargetKinds())

	// FR-029, SC-004. `id DESC` so phase 006's keyset reader stays index-only.
	audits.AddIndex(auditPatientTimeIndex, false,
		auditFieldPatient+", "+auditFieldOccurredAt+" DESC, id DESC", "")

	if err := app.Save(audits); err != nil {
		return fmt.Errorf("saving %s: %w", auditEventsCollection, err)
	}

	return nil
}

func auditEventsPatientDown(app core.App) error {
	audits, err := app.FindCollectionByNameOrId(auditEventsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", auditEventsCollection, err)
	}

	audits.RemoveIndex(auditPatientTimeIndex)
	audits.Fields.RemoveByName(auditFieldPatient)

	action, ok := audits.Fields.GetByName(auditFieldAction).(*core.SelectField)
	if !ok {
		return fmt.Errorf("%s.%s is not a select field", auditEventsCollection, auditFieldAction)
	}
	action.Values = phase1AuditActions

	targetKind, ok := audits.Fields.GetByName(auditFieldTargetKind).(*core.SelectField)
	if !ok {
		return fmt.Errorf("%s.%s is not a select field", auditEventsCollection, auditFieldTargetKind)
	}
	targetKind.Values = phase1AuditTargetKinds

	if err := app.Save(audits); err != nil {
		return fmt.Errorf("saving %s: %w", auditEventsCollection, err)
	}

	return nil
}
