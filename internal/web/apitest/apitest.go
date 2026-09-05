// Package apitest wires a whole MediKube instance for an HTTP test: the
// migrations, the seeded fixture, the medication kind and every middleware the
// composition root binds, in the order it binds them.
//
// It exists because the HTTP behaviour this phase has to prove is not a
// property of any one handler. FR-033's byte-identical refusal is produced by
// the service, the error mapper and the router together; the 401 on a record
// route comes from apis.RequireAuth, which only the real binding installs; and
// the lockdown's 404 comes from a middleware no handler can see. A test that
// assembled less than this would assert against something MediKube does not
// serve.
//
// Nothing outside a test may import it: it is a test harness, and a
// composition root that reached in here would be assembling itself out of one.
//
// It sits on the PocketBase side of the import boundary.
package apitest

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/obs"
	"medikube/internal/platform/pb"
	"medikube/internal/realtime"
	"medikube/internal/records"
	accessservice "medikube/internal/service/access"
	auditservice "medikube/internal/service/audit"
	"medikube/internal/service/medication"
	"medikube/internal/store"
	auditstore "medikube/internal/store/audit"
	storemedication "medikube/internal/store/medication"
	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/page"
	"medikube/internal/web/stream"
)

// Instance is one assembled MediKube, ready to be driven.
type Instance struct {
	App     *tests.TestApp
	Records *records.Handler
	Routes  *httproute.Registry

	// Hub is the one this instance's post-commit hooks publish to and its
	// stream handler subscribes to. A test that held its own would be
	// asserting against a fan-out nothing writes to.
	Hub *realtime.Hub

	// Accounts is the assembled identity stack: the service the auth and
	// account handlers call, and the authenticator whose comparison counter is
	// how T202 asserts the anti-enumeration mechanism rather than timing it.
	Accounts *api.Accounts
}

// Option adjusts one instance. The zero set is production's wiring; the
// overrides exist for the stream, where the production heartbeat is 25 seconds
// and a test that waited that long for its first assertion is a test nobody
// runs.
type Option func(*settings)

type settings struct {
	heartbeat time.Duration
	now       func() time.Time

	registrationOpen bool
}

// SessionTTL is MEDIKUBE_AUTH_SESSION_TTL's default, which is also what the
// users collection carries in the committed fixture. The two have to agree or
// the cookie and the token inside it would expire on different days.
const SessionTTL = 168 * time.Hour

// PublicURL is deliberately not a loopback address, so the session cookie is
// minted with Secure exactly as it is in a deployment.
const PublicURL = "https://medikube.example.test"

// AuditRetentionDays is MEDIKUBE_RETENTION_AUDIT_DAYS's default: two years
// (FR-037). The harness schedules the purge on the same horizon the binary
// does, because a job bound here on a different one would be a job the suite
// proves and the deployment does not have.
const AuditRetentionDays = 730

// WithRegistrationOpen opens self-registration, which is FR-002's operator
// switch. The zero value is closed, as MEDIKUBE_AUTH_REGISTRATION_OPEN is.
func WithRegistrationOpen(open bool) Option {
	return func(s *settings) { s.registrationOpen = open }
}

// WithStreamHeartbeat shortens the interval between $stream_beat frames.
func WithStreamHeartbeat(interval time.Duration) Option {
	return func(s *settings) { s.heartbeat = interval }
}

// WithStreamClock replaces the clock the heartbeat reads, so a test can assert
// the value on the wire rather than that it parses.
func WithStreamClock(now func() time.Time) Option {
	return func(s *settings) { s.now = now }
}

// New builds one isolated instance. Call it again for the next one: a
// tests.TestApp shared between two ApiScenario runs accumulates an OnServe
// handler per run and recurses until the goroutine stack ends the process
// (testsupport.NewApp documents the mechanism).
func New(t testing.TB, options ...Option) *Instance {
	t.Helper()

	app := testsupport.NewApp(t)

	instance, err := Wire(app, options...)
	require.NoError(t, err, "wiring a MediKube instance")

	return instance
}

