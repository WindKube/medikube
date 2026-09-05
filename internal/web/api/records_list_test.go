package api_test

import (
	"cmp"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/service/medication"
	"medikube/internal/testsupport"
	"medikube/internal/testsupport/seed"
	"medikube/internal/web"
)

// T142, FR-021 and FR-022. Every published ordering returns the documented
// sequence, the status filter narrows, the search narrows over both name
// columns, and a traversal forwards and then in the reverse ordering returns
// the same set with nothing repeated and nothing skipped.
//
// The expected sequences are computed from the seed by an independent sort
// rather than transcribed. A transcribed list is a list somebody updates to
// match whatever the query started returning; a second implementation of the
// documented rule disagrees when the query is wrong.

// ownedByAccountA is the fixture as the seeder wrote it, which is what the
// query is being checked against.
func ownedByAccountA() []clinical.Medication {
	var owned []clinical.Medication

	for _, one := range seed.Medications() {
		if one.PatientID == testsupport.AccountAPatientSelfID {
			owned = append(owned, one)
		}
	}

	return owned
}

// expectedOrder is the documented ordering, implemented against the entities
// rather than against SQL: the named term, then the identity tiebreaker, which
// is always id DESCENDING and is what makes every one of the six orderings
// stable (contracts/records.md, the index test in internal/store/migrations).
//
// "Rows with a null started_on sort last under both directions" is stated in
// the contract rather than left to the database, and it is stated here too —
// separately — because SQLite sorts the empty string FIRST ascending and would
// otherwise put a medication whose start date was never recorded at the top of
// "most recently started".
func expectedOrder(sort string) []string {
	owned := ownedByAccountA()

	descending := strings.HasPrefix(sort, "-")
	field := strings.TrimPrefix(sort, "-")

	slices.SortStableFunc(owned, func(left, right clinical.Medication) int {
		// "Rows with a null started_on sort last under both directions" is
		// contract, so this comparison sits OUTSIDE the direction flip. Put
		// inside it, the absent rows would lead the ascending list — which is
		// exactly what SQLite does with the empty string and exactly what the
		// contract stops.
		if absent := compareAbsence(field, left, right); absent != 0 {
			return absent
		}

		order := compareOn(field, left, right)

		if descending {
			order = -order
		}

		if order != 0 {
			return order
		}

		// The tiebreaker never flips with the direction.
		return cmp.Compare(right.ID, left.ID)
	})

	ids := make([]string, 0, len(owned))
	for _, one := range owned {
		ids = append(ids, one.ID)
	}

	return ids
}

// compareAbsence answers only when exactly one of the two has no start date.
func compareAbsence(field string, left, right clinical.Medication) int {
	if field != medication.FieldStartedOn || left.StartedOn.IsZero() == right.StartedOn.IsZero() {
		return 0
	}

	if left.StartedOn.IsZero() {
		return 1
	}

	return -1
}

func compareOn(field string, left, right clinical.Medication) int {
	switch field {
	case medication.FieldName:
		return cmp.Compare(left.Name, right.Name)
	case medication.FieldStartedOn:
		return left.StartedOn.Compare(right.StartedOn)
	default:
		// Every seeded row was written in one pass, so `updated` cannot order
		// them and only the tiebreaker can.
		return 0
	}
}

func idsOf(page listDTO) []string {
	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}

	return ids
}

