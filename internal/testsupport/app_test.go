package testsupport

import (
	"runtime"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

// T087. Two claims, and the second is the reason this file exists.

func TestTwoFactoryCallsProduceIndependentApps(t *testing.T) {
	t.Parallel()

	first := NewApp(t)
	second := NewApp(t)

	require.NotEqual(t, first.DataDir(), second.DataDir(),
		"both apps are working on the same directory, so neither is isolated")

	// A destructive change in one must be invisible in the other. Deleting is
	// the sharpest form of the question: a test that leaves an account without
	// its records would otherwise poison every test that ran after it.
	record, err := first.FindRecordById(kind.Medication.Collection(), NameOnlyMedicationID)
	require.NoError(t, err)
	require.NoError(t, first.Delete(record))

	_, err = first.FindRecordById(kind.Medication.Collection(), NameOnlyMedicationID)
	require.Error(t, err, "the delete did not take, so the rest of this proves nothing")

	_, err = second.FindRecordById(kind.Medication.Collection(), NameOnlyMedicationID)
	assert.NoError(t, err, "a delete in one app reached the other")

	// Tearing one down closes its connections and removes its directory. The
	// other must still answer.
	//
	// This is also the proof that NewApp's registered cleanup is safe alongside
	// the one tests.ApiScenario defers: t.Cleanup runs Cleanup on this app a
	// second time when the test ends, and the run is green under -race.
	first.Cleanup()

	_, err = second.FindRecordById(kind.Medication.Collection(), NameOnlyMedicationID)
	assert.NoError(t, err, "cleaning up one app broke the other")
}

func TestTheClonedFixtureCarriesMediKubesOwnSchema(t *testing.T) {
	t.Parallel()

	app := NewApp(t)

	// The blank import of internal/store/migrations in app.go is what puts
	// MediKube's migrations into core.AppMigrations. Without it the clone comes
	// up on PocketBase's stock schema and every later test passes against a
	// database with nothing in it to own.
	for _, name := range []string{usersCollection, kind.Medication.Collection(), "audit_events"} {
		collection, err := app.FindCollectionByNameOrId(name)
		require.NoErrorf(t, err, "%s is missing: are the migrations registered?", name)

		// The lockdown at the schema layer, which is what makes every one of
		// these tests meaningful: PocketBase's own record API cannot reach a
		// collection whose rules are nil (constitution Principle V).
		assert.Nil(t, collection.ListRule, "%s.ListRule", name)
		assert.Nil(t, collection.ViewRule, "%s.ViewRule", name)
		assert.Nil(t, collection.CreateRule, "%s.CreateRule", name)
		assert.Nil(t, collection.UpdateRule, "%s.UpdateRule", name)
		assert.Nil(t, collection.DeleteRule, "%s.DeleteRule", name)
	}
}

// TestSharingOneAppAcrossScenariosGrowsTheServeChainWithoutBound is the guard.
//
// tests.ApiScenario calls apis.NewRouter on every run (tests/api.go:189).
// apis.NewRouter calls bindUIExtensions, which binds an OnServe handler with no
// Id (apis/extensions.go:24-26), and hook.Bind only *replaces* when an Id was
// supplied — with none it appends (tools/hook/hook.go:94-96). The chain is then
// executed by nested e.Next() calls (tools/hook/event.go:30-35), which are real
// stack frames and not a loop.
//
// So a shared app runs a deeper stack on every scenario, without bound, until
// the goroutine stack limit ends the process:
//
//	apis.bindUIExtensions.func1 -> hook.(*Event).Next -> hook.Trigger.func1 -> ...
//	runtime: goroutine stack exceeds 1000000000-byte limit
//
// Asserting the overflow itself would need a gigabyte and a subprocess. What is
// asserted instead is the two quantities that cause it — the handler count and
// the executed stack depth — because they are the same defect measured before
// it becomes fatal, and they fail loudly the day upstream adds an Id.
func TestSharingOneAppAcrossScenariosGrowsTheServeChainWithoutBound(t *testing.T) {
	t.Parallel()

	const runs = 8

	shared := NewApp(t)

	// Measured after one run, not before it: the first run is what installs the
	// probe, and counting that as accumulation would overstate the effect by
	// one.
	depthBefore := serveOnce(t, shared)
	handlersBefore := shared.OnServe().Length()

	for range runs {
		serveOnce(t, shared)
	}

	handlersAfter := shared.OnServe().Length()
	depthAfter := serveOnce(t, shared)

	assert.Equal(t, handlersBefore+runs, handlersAfter,
		"a shared app should have accumulated one OnServe handler per scenario")
	assert.Greater(t, depthAfter, depthBefore,
		"the accumulated handlers should be executing on a deeper stack; if this "+
			"is now equal, PocketBase gave bindUIExtensions an Id and this guard can be retired")

	// And the fix, measured the same way: a new app per call is flat.
	firstFactoryDepth := serveOnce(t, NewApp(t))
	for range runs {
		lastFactoryDepth := serveOnce(t, NewApp(t))
		assert.Equal(t, firstFactoryDepth, lastFactoryDepth,
			"a new app per call must not accumulate anything")
	}
}

func TestNewAppFactoryBuildsANewAppPerCall(t *testing.T) {
	t.Parallel()

	factory := NewAppFactory()

	// It has to be assignable to the field itself, not merely to a function of
	// the same shape: that assignment is how every HTTP test in five phases
	// gets a fresh app, and a caller who cannot make it would reach for a
	// closure over one app they already had.
	scenario := tests.ApiScenario{TestAppFactory: factory}
	require.NotNil(t, scenario.TestAppFactory)

	first := factory(t)
	second := factory(t)

	assert.NotEqual(t, first.DataDir(), second.DataDir())
	assert.NotSame(t, first, second)
}

func TestABinderIsRunAndTheChainContinues(t *testing.T) {
	t.Parallel()

	var bound int
	app := NewAppWith(t, binderFunc(func(se *core.ServeEvent) error {
		bound++

		return nil
	}))

	var reachedTerminal bool

	router, err := apis.NewRouter(app)
	require.NoError(t, err)

	event := new(core.ServeEvent)
	event.App = app
	event.Router = router

	require.NoError(t, app.OnServe().Trigger(event, func(e *core.ServeEvent) error {
		reachedTerminal = true

		return nil
	}))

	assert.Equal(t, 1, bound, "the binder should have run exactly once")
	assert.True(t, reachedTerminal,
		"the harness must call se.Next() for its binders; a binder that stops the "+
			"chain would stop the scenario's own request from ever being sent")
}

type binderFunc func(se *core.ServeEvent) error

func (f binderFunc) Bind(se *core.ServeEvent) error { return f(se) }

// serveOnce does what one tests.ApiScenario run does — build a router, trigger
// OnServe — and reports the depth of the goroutine stack the chain executed on.
func serveOnce(t *testing.T, app core.App) int {
	t.Helper()

	router, err := apis.NewRouter(app)
	require.NoError(t, err)

	var depth int

	// A fixed Id, so the probe replaces itself rather than becoming another
	// instance of the accumulation it is measuring. The priority puts it last,
	// after every handler whose frames are being counted.
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Id:       "medikubeServeChainProbe",
		Priority: 1 << 20,
		Func: func(e *core.ServeEvent) error {
			frames := make([]uintptr, 4096)
			depth = runtime.Callers(0, frames)

			return e.Next()
		},
	})

	event := new(core.ServeEvent)
	event.App = app
	event.Router = router

	require.NoError(t, app.OnServe().Trigger(event, func(e *core.ServeEvent) error { return nil }))

	return depth
}
