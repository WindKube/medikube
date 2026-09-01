package testsupport

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/stretchr/testify/require"

	// The migrations register themselves from their own init, and
	// tests.NewTestApp runs core.AppMigrations against the clone. Without this
	// import the list is empty and every test in the repository would run
	// against PocketBase's stock schema — no medications collection, no audit
	// trail, and an ownership matrix passing because there was nothing to own.
	// pocketbase/tests does exactly this for the system set.
	_ "medikube/internal/store/migrations"
)

// usersCollection is PocketBase's own auth collection, which MediKube amends
// rather than replaces.
const usersCollection = "users"

// The harness's own OnServe handler is named and ordered, so binding it twice
// replaces rather than appends — the accumulation this package exists to warn
// about is not one to reproduce. It runs ahead of PocketBase's cron starter
// (999) and admin-UI extensions (9999).
const (
	serveHookID       = "medikubeTestSupportServe"
	serveHookPriority = -1000
)

// ServeBinder registers routes and middleware on the serve event. It is
// declared structurally rather than imported so that this package does not
// depend on the route registry that does not exist yet in this phase:
// internal/httproute's registry and internal/platform/pb's RouteBinder both
// satisfy it as written.
type ServeBinder interface {
	Bind(se *core.ServeEvent) error
}

// FixtureDir is the committed data directory every test app is a clone of.
//
// It is resolved from this file's own path and never from the working
// directory: `go test` runs each package in its own directory, so a relative
// literal would resolve from internal/testsupport for one caller and from
// internal/web/api for the next.
func FixtureDir() string {
	_, file, _, _ := runtime.Caller(0)

	return filepath.Join(filepath.Dir(file), "..", "testdata", "pb_data")
}

// NewApp builds one isolated MediKube test application, and it must be called
// again for the next one.
//
// **Never share the result between two tests.ApiScenario runs.** ApiScenario
// calls apis.NewRouter on every run, apis.NewRouter binds a fresh OnServe
// handler through bindUIExtensions (apis/extensions.go:24) with no Id, and
// hook.Bind only replaces a handler when an Id was supplied — with none it
// appends (tools/hook/hook.go:94). The chain therefore grows by one handler per
// scenario and is executed by nested e.Next() calls, which are real stack
// frames rather than a loop, so a shared app runs a deeper stack on every
// scenario until the goroutine stack limit ends the process. app_test.go
// measures both halves of that.
//
// The cost of not sharing is roughly ten milliseconds: the fixture is copied
// into a temp directory and PocketBase applies whatever migrations the clone is
// missing.
func NewApp(t testing.TB) *tests.TestApp {
	t.Helper()

	return NewAppWith(t)
}

// NewAppWith is NewApp plus whatever this phase's caller needs bound to the
// serve event — the route registry, a middleware under test. Later phases wire
// their registry through here rather than editing NewApp, so the never-shared
// guarantee has one implementation.
func NewAppWith(t testing.TB, binders ...ServeBinder) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp(FixtureDir())
	require.NoErrorf(t, err, "cloning the fixture at %s: is it committed?", FixtureDir())

	// Cleanup is registered even though tests.ApiScenario defers its own: the
	// second call is a no-op, because ResetBootstrapState nils each connection
	// after closing it and os.RemoveAll of a removed directory succeeds. Without
	// it, every NewApp used outside a scenario leaks a temp directory.
	t.Cleanup(app.Cleanup)

	// One handler for all of them, and it is what calls se.Next(). A binder
	// registers and returns; the chain is the caller's to continue, which is
	// the same contract internal/platform/pb's BindServe holds its RouteBinder
	// to. A binder bound directly would stop the chain and the scenario's own
	// terminal function — the one that sends the request — would never run.
	if len(binders) > 0 {
		app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
			Id:       serveHookID,
			Priority: serveHookPriority,
			Func: func(se *core.ServeEvent) error {
				for _, binder := range binders {
					if err := binder.Bind(se); err != nil {
						return err
					}
				}

				return se.Next()
			},
		})
	}

	return app
}

// NewAppFactory is NewAppWith as a tests.ApiScenario TestAppFactory. The
// binders are captured once and a new app is still built per call, which is the
// property the field's contract requires and the one that is easy to lose when
// a caller reaches for a closure over an app it already has.
func NewAppFactory(binders ...ServeBinder) func(testing.TB) *tests.TestApp {
	return func(t testing.TB) *tests.TestApp {
		t.Helper()

		return NewAppWith(t, binders...)
	}
}
