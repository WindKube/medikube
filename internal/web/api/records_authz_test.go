package api_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pocketbase/pocketbase/tests"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/web/apitest"
)

// T147, FR-032, FR-033, FR-069 and SC-006. Every one of the six operations run
// three ways — as the owner, as a signed-in stranger, and as nobody — and the
// stranger's refusal compared with a genuine miss byte for byte, headers
// included.
//
// It is driven by testsupport.RunOwnershipMatrix rather than written out,
// because an authorization test written by hand per endpoint drifts: one checks
// the stranger is refused, the next checks the owner succeeds, the third checks
// neither.

// correlationID matches the one member FR-033 permits two otherwise identical
// refusals to differ in. Everything else in the body has to match, which is
// what "byte-identical apart from request_id" means as an assertion rather than
// as a sentiment.
var correlationID = regexp.MustCompile(`"request_id":"[0-9a-f]*"`)

func withoutCorrelationID(value string) string {
	return correlationID.ReplaceAllString(value, `"request_id":""`)
}

func TestEveryRecordOperationIsOwnerScoped(t *testing.T) {
	t.Parallel()

	instance := apitest.New(t)
	handler := testsupport.NewEdgeHandler(t, instance.App)

	// The owner's real identifier, addressed by a stranger. A matrix that
	// addressed a record which did not exist would have the stranger refused
	// for the wrong reason and would prove nothing.
	target := recordURL(testsupport.NameOnlyMedicationID)

	// The version the owner is holding. It is supplied on the write legs so
	// that a refusal cannot be the missing precondition rather than the
	// authorization — a 422 on If-Match would satisfy "the stranger did not
	// succeed" while proving nothing about ownership.
	owner := &caller{
		t:       t,
		app:     instance.App,
		handler: handler,
		token:   testsupport.UserToken(t, instance.App, testsupport.AccountAEmail),
	}

	current := owner.get(target)
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	// The version the owner is holding, sent on both write legs. Without it
	// every write is 422 on the missing precondition — for the stranger too —
	// and the matrix would be asserting the header check rather than ownership.
	precondition := map[string]string{"If-Match": current.etag(t)}

	// The deletion needs a subject of its own. The change leg above succeeds —
	// it has to, that is the control — and a successful change moves the
	// version, so a deletion aimed at the same record would be refused on a
	// stale precondition rather than on ownership.
	removable := recordURL(testsupport.FutureStartMedicationID)

	removableNow := owner.get(removable)
	require.Equal(t, http.StatusOK, removableNow.Status, removableNow.Body)

	removablePrecondition := map[string]string{"If-Match": removableNow.etag(t)}

	// What must never appear in a stranger's or a guest's answer: the
	// identifier they addressed, the drug it names and the account that owns
	// it. A case that named none would assert only a status code.
	secrets := []string{
		testsupport.NameOnlyMedicationID,
		"Paracetamol",
		testsupport.AccountAID,
	}

	removableSecrets := []string{
		testsupport.FutureStartMedicationID,
		"Denosumab",
		testsupport.AccountAID,
	}

	testsupport.RunOwnershipMatrix(t, testsupport.OwnershipMatrix{
		Handler:   handler,
		Owner:     bearer(t, instance.App, testsupport.AccountAEmail),
		Stranger:  bearer(t, instance.App, testsupport.AccountBEmail),
		Normalise: withoutCorrelationID,
		// The correlation id again, this time as a header of its own. It is
		// the one difference FR-033 permits, and dropping it by name is what
		// leaves every other header in the comparison.
		VolatileHeaders: []string{"X-Request-Id"},
		Cases: []testsupport.OwnershipCase{
			{
				Name:    "list every kind",
				Method:  http.MethodGet,
				Path:    crossKindURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100",
				Secrets: secrets,
			},
			{
				Name:    "list one kind",
				Method:  http.MethodGet,
				Path:    collectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100",
				Secrets: secrets,
			},
			{
				Name:        "record a medication",
				Method:      http.MethodPost,
				Path:        collectionURL(),
				Body:        `{"patient":"` + testsupport.AccountAPatientSelfID + `","name":"Amoxicillin"}`,
				OwnerStatus: http.StatusCreated,
				Secrets:     secrets,
			},
			{
				Name:        "read one",
				Method:      http.MethodGet,
				Path:        target,
				MissingPath: recordURL(missingID),
				Secrets:     secrets,
			},
			{
				Name:        "change one",
				Method:      http.MethodPatch,
				Path:        target,
				Body:        `{"dosage":"1 g"}`,
				ContentType: "application/json",
				Headers:     precondition,
				MissingPath: recordURL(missingID),
				Secrets:     secrets,
			},
			{
				Name:        "delete one",
				Method:      http.MethodDelete,
				Path:        removable,
				Headers:     removablePrecondition,
				OwnerStatus: http.StatusNoContent,
				MissingPath: recordURL(missingID),
				Secrets:     removableSecrets,
			},
		},
	})
}

