package symptom_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	service "medikube/internal/service/symptom"
	"medikube/internal/service/symptom/symptomtest"
	"medikube/internal/store"
	pbsymptom "medikube/internal/store/symptom"

	// Registers the migrations this package's collection needs.
	_ "medikube/internal/store/migrations"
)

const symptomOwnerEmail = "symptom-owner@example.test"

// testCodec is the identity boundary a store-level contract run needs: no
// unit conversion (research D-15 has nothing to convert here), and every
// fixture body is the domain entity itself rather than a wire DTO.
type testCodec struct{}

func (testCodec) Summary(s clinical.Symptom) any { return s }
func (testCodec) Detail(s clinical.Symptom) any  { return s }

func (testCodec) Draft(body any) (clinical.Symptom, error) {
	draft, ok := body.(clinical.Symptom)
	if !ok {
		return clinical.Symptom{}, errors.New("the fixture body is not a clinical.Symptom")
	}

	return draft, nil
}

func (testCodec) Patch(body any) (service.Patch, error) {
	patch, ok := body.(*service.Patch)
	if !ok {
		return service.Patch{}, errors.New("the fixture body is not a *service.Patch")
	}

	return *patch, nil
}

func newSymptomHarness(t *testing.T) recordstest.RepositoryHarness {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbsymptom.New(app, codec)
	require.NoError(t, err)

	owner := symptomSeedAccount(t, app, symptomOwnerEmail)
	patientID := symptomSeedPatient(t, app, owner)

	svc, err := service.New(repo, symptomtest.Authorizer{OwnerID: owner})
	require.NoError(t, err)

	adapter, err := service.NewAdapter(svc, testCodec{})
	require.NoError(t, err)

	var _ records.Service = adapter

	return recordstest.RepositoryHarness{
		Service:   adapter,
		Owner:     access.Actor{UserID: owner, RequestID: "req-1"},
		PatientID: patientID,
		Stranger:  access.Actor{UserID: "somebody-else", RequestID: "req-2"},
	}
}

func symptomSeedAccount(t *testing.T, app core.App, email string) string {
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

func symptomSeedPatient(t *testing.T, app core.App, ownerID string) string {
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

func symptomFixture(name string, occurredAt clinical.Instant) func(patientID string) any {
	return func(patientID string) any {
		return clinical.Symptom{
			PatientID:  patientID,
			Name:       name,
			Severity:   clinical.SeverityModerate,
			OccurredAt: occurredAt,
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

			return newSymptomHarness(t)
		},
		Fixture: recordstest.Fixture{
			Minimal: symptomFixture("Headache", now),
			Full:    symptomFixture("Headache", now),
		},
		NewPatch: func() any { return &service.Patch{} },
		Sort:     service.Sorts(),
		NullPrimaryDateSkip: "occurred_at is required (FR-029): every symptom episode names when it " +
			"happened, so there is no undated episode to construct",
		CascadeSkip: "symptom's cascade-on-patient-delete is asserted against the real migrated schema " +
			"by internal/store/migrations/assertions_test.go's TestTheCascadeMatrixIsExactlyWhatDataModelDeclares; " +
			"this harness has no patient to delete without reaching past records.Service into the store directly",
	})
}

// T087's other half: the (patient, name, occurred_at) index the aggregate
// query relies on actually exists on the migrated schema and is the one
// SQLite's own planner picks for the correlated GROUP BY.
func TestThePatientNameOccurredAtIndexExistsAndIsUsedByTheAggregate(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	collection, err := app.FindCollectionByNameOrId(kind.Symptom.Collection())
	require.NoError(t, err)

	var indexName string

	for _, index := range collection.Indexes {
		if strings.Contains(index, "patient") && strings.Contains(strings.ToLower(index), "name") {
			indexName = index

			break
		}
	}

	require.NotEmpty(t, indexName, "no index over (patient, name) was found on %s", kind.Symptom.Collection())

	owner := symptomSeedAccount(t, app, "aggregate-index@example.test")
	patientID := symptomSeedPatient(t, app, owner)

	var plan []struct {
		Detail string `db:"detail"`
	}

	q := "EXPLAIN QUERY PLAN SELECT LOWER(name) AS agg_key, COUNT(*) AS agg_count, MAX(occurred_at) AS agg_last" +
		" FROM " + kind.Symptom.Collection() +
		" WHERE patient = {:patient} GROUP BY LOWER(name)"

	require.NoError(t, app.DB().NewQuery(q).Bind(map[string]any{"patient": patientID}).All(&plan))

	var usesIndex bool

	for _, row := range plan {
		if strings.Contains(row.Detail, "idx_"+kind.Symptom.Collection()) {
			usesIndex = true
		}
	}

	assert.Truef(t, usesIndex, "the aggregate query's plan does not name the patient/name index: %v", plan)
}
