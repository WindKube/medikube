package api

import (
	"context"
	"net/http"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/rs/zerolog"

	"medikube/internal/httproute"
	"medikube/internal/obs"
	"medikube/internal/web"
)

// The operation ids of contracts/health.md's two operations.
const (
	OpHealthz = "healthz"
	OpReadyz  = "readyz"
)

// databaseCheckDeadline is the 2-second bound contracts/health.md requires on
// the one query readyz runs.
const databaseCheckDeadline = 2 * time.Second

// checkOK and checkError are the closed check vocabulary contracts/health.md
// requires — no message field, ever.
const (
	checkOK    = "ok"
	checkError = "error"
)

// statusReady, statusNotReady and statusDraining are readyz's three top-level
// answers.
const (
	statusReady     = "ready"
	statusNotReady  = "not_ready"
	statusDraining  = "draining"
	statusProcessOK = "ok"
)

// PendingMigrations is internal/store/migrations.Pending's signature, handed
// in rather than imported directly: importing that package here would
// register MediKube's migrations globally (core.AppMigrations), which
// internal/platform/pb's tests rely on NOT happening.
type PendingMigrations func(app core.App) ([]string, error)

// HealthDeps is what healthz and readyz need from the composition root.
// Nothing here is a database or filesystem handle (FR-052).
type HealthDeps struct {
	// Version is the build stamp (-ldflags -X main.version).
	Version string
	// StartedAt is when this process began.
	StartedAt time.Time
	// Readiness is the shared drain flag. Nil answers as though never draining.
	Readiness *obs.Readiness
	// Pending is internal/store/migrations.Pending. Nil answers as though the
	// schema were current.
	Pending PendingMigrations
}

// healthResponse is healthz's whole body (contracts/health.md).
type healthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	StartedAt string `json:"started_at"`
}

// readyResponse is readyz's whole body. Checks is never nil so that the
// draining case's "empty check set" marshals as `{}` and not `null`.
type readyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// HealthOperations is the two operation ids this file serves.
func HealthOperations() []string {
	return []string{OpHealthz, OpReadyz}
}

// HealthHandlers wires healthz and readyz.
func HealthHandlers(deps HealthDeps) httproute.Handlers {
	return httproute.Handlers{
		OpHealthz: healthz(deps),
		OpReadyz:  readyz(deps),
	}
}

// healthz answers about the process and nothing else: no database, no
// filesystem.
func healthz(deps HealthDeps) httproute.Handler {
	return func(e *core.RequestEvent) error {
		return web.WriteJSON(e, http.StatusOK, healthResponse{
			Status:    statusProcessOK,
			Version:   deps.Version,
			StartedAt: deps.StartedAt.UTC().Format(time.RFC3339),
		})
	}
}

// readyz answers whether the instance can serve: database, migrations,
// storage. Draining is checked first and short-circuits the rest.
func readyz(deps HealthDeps) httproute.Handler {
	return func(e *core.RequestEvent) error {
		if deps.Readiness.Draining() {
			return web.WriteJSON(e, http.StatusServiceUnavailable, readyResponse{
				Status: statusDraining,
				Checks: map[string]string{},
			})
		}

		checks := map[string]string{
			"database":   checkOK,
			"migrations": checkOK,
			"storage":    checkOK,
		}

		ready := true

		if err := checkDatabase(e.Request.Context(), e.App.ConcurrentDB()); err != nil {
			checks["database"] = checkError
			ready = false

			logCheckFailure(e, "database", err)
		}

		if err := checkMigrations(e.App, deps.Pending); err != nil {
			checks["migrations"] = checkError
			ready = false

			if !isOutstandingMigrations(err) {
				logCheckFailure(e, "migrations", err)
			}
		}

		if err := checkStorage(e.App); err != nil {
			checks["storage"] = checkError
			ready = false

			logCheckFailure(e, "storage", err)
		}

		status := statusReady
		code := http.StatusOK

		if !ready {
			status = statusNotReady
			code = http.StatusServiceUnavailable
		}

		return web.WriteJSON(e, code, readyResponse{Status: status, Checks: checks})
	}
}

// checkDatabase runs a real statement rather than a Ping, which on SQLite
// proves nothing.
func checkDatabase(ctx context.Context, db dbx.Builder) error {
	ctx, cancel := context.WithTimeout(ctx, databaseCheckDeadline)
	defer cancel()

	var one int

	return db.NewQuery("SELECT 1").WithContext(ctx).Row(&one)
}

// outstandingMigrations marks "schema not yet current" so it is not logged as
// an error the way a genuine query failure is.
type outstandingMigrations struct{ pending []string }

func (outstandingMigrations) Error() string {
	return "the applied migration set is not the registered set"
}

// checkMigrations reports whether the schema this build expects is the schema
// the database has.
func checkMigrations(app core.App, pendingFunc PendingMigrations) error {
	if pendingFunc == nil {
		return nil
	}

	pending, err := pendingFunc(app)
	if err != nil {
		return err
	}

	if len(pending) > 0 {
		return outstandingMigrations{pending: pending}
	}

	return nil
}

func isOutstandingMigrations(err error) bool {
	_, ok := err.(outstandingMigrations) //nolint:errorlint // a sentinel this package alone constructs and never wraps
	return ok
}

// checkStorage proves app.NewFilesystem() opens and closes cleanly.
func checkStorage(app core.App) error {
	fsys, err := app.NewFilesystem()
	if err != nil {
		return err
	}

	return fsys.Close()
}

// logCheckFailure is the one place a check's real error (a path, a DSN) may
// go — never the response body (FR-052).
func logCheckFailure(e *core.RequestEvent, check string, err error) {
	zerolog.Ctx(e.Request.Context()).Error().
		Err(err).
		Str("check", check).
		Msg("readyz reported a failing check")
}