func TestEveryPublishedOrderingReturnsTheDocumentedSequence(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	for _, sort := range publishedSorts() {
		t.Run(sort, func(t *testing.T) {
			answer := caller.get(collectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100&sort=" + url.QueryEscape(sort))
			require.Equal(t, http.StatusOK, answer.Status, answer.Body)

			page := answer.list(t)

			// The property, asserted for all six: the named key is monotonic
			// in the direction asked, an unrecorded start date is last under
			// BOTH directions, and equal keys are broken by the identity
			// descending — which is the tiebreaker every index ends in and the
			// only reason a cursor over this ordering is stable.
			assertOrderedBy(t, page, sort, caller.storedInstants(medication.FieldUpdated))

			// The sequence itself, for the two orderings the fixture fixes. It
			// is deliberately not asserted for `updated`: the seeder writes no
			// timestamp, so the only expectation available would be whatever
			// the database happened to produce.
			if strings.TrimPrefix(sort, "-") != medication.FieldUpdated {
				assert.Equal(t, expectedOrder(sort), idsOf(page),
					"the ordering %q is not the one contracts/records.md publishes", sort)
			}
		})
	}
}

// assertOrderedBy is the ordering rule read off the response rather than off
// the fixture, so it holds for a list the seeder does not fix the values of.
func assertOrderedBy(t *testing.T, page listDTO, sort string, stored map[string]string) {
	t.Helper()

	require.NotEmpty(t, page.Items, "an empty page satisfies every ordering")

	descending := strings.HasPrefix(sort, "-")
	field := strings.TrimPrefix(sort, "-")

	for index := 1; index < len(page.Items); index++ {
		previous, current := page.Items[index-1], page.Items[index]

		previousKey, previousRecorded := sortKeyOf(previous, field, stored)
		currentKey, currentRecorded := sortKeyOf(current, field, stored)

		// Absent last under both directions, stated here rather than left to
		// the database: SQLite sorts the empty string FIRST ascending, which
		// would put a medication whose start date was never recorded at the
		// top of "most recently started".
		if previousRecorded != currentRecorded {
			assert.Falsef(t, currentRecorded && !previousRecorded,
				"%s has no %s and is listed before %s, which has one", previous.ID, field, current.ID)

			continue
		}

		if !previousRecorded {
			assertIdentityTiebreak(t, previous, current, field)

			continue
		}

		switch {
		case previousKey == currentKey:
			assertIdentityTiebreak(t, previous, current, field)
		case descending:
			assert.Greaterf(t, previousKey, currentKey,
				"%s is listed before %s under %q", previous.ID, current.ID, sort)
		default:
			assert.Lessf(t, previousKey, currentKey,
				"%s is listed before %s under %q", previous.ID, current.ID, sort)
		}
	}
}

// assertIdentityTiebreak is the half that makes a cursor over the ordering
// stable. It never flips with the direction: an index ending in `id DESC` is
// read forwards for both, and a tiebreaker that turned around would make one of
// the two orderings a filesort with no boundary a cursor could name.
func assertIdentityTiebreak(t *testing.T, previous, current medicationDTO, field string) {
	t.Helper()

	assert.Greaterf(t, previous.ID, current.ID,
		"%s and %s share a %s and are not in descending identity order", previous.ID, current.ID, field)
}

// sortKeyOf reads the ordering key at the precision the ORDER BY sees it.
//
// `updated` has to come from stored data and not from the response, and that is
// a finding rather than a convenience: PocketBase stores the column to the
// millisecond and contracts/records.md publishes it as RFC3339 with no
// fractional part, so two rows written 4 ms apart carry the same `updated_at`
// on the wire. A client cannot reproduce this ordering from what it was sent,
// and neither could this assertion.
func sortKeyOf(item medicationDTO, field string, stored map[string]string) (string, bool) {
	switch field {
	case medication.FieldName:
		return item.Name, true
	case medication.FieldStartedOn:
		if item.StartedOn == nil {
			return "", false
		}

		return *item.StartedOn, true
	default:
		if precise, known := stored[item.ID]; known {
			return precise, true
		}

		return item.UpdatedAt, true
	}
}

func publishedSorts() []string {
	published := medication.Sorts()

	spellings := make([]string, 0, len(published))
	for _, key := range published {
		spellings = append(spellings, key.String())
	}

	return spellings
}

// TestTheDefaultOrderingIsTheOneTheContractNames pins the default separately.
// A request with no `sort` is what every page after the first sends, so a
// default that drifted would reorder the whole application without any request
// naming an ordering at all.
func TestTheDefaultOrderingIsTheOneTheContractNames(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	unstated := caller.get(collectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100")
	require.Equal(t, http.StatusOK, unstated.Status, unstated.Body)

	assert.Equal(t, expectedOrder(medication.Sorts()[0].String()), idsOf(unstated.list(t)))
}

func TestTheStatusFilterNarrowsToExactlyTheStatesAsked(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	cases := []struct {
		name     string
		asked    []clinical.TherapyStatus
		expected int
	}{
		{"one state", []clinical.TherapyStatus{clinical.TherapyStatusActive}, 0},
		{"two states", []clinical.TherapyStatus{clinical.TherapyStatusCompleted, clinical.TherapyStatusStopped}, 0},
		{"a state nothing is in", []clinical.TherapyStatus{clinical.TherapyStatusOnHold}, 0},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			wanted := make([]string, 0)

			for _, row := range ownedByAccountA() {
				if slices.Contains(one.asked, row.Status) {
					wanted = append(wanted, row.ID)
				}
			}

			require.NotEmpty(t, wanted, "the fixture holds nothing in %v, so the case asserts nothing", one.asked)

			answer := caller.get(collectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100&" + medication.FilterStatus + "=" + statusList(one.asked))
			require.Equal(t, http.StatusOK, answer.Status, answer.Body)

			got := idsOf(answer.list(t))

			slices.Sort(wanted)
			slices.Sort(got)

			assert.Equal(t, wanted, got)
		})
	}
}

func statusList(statuses []clinical.TherapyStatus) string {
	spellings := make([]string, 0, len(statuses))
	for _, status := range statuses {
		spellings = append(spellings, string(status))
	}

	return strings.Join(spellings, ",")
}

// TestAStateOutsideTheVocabularyIsRefusedRatherThanDropped is the other half of
// the filter: a dropped term narrows to everything and reads as a list that is
// simply long.
func TestAStateOutsideTheVocabularyIsRefusedRatherThanDropped(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	answer := caller.get(collectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&" + medication.FilterStatus + "=discontinued")

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	assert.Contains(t, answer.envelope(t).fieldCodes(), [2]string{medication.FilterStatus, "invalid_value"})
}

func TestTheSearchNarrowsOverBothNameColumns(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	cases := []struct {
		name     string
		needle   string
		expected []string
	}{
		{"the recorded name", "metfor", []string{"mkmedamara00002"}},
		{"the alternative name", "glucophage", []string{"mkmedamara00002"}},
		{"a needle in neither", "zzzznothing", nil},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			answer := caller.get(collectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100&" + web.ParamSearch + "=" + url.QueryEscape(one.needle))
			require.Equal(t, http.StatusOK, answer.Status, answer.Body)

			assert.Equal(t, one.expected, emptyToNil(idsOf(answer.list(t))))
		})
	}
}

func emptyToNil(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}

	return ids
}

