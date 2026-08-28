package pb_test

import (
	"errors"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/obs"
	"medikube/internal/platform/pb"
)

// recordingBinder stands in for internal/httproute.Registry, which does not
// exist yet. The seam is an interface with one method, so the registry
// satisfies it by existing rather than by anybody editing serve.go.
type recordingBinder struct {
	calls  int
	events []*core.ServeEvent
	err    error
}

func (b *recordingBinder) Bind(se *core.ServeEvent) error {
	b.calls++
	b.events = append(b.events, se)

	return b.err
}

func serveApp(t *testing.T) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp()
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	return app
}

// triggerServe reproduces what apis.Serve does around the OnServe hook, and
// returns the event and the trigger's error so a refusal is observable.
func triggerServe(t *testing.T, app *tests.TestApp, server *http.Server) (*core.ServeEvent, error) {
	t.Helper()

	r, err := apis.NewRouter(app)
	require.NoError(t, err)

	se := new(core.ServeEvent)
	se.App = app
	se.Router = r
	se.Server = server
	se.InstallerFunc = apis.DefaultInstallerFunc

	return se, app.OnServe().Trigger(se, func(*core.ServeEvent) error { return nil })
}

func pocketBaseServer() *http.Server {
	// apis/serve.go:144-160, the literal PocketBase hands to the ServeEvent.
	return &http.Server{
		WriteTimeout:      5 * time.Minute,
		ReadTimeout:       5 * time.Minute,
		ReadHeaderTimeout: time.Minute,
	}
}

// PocketBase hardcodes WriteTimeout at five minutes in a struct literal with no
// configuration field (apis/serve.go:152). Datastar never touches the write
// deadline, so every long-lived stream dies at exactly 5:00 with a write error
// and the browser reconnect-loops — and it passes every test shorter than five
// minutes, which is what makes it dangerous (research D-34, SC-007).
func TestServeOverridesPocketBasesFiveMinuteWriteTimeout(t *testing.T) {
	t.Parallel()

	t.Run("the default removes the deadline entirely", func(t *testing.T) {
		t.Parallel()

		app := serveApp(t)
		pb.BindServe(app, pb.ServeOptions{})

		se, err := triggerServe(t, app, pocketBaseServer())
		require.NoError(t, err)

		assert.Zero(t, se.Server.WriteTimeout,
			"a server-wide write deadline is a silent cap on every stream and every large download")
		assert.NotEqual(t, 5*time.Minute, se.Server.WriteTimeout)
	})

	t.Run("an explicit value is honoured", func(t *testing.T) {
		t.Parallel()

		app := serveApp(t)
		pb.BindServe(app, pb.ServeOptions{WriteTimeout: 30 * time.Second})

		se, err := triggerServe(t, app, pocketBaseServer())
		require.NoError(t, err)

		assert.Equal(t, 30*time.Second, se.Server.WriteTimeout)
	})

	t.Run("the read deadlines are left alone", func(t *testing.T) {
		t.Parallel()

		app := serveApp(t)
		pb.BindServe(app, pb.ServeOptions{})

		se, err := triggerServe(t, app, pocketBaseServer())
		require.NoError(t, err)

		// ReadHeaderTimeout is the slow-loris defence and has nothing to do
		// with streaming; removing it too would trade one bug for another.
		assert.Equal(t, time.Minute, se.Server.ReadHeaderTimeout)
		assert.Equal(t, 5*time.Minute, se.Server.ReadTimeout)
	})

	t.Run("a ServeEvent with no server is tolerated", func(t *testing.T) {
		t.Parallel()

		// tests.ApiScenario builds the event by hand and sets only App and
		// Router (tests/api.go:192-195). Without the nil guard every
		// scenario-based HTTP test in the repository panics.
		app := serveApp(t)
		pb.BindServe(app, pb.ServeOptions{})

		_, err := triggerServe(t, app, nil)
		assert.NoError(t, err)
	})
}

// T066. The chain MediKube serves, written out end to end.
//
// It is written out rather than probed because the lockdown short-circuits: the
// chain is not a list of things that all run, it is a list of things that run
// *before* the 404, and everything after it is skipped on a locked route. Every
// neighbour is therefore load-bearing —
//
//   - the request logger first, ahead of everything PocketBase binds, so a
//     request the rate limiter refuses is still correlated (research D-29);
//   - loadAuthToken before the lockdown, or e.Auth is nil and the superuser
//     carve-out cannot exist;
//   - securityHeaders before the lockdown, or a locked 404 arrives without the
//     four headers a genuine 404 carries and one of them answers the question
//     the 404 exists to refuse (commit c51a563);
//   - rateLimit and bodyLimit after it, which is what makes them unreachable on
//     a locked route.
//
// The priorities are literals rather than PocketBase's constants. LockdownPriority
// is derived — DefaultSecurityHeadersMiddlewarePriority + 1 — so an upgrade that
// renumbered securityHeaders would carry the lockdown along with it, and an
// expectation written in the same constants would follow both and assert nothing.
var medikubeMiddlewareOrder = []boundMiddleware{
	{id: obs.RequestLoggerID, priority: -1050},
	{id: "pbPanicRecover", priority: -1030},
	{id: "pbLoadAuthToken", priority: -1020},
	{id: "pbSuperuserIPsWhitelist", priority: -1015},
	{id: "pbSecurityHeaders", priority: -1010},
	{id: pb.LockdownMiddlewareID, priority: -1009},
	{id: "pbRateLimit", priority: -1000},
	{id: "pbBodyLimit", priority: -990},
}

type boundMiddleware struct {
	id       string
	priority int
}

