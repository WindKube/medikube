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
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/obs"
	"medikube/internal/platform/pb"
	"medikube/internal/realtime"
	"medikube/internal/records"
	"medikube/internal/records/kinds"
	accessservice "medikube/internal/service/access"
	auditservice "medikube/internal/service/audit"
	"medikube/internal/service/encounter"
	"medikube/internal/service/equipment"
	facilitysvc "medikube/internal/service/facility"
	"medikube/internal/service/familymember"
	serviceidentity "medikube/internal/service/identity"
	"medikube/internal/service/insurance"
	"medikube/internal/service/medication"
	"medikube/internal/service/patient"
	practitionersvc "medikube/internal/service/practitioner"
	"medikube/internal/service/procedure"
	searchsvc "medikube/internal/service/search"
	"medikube/internal/service/symptom"
	tagsvc "medikube/internal/service/tag"
	"medikube/internal/service/treatment"
	"medikube/internal/service/vitals"
	"medikube/internal/store"
	storeallergy "medikube/internal/store/allergy"
	auditstore "medikube/internal/store/audit"
	storecondition "medikube/internal/store/condition"
	storeemergencycontact "medikube/internal/store/emergencycontact"
	storeencounter "medikube/internal/store/encounter"
	storeequipment "medikube/internal/store/equipment"
	facilitystore "medikube/internal/store/facility"
	storefamilymember "medikube/internal/store/familymember"
	storeidentity "medikube/internal/store/identity"
	storeimmunization "medikube/internal/store/immunization"
	storeinjury "medikube/internal/store/injury"
	storeinsurance "medikube/internal/store/insurance"
	storemedication "medikube/internal/store/medication"
	patientstore "medikube/internal/store/patient"
	practitionerstore "medikube/internal/store/practitioner"
	storeprocedure "medikube/internal/store/procedure"
	searchstore "medikube/internal/store/search"
	storesymptom "medikube/internal/store/symptom"
	tagstore "medikube/internal/store/tag"
	storetreatment "medikube/internal/store/treatment"
	storevitals "medikube/internal/store/vitals"
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

	// Search is the search_index write side's real repository, wired the same
	// way the composition root wires it (T030): every registered kind's
	// Service indexes through it, and a test reads it back to prove that.
	Search *searchstore.Repo
}

// Option adjusts one instance. The zero set is production's wiring; the
// overrides exist for the stream, where the production heartbeat is 25 seconds
// and a test that waited that long for its first assertion is a test nobody
// runs.
type Option func(*settings)

