package patient_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/person"
	"medikube/internal/service/patient/patienttest"
	"medikube/internal/store"

	_ "medikube/internal/store/migrations"
)

// T049. idx_patients_self (internal/store/migrations/1756200300_patients.go)
// is a partial unique index: one owner, one is_self_record row. This bypasses
// patient.Service.CreateSelfRecord's own check entirely and writes straight to
// the database, so the guarantee (FR-004) is the schema's, not merely the
// service layer's — a second write through any future caller is refused too.
func TestASecondSelfRecordForOneOwnerIsRefusedByTheSchemaItself(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	remapAccounts(t, app)

	collection, err := app.FindCollectionByNameOrId(store.PatientCollection)
	require.NoError(t, err)

	first := core.NewRecord(collection)
	require.NoError(t, store.PatientToRecord(first, person.Patient{
		OwnerID: patienttest.OwnerID, FirstName: "Amara", LastName: "Okonkwo", IsSelfRecord: true,
	}))
	require.NoError(t, app.Save(first))

	second := core.NewRecord(collection)
	require.NoError(t, store.PatientToRecord(second, person.Patient{
		OwnerID: patienttest.OwnerID, FirstName: "Amara", LastName: "Okonkwo (again)", IsSelfRecord: true,
	}))
	require.Error(t, app.Save(second), "idx_patients_self must refuse a second self-record for the same owner")
}

// TestASecondSelfRecordIsFineForADifferentOwner is the index's other half:
// the uniqueness is scoped per owner, not global.
func TestASecondSelfRecordIsFineForADifferentOwner(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	remapAccounts(t, app)

	collection, err := app.FindCollectionByNameOrId(store.PatientCollection)
	require.NoError(t, err)

	one := core.NewRecord(collection)
	require.NoError(t, store.PatientToRecord(one, person.Patient{
		OwnerID: patienttest.OwnerID, FirstName: "Amara", LastName: "Okonkwo", IsSelfRecord: true,
	}))
	require.NoError(t, app.Save(one))

	two := core.NewRecord(collection)
	require.NoError(t, store.PatientToRecord(two, person.Patient{
		OwnerID: patienttest.StrangerID, FirstName: "Chiamaka", LastName: "Eze", IsSelfRecord: true,
	}))
	require.NoError(t, app.Save(two))
}

// A non-self record never competes for the index (its predicate is
// is_self_record = 1), so an owner may hold as many of those as they like
// alongside their one self-record.
func TestANonSelfRecordDoesNotCompeteForTheSelfRecordIndex(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	remapAccounts(t, app)

	collection, err := app.FindCollectionByNameOrId(store.PatientCollection)
	require.NoError(t, err)

	self := core.NewRecord(collection)
	require.NoError(t, store.PatientToRecord(self, person.Patient{
		OwnerID: patienttest.OwnerID, FirstName: "Amara", LastName: "Okonkwo", IsSelfRecord: true,
	}))
	require.NoError(t, app.Save(self))

	dependent := core.NewRecord(collection)
	require.NoError(t, store.PatientToRecord(dependent, person.Patient{
		OwnerID: patienttest.OwnerID, FirstName: "Chiamaka", LastName: "Okonkwo", IsSelfRecord: false,
	}))
	require.NoError(t, app.Save(dependent))
}
