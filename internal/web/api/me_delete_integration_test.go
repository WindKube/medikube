package api_test

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	domainaudit "medikube/internal/domain/audit"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	serviceaudit "medikube/internal/service/audit"
	serviceidentity "medikube/internal/service/identity"
	"medikube/internal/service/identity/identitytest"
	"medikube/internal/store"
	pbaudit "medikube/internal/store/audit"
	pbidentity "medikube/internal/store/identity"
	"medikube/internal/testsupport"
)

// T198. What `deleteMe` leaves behind, asserted against STORED DATA rather than
// against the response it answered with.
//
// The operation is driven through the service the handler calls, wired to the
// real repositories against a real instance: the assertions below are about the
// database, and a handler-level round trip would only add a status code between
// the delete and the rows it is supposed to have taken with it. The two counts
// are the requirement (FR-014, SC-012) and they pull in opposite directions —
// one cascade that must fire and one that must not — so a schema change that
// got either wrong would leave a person's medications behind or erase the
// evidence that their account was deleted at all.

// deletionRig is the operation under test with nothing faked below it.
type deletionRig struct {
	app     *tests.TestApp
	service *serviceidentity.Service
}

func newDeletionRig(t *testing.T) deletionRig {
	t.Helper()

	app := testsupport.NewApp(t)

	repo, err := pbidentity.NewRepository(app)
	require.NoError(t, err)

	auth, err := pbidentity.NewAuthenticator(app)
	require.NoError(t, err)

	trail, err := pbaudit.New(app)
	require.NoError(t, err)

	writer, err := serviceaudit.New(trail)
	require.NoError(t, err)

	accounts, err := serviceidentity.New(serviceidentity.Config{
		Repository:    repo,
		Authenticator: auth,
		Mailer:        identitytest.NewMailer(),
		Auditor:       writer,
		Clock:         serviceidentity.SystemClock{},
	})
	require.NoError(t, err)

	return deletionRig{app: app, service: accounts}
}

// medicationsOf is one account's medication rows, counted through the patient
// it now files against (research D-13): medications.owner is gone, so
// "this account's rows" is "rows filed against a patient this account owns".
func (r deletionRig) medicationsOf(t *testing.T, ownerID string) int {
	t.Helper()

	owners, err := store.NewPatientOwners(r.app)
	require.NoError(t, err)

	patientIDs, err := owners.PatientsOfOwner(t.Context(), ownerID)
	require.NoError(t, err)

	if len(patientIDs) == 0 {
		return 0
	}

	ids := make([]interface{}, len(patientIDs))
	for i, id := range patientIDs {
		ids[i] = id
	}

	var total int

	require.NoError(t, r.app.DB().
		Select("count(*)").
		From(kind.Medication.Collection()).
		Where(dbx.HashExp{"patient": ids}).
		Row(&total))

	return total
}

func (r deletionRig) accountsWithID(t *testing.T, id string) int {
	t.Helper()

	var total int

	require.NoError(t, r.app.DB().
		Select("count(*)").
		From(store.AccountCollection).
		Where(dbx.HashExp{"id": id}).
		Row(&total))

	return total
}

// TestDeletingAnAccountTakesItsMedicationsAndLeavesTheTrail.
func TestDeletingAnAccountTakesItsMedicationsAndLeavesTheTrail(t *testing.T) {
	t.Parallel()

	rig := newDeletionRig(t)

	// Preconditions, with require rather than assert: every count below is
	// meaningless if the fixture did not hold what it says it holds, and a zero
	// that was already zero is the shape of a test that cannot fail.
	require.Equal(t, testsupport.AccountAMedicationCount, rig.medicationsOf(t, testsupport.AccountAID))
	require.Equal(t, testsupport.AccountBMedicationCount, rig.medicationsOf(t, testsupport.AccountBID))
	require.Positive(t, testsupport.AccountAMedicationCount,
		"the account being deleted has nothing to cascade, so the cascade assertion would pass on an empty set")

	actor := access.Actor{
		UserID:    testsupport.AccountAID,
		Role:      domainidentity.RoleUser,
		RequestID: "req-me-delete-integration",
	}

	require.NoError(t, rig.service.DeleteAccount(
		t.Context(),
		actor,
		testsupport.Password,
		domainidentity.DeleteConfirmationPhrase,
	))

	// The account itself.
	assert.Equal(t, 0, rig.accountsWithID(t, testsupport.AccountAID), "the account survived its own deletion")

	// FR-014 / SC-012: the medications are gone from stored data. Counted in
	// the database, because an API answering 204 says nothing about whether the
	// rows went with it — `medications.owner` is Required and CascadeDelete,
	// and this is what proves the cascade actually fires.
	assert.Equal(t, 0, rig.medicationsOf(t, testsupport.AccountAID),
		"a deleted account's medication rows are still stored")

	// And nobody else's went with them.
	assert.Equal(t, testsupport.AccountBMedicationCount, rig.medicationsOf(t, testsupport.AccountBID),
		"deleting one account took another account's medication rows")
}

