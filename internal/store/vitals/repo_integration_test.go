package vitals_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	service "medikube/internal/service/vitals"
	"medikube/internal/service/vitals/vitalstest"
	"medikube/internal/store"
	pbvitals "medikube/internal/store/vitals"

	// Registers the migrations this package's collection needs.
	_ "medikube/internal/store/migrations"
)

const vitalsOwnerEmail = "measurements-owner@example.test"

// testCodec is the identity boundary a store-level contract run needs: no
// unit conversion (research D-15 has nothing to convert against a fixed
// metric viewer), and every fixture body is the domain entity itself rather
// than a wire DTO.
type testCodec struct{}

func (testCodec) Summary(v clinical.Vitals, _ identity.UnitSystem) any { return v }
func (testCodec) Detail(v clinical.Vitals, _ identity.UnitSystem) any  { return v }

func (testCodec) Draft(body any, _ identity.UnitSystem) (clinical.Vitals, error) {
	draft, ok := body.(clinical.Vitals)
	if !ok {
		return clinical.Vitals{}, errors.New("the fixture body is not a clinical.Vitals")
	}

	return draft, nil
}

func (testCodec) Patch(body any, _ identity.UnitSystem) (service.Patch, error) {
	patch, ok := body.(*service.Patch)
	if !ok {
		return service.Patch{}, errors.New("the fixture body is not a *service.Patch")
	}

	return *patch, nil
}

func metricAlways(context.Context, access.Actor) (identity.UnitSystem, error) {
	return identity.UnitSystemMetric, nil
}

func newVitalsHarness(t *testing.T) recordstest.RepositoryHarness {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbvitals.New(app, codec)
	require.NoError(t, err)

	owner := vitalsSeedAccount(t, app, vitalsOwnerEmail)
	patientID := vitalsSeedPatient(t, app, owner)

	svc, err := service.New(repo, vitalstest.Authorizer{OwnerID: owner})
	require.NoError(t, err)

	adapter, err := service.NewAdapter(svc, testCodec{}, metricAlways)
	require.NoError(t, err)

	var _ records.Service = adapter

	return recordstest.RepositoryHarness{
		Service:   adapter,
		Owner:     access.Actor{UserID: owner, RequestID: "req-1"},
		PatientID: patientID,
		Stranger:  access.Actor{UserID: "somebody-else", RequestID: "req-2"},
	}
}

func vitalsSeedAccount(t *testing.T, app core.App, email string) string {
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

func vitalsSeedPatient(t *testing.T, app core.App, ownerID string) string {
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

func vitalsFixture(recordedAt clinical.Instant) func(patientID string) any {
	return func(patientID string) any {
		weight := 70.0

		return clinical.Vitals{
			PatientID:  patientID,
			RecordedAt: recordedAt,
			WeightKg:   &weight,
		}
	}
}

// T087: the shared storage-tier contract, run against the real PocketBase
// repository through a bare adapter — no HTTP, no wire DTOs.
func TestThePocketBaseRepositoryPassesTheSharedRepositoryContract(t *testing.T) {
	t.Parallel()

	now := clinical.NewInstant(time.Now().UTC())

	recordstest.RunRepositoryContract(t, recordstest.RepositoryContractOptions{
		NewHarness: func(t *testing.T) recordstest.RepositoryHarness {
			t.Helper()

			return newVitalsHarness(t)
		},
		Fixture: recordstest.Fixture{
			Minimal: vitalsFixture(now),
			Full:    vitalsFixture(now),
		},
		NewPatch: func() any { return &service.Patch{} },
		Sort:     service.Sorts(),
		NullPrimaryDateSkip: "recorded_at is required (FR-033): every measurement set names when it " +
			"was recorded, so there is no undated set to construct",
		CascadeSkip: "measurements' cascade-on-patient-delete is asserted against the real migrated schema " +
			"by internal/store/migrations/assertions_test.go's TestTheCascadeMatrixIsExactlyWhatDataModelDeclares; " +
			"this harness has no patient to delete without reaching past records.Service into the store directly",
	})
}

// The (patient, recorded_at) index the list ordering relies on actually
// exists on the migrated schema.
func TestThePatientRecordedAtIndexExists(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	collection, err := app.FindCollectionByNameOrId(kind.Vitals.Collection())
	require.NoError(t, err)

	var found bool

	for _, index := range collection.Indexes {
		if strings.Contains(index, "patient") && strings.Contains(index, "recorded_at") {
			found = true

			break
		}
	}

	require.True(t, found, "no index over (patient, recorded_at) was found on %s", kind.Vitals.Collection())
}
