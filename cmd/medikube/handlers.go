package main

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sync"

	"github.com/pocketbase/pocketbase/core"
	"github.com/rs/zerolog"

	"medikube/internal/config"
	"medikube/internal/domain/access"
	"medikube/internal/domain/identity"
	"medikube/internal/httproute"
	"medikube/internal/obs"
	"medikube/internal/platform/pb"
	"medikube/internal/realtime"
	"medikube/internal/records"
	accessservice "medikube/internal/service/access"
	auditservice "medikube/internal/service/audit"
	serviceidentity "medikube/internal/service/identity"
	"medikube/internal/service/medication"
	"medikube/internal/service/patient"
	"medikube/internal/store"
	auditstore "medikube/internal/store/audit"
	medicationstore "medikube/internal/store/medication"
	patientstore "medikube/internal/store/patient"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/page"
	"medikube/internal/web/stream"
)

// operations is MediKube's handler table: the handlers this build has, and a
// 501 for every route whose handler is a later task.
//
// The stub set is derived from the route table rather than listed, so a route
// added to internal/httproute/routes.go cannot leave the composition root
// unable to boot, and a handler wired under a name that is not a route is
// still refused by httproute.New. What is *not* derived is the inventory of
// what is still missing: that lives in cmd/medikube/main_test.go, where a
// finished handler left behind a stub is a failing test and a one-line diff.
func operations(
	app core.App,
	cfg config.Config,
	resolve api.Resolve,
	hub *realtime.Hub,
	health api.HealthDeps,
) (httproute.Handlers, error) {
	table := make(httproute.Handlers)

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindExternal {
			// PocketBase's own. Binding one would shadow the real handler
			// with MediKube's, which is what httproute.Handle refuses.
			continue
		}

		table[route.OpID] = notImplemented(route.OpID)
	}

	// The real ones win. This is the only line each later group touches.
	served, err := wired(resolve, hub)
	if err != nil {
		return nil, err
	}

	maps.Copy(table, served)

	// healthz and readyz need neither the kind registry nor the application:
	// only the build stamp, the start instant and the drain flag, all of which
	// the composition root already holds by the time it calls this.
	maps.Copy(table, api.HealthHandlers(health))

	// contracts/patients.md and contracts/patient-photo.md. The stack is
	// resolved lazily (patientFamily), the same reason recordFamily is: the
	// repository needs the cursor codec, which is keyed from a secret the
	// migrations have only just created. Resolved here, ahead of the account
	// surface, so registration can provision the self-record FR-005 requires.
	patientResolve, photoResolve := patientFamily(app, cfg)

	// The account surface is assembled separately because it is the one group
	// that needs the application itself: PocketBase owns the credential, the
	// token and the mailer, so the identity stack cannot be built out of the
	// kind registry alone. api.AccountOperations is what lets unimplemented()
	// below know what this line serves without building it.
	accounts, err := api.NewAccounts(app, api.AccountsConfig{
		RegistrationOpen: cfg.Auth.RegistrationOpen,
		SessionTTL:       cfg.Auth.SessionTTL,
		PublicURL:        cfg.PublicURL,
		Resolve:          resolve,
		SelfRecord:       api.SelfRecordOf(patientResolve),
	})
	if err != nil {
		return nil, err
	}

	accountOps, err := accounts.Handlers()
	if err != nil {
		return nil, err
	}

	maps.Copy(table, accountOps)

	// The six pages of the same surface, which need the same stack: the
	// settings page renders the signed-in account, the sign-up page renders the
	// operator's switch, and the two token pages resolve the link they were
	// opened with — none of which can be built out of the kind registry.
	//
	// Links is the authenticator the identity service redeems through, and not
	// a second one: the answer this page renders has to be the answer the
	// submission will get (FR-074).
	accountPages, err := page.AccountPages(page.AccountDeps{
		Accounts: accounts.Service,
		Counts:   accounts.Deps.Counts,
		Mail:     accounts.Deps.Mail,
		Links:    accounts.Authenticator,
	})
	if err != nil {
		return nil, err
	}

	maps.Copy(table, accountPages)

	patientOps, err := api.PatientHandlers(patientResolve, unitSystemOf(accounts.Service))
	if err != nil {
		return nil, err
	}

	maps.Copy(table, patientOps)

	photoOps, err := api.PatientPhotoHandlers(photoResolve)
	if err != nil {
		return nil, err
	}

	maps.Copy(table, photoOps)

	patientPages, err := page.PatientPages(page.PatientDeps{
		Resolve: patientResolve,
		UnitOf:  unitSystemOf(accounts.Service),
	})
	if err != nil {
		return nil, err
	}

	maps.Copy(table, patientPages)

	// P3, the application root. It needs only the counter the account surface
	// already built, so it is wired here rather than in wired() alongside the
	// record family it counts through.
	overviewPage, err := page.OverviewPage(page.OverviewDeps{Counts: accounts.Deps.Counts})
	if err != nil {
		return nil, err
	}

	maps.Copy(table, overviewPage)

	// The two embedded assets every page's head links. Neither needs the
	// application: they are compiled into the binary, so they are wired here
	// rather than in wired() only because that is where every other group
	// that needs nothing lives.
	maps.Copy(table, assetHandlers())

	page.SetBuildVersion(version)

	return table, nil
}

