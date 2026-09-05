package store

import (
	"context"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// recordPatientField is every registered kind's own anchor (data-model §6):
// one column, spelled identically in every clinical collection, which is what
// lets the chart summary's per-kind count be one query shape rather than one
// per kind (contracts/patient-chart.md).
const recordPatientField = "patient"

// CountByPatient is the chart summary's one indexed `COUNT(*) WHERE patient =
// ?` per registered kind (FR-028, SC-007). It switches on nothing: collection
// is the only thing that varies between one registered kind and the next.
func CountByPatient(ctx context.Context, app core.App, collection, patientID string) (int, error) {
	var total int

	err := app.RecordQuery(collection).
		Select("count(*)").
		AndWhere(dbx.HashExp{recordPatientField: patientID}).
		WithContext(ctx).
		Row(&total)
	if err != nil {
		return 0, fmt.Errorf("store: counting %s for a patient: %w", collection, err)
	}

	return total, nil
}
