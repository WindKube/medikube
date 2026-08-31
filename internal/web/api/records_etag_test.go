package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/testsupport"
	"medikube/internal/web"
)

// T146, FR-026 and US1-9. If-Match is REQUIRED on both writes, not merely
// honoured: an optional precondition is a precondition nobody sends. A stale
// one answers 412 carrying the representation the server currently holds, so
// "the current values are shown so they can decide what to do" is a property of
// the response rather than a second request the page has to remember to make.

// staleVersion is a version this server never issued, in the shape it issues
// them. A malformed one would be refused by the parser and would prove nothing
// about the comparison.
const staleVersion = `"0123456789abcdef"`

func TestBothWritesRefuseAMissingPrecondition(t *testing.T) {
	t.Parallel()

	target := recordURL(testsupport.SingleDayMedicationID)

	cases := []struct {
		name   string
		method string
		body   string
	}{
		{"a change", http.MethodPatch, `{"dosage":"1 g"}`},
		{"a deletion", http.MethodDelete, ""},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			caller := newCaller(t)

			answer := caller.do(one.method, target, one.body, nil)

			require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
			assert.Equal(t, [][2]string{{web.IfMatchHeader, domain.CodeRequired}}, answer.envelope(t).fieldCodes())
		})
	}
}

// TestOnlyAnEntityTagThisServerIssuedIsAccepted covers the preconditions that
// are present and wrong in shape. `*` is the interesting one: it is a
// precondition that always passes, which is exactly the overwrite FR-026 exists
// to prevent.
func TestOnlyAnEntityTagThisServerIssuedIsAccepted(t *testing.T) {
	t.Parallel()

	target := recordURL(testsupport.SingleDayMedicationID)

	for _, supplied := range []string{`*`, `W/"0123456789abcdef"`, `0123456789abcdef`, `"a", "b"`} {
		t.Run(supplied, func(t *testing.T) {
			t.Parallel()

			caller := newCaller(t)

			answer := caller.patch(target, `{"dosage":"1 g"}`, supplied)

			require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
			assert.Equal(t, [][2]string{{web.IfMatchHeader, domain.CodeInvalidValue}}, answer.envelope(t).fieldCodes())

			stored, err := caller.stored(testsupport.SingleDayMedicationID)
			require.NoError(t, err)
			assert.NotEqual(t, "1 g", stored.GetString("dosage"), "a refused precondition still wrote")
		})
	}
}

func TestAStaleChangeAnswers412CarryingTheCurrentRepresentation(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	target := recordURL(testsupport.SingleDayMedicationID)

	current := caller.get(target)
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	answer := caller.patch(target, `{"dosage":"1 g"}`, staleVersion)

	require.Equal(t, http.StatusPreconditionFailed, answer.Status, answer.Body)

	envelope := answer.envelope(t)
	assert.Equal(t, web.CodeVersionMismatch, envelope.Error.Code)

	// The representation, not a pointer to it. A 412 that said only "this
	// changed" would leave the page to fetch what it changed to, which is the
	// second request FR-026 exists to remove.
	assert.Equal(t, current.medication(t), envelope.Current)

	// And the version to retry with, so the retry needs nothing else.
	assert.Equal(t, current.Header.Get("ETag"), answer.Header.Get("ETag"))

	stored, err := caller.stored(testsupport.SingleDayMedicationID)
	require.NoError(t, err)
	assert.NotEqual(t, "1 g", stored.GetString("dosage"), "the stale change was applied")
}

func TestAStaleDeletionAnswers412AndKeepsTheRow(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	target := recordURL(testsupport.SingleDayMedicationID)

	current := caller.get(target)
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	answer := caller.delete(target, staleVersion)

	require.Equal(t, http.StatusPreconditionFailed, answer.Status, answer.Body)
	assert.Equal(t, current.medication(t), answer.envelope(t).Current)

	_, err := caller.stored(testsupport.SingleDayMedicationID)
	assert.NoError(t, err, "a refused deletion removed the row")
}

// TestAVersionStopsBeingCurrentAfterAChange is the mechanism the two tests
// above assume: a write moves the version, so the tag the caller is holding is
// the one that is now stale.
func TestAVersionStopsBeingCurrentAfterAChange(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	target := recordURL(testsupport.SingleDayMedicationID)

	first := caller.get(target)
	require.Equal(t, http.StatusOK, first.Status, first.Body)

	changed := caller.patch(target, `{"dosage":"1 g"}`, first.etag(t))
	require.Equal(t, http.StatusOK, changed.Status, changed.Body)

	assert.NotEqual(t, first.etag(t), changed.etag(t), "the version did not move, so a stale write cannot be detected")
	assert.Equal(t, "1 g", changed.medication(t).Dosage)

	// Only the supplied member changed.
	assert.Equal(t, first.medication(t).Name, changed.medication(t).Name)
	assert.Equal(t, first.medication(t).StartedOn, changed.medication(t).StartedOn)

	// The tag the caller was holding is now the stale one, and the same
	// request a second time is refused.
	replay := caller.patch(target, `{"dosage":"2 g"}`, first.etag(t))
	assert.Equal(t, http.StatusPreconditionFailed, replay.Status, replay.Body)
}

// TestAnExplicitNullClearsADateAndAnAbsentMemberDoesNot is the distinction
// web.Optional exists for: contracts/records.md's `**string` cannot carry it,
// and without it a change to the dose would silently clear both dates.
func TestAnExplicitNullClearsADateAndAnAbsentMemberDoesNot(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	target := recordURL(testsupport.SingleDayMedicationID)

	before := caller.get(target)
	require.Equal(t, http.StatusOK, before.Status, before.Body)
	require.NotNil(t, before.medication(t).StartedOn, "the fixture row has no start date to clear")

	untouched := caller.patch(target, `{"dosage":"1 g"}`, before.etag(t))
	require.Equal(t, http.StatusOK, untouched.Status, untouched.Body)
	assert.Equal(t, before.medication(t).StartedOn, untouched.medication(t).StartedOn,
		"a change that said nothing about the start date cleared it")

	// The end date is cleared first: clearing the start while an end is stored
	// is a legal state, and clearing both is what the fixture allows in either
	// order.
	clearedEnd := caller.patch(target, `{"ended_on":null}`, untouched.etag(t))
	require.Equal(t, http.StatusOK, clearedEnd.Status, clearedEnd.Body)
	assert.Nil(t, clearedEnd.medication(t).EndedOn, "an explicit null did not clear the date")

	cleared := caller.patch(target, `{"started_on":null}`, clearedEnd.etag(t))
	require.Equal(t, http.StatusOK, cleared.Status, cleared.Body)
	assert.Nil(t, cleared.medication(t).StartedOn, "an explicit null did not clear the date")
}
