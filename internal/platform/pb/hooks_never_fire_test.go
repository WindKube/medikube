package pb_test

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordRequestHookCounts counts the record-CRUD *request* hook family — the
// one family .golangci.yml's forbidigo rule bans MediKube from binding.
//
// This file is the evidence behind that ban, which is why it is the second file
// allowed to name the family, and why the exclusion in .golangci.yml names it
// beside hooks.go instead of exempting _test.go everywhere. Without the
// evidence the rule reads as dogma; with it, the rule is a measurement.
type recordRequestHookCounts struct {
	create atomic.Int64
	update atomic.Int64
	remove atomic.Int64
	view   atomic.Int64
	list   atomic.Int64
}

func (c *recordRequestHookCounts) bind(app core.App) {
	app.OnRecordCreateRequest().BindFunc(func(e *core.RecordRequestEvent) error {
		c.create.Add(1)

		return e.Next()
	})
	app.OnRecordUpdateRequest().BindFunc(func(e *core.RecordRequestEvent) error {
		c.update.Add(1)

		return e.Next()
	})
	app.OnRecordDeleteRequest().BindFunc(func(e *core.RecordRequestEvent) error {
		c.remove.Add(1)

		return e.Next()
	})
	app.OnRecordViewRequest().BindFunc(func(e *core.RecordRequestEvent) error {
		c.view.Add(1)

		return e.Next()
	})
	app.OnRecordsListRequest().BindFunc(func(e *core.RecordsListRequestEvent) error {
		c.list.Add(1)

		return e.Next()
	})
}

func (c *recordRequestHookCounts) total() int64 {
	return c.create.Load() + c.update.Load() + c.remove.Load() + c.view.Load() + c.list.Load()
}

// The hooks are bound *inside* the built-in CRUD handlers, and the lockdown
// answers before those handlers run. So business logic placed there is not
// "sometimes skipped", it is unreachable — dead code that looks live, which is
// the worst failure mode available (research D-14, reconciliation C13).
func TestRecordRequestHooksNeverFireUnderTheLockdown(t *testing.T) {
	t.Parallel()

	var counts recordRequestHookCounts

	h := newHarness(t, func(app *tests.TestApp) {
		counts.bind(app)
		bindMediKubeServe(app)
	})

	for _, actor := range []struct {
		name  string
		token string
	}{
		{name: "anonymous", token: ""},
		{name: "an ordinary signed-in person", token: h.userToken(t)},
	} {
		t.Run(actor.name, func(t *testing.T) {
			for _, collection := range h.collectionNames(t) {
				for _, req := range recordCrudRequests(collection) {
					res := h.do(t, req.method, req.target, actor.token, req.body)
					require.Equalf(t, http.StatusNotFound, res.Status, "%s %s", req.method, req.target)
				}
			}

			// The other door into the same handler bodies: batch calls
			// recordCreate/recordUpdate/recordDelete directly rather than
			// through the router (apis/batch.go:38-88).
			res := h.do(t, http.MethodPost, "/api/batch", actor.token,
				`{"requests":[{"method":"POST","url":"/api/collections/users/records","body":{}}]}`)
			require.Equal(t, http.StatusNotFound, res.Status)

			assert.Zero(t, counts.total(), "a record-CRUD request hook fired; anything bound there is not dead code after all")
		})
	}
}

// The carve-out, stated rather than hidden. A superuser passes the lockdown, so
// for a superuser these hooks DO fire — which is precisely why they still may
// not be used for MediKube's business logic: a rule that holds for every caller
// except the one who bypasses every other rule is not a rule.
func TestRecordRequestHooksDoFireForASuperuser(t *testing.T) {
	t.Parallel()

	var counts recordRequestHookCounts

	h := newHarness(t, func(app *tests.TestApp) {
		counts.bind(app)
		bindMediKubeServe(app)
	})

	token := h.superuserToken(t)

	res := h.do(t, http.MethodPost, "/api/collections/users/records", token,
		`{"email":"hook-probe@example.com","password":"1234567890","passwordConfirm":"1234567890"}`)
	require.Equal(t, http.StatusOK, res.Status, res.Body)

	assert.EqualValues(t, 1, counts.create.Load())

	require.Equal(t, http.StatusOK, h.do(t, http.MethodGet, "/api/collections/users/records", token, "").Status)
	assert.EqualValues(t, 1, counts.list.Load())
}