// NewPopulated is New with count extra medications on one account, written
// before anything is wired.
//
// Before, and that ordering is the point: the audit hooks are bound by Wire, so
// a bulk seed that ran after it would write one audit row per fixture row —
// which is both slow and a trail full of writes nobody made.
func NewPopulated(t testing.TB, ownerID string, count int, options ...Option) *Instance {
	t.Helper()

	app := testsupport.NewApp(t)

	require.NoError(t, Populate(app, ownerID, count), "seeding %d rows of %s", count, kind.Medication)

	instance, err := Wire(app, options...)
	require.NoError(t, err, "wiring a MediKube instance")

	return instance
}

// epoch is where the bulk rows' start dates begin walking forward from. It is
// far enough back that they never collide with the seeded fixture's own dates,
// so an ordering assertion can tell the two sets apart.
var epoch = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

// Populate writes count medications for one account in a single transaction.
//
// They are deliberately not identical: the name carries the row's ordinal and
// the start date walks backwards a day at a time, so an ordering assertion over
// a large list has something to be wrong about. The status cycles through the
// published set for the same reason.
func Populate(app core.App, ownerID string, count int) error {
	collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	if err != nil {
		return fmt.Errorf("apitest: reading %s: %w", kind.Medication, err)
	}

	statuses := clinical.TherapyStatuses()

	return store.RunInTransaction(app, func(txApp core.App) error {
		for index := range count {
			record := core.NewRecord(collection)

			day := epoch.AddDate(0, 0, index)

			started, err := domain.NewDate(day.Year(), day.Month(), day.Day())
			if err != nil {
				return err
			}

			if err := store.MedicationToRecord(record, clinical.Medication{
				OwnerID:   ownerID,
				Name:      fmt.Sprintf("Bulk %06d", index),
				Status:    statuses[index%len(statuses)],
				StartedOn: started,
			}); err != nil {
				return err
			}

			// No validation and no hooks worth speaking of: the values come
			// from the domain entity above, so the check would be of this
			// function rather than of the data.
			if err := txApp.SaveNoValidate(record); err != nil {
				return fmt.Errorf("apitest: writing bulk medication %d: %w", index, err)
			}
		}

		return nil
	})
}

// Factory is New as a tests.ApiScenario TestAppFactory: a new app per case,
// each one wired.
func Factory() func(testing.TB) *tests.TestApp {
	return func(t testing.TB) *tests.TestApp {
		t.Helper()

		return New(t).App
	}
}

// Handler is the http.Handler an instance actually serves — the built mux with
// everything the OnServe chain wrapped it in. It is what the ownership matrix
// is driven through.
func Handler(t testing.TB) http.Handler {
	t.Helper()

	return testsupport.NewEdgeHandler(t, New(t).App)
}

