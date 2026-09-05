package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/audit"
)

// The additive extension of data-model §5.4 / research D-19: audit_events
// gains no field and no action, and target_kind gains exactly tag and search.
// down restores the twenty-five-value set phase 001/002 left it with.
func init() {
	register(auditVocabUp, auditVocabDown)
}

func auditVocabUp(app core.App) error {
	audits, err := app.FindCollectionByNameOrId(auditEventsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", auditEventsCollection, err)
	}

	targetKind, ok := audits.Fields.GetByName(auditFieldTargetKind).(*core.SelectField)
	if !ok {
		return fmt.Errorf("%s.%s is not a select field", auditEventsCollection, auditFieldTargetKind)
	}
	targetKind.Values = enumValues(audit.TargetKinds())

	if err := app.Save(audits); err != nil {
		return fmt.Errorf("saving %s: %w", auditEventsCollection, err)
	}

	return nil
}

func auditVocabDown(app core.App) error {
	audits, err := app.FindCollectionByNameOrId(auditEventsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", auditEventsCollection, err)
	}

	targetKind, ok := audits.Fields.GetByName(auditFieldTargetKind).(*core.SelectField)
	if !ok {
		return fmt.Errorf("%s.%s is not a select field", auditEventsCollection, auditFieldTargetKind)
	}
	targetKind.Values = phase2AuditTargetKinds

	if err := app.Save(audits); err != nil {
		return fmt.Errorf("saving %s: %w", auditEventsCollection, err)
	}

	return nil
}
