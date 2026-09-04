package pb_test

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/obs"
	"medikube/internal/platform/pb"
)

// T273: the drain handler at -10000 runs before PocketBase's own shutdown at
// -9999, flips readiness, waits the delay, and waits for in-flight work.
func TestDrainRunsBeforePocketBasesShutdownAndWaitsForInFlightWork(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	readiness := obs.NewReadiness()
	readiness.Begin() // one request in flight

	const delay = 20 * time.Millisecond

	pb.BindDrain(app, pb.DrainOptions{
		Readiness: readiness,
		Delay:     delay,
		Max:       time.Second,
		Log:       zerolog.Nop(),
	})

	var drainingAtMinusNineNineNineNine bool
	var inFlightAtMinusNineNineNineNine int64

	app.OnTerminate().Bind(&hook.Handler[*core.TerminateEvent]{
		Id:       "fakePocketBaseShutdown",
		Priority: -9999,
		Func: func(e *core.TerminateEvent) error {
			drainingAtMinusNineNineNineNine = readiness.Draining()
			inFlightAtMinusNineNineNineNine = readiness.InFlight()

			return e.Next()
		},
	})

	go func() {
		time.Sleep(2 * delay)
		readiness.End()
	}()

	start := time.Now()

	terminate := new(core.TerminateEvent)
	terminate.App = app
	require.NoError(t, app.OnTerminate().Trigger(terminate, func(*core.TerminateEvent) error { return nil }))

	elapsed := time.Since(start)

	assert.True(t, readiness.Draining())
	assert.True(t, drainingAtMinusNineNineNineNine,
		"PocketBase's own shutdown ran before readiness had flipped to draining")
	assert.Zero(t, inFlightAtMinusNineNineNineNine,
		"PocketBase's own shutdown ran before the in-flight request had finished")
	assert.GreaterOrEqual(t, elapsed, delay)
}
