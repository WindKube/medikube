package pb_test

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/cron"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/logging"
	"medikube/internal/platform/pb"
)

// purger is the audit retention purge, recorded rather than performed. It is
// hand-written because internal/platform/pb is on the other side of the seam:
// what this file proves is that the schedule reaches the purge and that one
// tick is bounded, correlated and reported, none of which is a property of
// which rows leave the trail.
type purger struct {
	mu sync.Mutex

	calls     int
	removed   int
	err       error
	block     chan struct{}
	panicWith any

	// contexts is every context a tick handed the purge, so a test can assert
	// on what the tick did to it after the tick has returned.
	contexts []context.Context
}

func (p *purger) Purge(ctx context.Context) (int, error) {
	p.mu.Lock()
	p.calls++
	block := p.block
	err := p.err
	removed := p.removed
	panicWith := p.panicWith
	p.contexts = append(p.contexts, ctx)
	p.mu.Unlock()

	if panicWith != nil {
		panic(panicWith)
	}

	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	return removed, err
}

func (p *purger) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.calls
}

func (p *purger) Context(index int) context.Context {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.contexts[index]
}

// lines collects the one stream, so an assertion can read what an operator
// would read rather than what the code intended to say.
type lines struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.b.Write(p)
}

func (l *lines) records(t *testing.T) []map[string]any {
	t.Helper()

	l.mu.Lock()
	raw := l.b.String()
	l.mu.Unlock()

	var out []map[string]any

	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}

		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record), "the job wrote a line that is not JSON: %s", line)

		out = append(out, record)
	}

	return out
}

func cronApp(t *testing.T) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp()
	require.NoError(t, err, "boot a throwaway PocketBase instance")
	t.Cleanup(app.Cleanup)

	return app
}

// bindCron wires one instance and hands back the job the scheduler holds.
func bindCron(t *testing.T, app *tests.TestApp, opts pb.CronOptions) *cron.Job {
	t.Helper()

	require.NoError(t, pb.BindCron(app, opts))

	for _, job := range app.Cron().Jobs() {
		if job.Id() == pb.AuditRetentionJobID {
			return job
		}
	}

	t.Fatalf("%s is not among the scheduled jobs after binding it", pb.AuditRetentionJobID)

	return nil
}

func TestTheRetentionPurgeIsScheduledUnderItsPublishedIdentity(t *testing.T) {
	t.Parallel()

	app := cronApp(t)
	purge := &purger{}

	job := bindCron(t, app, pb.CronOptions{Retention: purge, Log: zerolog.Nop()})

	assert.Equal(t, pb.AuditRetentionJobID, job.Id())
	assert.Equal(t, pb.AuditRetentionSchedule, job.Expression())

	// PocketBase's five own jobs are all prefixed __pb. A collision would make
	// cron.Add delete theirs and MediKube would silently stop PocketBase's
	// WAL checkpoint or its log cleanup.
	assert.False(t, strings.HasPrefix(pb.AuditRetentionJobID, "__pb"),
		"MediKube's job id is in PocketBase's own namespace, where adding it deletes one of theirs")
}

// The schedule is a real daily 03:17 and not an expression that parses and is
// then never due — which is what a purge that quietly never runs looks like
// from every other angle.
func TestTheScheduleIsDueOnceADayAtTheHourItClaims(t *testing.T) {
	t.Parallel()

	schedule, err := cron.NewSchedule(pb.AuditRetentionSchedule)
	require.NoError(t, err)

	day := time.Date(2026, time.March, 14, 0, 0, 0, 0, time.UTC)

	due := 0
	minutes := 0

	// A whole day, minute by minute. Exactly one of them is the purge.
	for offset := range 24 * 60 {
		minutes++

		if schedule.IsDue(cron.NewMoment(day.Add(time.Duration(offset) * time.Minute))) {
			due++

			assert.Equal(t, 3, day.Add(time.Duration(offset)*time.Minute).Hour())
			assert.Equal(t, 17, day.Add(time.Duration(offset)*time.Minute).Minute())
		}
	}

	require.Equal(t, 24*60, minutes, "the walk did not cover a day, so counting one due minute in it means nothing")
	assert.Equal(t, 1, due, "the purge is due %d times a day", due)
}

func TestATickRunsThePurgeAndReportsWhatLeftTheTrail(t *testing.T) {
	t.Parallel()

	app := cronApp(t)
	stream := &lines{}
	purge := &purger{removed: 41}

	job := bindCron(t, app, pb.CronOptions{Retention: purge, Log: zerolog.New(stream)})

	job.Run()

	require.Equal(t, 1, purge.Calls())

	records := stream.records(t)
	require.Len(t, records, 1, "one tick wrote %d lines", len(records))

	assert.Equal(t, "purged the audit trail past its retention horizon", records[0][zerolog.MessageFieldName])
	assert.Equal(t, float64(41), records[0]["removed"])
	assert.Equal(t, pb.AuditRetentionJobID, records[0]["job"])
}

// FR-054 for a row nobody requested. The handle is the same field and the same
// shape a request's line carries, so an operator greps for one thing.
var handleShape = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestATickIsCorrelatedTheWayARequestIs(t *testing.T) {
	t.Parallel()

	app := cronApp(t)
	stream := &lines{}

	job := bindCron(t, app, pb.CronOptions{Retention: &purger{}, Log: zerolog.New(stream)})

	job.Run()
	job.Run()

	records := stream.records(t)
	require.Len(t, records, 2)

	first, ok := records[0][logging.CorrelationField].(string)
	require.True(t, ok, "the tick's line carries no %s at all, so it joins to nothing", logging.CorrelationField)
	assert.Regexp(t, handleShape, first)

	second, _ := records[1][logging.CorrelationField].(string)
	assert.NotEqual(t, first, second, "two nights of purging carry one handle, so their lines cannot be told apart")
}