// Wire assembles the instance and binds it to the serve event.
//
// The order below is cmd/medikube's, and deliberately so: the request logger
// outside everything, the error envelope outside PocketBase's panic recovery,
// the actor immediately after the token load, the security headers before the
// routes.
func Wire(app *tests.TestApp, options ...Option) (*Instance, error) {
	var chosen settings
	for _, option := range options {
		option(&chosen)
	}

	hub := realtime.New()

	registry, err := registerKinds(app, hub)
	if err != nil {
		return nil, err
	}

	handler := records.NewHandler(registry)

	resolve := api.Resolve(func() (*records.Handler, error) { return handler, nil })

	accounts, err := api.NewAccounts(app, api.AccountsConfig{
		RegistrationOpen: chosen.registrationOpen,
		SessionTTL:       SessionTTL,
		PublicURL:        PublicURL,
		Resolve:          resolve,
	})
	if err != nil {
		return nil, err
	}

	table, err := handlerTable(resolve, hub, chosen, accounts)
	if err != nil {
		return nil, err
	}

	routes, err := httproute.New(table)
	if err != nil {
		return nil, fmt.Errorf("apitest: wiring the route table: %w", err)
	}

	// The same three error views the composition root binds. Wiring nil here
	// instead would leave every page-surface failure answering with the JSON
	// envelope, so a test asserting on a 404's shape would be asserting on a
	// response no browser ever receives (FR-046).
	errorPages, err := page.NewErrorPages()
	if err != nil {
		return nil, fmt.Errorf("apitest: wiring the error views: %w", err)
	}

	pb.BindServe(app, pb.ServeOptions{
		Middlewares: []*hook.Handler[*core.RequestEvent]{
			// The correlation id first, and it is not optional here: it is
			// what fills request_id on every refusal, and FR-033's
			// byte-identical comparison would be trivially satisfied by two
			// bodies that both carried an empty one.
			obs.RequestLogger(zerolog.Nop()),
			web.Errors(errorPages.Render),
			// -1021: before PocketBase's loadAuthToken, which is the whole of
			// how a browser's cookie becomes a bearer token. Without it every
			// cookie-authenticated test in the repository would be asserting
			// against an anonymous request.
			web.Sessions(),
			web.Actors(),
		},
		Routes: binders{web.SecurityBinder{}, routes},
	})

	// Bound so this harness and the binary bind the same platform hooks
	// (internal/architecture); nothing here ever triggers a slow drain since
	// no test starts the in-flight middleware.
	pb.BindDrain(app, pb.DrainOptions{
		Readiness: obs.NewReadiness(),
		Delay:     0,
		Max:       time.Second,
		Log:       zerolog.Nop(),
	})

	return &Instance{App: app, Records: handler, Routes: routes, Hub: hub, Accounts: accounts}, nil
}

// handlerTable is cmd/medikube's operations(): a 501 under every registered
// operation id, then the real handlers on top. Without the stubs
// httproute.New refuses the table, because a route nothing serves is a route
// that would panic on its first request.
func handlerTable(
	resolve api.Resolve,
	hub *realtime.Hub,
	chosen settings,
	accounts *api.Accounts,
) (httproute.Handlers, error) {
	table := make(httproute.Handlers)

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindExternal {
			continue
		}

		table[route.OpID] = notImplemented(route.OpID)
	}

	recordOps, err := api.Handlers(resolve)
	if err != nil {
		return nil, err
	}

	pageOps, err := page.Handlers(resolve)
	if err != nil {
		return nil, err
	}

	streamOps, err := stream.Handlers(stream.Deps{
		Resolve:   resolve,
		Hub:       hub,
		Heartbeat: chosen.heartbeat,
		Now:       chosen.now,
	})
	if err != nil {
		return nil, err
	}

	accountOps, err := accounts.Handlers()
	if err != nil {
		return nil, err
	}

	accountPages, err := page.AccountPages(page.AccountDeps{
		Accounts: accounts.Service,
		Counts:   accounts.Deps.Counts,
		Mail:     accounts.Deps.Mail,
		Links:    accounts.Authenticator,
	})
	if err != nil {
		return nil, err
	}

	for _, group := range []httproute.Handlers{recordOps, pageOps, streamOps, accountOps, accountPages} {
		for opID, handler := range group {
			table[opID] = handler
		}
	}

	return table, nil
}

func notImplemented(opID string) httproute.Handler {
	stub := &web.Coded{Status: http.StatusNotImplemented, Code: web.CodeInternal}

	return func(*core.RequestEvent) error {
		return fmt.Errorf("%s has no handler in this build: %w", opID, stub)
	}
}