// TestATraversalRepeatsNothingAndSkipsNothing is FR-023 at the HTTP edge. The
// store's own contract covers the mid-page insert; this covers the thing a
// client actually does, which is follow next_cursor until it is null.
func TestATraversalRepeatsNothingAndSkipsNothing(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	forwards := traverse(t, caller, collectionURL()+"?patient="+testsupport.AccountAPatientSelfID+"&limit=5")

	assert.Len(t, forwards, testsupport.AccountAMedicationCount)
	assert.Equal(t, expectedOrder(medication.Sorts()[0].String()), forwards)

	// The same set in the opposite direction. There is no backwards cursor —
	// the ordering is what a caller reverses — so this is the traversal a page
	// makes when somebody clicks the other sort arrow, and it must reach every
	// row the first one did.
	backwards := traverse(t, caller, collectionURL()+"?patient="+testsupport.AccountAPatientSelfID+"&limit=5&sort="+url.QueryEscape(medication.Sorts()[1].String()))

	assert.ElementsMatch(t, forwards, backwards,
		"paging forwards and then in the reverse ordering did not reach the same set")

	assert.Equal(t, expectedOrder(medication.Sorts()[1].String()), backwards)
}

// traverse follows next_cursor to the end and fails on a repeat rather than
// counting one at the end, so the page that repeated is named.
func traverse(t *testing.T, caller *caller, first string) []string {
	t.Helper()

	var (
		collected []string
		seen      = map[string]int{}
		address   = first
	)

	for page := 1; ; page++ {
		require.LessOrEqual(t, page, 50, "the traversal is not terminating")

		answer := caller.get(address)
		require.Equal(t, http.StatusOK, answer.Status, answer.Body)

		body := answer.list(t)

		for _, item := range body.Items {
			if earlier, repeated := seen[item.ID]; repeated {
				t.Fatalf("%s was served on page %d and again on page %d", item.ID, earlier, page)
			}

			seen[item.ID] = page
			collected = append(collected, item.ID)
		}

		if body.NextCursor == nil {
			return collected
		}

		address = first + "&" + web.ParamCursor + "=" + url.QueryEscape(*body.NextCursor)
	}
}