func TestServeEstablishesTheMiddlewareOrder(t *testing.T) {
	t.Parallel()

	app := serveApp(t)

	// MediKube's own request logger, not a stand-in: its priority is part of
	// the order under test, and a probe placed by this test would be asserting
	// a number this test chose.
	pb.BindServe(app, pb.ServeOptions{
		Middlewares: []*hook.Handler[*core.RequestEvent]{obs.RequestLogger(zerolog.Nop())},
	})

	se, err := triggerServe(t, app, pocketBaseServer())
	require.NoError(t, err)

	t.Run("the chain is exactly as designed, in execution order", func(t *testing.T) {
		// Sorted stably by priority, which is what the router does per route
		// (tools/hook/hook.go:98-100) when it builds the chain out of
		// Router.Middlewares — that slice is in bind order, not run order.
		bound := make([]boundMiddleware, 0, len(se.Router.Middlewares))
		for _, middleware := range se.Router.Middlewares {
			bound = append(bound, boundMiddleware{id: middleware.Id, priority: middleware.Priority})
		}

		sort.SliceStable(bound, func(i, j int) bool { return bound[i].priority < bound[j].priority })

		assert.Equal(t, medikubeMiddlewareOrder, bound)
	})

	t.Run("PocketBase's activity logger is gone", func(t *testing.T) {
		// It records the full request URI, and a query string is where a search
		// for a medication name ends up (FR-038). MediKube's own request logger
		// records the path only, and takes the slot it vacated.
		assert.Nil(t, findMiddleware(se.Router.Middlewares, apis.DefaultActivityLoggerMiddlewareId),
			"the activity logger writes the query string into a second log store")
	})

	t.Run("the request logger takes the slot ahead of everything PocketBase binds", func(t *testing.T) {
		logger := findMiddleware(se.Router.Middlewares, obs.RequestLoggerID)
		require.NotNil(t, logger)

		assert.Equal(t, apis.DefaultActivityLoggerMiddlewarePriority-10, logger.Priority)
		assert.Less(t, logger.Priority, apis.DefaultRateLimitMiddlewarePriority,
			"a request refused by the rate limiter must still be correlated")
	})

	t.Run("the first-run installer is not offered", func(t *testing.T) {
		// PocketBase's installer page would make a freshly seeded instance
		// non-deterministic for the browser gate, and it is a superuser
		// creation flow reachable without credentials (research D-06).
		assert.Nil(t, se.InstallerFunc)
	})
}

func TestServeHandsTheEventToTheRouteRegistry(t *testing.T) {
	t.Parallel()

	t.Run("exactly once, with the same event", func(t *testing.T) {
		t.Parallel()

		app := serveApp(t)
		binder := &recordingBinder{}
		pb.BindServe(app, pb.ServeOptions{Routes: binder})

		se, err := triggerServe(t, app, pocketBaseServer())
		require.NoError(t, err)

		require.Equal(t, 1, binder.calls)
		assert.Same(t, se, binder.events[0])
	})

	t.Run("and a registry that refuses stops the boot", func(t *testing.T) {
		t.Parallel()

		sentinel := errors.New("route table is inconsistent")

		app := serveApp(t)
		pb.BindServe(app, pb.ServeOptions{Routes: &recordingBinder{err: sentinel}})

		// A non-nil error out of an OnServe handler aborts the serve before the
		// listener is created (apis/serve.go:265-267), which is the only
		// "refuse to start" PocketBase offers at this point.
		_, err := triggerServe(t, app, pocketBaseServer())
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("and no registry is not an error", func(t *testing.T) {
		t.Parallel()

		app := serveApp(t)
		pb.BindServe(app, pb.ServeOptions{})

		_, err := triggerServe(t, app, pocketBaseServer())
		assert.NoError(t, err)
	})
}

// The lockdown keys on the router's own matched pattern. If an upgrade renames
// one of those routes the middleware silently stops matching it and the route
// opens — no error, no log line, just a working public API over somebody's
// medical records. This turns that into a boot failure.
func TestServeRefusesToStartIfALockedRouteHasDisappeared(t *testing.T) {
	t.Parallel()

	t.Run("PocketBase's own router still registers every locked route", func(t *testing.T) {
		t.Parallel()

		app := serveApp(t)
		r, err := apis.NewRouter(app)
		require.NoError(t, err)

		assert.NoError(t, pb.AssertLockedRoutesRegistered(r))
	})

	t.Run("a router missing them is refused, by name", func(t *testing.T) {
		t.Parallel()

		empty := router.NewRouter[*core.RequestEvent](
			func(_ http.ResponseWriter, _ *http.Request) (*core.RequestEvent, router.EventCleanupFunc) {
				return nil, nil
			},
		)

		err := pb.AssertLockedRoutesRegistered(empty)

		require.Error(t, err)
		for _, locked := range pb.LockedRoutes() {
			assert.Contains(t, err.Error(), locked.Pattern())
		}
	})

	t.Run("the locked set is the record subtree, batch, realtime and the file routes", func(t *testing.T) {
		t.Parallel()

		patterns := make([]string, 0, len(pb.LockedRoutes()))
		for _, locked := range pb.LockedRoutes() {
			patterns = append(patterns, locked.Pattern())
		}

		assert.ElementsMatch(t, []string{
			"GET /api/collections/{collection}/records",
			"GET /api/collections/{collection}/records/{id}",
			"POST /api/collections/{collection}/records",
			"PATCH /api/collections/{collection}/records/{id}",
			"DELETE /api/collections/{collection}/records/{id}",
			"POST /api/batch",
			"POST /api/realtime",
			"POST /api/files/token",
			"GET /api/files/{collection}/{recordId}/{filename}",
		}, patterns)
	})
}