type settings struct {
	heartbeat time.Duration
	now       func() time.Time
	logTo     io.Writer

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

// WithStreamHeartbeat shortens the interval between $_stream_beat frames.
func WithStreamHeartbeat(interval time.Duration) Option {
	return func(s *settings) { s.heartbeat = interval }
}

// WithStreamClock replaces the clock the heartbeat reads, so a test can assert
// the value on the wire rather than that it parses.
func WithStreamClock(now func() time.Time) Option {
	return func(s *settings) { s.now = now }
}

// WithLogWriter replaces the request logger's sink with w, so a test can
// capture the log stream itself — FR-046's "no substring of the uploaded
// filename" is an assertion about what a real deployment's logger would have
// recorded, and the production wiring's zerolog.Nop() discards everything a
// test could scan.
func WithLogWriter(w io.Writer) Option {
	return func(s *settings) { s.logTo = w }
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
func NewPopulated(t testing.TB, patientID string, count int, options ...Option) *Instance {
	t.Helper()

	app := testsupport.NewApp(t)

	require.NoError(t, Populate(app, patientID, count), "seeding %d rows of %s", count, kind.Medication)

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
func Populate(app core.App, patientID string, count int) error {
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
				PatientID: patientID,
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

	registry, patientService, photos, searchRepo, tagService, err := registerKinds(app, hub)
	if err != nil {
		return nil, err
	}

	handler := records.NewHandler(registry)

	resolve := api.Resolve(func() (*records.Handler, error) { return handler, nil })
	patientResolve := api.PatientResolve(func() (*patient.Service, error) { return patientService, nil })
	photoResolve := api.PatientPhotoResolve(func() (*patient.Service, api.PhotoServer, error) { return patientService, photos, nil })

	accounts, err := api.NewAccounts(app, api.AccountsConfig{
		RegistrationOpen: chosen.registrationOpen,
		SessionTTL:       SessionTTL,
		PublicURL:        PublicURL,
		Resolve:          resolve,
		SelfRecord:       api.SelfRecordOf(patientResolve),
		Patients:         patientResolve,
	})
	if err != nil {
		return nil, err
	}

	practitionerService, facilityService, err := registerDirectory(app)
	if err != nil {
		return nil, err
	}

	directoryOps, err := directoryHandlers(practitionerService, facilityService)
	if err != nil {
		return nil, err
	}

	tagOps, err := tagHandlers(tagService)
	if err != nil {
		return nil, err
	}

	table, err := handlerTable(resolve, patientResolve, photoResolve, hub, chosen, accounts, directoryOps, tagOps)
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

	requestLog := zerolog.Nop()
	if chosen.logTo != nil {
		requestLog = zerolog.New(chosen.logTo)
	}

	pb.BindServe(app, pb.ServeOptions{
		Middlewares: []*hook.Handler[*core.RequestEvent]{
			// The correlation id first, and it is not optional here: it is
			// what fills request_id on every refusal, and FR-033's
			// byte-identical comparison would be trivially satisfied by two
			// bodies that both carried an empty one.
			obs.RequestLogger(requestLog),
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

	return &Instance{App: app, Records: handler, Routes: routes, Hub: hub, Accounts: accounts, Search: searchRepo}, nil
}

// handlerTable is cmd/medikube's operations(): a 501 under every registered
// operation id, then the real handlers on top. Without the stubs
// httproute.New refuses the table, because a route nothing serves is a route
// that would panic on its first request.
func handlerTable(
	resolve api.Resolve,
	patientResolve api.PatientResolve,
	photoResolve api.PatientPhotoResolve,
	hub *realtime.Hub,
	chosen settings,
	accounts *api.Accounts,
	directoryOps httproute.Handlers,
	tagOps httproute.Handlers,
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

	pageOps, err := page.Handlers(resolve, patientResolve)
	if err != nil {
		return nil, err
	}

	insurancePageOps, err := page.InsuranceHandlers(resolve, patientResolve)
	if err != nil {
		return nil, err
	}

	equipmentPageOps, err := page.EquipmentHandlers(resolve, patientResolve)
	if err != nil {
		return nil, err
	}

	symptomPageOps, err := page.SymptomHandlers(resolve, patientResolve)
	if err != nil {
		return nil, err
	}

	vitalsPageOps, err := page.VitalsHandlers(resolve, patientResolve)
	if err != nil {
		return nil, err
	}

	encounterPageOps, err := page.EncounterHandlers(resolve, patientResolve)
	if err != nil {
		return nil, err
	}

	procedurePageOps, err := page.ProcedureHandlers(resolve, patientResolve)
	if err != nil {
		return nil, err
	}

	treatmentPageOps, err := page.TreatmentHandlers(resolve, patientResolve)
	if err != nil {
		return nil, err
	}

	familyMemberPageOps, err := page.FamilyMemberHandlers(resolve, patientResolve)
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

	overviewPage, err := page.OverviewPage(page.OverviewDeps{Counts: accounts.Deps.Counts})
	if err != nil {
		return nil, err
	}

	patientDeps := page.PatientDeps{
		Resolve: patientResolve,
		UnitOf:  unitSystemOf(accounts.Service),
		Records: resolve,
	}

	patientForms, err := page.NewPatientForms(patientDeps)
	if err != nil {
		return nil, err
	}

	patientOps, err := api.PatientHandlers(patientResolve, unitSystemOf(accounts.Service), resolve, patientForms)
	if err != nil {
		return nil, err
	}

	photoOps, err := api.PatientPhotoHandlers(photoResolve)
	if err != nil {
		return nil, err
	}

	activePatientOps, err := api.ActivePatientHandlers(patientResolve)
	if err != nil {
		return nil, err
	}

	patientPages, err := page.PatientPages(patientDeps)
	if err != nil {
		return nil, err
	}

	injuryPageOps, err := page.InjuryHandlers(resolve, patientResolve)
	if err != nil {
		return nil, err
	}

	immunizationPageOps, err := page.ImmunizationHandlers(resolve, patientResolve)
	if err != nil {
		return nil, err
	}

	assets := httproute.Handlers{
		"assetAppCSS":     web.ServeAppCSS,
		"assetDatastarJS": web.ServeDatastarJS,
	}

	groups := []httproute.Handlers{
		recordOps, pageOps, symptomPageOps, vitalsPageOps, encounterPageOps, procedurePageOps, treatmentPageOps,
		familyMemberPageOps, streamOps, accountOps, accountPages, overviewPage, assets,
		patientOps, photoOps, activePatientOps, patientPages, directoryOps, injuryPageOps, immunizationPageOps,
		insurancePageOps, equipmentPageOps, tagOps,
	}

	for _, group := range groups {
		for opID, handler := range group {
			table[opID] = handler
		}
	}

	return table, nil
}

// unitSystemOf mirrors cmd/medikube's own: the patient surface's Display
// rendering (FR-007) reads the same account row contracts/account.md does.
func unitSystemOf(accounts *serviceidentity.Service) api.UnitSystemOf {
	return func(ctx context.Context, actor access.Actor) (identity.UnitSystem, error) {
		user, err := accounts.Me(ctx, actor)
		if err != nil {
			return "", err
		}

		return user.UnitSystem, nil
	}
}

func vitalsUnitSystemOf(accounts *serviceidentity.Service) vitals.UnitSystemOf {
	return func(ctx context.Context, actor access.Actor) (identity.UnitSystem, error) {
		user, err := accounts.Me(ctx, actor)
		if err != nil {
			return "", err
		}

		return user.UnitSystem, nil
	}
}

func notImplemented(opID string) httproute.Handler {
	stub := &web.Coded{Status: http.StatusNotImplemented, Code: web.CodeInternal}

	return func(*core.RequestEvent) error {
		return fmt.Errorf("%s has no handler in this build: %w", opID, stub)
	}
}

// registerKinds builds the kind registry this instance serves. One call, seven
// consumers, no route — the same call cmd/medikube will make.
func registerKinds(
	app core.App, hub *realtime.Hub,
) (*records.Registry, *patient.Service, *patientstore.PhotoStore, *searchstore.Repo, *tagsvc.Service, error) {
	secret, err := store.CursorSecret(app, "")
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	codec, err := store.NewCursorCodec(secret)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	repository, err := storemedication.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	owners, err := store.NewOwners(app)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	trail, err := auditstore.New(app)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	// WithRequestID, as the composition root wires it: without it a row the
	// hooks write carries a handle minted on the spot instead of the one the
	// request's own log lines carry, and FR-054's join is broken in exactly the
	// place no assertion looks (T231).
	auditor, err := auditservice.New(trail, auditservice.WithRequestID(obs.CorrelationID))
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	patientRepo, err := patientstore.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	authorizer, err := accessservice.New(owners, accessservice.WithPatients(patientRepo, auditor))
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	views, err := page.NewMedicationViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	searchRepo, err := searchstore.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	indexer, err := searchsvc.NewIndexer(searchRepo)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	registry := records.NewRegistry()
	registry.SetIndexer(indexer)
	registry.SetSearchReader(searchRepo)

	tagRepository, err := tagstore.New(app, codec, func() []string {
		var collections []string
		for _, k := range registry.Kinds() {
			collections = append(collections, k.Collection())
		}

		return collections
	})
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	tagService, err := tagsvc.New(tagRepository, tagRepository, tagRepository, tagsvc.DefaultAuthorizer, auditor)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	registry.SetTagChecker(tagService)

	if registerErr := medication.Register(registry, medication.Wiring{
		Repository:   repository,
		Authorizer:   authorizer,
		Codec:        api.MedicationCodec{},
		Schema:       api.MedicationSchema(),
		Views:        views,
		SearchFields: api.MedicationSearchFields,
		Basis:        api.MedicationBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	allergyViews, err := page.NewAllergyViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	allergyRepo, err := storeallergy.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if registerErr := kinds.RegisterAllergy(registry, kinds.AllergyWiring{
		Repository:   allergyRepo,
		Authorizer:   authorizer,
		Codec:        api.AllergyCodec{},
		Schema:       api.AllergySchema(),
		Views:        allergyViews,
		SearchFields: api.AllergySearchFields,
		Basis:        api.AllergyBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	conditionViews, err := page.NewConditionViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	conditionRepo, err := storecondition.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if registerErr := kinds.RegisterCondition(registry, kinds.ConditionWiring{
		Repository:   conditionRepo,
		Authorizer:   authorizer,
		Codec:        api.ConditionCodec{},
		Schema:       api.ConditionSchema(),
		Views:        conditionViews,
		SearchFields: api.ConditionSearchFields,
		Basis:        api.ConditionBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	emergencyContactViews, err := page.NewEmergencyContactViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	emergencyContactRepo, err := storeemergencycontact.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if registerErr := kinds.RegisterEmergencyContact(registry, kinds.EmergencyContactWiring{
		Repository:   emergencyContactRepo,
		Authorizer:   authorizer,
		Codec:        api.EmergencyContactCodec{},
		Schema:       api.EmergencyContactSchema(),
		Views:        emergencyContactViews,
		SearchFields: api.EmergencyContactSearchFields,
		Basis:        api.EmergencyContactBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	immunizationRepo, err := storeimmunization.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	immunizationViews, err := page.NewImmunizationViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if registerErr := kinds.Register(registry, kinds.Wiring{
		Repository:   immunizationRepo,
		Authorizer:   authorizer,
		Codec:        api.ImmunizationCodec{},
		Schema:       api.ImmunizationSchema(),
		Views:        immunizationViews,
		SearchFields: api.ImmunizationSearchFields,
		Basis:        api.ImmunizationBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	injuryRepo, err := storeinjury.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	injuryViews, err := page.NewInjuryViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if registerErr := kinds.RegisterInjury(registry, kinds.InjuryWiring{
		Repository:   injuryRepo,
		Authorizer:   authorizer,
		Codec:        api.InjuryCodec{},
		Schema:       api.InjurySchema(),
		Views:        injuryViews,
		SearchFields: api.InjurySearchFields,
		Basis:        api.InjuryBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	insuranceViews, err := page.NewInsuranceViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	insuranceRepository, err := storeinsurance.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if registerErr := insurance.Register(registry, insurance.Wiring{
		Repository:   insuranceRepository,
		Authorizer:   authorizer,
		Codec:        api.InsuranceCodec{},
		Schema:       api.InsuranceSchema(),
		Views:        insuranceViews,
		SearchFields: api.InsuranceSearchFields,
		Basis:        api.InsuranceBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	equipmentViews, err := page.NewEquipmentViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	equipmentRepository, err := storeequipment.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if registerErr := equipment.Register(registry, equipment.Wiring{
		Repository:   equipmentRepository,
		Authorizer:   authorizer,
		Codec:        api.EquipmentCodec{},
		Schema:       api.EquipmentSchema(),
		Views:        equipmentViews,
		SearchFields: api.EquipmentSearchFields,
		Basis:        api.EquipmentBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	symptomViews, err := page.NewSymptomViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	symptomRepo, err := storesymptom.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if registerErr := symptom.Register(registry, symptom.Wiring{
		Repository:   symptomRepo,
		Authorizer:   authorizer,
		Codec:        api.SymptomCodec{},
		Schema:       api.SymptomSchema(),
		Views:        symptomViews,
		SearchFields: api.SymptomSearchFields,
		Basis:        api.SymptomBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	identityRepo, err := storeidentity.NewRepository(app)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	identityAuth, err := storeidentity.NewAuthenticator(app)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	mailer, err := pb.NewMailer(app)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	identityService, err := serviceidentity.New(serviceidentity.Config{
		Repository:    identityRepo,
		Authenticator: identityAuth,
		Mailer:        mailer,
		Auditor:       auditor,
		Clock:         serviceidentity.SystemClock{},
	})
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	vitalsViews, err := page.NewVitalsViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	vitalsRepo, err := storevitals.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if registerErr := vitals.Register(registry, vitals.Wiring{
		Repository:   vitalsRepo,
		Authorizer:   authorizer,
		Codec:        api.VitalsCodec{},
		UnitSystemOf: vitalsUnitSystemOf(identityService),
		Schema:       api.VitalsSchema(),
		Views:        vitalsViews,
		SearchFields: api.VitalsSearchFields,
		Basis:        api.VitalsBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	encounterViews, err := page.NewEncounterViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	encounterRepo, err := storeencounter.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if registerErr := encounter.Register(registry, encounter.Wiring{
		Repository:   encounterRepo,
		Authorizer:   authorizer,
		Codec:        api.EncounterCodec{},
		Schema:       api.EncounterSchema(),
		Views:        encounterViews,
		SearchFields: api.EncounterSearchFields,
		Basis:        api.EncounterBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	procedureViews, err := page.NewProcedureViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	procedureRepo, err := storeprocedure.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if registerErr := procedure.Register(registry, procedure.Wiring{
		Repository:   procedureRepo,
		Authorizer:   authorizer,
		Codec:        api.ProcedureCodec{},
		Schema:       api.ProcedureSchema(),
		Views:        procedureViews,
		SearchFields: api.ProcedureSearchFields,
		Basis:        api.ProcedureBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	treatmentViews, err := page.NewTreatmentViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	treatmentRepo, err := storetreatment.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if registerErr := treatment.Register(registry, treatment.Wiring{
		Repository:   treatmentRepo,
		Authorizer:   authorizer,
		Codec:        api.TreatmentCodec{},
		Schema:       api.TreatmentSchema(),
		Views:        treatmentViews,
		SearchFields: api.TreatmentSearchFields,
		Basis:        api.TreatmentBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	familyMemberViews, err := page.NewFamilyMemberViews()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	familyMemberRepo, err := storefamilymember.New(app, codec)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if registerErr := familymember.Register(registry, familymember.Wiring{
		Repository:   familyMemberRepo,
		Authorizer:   authorizer,
		Codec:        api.FamilyMemberCodec{},
		Schema:       api.FamilyMemberSchema(),
		Views:        familyMemberViews,
		SearchFields: api.FamilyMemberSearchFields,
		Basis:        api.FamilyMemberBasis,
	}); registerErr != nil {
		return nil, nil, nil, nil, nil, registerErr
	}

	// FR-037's append-only trail, bound where the binary binds it.
	if guardErr := pb.BindAuditImmutability(app); guardErr != nil {
		return nil, nil, nil, nil, nil, guardErr
	}

	// The nightly purge. It is bound and never ticks here — nothing starts the
	// scheduler under tests.TestApp — and that is exactly the point:
	// internal/architecture holds the binary and this harness to the same set of
	// platform bindings, so a hook the deployment has and the suite does not is
	// a failure rather than an omission nobody notices.
	retention, retentionErr := auditservice.NewRetention(trail, AuditRetentionDays, auditservice.SystemClock{})
	if retentionErr != nil {
		return nil, nil, nil, nil, nil, retentionErr
	}

	if cronErr := pb.BindCron(app, pb.CronOptions{Retention: retention, Log: zerolog.Nop()}); cronErr != nil {
		return nil, nil, nil, nil, nil, cronErr
	}

	// FR-036's three rows, from the post-commit hooks and never from a
	// handler. Bound after the kinds are registered, so the hook is bound to
	// exactly what this instance serves.
	if recordAuditErr := pb.BindRecordAudit(app, pb.RecordAudit{
		Trail:   auditor,
		Kinds:   registry.Kinds(),
		Actor:   web.ActorFrom,
		Request: obs.CorrelationID,
	}); recordAuditErr != nil {
		return nil, nil, nil, nil, nil, recordAuditErr
	}

	// FR-036's sign-in rows, from OnRecordAuthRequest and never from a handler
	// (research D-14): PocketBase's native auth route stays reachable, so a
	// handler-side audit would leave one of the two paths to a session
	// unrecorded.
	if authAuditErr := pb.BindAuthAudit(app, pb.AuthAudit{
		Trail:   auditor,
		Request: obs.CorrelationID,
	}); authAuditErr != nil {
		return nil, nil, nil, nil, nil, authAuditErr
	}

	// contracts/streams.md's post-commit publisher, bound to the same kinds
	// for the same reason: a live view of a kind this instance does not serve
	// is a live view of nothing.
	if streamErr := pb.BindRecordStream(app, pb.RecordStream{Hub: hub, Kinds: registry.Kinds()}); streamErr != nil {
		return nil, nil, nil, nil, nil, streamErr
	}

	// Phase 002's patients audit, bound where the binary binds it
	// (internal/architecture holds the two to the same set of platform
	// bindings).
	if patientAuditErr := pb.BindPatientAudit(app, pb.PatientAudit{
		Trail:   auditor,
		Actor:   web.ActorFrom,
		Request: obs.CorrelationID,
	}); patientAuditErr != nil {
		return nil, nil, nil, nil, nil, patientAuditErr
	}

	photos, err := patientstore.NewPhotoStore(app, []string{"100x100t", "400x400f"})
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	activePatient, err := patientstore.NewActivePatientRepo(app)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	counter, err := records.NewCounter(registry, func(ctx context.Context, collection, patientID string) (int, error) {
		return store.CountByPatient(ctx, app, collection, patientID)
	})
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	activity, err := auditservice.NewRecentActivity(trail)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	patientService, err := patient.New(patientRepo, photos, authorizer, activePatient, auditor, counter, activity)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	return registry, patientService, photos, searchRepo, tagService, nil
}

// registerDirectory builds the two directory services this instance serves —
// the same call cmd/medikube's registerDirectory makes — and binds their
// audit hooks. Practitioners and facilities are not a kind.Kind (research
// D-05), so neither goes through registerKinds.
func registerDirectory(app core.App) (*practitionersvc.Service, *facilitysvc.Service, error) {
	secret, err := store.CursorSecret(app, "")
	if err != nil {
		return nil, nil, err
	}

	codec, err := store.NewCursorCodec(secret)
	if err != nil {
		return nil, nil, err
	}

	trail, err := auditstore.New(app)
	if err != nil {
		return nil, nil, err
	}

	auditor, err := auditservice.New(trail, auditservice.WithRequestID(obs.CorrelationID))
	if err != nil {
		return nil, nil, err
	}

	practitionerRepo, err := practitionerstore.New(app, codec)
	if err != nil {
		return nil, nil, err
	}

	practitionerService, err := practitionersvc.New(practitionerRepo, practitionersvc.DefaultAuthorizer, auditor)
	if err != nil {
		return nil, nil, err
	}

	facilityRepo, err := facilitystore.New(app, codec)
	if err != nil {
		return nil, nil, err
	}

	facilityService, err := facilitysvc.New(facilityRepo, facilitysvc.NewAuthorizer(), auditor)
	if err != nil {
		return nil, nil, err
	}

	if err := pb.BindDirectoryAudit(app, pb.DirectoryAudit{
		Trail: auditor,
		Collections: map[string]audit.TargetKind{
			"practitioners": audit.TargetKindPractitioner,
			"facilities":    audit.TargetKindFacility,
		},
		Actor:   web.ActorFrom,
		Request: obs.CorrelationID,
	}); err != nil {
		return nil, nil, err
	}

	return practitionerService, facilityService, nil
}

// directoryHandlers is the ten operations, resolved to the two services
// already built — this harness never needs the lazy Resolve indirection
// cmd/medikube uses, because the instance is already migrated by the time
// Wire runs.
func directoryHandlers(practitionerService *practitionersvc.Service, facilityService *facilitysvc.Service) (httproute.Handlers, error) {
	practitionerResolve := api.PractitionerResolve(func() (*practitionersvc.Service, error) { return practitionerService, nil })
	facilityResolve := api.FacilityResolve(func() (*facilitysvc.Service, error) { return facilityService, nil })

	practitionerForms, err := page.NewPractitionerForms(practitionerResolve, facilityResolve)
	if err != nil {
		return nil, err
	}

	facilityForms, err := page.NewFacilityForms(facilityResolve)
	if err != nil {
		return nil, err
	}

	practitionerOps, err := api.PractitionerHandlers(api.PractitionerDeps{
		Resolve:    practitionerResolve,
		Facilities: facilityResolve,
		Forms:      practitionerForms,
	})
	if err != nil {
		return nil, err
	}

	facilityOps, err := api.FacilityHandlers(api.FacilityDeps{Resolve: facilityResolve, Forms: facilityForms})
	if err != nil {
		return nil, err
	}

	practitionerPages, err := page.PractitionerHandlers(practitionerResolve, facilityResolve)
	if err != nil {
		return nil, err
	}

	facilityPages, err := page.FacilityHandlers(facilityResolve)
	if err != nil {
		return nil, err
	}

	table := make(httproute.Handlers,
		len(practitionerOps)+len(facilityOps)+len(practitionerPages)+len(facilityPages))

	for _, group := range []httproute.Handlers{practitionerOps, facilityOps, practitionerPages, facilityPages} {
		for opID, handler := range group {
			table[opID] = handler
		}
	}

	return table, nil
}

// tagHandlers is directoryHandlers' twin for contracts/tags.md's four
// operations and the /tags page.
func tagHandlers(tagService *tagsvc.Service) (httproute.Handlers, error) {
	tagResolve := api.TagResolve(func() (*tagsvc.Service, error) { return tagService, nil })

	tagForms, err := page.NewTagForms(tagResolve)
	if err != nil {
		return nil, err
	}

	tagOps, err := api.TagHandlers(api.TagDeps{Resolve: tagResolve, Forms: tagForms})
	if err != nil {
		return nil, err
	}

	tagPages, err := page.TagHandlers(tagResolve)
	if err != nil {
		return nil, err
	}

	table := make(httproute.Handlers, len(tagOps)+len(tagPages))

	for _, group := range []httproute.Handlers{tagOps, tagPages} {
		for opID, handler := range group {
			table[opID] = handler
		}
	}

	return table, nil
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
