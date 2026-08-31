package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/apitest"
)

// T184, SC-002 and the spec's "five thousand medications" edge case.
//
// SC-002 is a latency budget — every page of a 1,000-medication list within two
// seconds — and the edge case is a correctness one. They are separate tests
// here for that reason: a slow machine should fail the budget and a wrong
// cursor should fail the traversal, and a single test that did both would be
// impossible to read the failure of.

// The two sizes the spec names. The first is SC-002's; the second is the edge
// case, which is exercised for correctness rather than for latency.
const (
	budgetRows = 1000
	edgeRows   = 5000

	// SC-002's own number, and the assertion is deliberately of the whole
	// budget rather than of some fraction of it: a threshold tightened to what
	// the machine currently does is a threshold that fails on a busy CI runner
	// and tells nobody anything.
	pageBudget = 2 * time.Second

	// A page big enough that a 5,000-row traversal is fifty requests rather
	// than two hundred, and still within web.MaxLimit.
	traversalPage = 100
)

// bulk drives one populated instance. It is separate from caller because a
// benchmark holds a *testing.B and because nothing here needs the DTO helpers.
type bulk struct {
	tb      testing.TB
	handler http.Handler
	token   string
}

func newBulk(tb testing.TB, rows int) *bulk {
	tb.Helper()

	instance := apitest.NewPopulated(tb, testsupport.AccountAID, rows)

	return &bulk{
		tb:      tb,
		handler: testsupport.NewEdgeHandler(tb, instance.App),
		token:   testsupport.UserToken(tb, instance.App, testsupport.AccountAEmail),
	}
}

// page requests one page and returns it with how long the whole round trip
// took, which is what the budget is expressed in.
func (b *bulk) page(limit int, cursor string) (listDTO, time.Duration) {
	b.tb.Helper()

	query := url.Values{web.ParamLimit: {strconv.Itoa(limit)}}
	if cursor != "" {
		query.Set(web.ParamCursor, cursor)
	}

	request := httptest.NewRequestWithContext(b.tb.Context(), http.MethodGet,
		collectionURL()+"?"+query.Encode(), nil)
	request.Header.Set("Authorization", b.token)

	recorder := httptest.NewRecorder()

	started := time.Now()
	b.handler.ServeHTTP(recorder, request)
	took := time.Since(started)

	require.Equalf(b.tb, http.StatusOK, recorder.Code, "the list was refused: %s", recorder.Body.String())

	var listing listDTO
	require.NoError(b.tb, json.Unmarshal(recorder.Body.Bytes(), &listing))

	return listing, took
}

// SC-002. Every page of a thousand, not merely the first: the first page of a
// keyset traversal is the cheap one, and a cursor predicate that degraded with
// depth would pass a test that only asked for the front of the list.
func TestEveryPageOfAThousandRowListIsServedWithinTheBudget(t *testing.T) {
	t.Parallel()

	driver := newBulk(t, budgetRows)

	var (
		cursor string
		pages  int
		worst  time.Duration
	)

	for {
		listing, took := driver.page(traversalPage, cursor)

		pages++
		worst = max(worst, took)

		assert.LessOrEqualf(t, took, pageBudget,
			"page %d of a %d-row list took %s, and SC-002 allows %s", pages, budgetRows, took, pageBudget)

		if listing.NextCursor == nil {
			break
		}

		cursor = *listing.NextCursor
	}

	require.Greater(t, pages, 1, "a thousand rows came back in one page, so nothing about paging was measured")
	t.Logf("%d pages of %d rows, worst page %s of a %s budget", pages, traversalPage, worst, pageBudget)
}

// The edge case, for correctness. Five thousand rows traversed end to end: the
// union is every row exactly once, which is FR-023's promise at a size where a
// cursor that encoded an offset, or a sort key with ties it could not break,
// would finally repeat or skip.
func TestFiveThousandRowsTraverseWithoutRepeatingOrSkipping(t *testing.T) {
	t.Parallel()

	driver := newBulk(t, edgeRows)

	var (
		cursor string
		first  time.Duration
		last   time.Duration
	)

	seen := make(map[string]int, edgeRows)

	for {
		listing, took := driver.page(traversalPage, cursor)

		if first == 0 {
			first = took
		}

		last = took

		for _, item := range listing.Items {
			seen[item.ID]++
		}

		if listing.NextCursor == nil {
			break
		}

		cursor = *listing.NextCursor
	}

	// The fixture's own rows for this account are in the list too, and they
	// belong there: the traversal is of everything the account owns.
	assert.Len(t, seen, edgeRows+testsupport.AccountAMedicationCount)

	for id, times := range seen {
		assert.Equalf(t, 1, times, "%s came back %d times", id, times)
	}

	// contracts/records.md: the last page is not materially slower than the
	// first. Expressed as the same budget both pages have to meet, because a
	// ratio between two sub-millisecond measurements is noise.
	assert.LessOrEqual(t, last, pageBudget, "the last page of %d rows took %s", edgeRows, last)
	t.Logf("first page %s, last page %s", first, last)
}

func BenchmarkTheFirstPageOfALargeList(b *testing.B) {
	for _, rows := range []int{budgetRows, edgeRows} {
		b.Run(strconv.Itoa(rows), func(b *testing.B) {
			driver := newBulk(b, rows)

			b.ResetTimer()

			for b.Loop() {
				driver.page(web.DefaultLimit, "")
			}
		})
	}
}
