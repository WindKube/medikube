package familymember_test

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	service "medikube/internal/service/familymember"
	"medikube/internal/service/familymember/familymembertest"
	"medikube/internal/store"
	pbfamilymember "medikube/internal/store/familymember"

	// Registers the migrations this package's collection needs.
	_ "medikube/internal/store/migrations"
)

const familyOwnerEmail = "family-owner@example.test"

type testCodec struct{}

func (testCodec) Summary(m clinical.FamilyMember) any { return m }
func (testCodec) Detail(m clinical.FamilyMember) any  { return m }

func (testCodec) Draft(body any) (clinical.FamilyMember, error) {
	draft, ok := body.(clinical.FamilyMember)
	if !ok {
		return clinical.FamilyMember{}, errors.New("the fixture body is not a clinical.FamilyMember")
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

func newFamilyMemberHarness(t *testing.T) recordstest.RepositoryHarness {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbfamilymember.New(app, codec)
	require.NoError(t, err)

	owner := familySeedAccount(t, app, familyOwnerEmail)
	patientID := familySeedPatient(t, app, owner)

	svc, err := service.New(repo, familymembertest.NewAuthorizer(owner))
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

func familySeedAccount(t *testing.T, app core.App, email string) string {
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

func familySeedPatient(t *testing.T, app core.App, ownerID string) string {
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

func familyMemberFixture(name string) func(patientID string) any {
	return func(patientID string) any {
		return clinical.FamilyMember{
			PatientID:    patientID,
			Name:         name,
			Relationship: clinical.FamilyRelationshipMother,
		}
	}
}

// T192: the shared storage-tier contract, run against the real PocketBase
// repository through a bare adapter — no HTTP, no wire DTOs — plus a
// conditions JSON round trip against a real instance.
func TestThePocketBaseRepositoryPassesTheSharedRepositoryContract(t *testing.T) {
	t.Parallel()

	recordstest.RunRepositoryContract(t, recordstest.RepositoryContractOptions{
		NewHarness: func(t *testing.T) recordstest.RepositoryHarness {
			t.Helper()

			return newFamilyMemberHarness(t)
		},
		Fixture: recordstest.Fixture{
			Minimal: familyMemberFixture("Nadia Okonkwo"),
			Full:    familyMemberFixture("Nadia Okonkwo"),
		},
		NewPatch: func() any { return &service.Patch{} },
		Sort:     service.Sorts(),
		NullPrimaryDateSkip: "family_members has no primary date (data-model §4.13): a relative is not " +
			"an event with an occurred_on to leave unset",
		CascadeSkip: "family_member's cascade-on-patient-delete is asserted against the real migrated schema " +
			"by internal/store/migrations/assertions_test.go's TestTheCascadeMatrixIsExactlyWhatDataModelDeclares; " +
			"this harness has no patient to delete without reaching past records.Service into the store directly",
	})
}

func TestConditionsRoundTripAsJSONAgainstARealInstance(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbfamilymember.New(app, codec)
	require.NoError(t, err)

	owner := familySeedAccount(t, app, "family-conditions@example.test")
	patientID := familySeedPatient(t, app, owner)

	age := 62

	created, err := repo.Create(t.Context(), clinical.FamilyMember{
		PatientID: patientID, Name: "Nadia Okonkwo", Relationship: clinical.FamilyRelationshipGrandmother,
		Conditions: []clinical.FamilyCondition{
			{Name: "Breast cancer", DiagnosedAge: &age, Severity: clinical.SeveritySevere, Status: clinical.ConditionStatusResolved, Notes: "double mastectomy"},
		},
	})
	require.NoError(t, err)
	require.Len(t, created.Conditions, 1)
	assert.Equal(t, "Breast cancer", created.Conditions[0].Name)
	assert.Equal(t, 62, *created.Conditions[0].DiagnosedAge)

	found, err := repo.Get(t.Context(), created.ID)
	require.NoError(t, err)
	require.Len(t, found.Conditions, 1)
	assert.Equal(t, "double mastectomy", found.Conditions[0].Notes)

	// A relative with no conditions round-trips as an empty slice, never nil,
	// so the DTO layer's "[] never null" contract has something real to read.
	withoutConditions, err := repo.Create(t.Context(), clinical.FamilyMember{
		PatientID: patientID, Name: "Uncle Femi", Relationship: clinical.FamilyRelationshipUncle,
	})
	require.NoError(t, err)
	assert.Empty(t, withoutConditions.Conditions)
}
