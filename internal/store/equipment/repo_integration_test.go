package equipment_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	domainaudit "medikube/internal/domain/audit"
	"medikube/internal/records/recordstest"
	accesssvc "medikube/internal/service/access"
	"medikube/internal/service/equipment"
	"medikube/internal/service/equipment/equipmenttest"
	"medikube/internal/store"
	pbequipment "medikube/internal/store/equipment"

	// Registers the migrations against the test instance.
	_ "medikube/internal/store/migrations"
)

type noopAuditor struct{}

func (noopAuditor) Record(context.Context, domainaudit.Event) error { return nil }

// appsByPatient is how the delete-cascade clause finds the one test app a
// given harness built: RunRepositoryContract's DeletePatient callback is
// handed only a patient id, and every subtest builds its own instance in
// parallel, so a package-level variable would race.
var appsByPatient sync.Map

func newTestHarness(t *testing.T) recordstest.RepositoryHarness {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	cursors, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbequipment.New(app, cursors)
	require.NoError(t, err)

	owners, err := store.NewOwners(app)
	require.NoError(t, err)

	patientOwners, err := store.NewPatientOwners(app)
	require.NoError(t, err)

	authorizer, err := accesssvc.New(owners, accesssvc.WithPatients(patientOwners, noopAuditor{}))
	require.NoError(t, err)

	service, err := equipment.New(repo, authorizer)
	require.NoError(t, err)

	adapter, err := equipment.NewAdapter(service, equipmenttest.NewCodec())
	require.NoError(t, err)

	owner := seedAccount(t, app, "owner+equipment@example.test")
	stranger := seedAccount(t, app, "stranger+equipment@example.test")
	patientID := seedPatient(t, app, owner)

	appsByPatient.Store(patientID, app)
	t.Cleanup(func() { appsByPatient.Delete(patientID) })

	return recordstest.RepositoryHarness{
		Service:   adapter,
		Owner:     access.Actor{UserID: owner},
		PatientID: patientID,
		Stranger:  access.Actor{UserID: stranger},
	}
}

func seedAccount(t *testing.T, app core.App, email string) string {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)

	record := core.NewRecord(users)
	record.SetEmail(email)
	record.SetPassword("correct-horse-battery-staple")
	record.Set("name", "Test Person")
	record.Set("role", "user")
	record.Set("unit_system", "metric")
	record.Set("locale", "en")
	record.Set("date_format", "iso")
	record.Set("theme", "system")

	require.NoError(t, app.Save(record))

	return record.Id
}

func seedPatient(t *testing.T, app core.App, ownerID string) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("patients")
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set("owner", ownerID)
	record.Set("first_name", "Test")
	record.Set("last_name", "Patient")

	require.NoError(t, app.Save(record))

	return record.Id
}

func minimalCreate(patientID string) any {
	return &equipmenttest.Create{Patient: patientID, Name: "CPAP machine", Type: "cpap"}
}

func fullCreate(patientID string) any {
	return &equipmenttest.Create{
		Patient: patientID, Name: "CPAP machine", Type: "cpap", PrescribedOn: "2026-01-01",
	}
}

func deletePatient(ctx context.Context, patientID string) error {
	found, ok := appsByPatient.Load(patientID)
	if !ok {
		return fmt.Errorf("equipment_test: no test app known for patient %s", patientID)
	}

	app, ok := found.(core.App)
	if !ok {
		return fmt.Errorf("equipment_test: the stored value for patient %s is not a core.App", patientID)
	}

	record, err := app.FindRecordById("patients", patientID)
	if err != nil {
		return err
	}

	return app.DeleteWithContext(ctx, record)
}

func TestRepositoryContract(t *testing.T) {
	t.Parallel()

	recordstest.RunRepositoryContract(t, recordstest.RepositoryContractOptions{
		NewHarness: newTestHarness,
		Fixture:    recordstest.Fixture{Minimal: minimalCreate, Full: fullCreate},
		NewPatch:   func() any { return &equipmenttest.Patch{} },
		Sort:       equipment.Sorts()[:1],
		HasPrimaryDate: func(body any) bool {
			detail, ok := body.(*equipmenttest.Detail)
			return ok && detail.PrescribedOn != ""
		},
		DeletePatient: deletePatient,
	})
}
