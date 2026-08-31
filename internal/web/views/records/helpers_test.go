package records_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/testsupport/seed"
	"medikube/internal/web/views/records"
)

// The seeded rows are the corpus data-model §6 built for exactly this: a fully
// populated row, a row with a name and a state and nothing else, and a row
// whose name is right-to-left text mixed with characters that look like markup.
// A test that invented its own would be asserting against data no other tier
// ever sees.
func seeded(t *testing.T, id string) clinical.Medication {
	t.Helper()

	for _, medication := range seed.Medications() {
		if medication.ID == id {
			// The seeder leaves the timestamps to the store. The views show the
			// last-changed time (FR-020), so the tests supply one.
			medication.CreatedAt = time.Date(2026, time.January, 4, 8, 30, 0, 0, time.UTC)
			medication.UpdatedAt = time.Date(2026, time.August, 27, 9, 14, 5, 0, time.UTC)
			medication.Version = "v1"
			return medication
		}
	}

	require.FailNowf(t, "no such seeded row", "%s is not in the fixture", id)

	return clinical.Medication{}
}

// everyFieldFilledIn is the row no fixture has: all twelve recorded values at
// once, so "every recorded value is present" has twelve things to be wrong
// about rather than seven.
func everyFieldFilledIn(t *testing.T) clinical.Medication {
	t.Helper()

	started, err := domain.NewDate(2025, time.March, 4)
	require.NoError(t, err)

	ended, err := domain.NewDate(2025, time.September, 30)
	require.NoError(t, err)

	return clinical.Medication{
		ID:              seed.SingleDayID,
		OwnerID:         seed.AccountAID,
		Name:            "Atorvastatin",
		AlternativeName: "Lipitor",
		Type:            clinical.MedicationTypePrescription,
		Dosage:          "20 mg",
		Frequency:       "once daily at night",
		Route:           clinical.MedicationRouteOral,
		Indication:      "raised cholesterol",
		StartedOn:       started,
		EndedOn:         ended,
		Status:          clinical.TherapyStatusCompleted,
		SideEffects:     "aching calves in the first fortnight",
		Notes:           "reviewed at the six-month appointment",
		CreatedAt:       time.Date(2025, time.March, 4, 7, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, time.August, 27, 9, 14, 5, 0, time.UTC),
		Version:         "v7",
	}
}

// The URLs the views are handed. They are built from the kind table the same
// way internal/httproute builds them, because a view that spelled the segment
// itself would be the fourth spelling research D-05 exists to prevent.
func links(id string) records.MedicationLinks {
	list := "/" + kind.Medication.Segment()
	record := "/api/v1/records/" + kind.Medication.Segment() + "/" + id

	return records.MedicationLinks{
		Detail: list + "/" + id,
		Edit:   list + "/" + id + "/edit",
		Record: record,
	}
}

func view(t *testing.T, medication clinical.Medication) records.MedicationView {
	t.Helper()

	return records.NewMedicationView(medication, links(medication.ID))
}