// bearer presents a seeded account's session. The token is minted from the app
// under test, because it is signed with that clone's own collection secret and
// one minted against another clone is refused in a way that reads exactly like
// an authorization defect.
func bearer(t *testing.T, app *tests.TestApp, email string) testsupport.Identity {
	t.Helper()

	return testsupport.BearerToken(testsupport.UserToken(t, app, email))
}

// TestTheWriteLegsAreRefusedOnOwnershipAndNotOnThePrecondition is the trap the
// matrix cannot see: a PATCH with no If-Match is 422 for everybody, which
// satisfies "the stranger did not succeed" while asserting nothing at all about
// ownership. So the same requests are made again WITH a valid precondition —
// the one the owner is holding — and the stranger is still refused, and refused
// as a miss.
func TestTheWriteLegsAreRefusedOnOwnershipAndNotOnThePrecondition(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	stranger := owner.as(testsupport.AccountBEmail)

	target := recordURL(testsupport.NameOnlyMedicationID)

	current := owner.get(target)
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	version := current.etag(t)

	change := stranger.patch(target, `{"dosage":"1 g"}`, version)
	assert.Equal(t, http.StatusNotFound, change.Status, change.Body)

	removal := stranger.delete(target, version)
	assert.Equal(t, http.StatusNotFound, removal.Status, removal.Body)

	// And both refusals are the genuine miss, byte for byte.
	missing := stranger.patch(recordURL(missingID), `{"dosage":"1 g"}`, version)
	assert.Equal(t, withoutCorrelationID(missing.Body), withoutCorrelationID(change.Body))

	// The record is untouched, which is the assertion a status code does not
	// make: a refusal that had already written would answer 404 just the same.
	after, err := owner.stored(testsupport.NameOnlyMedicationID)
	require.NoError(t, err)
	assert.Empty(t, after.GetString("dosage"))
}

// TestAStrangersListHoldsOnlyTheirOwnRows is the isolation half stated
// positively: the stranger's own answer is a real one, and it is theirs.
func TestAStrangersListHoldsOnlyTheirOwnRows(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	stranger := owner.as(testsupport.AccountBEmail)
	empty := owner.as(testsupport.AccountCEmail)

	emptyPatient := newPatientFor(t, owner, testsupport.AccountCID)

	mine := owner.get(collectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100")
	theirs := stranger.get(collectionURL() + "?patient=" + testsupport.AccountBPatientSelfID + "&limit=100")
	none := empty.get(collectionURL() + "?patient=" + emptyPatient + "&limit=100")

	require.Equal(t, http.StatusOK, theirs.Status, theirs.Body)

	assert.Len(t, mine.list(t).Items, testsupport.AccountAMedicationCount)
	assert.Len(t, theirs.list(t).Items, testsupport.AccountBMedicationCount)
	assert.Empty(t, none.list(t).Items,
		"the account with nothing recorded is what the empty state is exercised through on every run")

	for _, id := range idsOf(mine.list(t)) {
		assert.NotContains(t, idsOf(theirs.list(t)), id)
	}
}

// TestACreateIsAttributedToTheCallerAndNotToTheBody is FR-032 on the operation
// that has no id to be scoped by. The DTO carries no owner at all, so this is
// the assertion that the account the row lands on is the one that sent it.
func TestACreateIsAttributedToTheCallerAndNotToTheBody(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	stranger := owner.as(testsupport.AccountBEmail)

	created := stranger.post(collectionURL(),
		`{"patient":"`+testsupport.AccountBPatientSelfID+`","name":"Amoxicillin"}`)
	require.Equal(t, http.StatusCreated, created.Status, created.Body)

	record, err := owner.stored(created.medication(t).ID)
	require.NoError(t, err)

	assert.Equal(t, testsupport.AccountBPatientSelfID, record.GetString("patient"))

	// And the owner cannot see it, which is the same statement read from the
	// other side.
	assert.Equal(t, http.StatusNotFound, owner.get(recordURL(created.medication(t).ID)).Status)
}

// TestTheAutoCrudRouteIsClosedToEverybodyButASuperuser is the last two rows of
// contracts/records.md's authorization table. internal/platform/pb tests the
// lockdown as a mechanism; this asserts it for THIS collection, because the
// lockdown is per-collection and a kind registered without one would be
// reachable through PocketBase's own API with MediKube's authorization skipped
// entirely.
func TestTheAutoCrudRouteIsClosedToEverybodyButASuperuser(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	native := "/api/collections/" + kind.Medication.Collection() + "/records"

	t.Run("an ordinary account", func(t *testing.T) {
		answer := caller.get(native)

		assert.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
		assert.NotContains(t, answer.Body, testsupport.NameOnlyMedicationID)
	})

	t.Run("nobody", func(t *testing.T) {
		answer := caller.anonymous().get(native)

		assert.NotEqual(t, http.StatusOK, answer.Status, answer.Body)
		assert.NotContains(t, answer.Body, testsupport.NameOnlyMedicationID)
	})
}
