package api_test

import (
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
)

// T195, FR-013. `MeCounts` is what the danger zone reads so the deletion
// confirmation can state what will be destroyed rather than asking the person
// to take it on trust. A count that included somebody else's rows would say the
// wrong number to the one person entitled to an exact one — and would be a
// disclosure about a stranger's data volume in the same breath.
//
// There is no id parameter to get wrong here, which is the point: the count is
// resolved from the ACTOR and from nothing else, through the same authorization
// checkpoint every record read passes. So the assertion that matters is that
// the same instance answers three different accounts differently.

func TestTheAccountCountsItsOwnRecordsAndNobodyElses(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	tests := []struct {
		name  string
		email string
		want  int
	}{
		{name: "an account with a full list", email: testsupport.AccountAEmail, want: testsupport.AccountAMedicationCount},
		{name: "an account with a few", email: testsupport.AccountBEmail, want: testsupport.AccountBMedicationCount},
		{name: "an account with none", email: testsupport.AccountCEmail, want: testsupport.AccountCMedicationCount},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			answer := instance.as(test.email).get(meURL)

			require.Equal(t, http.StatusOK, answer.Status, answer.Body)
			assert.Equal(t, test.want, answer.me(t).Counts[kind.Medication.Segment()])
		})
	}

	// The three answers are not the same number, so a count that had quietly
	// become "every row on the instance" could not pass the table above.
	assert.NotEqual(t, testsupport.AccountAMedicationCount, testsupport.AccountBMedicationCount,
		"the fixture gives two accounts the same count, so this file cannot tell scoped from unscoped")
}

// The count follows the account's own writes and nobody else's.
func TestOneAccountsNewRecordDoesNotChangeAnothersCount(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	amara := instance.as(testsupport.AccountAEmail)
	boris := instance.as(testsupport.AccountBEmail)

	created := amara.post(collectionURL(), everyField)
	require.Equal(t, http.StatusCreated, created.Status, created.Body)

	assert.Equal(t, testsupport.AccountAMedicationCount+1, amara.get(meURL).me(t).Counts[kind.Medication.Segment()])
	assert.Equal(t, testsupport.AccountBMedicationCount, boris.get(meURL).me(t).Counts[kind.Medication.Segment()],
		"another account's create moved this account's count")

	// And back down again, so the count is read rather than remembered.
	removed := amara.delete(created.Header.Get("Location"), created.etag(t))
	require.Equal(t, http.StatusNoContent, removed.Status, removed.Body)

	assert.Equal(t, testsupport.AccountAMedicationCount, amara.get(meURL).me(t).Counts[kind.Medication.Segment()])
}

// The count is on every representation of the account, not only on getMe: the
// settings page patches its own region with the result of a change (FR-047),
// and a danger zone whose number went stale on the first preference change
// would state the wrong figure at the one moment it matters.
func TestTheCountIsOnEveryAnswerThatCarriesTheAccount(t *testing.T) {
	t.Parallel()

	instance := newRig(t)
	signedIn := instance.as(testsupport.AccountAEmail)

	answers := map[string]response{
		"getMe":          signedIn.get(meURL),
		"updateMe":       signedIn.do(http.MethodPatch, meURL, body("theme", quoted("dark")), nil),
		"refreshSession": signedIn.post(refreshURL, ""),
	}

	for name, answer := range answers {
		require.Equalf(t, http.StatusOK, answer.Status, "%s: %s", name, answer.Body)
	}

	assert.Equal(t, testsupport.AccountAMedicationCount, answers["getMe"].me(t).Counts[kind.Medication.Segment()])
	assert.Equal(t, testsupport.AccountAMedicationCount, answers["updateMe"].me(t).Counts[kind.Medication.Segment()])
	assert.Equal(t, testsupport.AccountAMedicationCount,
		answers["refreshSession"].session(t).User.Counts[kind.Medication.Segment()])
}

// The counts object names every kind this build serves, and names it by the
// kind's own path segment.
//
// A map's missing key and a map's zero are the same value to a reader, so
// without this the "an account with none" case above would pass just as well
// against a counter that had silently stopped counting. It is also what makes
// the object grow with the kind registry rather than with somebody's memory:
// phases 002 through 006 add five kinds, and each one is a key here on the day
// it is registered.
func TestTheCountsObjectNamesEveryKindThisBuildServes(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	served := instance.instance.Records.Segments()
	require.NotEmpty(t, served, "the build serves no kinds, so the assertion below is about nothing")

	counts := instance.as(testsupport.AccountAEmail).get(meURL).me(t).Counts

	assert.ElementsMatch(t, served, slices.Collect(maps.Keys(counts)),
		"the counts object and the kinds this build serves have drifted apart")
	assert.Contains(t, counts, kind.Medication.Segment())
}