// TestTheDeletionRowOutlivesTheAccountItRecords is the opposite cascade, and it
// is the one that is easy to assert wrongly.
//
// `audit_events.actor` is CascadeDelete:false AND Required:false, so PocketBase
// does not leave a dangling id: deleteRefRecords UNSETS the relation and
// re-saves the row (core/record_model.go:1592). A query on `actor` therefore
// finds NOTHING after the delete — measured against v0.40.1 — and `actor_kind`
// is the only surviving evidence that a person rather than the system did it
// (research D-22, defect D17).
//
// The row is keyed on `target_id` AND `action`, and the surviving row's actor is
// asserted to be empty. A count alone would pass on a row about somebody else.
func TestTheDeletionRowOutlivesTheAccountItRecords(t *testing.T) {
	t.Parallel()

	rig := newDeletionRig(t)

	actor := access.Actor{
		UserID:    testsupport.AccountAID,
		Role:      domainidentity.RoleUser,
		RequestID: "req-me-delete-integration",
	}

	require.NoError(t, rig.service.DeleteAccount(
		t.Context(),
		actor,
		testsupport.Password,
		domainidentity.DeleteConfirmationPhrase,
	))

	var rows []struct {
		Actor     string `db:"actor"`
		ActorKind string `db:"actor_kind"`
		RequestID string `db:"request_id"`
	}

	require.NoError(t, rig.app.DB().
		Select("actor", "actor_kind", "request_id").
		From(store.AuditCollection).
		Where(dbx.HashExp{
			"target_id": testsupport.AccountAID,
			"action":    string(domainaudit.ActionAccountDelete),
		}).
		All(&rows))

	require.Len(t, rows, 1,
		"the account_delete row did not outlive the account, so nothing records that the deletion happened")

	assert.Empty(t, rows[0].Actor,
		"the actor still points at the deleted account: PocketBase unsets it rather than leaving a dangling id, and a test that expects one is asserting behaviour this version does not have")
	assert.Equal(t, string(domainaudit.ActorKindUser), rows[0].ActorKind,
		"actor_kind is the only surviving evidence that a person did this")
	assert.Equal(t, actor.RequestID, rows[0].RequestID)

	// The other half of the same measurement, pinned so that the assertion
	// above cannot quietly be rewritten to key on `actor` again.
	var byActor int

	require.NoError(t, rig.app.DB().
		Select("count(*)").
		From(store.AuditCollection).
		Where(dbx.HashExp{"actor": testsupport.AccountAID}).
		Row(&byActor))

	assert.Equal(t, 0, byActor,
		"a query on `actor` found rows for a deleted account, which is not how a non-cascading, non-required relation behaves")
}

// TestARefusedDeletionLeavesEverythingWhereItWas. The confirmation phrase and
// the re-entered password are refusals, not warnings (FR-013): a deletion that
// half-happened would take the medications with it.
func TestARefusedDeletionLeavesEverythingWhereItWas(t *testing.T) {
	t.Parallel()

	refusals := []struct {
		name         string
		password     string
		confirmation string
	}{
		{
			name:         "the phrase in the wrong case",
			password:     testsupport.Password,
			confirmation: "delete my account",
		},
		{
			name:         "the phrase with something around it",
			password:     testsupport.Password,
			confirmation: " " + domainidentity.DeleteConfirmationPhrase + " ",
		},
		{
			name:         "no phrase at all",
			password:     testsupport.Password,
			confirmation: "",
		},
		{
			name:         "the phrase and somebody else's password",
			password:     "not-the-password-on-the-account",
			confirmation: domainidentity.DeleteConfirmationPhrase,
		},
	}

	for _, refusal := range refusals {
		t.Run(refusal.name, func(t *testing.T) {
			t.Parallel()

			rig := newDeletionRig(t)

			actor := access.Actor{
				UserID:    testsupport.AccountAID,
				Role:      domainidentity.RoleUser,
				RequestID: "req-me-delete-refused",
			}

			require.Error(t, rig.service.DeleteAccount(t.Context(), actor, refusal.password, refusal.confirmation))

			assert.Equal(t, 1, rig.accountsWithID(t, testsupport.AccountAID), "a refused deletion deleted the account")
			assert.Equal(t, testsupport.AccountAMedicationCount, rig.medicationsOf(t, testsupport.AccountAID),
				"a refused deletion took the medication rows anyway")

			var deletions int

			require.NoError(t, rig.app.DB().
				Select("count(*)").
				From(store.AuditCollection).
				Where(dbx.HashExp{
					"target_id": testsupport.AccountAID,
					"action":    string(domainaudit.ActionAccountDelete),
				}).
				Row(&deletions))

			assert.Equal(t, 0, deletions, "a refused deletion left a row saying the account was deleted")
		})
	}
}

// A deletion publishes one record-stream event per medication and none for the
// account, and that is load-bearing rather than incidental: internal/platform/pb
// filters the stream by collection name, so the users delete is dropped while
// the twelve medication deletes go out to whoever is still listening. Asserted
// here because the count comes from the cascade — nothing in the hook says
// twelve — so a change to either side moves it.
func TestTheCascadePublishesOneEventPerMedicationAndNoneForTheAccount(t *testing.T) {
	t.Parallel()

	rig := newDeletionRig(t)

	var deleted []string

	rig.app.OnRecordAfterDeleteSuccess().BindFunc(func(e *core.RecordEvent) error {
		deleted = append(deleted, e.Record.Collection().Name)

		return e.Next()
	})

	require.NoError(t, rig.service.DeleteAccount(
		t.Context(),
		access.Actor{
			UserID:    testsupport.AccountAID,
			Role:      domainidentity.RoleUser,
			RequestID: "req-me-delete-stream",
		},
		testsupport.Password,
		domainidentity.DeleteConfirmationPhrase,
	))

	owned := map[string]bool{"patients": true, "practitioners": true, "facilities": true, "search_index": true}

	var medications, accounts int

	for _, collection := range deleted {
		switch collection {
		case kind.Medication.Collection():
			medications++
		case store.AccountCollection:
			accounts++
		default:
			assert.True(t, owned[collection], "a %s row was deleted, and the account does not own that collection", collection)
		}
	}

	assert.Equal(t, testsupport.AccountAMedicationCount, medications,
		"the cascade did not delete one medication at a time, so the record stream tells nobody their list changed")
	assert.Equal(t, 1, accounts)
}
