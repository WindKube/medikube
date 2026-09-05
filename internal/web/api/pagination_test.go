package api_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/web"
)

// T162, FR-053, spec Edge Case "Paging while data changes". Each of this
// phase's three new lists is a keyset traversal (research D-29): a row that
// existed for the whole walk must be served exactly once, regardless of what
// somebody else inserts or deletes on the same list in between two of the
// caller's own requests.
//
// itemsPage is a minimal, list-agnostic decode of contracts/README.md's one
// list envelope — only the id is read, because that is all a traversal needs
// to prove nothing repeated and nothing already-there was skipped.
type itemsPage struct {
	Items []struct {
		ID string `json:"id"`
	} `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

func (r response) items(t *testing.T) itemsPage {
	t.Helper()

	var page itemsPage
	r.decode(t, &page)

	return page
}

// pagingList is one list under test: how to create a row that sorts
// predictably, and the URL its list lives at.
type pagingList struct {
	name string
	url  string
	// create returns the request body for the nth seeded row (1-based), named
	// so the list's default sort produces a stable, predictable order.
	create func(n int) string
}

func TestPagingWhileRowsAreInsertedAndDeletedRepeatsNothingAndSkipsNothing(t *testing.T) {
	t.Parallel()

	lists := []pagingList{
		{
			name: "patients",
			url:  patientsURL(),
			create: func(n int) string {
				return fmt.Sprintf(`{"first_name":"Paging","last_name":"Row%02d","birth_date":"1990-01-01"}`, n)
			},
		},
		{
			name: "practitioners",
			url:  practitionersURL(),
			create: func(n int) string {
				return fmt.Sprintf(`{"name":"Dr. Paging Row%02d"}`, n)
			},
		},
		{
			name: "facilities",
			url:  facilitiesURL(),
			create: func(n int) string {
				return fmt.Sprintf(`{"kind":"practice","name":"Paging Row%02d"}`, n)
			},
		},
	}

	for _, list := range lists {
		t.Run(list.name, func(t *testing.T) {
			t.Parallel()

			caller := newCaller(t)

			// Seven rows sort predictably (Row01..Row07) under every one of
			// the three default orderings, which all end in name then id.
			const seeded = 7

			present := make(map[string]bool, seeded)

			for n := 1; n <= seeded; n++ {
				created := caller.post(list.url, list.create(n))
				require.Equalf(t, http.StatusCreated, created.Status, "seeding row %d: %s", n, created.Body)

				present[created.items1(t).ID] = true
			}

			var (
				seen       = map[string]int{}
				address    = list.url + "?limit=2"
				page       = 0
				mutated    bool
				deletedID  string
				insertedID string
			)

			for {
				page++
				require.LessOrEqual(t, page, seeded+3, "the %s traversal is not terminating", list.name)

				answer := caller.get(address)
				require.Equalf(t, http.StatusOK, answer.Status, "%s page %d: %s", list.name, page, answer.Body)

				body := answer.items(t)

				for _, item := range body.Items {
					if earlier, repeated := seen[item.ID]; repeated {
						t.Fatalf("%s: %s was served on page %d and again on page %d", list.name, item.ID, earlier, page)
					}

					seen[item.ID] = page
				}

				// Once, after the first page has been read and before the
				// second request: delete a row the traversal has not reached
				// yet, and insert a new one. Both happen on the account's
				// live list, between two of the caller's own requests — the
				// scenario the edge case names.
				if !mutated && body.NextCursor != nil {
					mutated = true

					for id := range present {
						if _, alreadySeen := seen[id]; !alreadySeen {
							deletedID = id

							break
						}
					}
					require.NotEmptyf(t, deletedID, "%s: every seeded row was already served before any mutation happened", list.name)

					current := caller.get(itemURL(list.url, deletedID))
					require.Equal(t, http.StatusOK, current.Status)
					deleted := caller.delete(itemURL(list.url, deletedID), current.etag(t))
					require.Equalf(t, http.StatusNoContent, deleted.Status, "%s: deleting %s mid-traversal", list.name, deletedID)
					delete(present, deletedID)

					inserted := caller.post(list.url, list.create(seeded+1))
					require.Equalf(t, http.StatusCreated, inserted.Status, "%s: inserting a row mid-traversal", list.name)
					insertedID = inserted.items1(t).ID
				}

				if body.NextCursor == nil {
					break
				}

				address = list.url + "?limit=2&" + web.ParamCursor + "=" + url.QueryEscape(*body.NextCursor)
			}

			for id := range present {
				assert.Containsf(t, seen, id, "%s: %s existed for the whole traversal and was skipped", list.name, id)
			}

			assert.NotContainsf(t, seen, deletedID, "%s: the row deleted mid-traversal was still served afterwards", list.name)

			// The inserted row is not asserted either way: whether a keyset
			// traversal picks up a row created after the walk began depends
			// on where it sorts relative to the current boundary, and both
			// answers are correct. What would be a bug is serving it twice,
			// which the repeat check above already covers if it happened.
			_ = insertedID
		})
	}
}

// items1 decodes a single-object response (a create) far enough to read its
// id, mirroring response.items' list decode.
func (r response) items1(t *testing.T) struct {
	ID string `json:"id"`
} {
	t.Helper()

	var item struct {
		ID string `json:"id"`
	}
	r.decode(t, &item)

	return item
}

func itemURL(listURL, id string) string { return listURL + "/" + id }
