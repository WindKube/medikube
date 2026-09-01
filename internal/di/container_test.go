package di_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/samber/do/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
	"medikube/internal/di"
)

// testDeps is what the composition root would hand the container, with the
// logger pointed at a buffer the test can read.
func testDeps(tb testing.TB) (di.Deps, *syncBuffer) {
	tb.Helper()

	var sink syncBuffer

	return di.Deps{
		Config: config.Config{
			Env:       "development",
			DataDir:   tb.TempDir(),
			HTTPAddr:  "127.0.0.1:0",
			PublicURL: "http://127.0.0.1:8090",
		},
		Logger: zerolog.New(&sink).Level(zerolog.DebugLevel),
	}, &sink
}

// The gate T130 exists for. A provider nothing reaches is dead wiring: it
// compiles, it lints, and the service it describes is never built — so the
// mistake surfaces as a nil dereference on somebody's first request instead of
// here.
//
// samber/do registers providers blindly and resolves lazily, so neither
// building the injector nor listing its services proves anything on its own
// (do/v2 v2.1.0: Provide never runs the constructor; HealthCheckNamed on an
// un-invoked service returns nil without instantiating it, and the
// resolve-everything-by-name helper is unexported). The container therefore
// resolves every root itself and compares the two sets, and this asserts the
// same thing from outside.
func TestEveryProviderTheContainerDeclaresIsAlsoResolved(t *testing.T) {
	t.Parallel()

	deps, _ := testDeps(t)

	container, err := di.New(deps)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, container.Shutdown()) })

	require.NotEmpty(t, container.Provided(), "a container that provides nothing is not wiring, it is a struct")
	assert.ElementsMatch(t, container.Provided(), container.Invoked(),
		"a provider nothing resolves is dead wiring; a service resolved from nowhere is impossible")
}

func TestEveryServiceTheContainerHandsOutIsUsable(t *testing.T) {
	t.Parallel()

	deps, sink := testDeps(t)

	container, err := di.New(deps)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, container.Shutdown()) })

	t.Run("config is the one the root validated", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, deps.Config, container.Config())
	})

	t.Run("logger writes to the one stream", func(t *testing.T) {
		t.Parallel()

		logger := container.Logger()
		logger.Info().Msg("from the container")
		assert.Contains(t, sink.String(), "from the container")
	})

	t.Run("hub is live and empty", func(t *testing.T) {
		t.Parallel()

		hub := container.Hub()
		require.NotNil(t, hub)
		assert.Equal(t, 0, hub.Subscribers())

		// Discarded deliberately: nothing publishes here, and the point is
		// that the hub the container handed out is the live one. The
		// subscription goes away with the subtest's context.
		_ = hub.Subscribe(t.Context())
		assert.Equal(t, 1, hub.Subscribers())
	})

	t.Run("record registry is live and carries no kind yet", func(t *testing.T) {
		t.Parallel()

		registry := container.Records()
		require.NotNil(t, registry)
		assert.Empty(t, registry.Kinds(), "no kind is registered until a service exists to serve it")
	})

	// internal/records/recordstest exists so a kind can be exercised without
	// one. A production container that carried it would give the fake kind
	// real routes, real pages and a real OpenAPI branch.
	t.Run("no synthetic kind is ever in a production container", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, container.Records().SyntheticKinds())
	})
}

// plan.md forbids package-level globals and init() registration precisely so
// that two containers cannot see each other. A shared registry would make the
// first test that registered a kind change the behaviour of every later one,
// and the failure would move with the shuffle seed.
func TestTwoContainersShareNothing(t *testing.T) {
	t.Parallel()

	depsA, _ := testDeps(t)
	depsB, _ := testDeps(t)

	first, err := di.New(depsA)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, first.Shutdown()) })

	second, err := di.New(depsB)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, second.Shutdown()) })

	assert.NotSame(t, first.Hub(), second.Hub())
	assert.NotSame(t, first.Records(), second.Records())

	_ = first.Hub().Subscribe(t.Context())
	require.Equal(t, 1, first.Hub().Subscribers())
	assert.Equal(t, 0, second.Hub().Subscribers(), "a subscriber on one container's hub is invisible to the other")
}

func TestShutdownReleasesTheServicesThatHoldSomething(t *testing.T) {
	t.Parallel()

	deps, _ := testDeps(t)

	container, err := di.New(deps)
	require.NoError(t, err)

	hub := container.Hub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := hub.Subscribe(ctx)
	require.Equal(t, 1, hub.Subscribers())

	require.NoError(t, container.Shutdown())

	// Non-blocking: do.Shutdown returns only once every service is down, so a
	// container that did not reach the hub fails here and says so instead of
	// parking this goroutine until the package timeout.
	select {
	case _, open := <-events:
		assert.False(t, open, "the container's shutdown reached the hub, so no stream is left waiting on a process that has gone")
	default:
		t.Fatal("the container shut down and the hub's subscription is still open: nothing released it")
	}

	assert.Equal(t, 0, hub.Subscribers())

	assert.NoError(t, container.Shutdown(), "a second shutdown is a no-op: a signal handler and the root may both reach it")
}

