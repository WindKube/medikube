package store_test

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/store/migrations"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

// T205, FR-087, SC-005. Deleting a patient must destroy 100% of their records
// of every one of the fourteen kinds and every search_index row, leaving 0
// rows attributed to a patient that no longer exists.
//
// The link and treatment_medications leg is deferred: US6 is still in flight
// and lands its own cascade case alongside the link work.
func TestDeletingAPatientDestroysEveryKindOfItsRecordsAndItsSearchIndexRows(t *testing.T) {
	t.Parallel()

	instance := apitest.New(t)
	app := instance.App

	patientID := newCascadePatient(t, app, testsupport.AccountAID)
	actor := access.Actor{UserID: testsupport.AccountAID, RequestID: "req-cascade"}

	fixtures := cascadeFixtures()
	require.Len(t, fixtures, len(kind.Kinds()), "every registered kind must have a fixture in this test")

	for _, k := range kind.Kinds() {
		fixture, ok := fixtures[k]
		require.Truef(t, ok, "%s has no minimal fixture in cascadeFixtures", k)

		entry, err := instance.Records.Dispatch(k.Segment())
		require.NoErrorf(t, err, "dispatching %s", k)

		_, err = entry.Service.Create(t.Context(), actor, fixture(patientID))
		require.NoErrorf(t, err, "creating a %s for the cascade patient", k)
	}

	// A row per kind must exist before the delete, or the counts after it
	// prove nothing.
	for _, k := range kind.Kinds() {
		n, err := app.CountRecords(k.Collection(), dbx.HashExp{"patient": patientID})
		require.NoError(t, err)
		require.Equalf(t, int64(1), n, "%s: the fixture record was not seeded", k)
	}

	searchBefore, err := app.CountRecords(migrations.SearchIndexCollection, dbx.HashExp{"patient": patientID})
	require.NoError(t, err)
	require.Equalf(t, int64(len(kind.Kinds())), searchBefore,
		"every created record should have indexed a search_index row before the delete")

	patientRecord, err := app.FindRecordById(patientsCollection, patientID)
	require.NoError(t, err)
	require.NoError(t, app.Delete(patientRecord))

	for _, k := range kind.Kinds() {
		n, err := app.CountRecords(k.Collection(), dbx.HashExp{"patient": patientID})
		require.NoError(t, err)
		require.EqualValuesf(t, 0, n, "%s: a record still names the deleted patient", k)
	}

	searchAfter, err := app.CountRecords(migrations.SearchIndexCollection, dbx.HashExp{"patient": patientID})
	require.NoError(t, err)
	require.EqualValues(t, 0, searchAfter, "a search_index row still names the deleted patient")
}

const patientsCollection = "patients"

// newCascadePatient is one minimal patient owned by owner, exactly as
// internal/web/api/harness_test.go's newPatientFor builds one for the same
// reason: a fresh id to attribute every fixture record to, so this test's
// counts cannot be confused with anything the committed fixture already
// seeded.
func newCascadePatient(t *testing.T, app core.App, ownerID string) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(patientsCollection)
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set("owner", ownerID)
	record.Set("first_name", "Cascade")
	record.Set("last_name", "Patient")

	require.NoError(t, app.Save(record))

	return record.Id
}

// cascadeFixtures is every registered kind's minimal create body, the same
// shape each kind's own contract test builds its recordstest.Fixture.Minimal
// from, gathered in one place because this is the one test that creates all
// fourteen kinds against a single patient.
func cascadeFixtures() map[kind.Kind]func(patientID string) any {
	occurredOn := "2026-01-10"
	administeredOn := "2024-01-01"

	return map[kind.Kind]func(patientID string) any{
		kind.Medication: func(patientID string) any {
			return &api.MedicationCreate{Patient: patientID, Name: "Ibuprofen"}
		},
		kind.Allergy: func(patientID string) any {
			return &api.AllergyCreate{Patient: patientID, Allergen: "Penicillin", Severity: "mild"}
		},
		kind.Condition: func(patientID string) any {
			return &api.ConditionCreate{Patient: patientID, Diagnosis: "Seasonal allergy", Status: "active"}
		},
		kind.Encounter: func(patientID string) any {
			return &api.EncounterCreate{Patient: patientID, Reason: "Annual check-up", OccurredOn: &occurredOn}
		},
		kind.Procedure: func(patientID string) any {
			return &api.ProcedureCreate{
				Patient: patientID, Name: "Skin biopsy", OccurredOn: &occurredOn, Status: "completed",
			}
		},
		kind.Treatment: func(patientID string) any {
			started := "2026-01-10"
			return &api.TreatmentCreate{Patient: patientID, Name: "Physical therapy", StartedOn: &started}
		},
		kind.Symptom: func(patientID string) any {
			return &api.SymptomCreate{
				Patient: patientID, Name: "Headache", Severity: "moderate", OccurredAt: "2026-01-01T09:00:00Z",
			}
		},
		kind.Vitals: func(patientID string) any {
			weight := 70.0
			return &api.VitalsCreate{Patient: patientID, RecordedAt: "2026-01-01T09:00:00Z", WeightKg: &weight}
		},
		kind.Immunization: func(patientID string) any {
			return &api.ImmunizationCreate{Patient: patientID, VaccineName: "Influenza", AdministeredOn: &administeredOn}
		},
		kind.Injury: func(patientID string) any {
			return &api.InjuryCreate{Patient: patientID, Name: "Twisted ankle", BodyPart: "ankle"}
		},
		kind.Insurance: func(patientID string) any {
			return &api.InsuranceCreate{
				Patient: patientID, Type: "medical", Company: "Acme Health",
				MemberName: "Amara Okafor", MemberID: "MEM-001", EffectiveOn: "2024-01-01",
			}
		},
		kind.Equipment: func(patientID string) any {
			return &api.EquipmentCreate{Patient: patientID, Name: "CPAP machine", Type: "cpap"}
		},
		kind.EmergencyContact: func(patientID string) any {
			return &api.EmergencyContactCreate{
				Patient: patientID, Name: "Ngozi Okonkwo", Relationship: "spouse", Phone: "+1-555-0100",
			}
		},
		kind.FamilyMember: func(patientID string) any {
			return &api.FamilyMemberCreate{Patient: patientID, Name: "Chiamaka Okonkwo", Relationship: "aunt"}
		},
	}
}