// registerKinds builds the kind registry this instance serves. One call, seven
// consumers, no route — the same call cmd/medikube will make.
func registerKinds(app core.App, hub *realtime.Hub) (*records.Registry, error) {
	secret, err := store.CursorSecret(app, "")
	if err != nil {
		return nil, err
	}

	codec, err := store.NewCursorCodec(secret)
	if err != nil {
		return nil, err
	}

	repository, err := storemedication.New(app, codec)
	if err != nil {
		return nil, err
	}

	owners, err := store.NewOwners(app)
	if err != nil {
		return nil, err
	}

	authorizer, err := accessservice.New(owners)
	if err != nil {
		return nil, err
	}

	trail, err := auditstore.New(app)
	if err != nil {
		return nil, err
	}

	// WithRequestID, as the composition root wires it: without it a row the
	// hooks write carries a handle minted on the spot instead of the one the
	// request's own log lines carry, and FR-054's join is broken in exactly the
	// place no assertion looks (T231).
	auditor, err := auditservice.New(trail, auditservice.WithRequestID(obs.CorrelationID))
	if err != nil {
		return nil, err
	}

	views, err := page.NewMedicationViews()
	if err != nil {
		return nil, err
	}

	registry := records.NewRegistry()

	if err := medication.Register(registry, medication.Wiring{
		Repository: repository,
		Authorizer: authorizer,
		Auditor:    auditor,
		Codec:      api.MedicationCodec{},
		Schema:     api.MedicationSchema(),
		Views:      views,
	}); err != nil {
		return nil, err
	}

	// FR-037's append-only trail, bound where the binary binds it.
	if guardErr := pb.BindAuditImmutability(app); guardErr != nil {
		return nil, guardErr
	}

	// The nightly purge. It is bound and never ticks here — nothing starts the
	// scheduler under tests.TestApp — and that is exactly the point:
	// internal/architecture holds the binary and this harness to the same set of
	// platform bindings, so a hook the deployment has and the suite does not is
	// a failure rather than an omission nobody notices.
	retention, retentionErr := auditservice.NewRetention(trail, AuditRetentionDays, auditservice.SystemClock{})
	if retentionErr != nil {
		return nil, retentionErr
	}

	if cronErr := pb.BindCron(app, pb.CronOptions{Retention: retention, Log: zerolog.Nop()}); cronErr != nil {
		return nil, cronErr
	}

	// FR-036's three rows, from the post-commit hooks and never from a
	// handler. Bound after the kinds are registered, so the hook is bound to
	// exactly what this instance serves.
	if err := pb.BindRecordAudit(app, pb.RecordAudit{
		Trail:   auditor,
		Kinds:   registry.Kinds(),
		Actor:   web.ActorFrom,
		Request: obs.CorrelationID,
	}); err != nil {
		return nil, err
	}

	// FR-036's sign-in rows, from OnRecordAuthRequest and never from a handler
	// (research D-14): PocketBase's native auth route stays reachable, so a
	// handler-side audit would leave one of the two paths to a session
	// unrecorded.
	if err := pb.BindAuthAudit(app, pb.AuthAudit{
		Trail:   auditor,
		Request: obs.CorrelationID,
	}); err != nil {
		return nil, err
	}

	// contracts/streams.md's post-commit publisher, bound to the same kinds
	// for the same reason: a live view of a kind this instance does not serve
	// is a live view of nothing.
	if err := pb.BindRecordStream(app, pb.RecordStream{Hub: hub, Kinds: registry.Kinds()}); err != nil {
		return nil, err
	}

	return registry, nil
}

// Events reads the trail back, newest last, so a test can assert what one
// request recorded.
func Events(t testing.TB, app core.App) []audit.Event {
	t.Helper()

	records, err := app.FindAllRecords(store.AuditCollection)
	require.NoError(t, err)

	events := make([]audit.Event, 0, len(records))

	for _, record := range records {
		event, convertErr := store.AuditEventFromRecord(record)
		require.NoError(t, convertErr)

		events = append(events, event)
	}

	return events
}

// binders composes the several things that bind to the serve event into the
// one seam pb.ServeOptions has, exactly as cmd/medikube does.
type binders []pb.RouteBinder

func (b binders) Bind(se *core.ServeEvent) error {
	for _, binder := range b {
		if err := binder.Bind(se); err != nil {
			return err
		}
	}

	return nil
}