// The container's own chatter is a log line like any other, and Principle VI
// admits exactly one stream. samber/do defaults its Logf to a no-op, which is
// silent rather than wrong — but silent wiring is what T130 exists to end.
func TestTheContainersOwnLinesGoToTheInjectedLogger(t *testing.T) {
	t.Parallel()

	deps, sink := testDeps(t)

	container, err := di.New(deps)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, container.Shutdown()) })

	assert.Contains(t, sink.String(), "DI:", "the injector's own lines reach the one stream")
	assert.Contains(t, sink.String(), `"level":"debug"`, "and at debug, so they are off by default in production")
}

// The three library behaviours the container's eager resolution stands on,
// pinned against a graph built here rather than against MediKube's own. If any
// of them changes, New's guarantee — a wiring mistake is a boot failure and a
// test failure, never a first-request failure — is no longer true, and this
// says so in its own failure message rather than leaving New quietly weaker.
//
// Every subtest below builds its own do.New(). None of them touches
// MediKube's container and none of them would notice a cycle in it: a real
// Hub <-> Registry cycle in providers.go leaves all three green. The test
// that guards MediKube's own graph is
// TestMediKubesOwnServiceGraphHasNoCycleInIt.
func TestTheWiringMistakesSamberDoReportsAtResolutionTime(t *testing.T) {
	t.Parallel()

	// Four distinct types, because samber/do keys its services by type name.
	// They are empty: what is being resolved is the graph, not a value.
	type (
		leaf    struct{}
		missing struct{}
		left    struct{}
		right   struct{}
	)

	t.Run("a provider that cannot build reports at resolution, not at registration", func(t *testing.T) {
		t.Parallel()

		injector := do.New()
		do.Provide(injector, func(do.Injector) (*leaf, error) {
			return nil, assert.AnError
		})

		_, err := do.Invoke[*leaf](injector)
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("a dependency nobody provides reports at resolution", func(t *testing.T) {
		t.Parallel()

		injector := do.New()
		do.Provide(injector, func(i do.Injector) (*leaf, error) {
			_, err := do.Invoke[*missing](i)

			return &leaf{}, err
		})

		_, err := do.Invoke[*leaf](injector)
		require.ErrorIs(t, err, do.ErrServiceNotFound)
	})

	t.Run("a cycle in a graph built here reports at resolution and names the path", func(t *testing.T) {
		t.Parallel()

		injector := do.New()
		do.Provide(injector, func(i do.Injector) (*left, error) {
			_, err := do.Invoke[*right](i)

			return &left{}, err
		})
		do.Provide(injector, func(i do.Injector) (*right, error) {
			_, err := do.Invoke[*left](i)

			return &right{}, err
		})

		_, err := do.Invoke[*left](injector)
		require.ErrorIs(t, err, do.ErrCircularDependency)
		assert.Contains(t, err.Error(), "left")
		assert.Contains(t, err.Error(), "right")
	})
}

// The assertion the subtest above only looks like it makes: MediKube's OWN
// service graph is acyclic.
//
// The protection is New's eager resolution — samber/do reports a cycle when
// something resolves it, and nothing in production resolves anything until a
// request arrives, so a graph nobody resolved at boot is a graph whose cycle
// is discovered by whoever makes the first request at whatever hour that is.
// New resolving every root is what turns that into a boot failure, and this is
// what says so about MediKube's graph rather than about a pair of types
// invented for the occasion.
//
// It bites: introduce a real cycle between provideHub and provideRecordRegistry
// in providers.go and this fails naming both.
func TestMediKubesOwnServiceGraphHasNoCycleInIt(t *testing.T) {
	t.Parallel()

	deps, _ := testDeps(t)

	container, err := di.New(deps)

	// Ahead of the NoError, so that a cycle is reported as a cycle rather than
	// as whatever wrapped message the failure happens to carry.
	assert.NotErrorIs(t, err, do.ErrCircularDependency,
		"MediKube's own service graph has a cycle in it; samber/do names the path in the error")
	require.NoError(t, err)

	t.Cleanup(func() { assert.NoError(t, container.Shutdown()) })
}

// A container is built once, at boot, by the composition root. Nothing else in
// MediKube may reach for one, because a package that resolves its own
// dependencies has hidden them (plan.md's dependency-inversion rule and
// internal/di/doc.go's own promise).
func TestOnlyTheCompositionRootReachesForTheContainer(t *testing.T) {
	t.Parallel()

	importers := packagesImporting(t, "medikube/internal/di")

	assert.Equal(t, []string{"cmd/medikube"}, importers,
		"internal/di is wired from the composition root and nowhere else")
	assert.NotContains(t, strings.Join(importers, " "), "internal/web")
}
