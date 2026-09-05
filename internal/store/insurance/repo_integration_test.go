package insurance_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	domainaudit "medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/records/recordstest"
	accesssvc "medikube/internal/service/access"
	"medikube/internal/service/insurance"
	"medikube/internal/service/insurance/insurancetest"
	"medikube/internal/store"
	pbinsurance "medikube/internal/store/insurance"

	// Registers the migrations against the test instance.
	_ "medikube/internal/store/migrations"
)

type noopAuditor struct{}

func (noopAuditor) Record(context.Context, domainaudit.Event) error { return nil }

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

	repo, err := pbinsurance.New(app, cursors)
	require.NoError(t, err)

	owners, err := store.NewOwners(app)
	require.NoError(t, err)

	patientOwners, err := store.NewPatientOwners(app)
	require.NoError(t, err)

	authorizer, err := accesssvc.New(owners, accesssvc.WithPatients(patientOwners, noopAuditor{}))
	require.NoError(t, err)

	service, err := insurance.New(repo, authorizer)
	require.NoError(t, err)

	adapter, err := insurance.NewAdapter(service, insurancetest.NewCodec())
	require.NoError(t, err)

	owner := seedAccount(t, app, "owner+insurance@example.test")
	stranger := seedAccount(t, app, "stranger+insurance@example.test")
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
	return &insurancetest.Create{
		Patient: patientID, Type: "medical", Company: "Acme Health",
		MemberName: "Jamie Doe", MemberID: "M1", EffectiveOn: "2026-01-01",
	}
}

func TestRepositoryContract(t *testing.T) {
	t.Parallel()

	recordstest.RunRepositoryContract(t, recordstest.RepositoryContractOptions{
		NewHarness: newTestHarness,
		Fixture:    recordstest.Fixture{Minimal: minimalCreate, Full: minimalCreate},
		NewPatch:   func() any { return &insurancetest.Patch{} },
		Sort:       insurance.Sorts()[:1],
		// effective_on is required (FR-043), so Fixture.Minimal cannot omit
		// it: there is no undated row to exercise "sorts last" against.
		NullPrimaryDateSkip: "effective_on is required, so every fixture carries one",
		DeletePatient: func(ctx context.Context, patientID string) error {
			found, ok := appsByPatient.Load(patientID)
			if !ok {
				return fmt.Errorf("insurance_test: no test app known for patient %s", patientID)
			}

			app, ok := found.(core.App)
			if !ok {
				return fmt.Errorf("insurance_test: the stored value for patient %s is not a core.App", patientID)
			}

			record, err := app.FindRecordById("patients", patientID)
			if err != nil {
				return err
			}

			return app.DeleteWithContext(ctx, record)
		},
	})
}

// TestPartialUniqueIndexAllowsOnlyOnePrimary is data-model §4.10's structural
// guarantee (FR-045): the database itself refuses a second primary policy,
// independent of the service's own displacement logic.
func TestPartialUniqueIndexAllowsOnlyOnePrimary(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	owner := seedAccount(t, app, "owner+primary@example.test")
	patientID := seedPatient(t, app, owner)

	collection, err := app.FindCollectionByNameOrId(kind.Insurance.Collection())
	require.NoError(t, err)

	first := core.NewRecord(collection)
	first.Set("patient", patientID)
	first.Set("type", "medical")
	first.Set("company", "Acme Health")
	first.Set("member_name", "Jamie Doe")
	first.Set("member_id", "M1")
	first.Set("effective_on", "2026-01-01")
	first.Set("is_primary", true)
	require.NoError(t, app.Save(first))

	second := core.NewRecord(collection)
	second.Set("patient", patientID)
	second.Set("type", "medical")
	second.Set("company", "Other Health")
	second.Set("member_name", "Jamie Doe")
	second.Set("member_id", "M2")
	second.Set("effective_on", "2026-01-01")
	second.Set("is_primary", true)

	assert.Error(t, app.Save(second), "the partial unique index should refuse a second primary policy")
}