// assetHandlers is contracts/pages.md's two KindAsset routes: the compiled
// Tailwind stylesheet and the vendored Datastar runtime, both embedded and
// served with an immutable cache header.
func assetHandlers() httproute.Handlers {
	return httproute.Handlers{
		"assetAppCSS":     web.ServeAppCSS,
		"assetDatastarJS": web.ServeDatastarJS,
	}
}

// wired is where each group's handlers arrive as they land — every group that
// can be assembled from the kind registry alone. The record family, the two
// record pages and the Datastar stream are in; the account surface needs the
// application and is assembled by operations above.
func wired(resolve api.Resolve, hub *realtime.Hub) (httproute.Handlers, error) {
	table := make(httproute.Handlers)

	records, err := api.Handlers(resolve)
	if err != nil {
		return nil, err
	}

	pages, err := page.Handlers(resolve)
	if err != nil {
		return nil, err
	}

	streams, err := stream.Handlers(stream.Deps{Resolve: resolve, Hub: hub})
	if err != nil {
		return nil, err
	}

	maps.Copy(table, records)
	maps.Copy(table, pages)
	maps.Copy(table, streams)

	return table, nil
}

// unitSystemOf resolves an actor's own display preference through the
// identity service, so internal/web/api's Display rendering (FR-007) reads
// the same account row contracts/account.md does, rather than a second query
// of its own.
func unitSystemOf(accounts *serviceidentity.Service) api.UnitSystemOf {
	return func(ctx context.Context, actor access.Actor) (identity.UnitSystem, error) {
		user, err := accounts.Me(ctx, actor)
		if err != nil {
			return "", err
		}

		return user.UnitSystem, nil
	}
}

// patientStack is everything contracts/patients.md and
// contracts/patient-photo.md need, built once and shared by both resolvers
// below.
type patientStack struct {
	service *patient.Service
	photos  *patientstore.PhotoStore
}