func TestAFailedPurgeIsReportedRatherThanPassedOverInSilence(t *testing.T) {
	t.Parallel()

	app := cronApp(t)
	stream := &lines{}

	job := bindCron(t, app, pb.CronOptions{
		Retention: &purger{err: errors.New("the trail is unreachable")},
		Log:       zerolog.New(stream),
	})

	job.Run()

	records := stream.records(t)
	require.Len(t, records, 1)

	assert.Equal(t, "error", records[0][zerolog.LevelFieldName])
	assert.Contains(t, records[0][zerolog.ErrorFieldName], "the trail is unreachable")
}

// PocketBase runs a due job through routine.FireAndForget, whose own recover
// writes the panic and its stack to the standard log package — outside the one
// stream (Principle VI). The job has to catch its own first.
func TestAPanickingPurgeIsReportedInsideTheOneStream(t *testing.T) {
	t.Parallel()

	app := cronApp(t)
	stream := &lines{}

	job := bindCron(t, app, pb.CronOptions{
		Retention: &purger{panicWith: "the trail vanished"},
		Log:       zerolog.New(stream),
	})

	assert.NotPanics(t, job.Run, "the panic escaped into PocketBase's recover, which logs outside zerolog")

	records := stream.records(t)
	require.Len(t, records, 1)

	assert.Equal(t, "error", records[0][zerolog.LevelFieldName])
	assert.Equal(t, "the trail vanished", records[0]["panic"])
	assert.Regexp(t, handleShape, records[0][logging.CorrelationField])
}

func TestATickIsBoundedSoAHungPurgeDoesNotHoldTheWriteLockAllNight(t *testing.T) {
	t.Parallel()

	app := cronApp(t)
	stream := &lines{}
	purge := &purger{block: make(chan struct{})}

	job := bindCron(t, app, pb.CronOptions{
		Retention: purge,
		Timeout:   20 * time.Millisecond,
		Log:       zerolog.New(stream),
	})

	// In a goroutine, and asserted to have returned: the failure this guards
	// against is a tick that never ends, and a test that called Run inline
	// would hang on it rather than fail. A gate that hangs is a gate nobody
	// reads the output of.
	returned := make(chan struct{})

	go func() {
		defer close(returned)

		job.Run()
	}()

	select {
	case <-returned:
	case <-time.After(10 * time.Second):
		t.Fatal("the tick never ended; a purge that hangs holds SQLite's only write lock until the process is killed")
	}

	require.Equal(t, 1, purge.Calls())
	assert.ErrorIs(t, purge.Context(0).Err(), context.DeadlineExceeded,
		"the tick handed the purge a context with no deadline on it")

	records := stream.records(t)
	require.Len(t, records, 1)
	assert.Equal(t, "error", records[0][zerolog.LevelFieldName])
}

// A purge still running when the instance is going down holds SQLite's single
// write lock while everything else is trying to leave.
func TestShutdownCancelsAPurgeInFlight(t *testing.T) {
	t.Parallel()

	app := cronApp(t)
	purge := &purger{block: make(chan struct{})}

	job := bindCron(t, app, pb.CronOptions{
		Retention: purge,
		Timeout:   time.Hour,
		Log:       zerolog.Nop(),
	})

	running := make(chan struct{})

	go func() {
		close(running)
		job.Run()
	}()

	<-running

	require.Eventually(t, func() bool { return purge.Calls() == 1 }, time.Second, time.Millisecond,
		"the purge never started, so cancelling it proves nothing")

	require.NoError(t, app.OnTerminate().Trigger(
		&core.TerminateEvent{App: app},
		func(e *core.TerminateEvent) error { return nil },
	))

	assert.Eventually(t, func() bool { return errors.Is(purge.Context(0).Err(), context.Canceled) },
		time.Second, time.Millisecond,
		"the instance terminated and the purge kept the write lock")
}

func TestBindingTwiceLeavesOneJobRatherThanTwo(t *testing.T) {
	t.Parallel()

	app := cronApp(t)
	purge := &purger{}

	before := len(app.Cron().Jobs())

	bindCron(t, app, pb.CronOptions{Retention: purge, Log: zerolog.Nop()})
	bindCron(t, app, pb.CronOptions{Retention: purge, Log: zerolog.Nop()})

	mine := 0

	for _, job := range app.Cron().Jobs() {
		if job.Id() == pb.AuditRetentionJobID {
			mine++
		}
	}

	assert.Equal(t, 1, mine, "the trail would be purged twice a night, by two jobs neither of which knows about the other")
	assert.Equal(t, before+1, len(app.Cron().Jobs()), "binding twice disturbed PocketBase's own jobs")
}

func TestBindCronRefusesAScheduleThatWouldPurgeNothing(t *testing.T) {
	t.Parallel()

	app := cronApp(t)

	for name, opts := range map[string]pb.CronOptions{
		"no purge at all": {Retention: nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Error(t, pb.BindCron(app, opts))

			for _, job := range app.Cron().Jobs() {
				assert.NotEqual(t, pb.AuditRetentionJobID, job.Id(),
					"the binding was refused and the job was scheduled anyway")
			}
		})
	}

	require.Error(t, pb.BindCron(nil, pb.CronOptions{Retention: &purger{}}))
}
