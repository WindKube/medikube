package pb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/rs/zerolog"

	"medikube/internal/logging"
	auditservice "medikube/internal/service/audit"
)

// AuditRetentionJobID names the purge, so a second Bind replaces it rather than
// running it twice — cron.Add deletes by id before appending
// (tools/cron/cron.go:96-99) — and so an operator reading GET /api/crons can
// tell MediKube's one job from PocketBase's five.
const AuditRetentionJobID = "medikubeAuditRetention"

// AuditRetentionSchedule is 03:17 every day.
//
// Not 03:00: PocketBase's own __pbDBOptimize__ runs at 00:00 and its log
// cleanup every six hours on the hour, and a purge that holds SQLite's single
// write lock is the last thing that should collide with a wal_checkpoint. The
// seventeen minutes are the cheapest way to say "not on the hour with everyone
// else".
const AuditRetentionSchedule = "17 3 * * *"

// DefaultCronTimeout bounds one tick.
//
// A purge that hangs holds the only write lock the database has, and every
// request that writes anything queues behind it — so an unbounded job turns a
// slow delete into an outage with no error anywhere. An hour is far more than
// the operation needs and far less than a night.
const DefaultCronTimeout = time.Hour

// cronShutdownHookID names the terminate handler that cancels a purge in
// flight.
const cronShutdownHookID = "medikubeCronShutdown"

// AuditPurge is the seam the retention job runs, declared here by the consumer.
// It takes no argument it could be pointed at: the only thing it can be told is
// that it is time.
type AuditPurge interface {
	Purge(ctx context.Context) (int, error)
}

// CronOptions is what the composition root decides.
type CronOptions struct {
	// Retention is the audit trail's purge. Required: a cron bound with no
	// purge is a trail that grows without limit while every gate stays green.
	Retention AuditPurge

	// Timeout bounds one tick. Zero means DefaultCronTimeout.
	Timeout time.Duration

	// Log is MediKube's one stream. The zero value is a logger that writes to
	// stderr, which is a build nobody wants but not a nil dereference.
	Log zerolog.Logger
}

// BindCron installs MediKube's scheduled work: the audit retention purge, and
// the shutdown that stops one in flight.
//
// PocketBase starts the scheduler from its own OnServe handler at priority 999
// (core/base.go:1412-1420), so a job added any time before the instance serves
// is running by the time it does. It is added here rather than inside OnServe
// because a job is state on the application, not on a request.
func BindCron(app core.App, opts CronOptions) error {
	if app == nil {
		return errors.New("pb: the cron is bound to no application")
	}

	if opts.Retention == nil {
		return errors.New("pb: the cron is bound with no audit retention purge, so the trail would grow without limit and every gate would stay green while it did")
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultCronTimeout
	}

	// The lifetime of the process, not of a tick. A purge started at 03:17 and
	// still running when the container is drained holds the write lock while
	// the instance is trying to leave, and SIGKILL arrives to find SQLite
	// mid-transaction.
	lifetime, stop := context.WithCancel(context.Background())

	app.OnTerminate().Bind(&hook.Handler[*core.TerminateEvent]{
		Id: cronShutdownHookID,
		Func: func(e *core.TerminateEvent) error {
			stop()

			return e.Next()
		},
	})

	job := func() { purgeAuditTrail(lifetime, opts.Retention, timeout, opts.Log) }

	if err := app.Cron().Add(AuditRetentionJobID, AuditRetentionSchedule, job); err != nil {
		return fmt.Errorf("pb: schedule %s at %q: %w", AuditRetentionJobID, AuditRetentionSchedule, err)
	}

	return nil
}

// purgeAuditTrail is one tick.
//
// It opens a run and logs under the run's handle, so the line this writes and
// any row the run produces carry the same value in the same field as a
// request's do (FR-054) — which is what lets an operator ask what happened at
// 03:17 and get one answer rather than two unjoinable halves.
//
// It recovers its own panics, and that is not belt and braces. PocketBase runs
// a due job through routine.FireAndForget, whose recover writes the panic and
// two kilobytes of stack to the standard log package (tools/routine/routine.go:
// 27-30) — straight past zerolog, as the one non-JSON thing in the process
// (Principle VI). Recovering here is what keeps the report inside the stream.
func purgeAuditTrail(parent context.Context, purge AuditPurge, timeout time.Duration, base zerolog.Logger) {
	run, id := auditservice.StartRun(parent, "")

	log := base.With().
		Str("job", AuditRetentionJobID).
		Str(logging.CorrelationField, id).
		Logger()

	defer func() {
		if recovered := recover(); recovered != nil {
			log.Error().
				Str("panic", fmt.Sprint(recovered)).
				Msg("the audit retention purge panicked")
		}
	}()

	ctx, done := context.WithTimeout(run, timeout)
	defer done()

	removed, err := purge.Purge(ctx)
	if err != nil {
		log.Error().Err(err).Msg("purge the audit trail past its retention horizon")

		return
	}

	log.Info().Int("removed", removed).Msg("purged the audit trail past its retention horizon")
}