// patientFamily resolves the patient stack lazily, mirroring recordFamily:
// the repository and the photo store both need the cursor codec / a running
// filesystem, neither of which exists before the migrations have run.
func patientFamily(app core.App, cfg config.Config) (api.PatientResolve, api.PatientPhotoResolve) {
	once := sync.OnceValues(func() (patientStack, error) {
		secret, err := store.CursorSecret(app, "")
		if err != nil {
			return patientStack{}, err
		}

		cursors, err := store.NewCursorCodec(secret)
		if err != nil {
			return patientStack{}, err
		}

		owners, err := store.NewOwners(app)
		if err != nil {
			return patientStack{}, err
		}

		trail, err := auditstore.New(app)
		if err != nil {
			return patientStack{}, err
		}

		auditor, err := auditservice.New(trail, auditservice.WithRequestID(obs.CorrelationID))
		if err != nil {
			return patientStack{}, err
		}

		repository, err := patientstore.New(app, cursors)
		if err != nil {
			return patientStack{}, err
		}

		authorizer, err := accessservice.New(owners, accessservice.WithPatients(repository, auditor))
		if err != nil {
			return patientStack{}, err
		}

		photos, err := patientstore.NewPhotoStore(app, cfg.Files.PhotoThumbs)
		if err != nil {
			return patientStack{}, err
		}

		service, err := patient.New(repository, photos, authorizer)
		if err != nil {
			return patientStack{}, err
		}

		return patientStack{service: service, photos: photos}, nil
	})

	resolve := func() (*patient.Service, error) {
		stack, err := once()
		if err != nil {
			return nil, err
		}

		return stack.service, nil
	}

	photoResolve := func() (*patient.Service, api.PhotoServer, error) {
		stack, err := once()
		if err != nil {
			return nil, nil, err
		}

		return stack.service, stack.photos, nil
	}

	return resolve, photoResolve
}

// recordFamily resolves the kind registry, once, on first use.
//
// It cannot be resolved when the route table is wired, and that is a property
// of the instance rather than an inconvenience: a kind's repository needs the
// cursor codec, the codec is keyed from the auth collection's persisted secret
// (store.CursorSecret), and that collection does not exist until the
// migrations have run — which apis.Serve does inside OnServe, after this table
// has been built and bound.
//
// The boot gate calls it before the instance serves anything, so a
// registration that cannot be built is a boot failure and not a 500 on
// somebody's first request.
func recordFamily(app core.App, registry *records.Registry, hub *realtime.Hub) api.Resolve {
	return sync.OnceValues(func() (*records.Handler, error) {
		if err := registerKinds(app, registry, hub); err != nil {
			return nil, err
		}

		return records.NewHandler(registry), nil
	})
}

// registerKinds is the extension point phases 002 through 006 add a kind to.
// One call per kind, seven consumers wired by it, and no route.
func registerKinds(app core.App, registry *records.Registry, hub *realtime.Hub) error {
	secret, err := store.CursorSecret(app, "")
	if err != nil {
		return err
	}

	cursors, err := store.NewCursorCodec(secret)
	if err != nil {
		return err
	}

	owners, err := store.NewOwners(app)
	if err != nil {
		return err
	}

	trail, err := auditstore.New(app)
	if err != nil {
		return err
	}

	auditor, err := auditservice.New(trail, auditservice.WithRequestID(obs.CorrelationID))
	if err != nil {
		return err
	}

	patientOwners, err := store.NewPatientOwners(app)
	if err != nil {
		return err
	}

	authorizer, err := accessservice.New(owners, accessservice.WithPatients(patientOwners, auditor))
	if err != nil {
		return err
	}

	views, err := page.NewMedicationViews()
	if err != nil {
		return err
	}

	repository, err := medicationstore.New(app, cursors)
	if err != nil {
		return err
	}

	if err := medication.Register(registry, medication.Wiring{
		Repository: repository,
		Authorizer: authorizer,
		Codec:      api.MedicationCodec{},
		Schema:     api.MedicationSchema(),
		Views:      views,
	}); err != nil {
		return err
	}

	// FR-036's three rows, written by the post-commit hooks and by no handler
	// (research D-21). Bound after the kinds are registered, so it audits
	// exactly what this build serves and nothing else.
	if err := pb.BindRecordAudit(app, pb.RecordAudit{
		Trail:   auditor,
		Kinds:   registry.Kinds(),
		Actor:   web.ActorFrom,
		Request: obs.CorrelationID,
	}); err != nil {
		return err
	}

	// Patients are not a kind.Kind (research D-05), so their rows are bound
	// separately rather than through registry.Kinds().
	if err := pb.BindPatientAudit(app, pb.PatientAudit{
		Trail:   auditor,
		Actor:   web.ActorFrom,
		Request: obs.CorrelationID,
	}); err != nil {
		return err
	}

	// FR-036's sign-in rows, from OnRecordAuthRequest and never from a handler
	// (research D-14): PocketBase's native auth route stays reachable, so a
	// handler-side audit would leave one of the two paths to a session
	// unrecorded.
	if err := pb.BindAuthAudit(app, pb.AuthAudit{
		Trail:   auditor,
		Request: obs.CorrelationID,
	}); err != nil {
		return err
	}

	// contracts/streams.md's publisher, bound to the same three post-commit
	// hooks and to the same kinds: a live view of a kind this build does not
	// serve is a live view of nothing.
	return pb.BindRecordStream(app, pb.RecordStream{Hub: hub, Kinds: registry.Kinds()})
}

