package pb_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// probeRecordID is a syntactically valid record id that exists nowhere. Under
// the lockdown it never reaches a handler, which is the point: the answer must
// not depend on whether the record is real.
const probeRecordID = "abc123def456ghi"

type request struct {
	method string
	target string
	body   string
}

// recordCrudRequests is every method PocketBase binds under
// /api/collections/{collection}/records (apis/record_crud.go:28-33). All five,
// because a lockdown that covers four is not a lockdown.
func recordCrudRequests(collection string) []request {
	base := "/api/collections/" + collection + "/records"

	return []request{
		{http.MethodGet, base, ""},
		{http.MethodGet, base + "/" + probeRecordID, ""},
		{http.MethodPost, base, `{}`},
		{http.MethodPatch, base + "/" + probeRecordID, `{}`},
		{http.MethodDelete, base + "/" + probeRecordID, ""},
	}
}

// The most important test in the phase.
//
// 404 and not 403 is the whole requirement: a 403 confirms that the thing asked
// for exists, and "a request for a medication belonging to somebody else MUST be
// answered exactly as a request for one that has never existed" (FR-033). So the
// assertion is on the status *and* on the bytes, against a genuine 404 taken
// from the same instance.
func TestRecordCrudRoutesAnswer404ForEveryCollectionAndEveryOrdinaryCaller(t *testing.T) {
	t.Parallel()

	h := newHarness(t, bindMediKubeServe)

	genuine := h.genuine404(t)

	actors := []struct {
		name  string
		token string
	}{
		{name: "anonymous", token: ""},
		{name: "an ordinary signed-in person", token: h.userToken(t)},
	}

	collections := h.collectionNames(t)
	require.Contains(t, collections, "users")

	for _, collection := range collections {
		for _, actor := range actors {
			t.Run(collection+"/"+actor.name, func(t *testing.T) {
				for _, req := range recordCrudRequests(collection) {
					res := h.do(t, req.method, req.target, actor.token, req.body)

					assert.Equalf(t, http.StatusNotFound, res.Status,
						"%s %s must be 404 and not 403: a 403 confirms the collection exists", req.method, req.target)
					assert.Equalf(t, genuine.Body, res.Body,
						"%s %s must be byte-identical to a route that never existed (FR-033)", req.method, req.target)
				}
			})
		}
	}
}

// /api/batch re-enters the same record-CRUD handler bodies as direct function
// calls (apis/batch.go:38-88), so it bypasses the router and every route-level
// defence. Left to its own handler it answers 403 "Batch requests are not
// allowed." — which is both a different status and a different sentence, and
// therefore a disclosure.
//
// Note that batch is ENABLED in the fixture this runs against, so nothing here
// is being carried by Settings().Batch.Enabled; the 404 is the middleware.
func TestBatchAnswers404ForEveryOrdinaryCaller(t *testing.T) {
	t.Parallel()

	h := newHarness(t, bindMediKubeServe)

	genuine := h.genuine404(t)

	for _, actor := range []struct {
		name  string
		token string
	}{
		{name: "anonymous", token: ""},
		{name: "an ordinary signed-in person", token: h.userToken(t)},
	} {
		t.Run(actor.name, func(t *testing.T) {
			res := h.do(t, http.MethodPost, "/api/batch", actor.token,
				`{"requests":[{"method":"POST","url":"/api/collections/users/records","body":{}}]}`)

			assert.Equal(t, http.StatusNotFound, res.Status)
			assert.Equal(t, genuine.Body, res.Body)
		})
	}
}

// The admin UI is a superuser's only interface and it drives the record-CRUD
// routes directly, so the carve-out is not a convenience — without it the
// constitution's "the admin UI ships in production" clause is unimplementable.
func TestSuperusersKeepTheRecordSubtree(t *testing.T) {
	t.Parallel()

	h := newHarness(t, bindMediKubeServe)
	token := h.superuserToken(t)

	t.Run("every collection still lists", func(t *testing.T) {
		for _, collection := range h.collectionNames(t) {
			res := h.do(t, http.MethodGet, "/api/collections/"+collection+"/records", token, "")

			assert.Equalf(t, http.StatusOK, res.Status, "superuser list of %q", collection)
		}
	})

	t.Run("the full create-read-update-delete cycle still works", func(t *testing.T) {
		created := h.do(t, http.MethodPost, "/api/collections/users/records", token,
			`{"email":"lockdown-probe@example.com","password":"1234567890","passwordConfirm":"1234567890"}`)
		require.Equal(t, http.StatusOK, created.Status, "superuser create: %s", created.Body)

		var record struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.Unmarshal([]byte(created.Body), &record))
		require.NotEmpty(t, record.ID)

		target := "/api/collections/users/records/" + record.ID

		assert.Equal(t, http.StatusOK, h.do(t, http.MethodGet, target, token, "").Status)
		assert.Equal(t, http.StatusOK, h.do(t, http.MethodPatch, target, token, `{"name":"probe"}`).Status)
		assert.Equal(t, http.StatusNoContent, h.do(t, http.MethodDelete, target, token, "").Status)
	})

	t.Run("batch reaches its own handler", func(t *testing.T) {
		// PocketBase's own test fixture ships Settings().Batch.Enabled = true,
		// which makes this suite stronger than it would otherwise be: the 404
		// an ordinary caller gets from /api/batch is unambiguously the
		// middleware, not the setting. A superuser gets past the middleware and
		// meets the real handler, which rejects an empty request list.
		//
		// MediKube's own boot writes Batch.Enabled = false — asserted in
		// settings_test.go — because batch calls the record-CRUD handler bodies
		// directly and never passes a middleware at all. Both mechanisms are
		// required and each is tested where it lives.
		res := h.do(t, http.MethodPost, "/api/batch", token, `{"requests":[]}`)

		assert.NotEqual(t, http.StatusNotFound, res.Status)
	})
}

// A lockdown that only fires on paths a test happens to name is worthless. This
// asserts the shape of the discrimination itself: the router's own pattern, not
// a prefix of the URL, is what decides.
func TestNeighbouringPathsAreNotSweptUp(t *testing.T) {
	t.Parallel()

	h := newHarness(t, bindMediKubeServe)

	// A path that merely *starts* with the records prefix is not a records
	// route; Go's mux gives it the catch-all pattern, and the lockdown must
	// leave it to the 404 it already earns rather than claiming it.
	res := h.do(t, http.MethodGet, "/api/collections/users/recordsX", "", "")
	assert.Equal(t, http.StatusNotFound, res.Status)

	// Liveness must survive: it is one of the five routes an anonymous caller
	// is allowed to reach (FR-034).
	assert.NotEqual(t, http.StatusNotFound, h.do(t, http.MethodGet, "/api/health", "", "").Status)
}
