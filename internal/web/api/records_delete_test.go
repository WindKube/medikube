package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/web"
)

// T145, FR-028. A deletion removes the row outright: no deleted_at, no
// tombstone, no filtered-out survivor. Soft delete in MediKube is files only,
// and a record collection carrying a deletion column would be a schema-level
// contradiction of that (data-model §2).
//
// Every assertion below is against STORED data. Asserting against the API is
// how a soft delete passes: the row is still there and the list is filtering it
// out, which reads identically from outside and is a different application.

func TestADeletionRemovesTheRowFromStoredData(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	target := testsupport.SingleDayMedicationID

	before := caller.get(recordURL(target))
	require.Equal(t, http.StatusOK, before.Status, before.Body)

	answer := caller.delete(recordURL(target), before.etag(t))

	require.Equal(t, http.StatusNoContent, answer.Status, answer.Body)
	assert.Empty(t, answer.Body, "a 204 carries no body")

	_, err := caller.stored(target)
	require.Error(t, err, "the row is still in the database, so the deletion was a filter and not a deletion")

	assert.Equal(t, testsupport.AccountAMedicationCount-1, storedCountOf(t, caller, testsupport.AccountAID))

	// And nothing else moved: a deletion that cascaded or filtered would show
	// up here and nowhere else.
	assert.Equal(t, testsupport.AccountBMedicationCount, storedCountOf(t, caller, testsupport.AccountBID))
}

// TestADeletedRecordIsGoneFromEveryReadingOfIt is the second half: a survivor
// the list filters and the detail still serves would be invisible here and
// visible to a later phase's report.
func TestADeletedRecordIsGoneFromEveryReadingOfIt(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	target := testsupport.SingleDayMedicationID

	before := caller.get(recordURL(target))
	require.Equal(t, http.StatusOK, before.Status, before.Body)
	require.Equal(t, http.StatusNoContent, caller.delete(recordURL(target), before.etag(t)).Status)

	assert.Equal(t, http.StatusNotFound, caller.get(recordURL(target)).Status)

	listed := caller.get(collectionURL() + "?limit=100")
	require.Equal(t, http.StatusOK, listed.Status, listed.Body)
	assert.NotContains(t, idsOf(listed.list(t)), target)
	assert.Len(t, listed.list(t).Items, testsupport.AccountAMedicationCount-1)

	// The second place with it open. contracts/records.md gives this edge case
	// to this operation: the next action answers 404 and the page turns that
	// into a message, not a failure page.
	assert.Equal(t, http.StatusNotFound, caller.delete(recordURL(target), before.etag(t)).Status)
}

// TestTheRecordCollectionHasNoDeletionColumn is the schema half, and it is what
// makes the rule structural rather than a property of today's handler. A column
// nothing writes today is a column somebody starts filtering on tomorrow.
func TestTheRecordCollectionHasNoDeletionColumn(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	collection, err := caller.app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	for _, field := range collection.Fields {
		name := strings.ToLower(field.GetName())

		for _, tombstone := range []string{"deleted", "archived", "removed", "trashed", "discarded"} {
			assert.NotContainsf(t, name, tombstone,
				"the collection carries %q, which is a soft delete waiting for somebody to filter on it (FR-028)",
				field.GetName())
		}
	}
}

// TestADeletionIsRefusedWithoutAPreconditionAndStoresNothing keeps the two
// halves of FR-026 and FR-028 together: the row survives a refused deletion,
// which is the assertion a 422 alone does not make.
func TestADeletionIsRefusedWithoutAPreconditionAndStoresNothing(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	target := testsupport.SingleDayMedicationID

	answer := caller.do(http.MethodDelete, recordURL(target), "", nil)

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	assert.Contains(t, answer.envelope(t).fieldCodes(), [2]string{web.IfMatchHeader, "required"})

	_, err := caller.stored(target)
	assert.NoError(t, err, "a refused deletion removed the row")
}