// unimplemented lists, sorted, the operations still answered by the stub.
//
// It resolves nothing: the handler table's shape is decided by which groups
// have landed, not by whether an instance could build one, so the resolver
// handed in here is one that is never called.
func unimplemented() []string {
	implemented, err := wired(func() (*records.Handler, error) { return nil, nil }, realtime.New())
	if err != nil {
		panic("medikube: the handler groups cannot be assembled: " + err.Error())
	}

	var pending []string

	for _, opID := range api.AccountOperations() {
		implemented[opID] = nil
	}

	for _, opID := range page.AccountPageOperations() {
		implemented[opID] = nil
	}

	for _, opID := range api.PatientOperations() {
		implemented[opID] = nil
	}

	for _, opID := range page.PatientPageOperations() {
		implemented[opID] = nil
	}

	for _, opID := range api.HealthOperations() {
		implemented[opID] = nil
	}

	implemented[page.OpOverviewPage] = nil

	for opID := range assetHandlers() {
		implemented[opID] = nil
	}

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindExternal {
			continue
		}

		if _, done := implemented[route.OpID]; done {
			continue
		}

		pending = append(pending, route.OpID)
	}

	slices.Sort(pending)

	return pending
}

// errNotImplemented is a 501 in MediKube's own taxonomy.
//
// contracts/README.md's table has no code for it, and inventing one would put
// a value in the published vocabulary that no operation documents. It is
// internal_error instead, which is what every other 5xx MediKube did not
// choose resolves to: the status says what happened and the code says whose
// fault it is.
var errNotImplemented = &web.Coded{Status: http.StatusNotImplemented, Code: web.CodeInternal}

// notImplemented answers a documented operation whose handler does not exist.
//
// The operation id is in the error and therefore in the log line, and it is
// not in the response: the client is told the same thing every internal
// failure tells it, because which handlers are missing is not a client's
// business.
func notImplemented(opID string) httproute.Handler {
	return func(*core.RequestEvent) error {
		return fmt.Errorf("%s has no handler in this build: %w", opID, errNotImplemented)
	}
}

// bindRetention schedules FR-037's nightly purge.
//
// The repository is built here rather than taken from registerKinds, and the
// duplication is deliberate: registerKinds runs behind a sync.OnceValues that
// is not forced until the boot gate, and a job that reached through it would
// resolve the whole kind registry on its first tick — at 03:17, on a code path
// nothing else exercises.
//
// MEDIKUBE_RETENTION_AUDIT_DAYS is validated positive at load, so NewRetention
// is refusing a value this binary cannot be started with. It is checked anyway:
// the horizon is the one number in MediKube whose wrong value silently destroys
// medical history, and two guards on it is not one too many.
func bindRetention(app core.App, cfg config.Config, log zerolog.Logger) error {
	trail, err := auditstore.New(app)
	if err != nil {
		return fmt.Errorf("wire the audit retention purge: %w", err)
	}

	retention, err := auditservice.NewRetention(trail, cfg.Retention.AuditDays, auditservice.SystemClock{})
	if err != nil {
		return fmt.Errorf("wire the audit retention purge: %w", err)
	}

	if err := pb.BindCron(app, pb.CronOptions{Retention: retention, Log: log}); err != nil {
		return fmt.Errorf("schedule the audit retention purge: %w", err)
	}

	return nil
}
