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
	"medikube/internal/domain/audit"
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
	timelinesvc "medikube/internal/service/timeline"
	"medikube/internal/service/treatment"
	"medikube/internal/service/vitals"
	"medikube/internal/store"
	allergystore "medikube/internal/store/allergy"
	auditstore "medikube/internal/store/audit"
	conditionstore "medikube/internal/store/condition"
	emergencycontactstore "medikube/internal/store/emergencycontact"
	encounterstore "medikube/internal/store/encounter"
	equipmentstore "medikube/internal/store/equipment"
	facilitystore "medikube/internal/store/facility"
	familymemberstore "medikube/internal/store/familymember"
	storeidentity "medikube/internal/store/identity"
	storeimmunization "medikube/internal/store/immunization"
	storeinjury "medikube/internal/store/injury"
	insurancestore "medikube/internal/store/insurance"
	medicationstore "medikube/internal/store/medication"
	patientstore "medikube/internal/store/patient"
	practitionerstore "medikube/internal/store/practitioner"
	procedurestore "medikube/internal/store/procedure"
	searchstore "medikube/internal/store/search"
	symptomstore "medikube/internal/store/symptom"
	tagstore "medikube/internal/store/tag"
	timelinestore "medikube/internal/store/timeline"
	treatmentstore "medikube/internal/store/treatment"
	vitalsstore "medikube/internal/store/vitals"
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
	searchResolve api.SearchResolve,
	registry *records.Registry,
	resolveDirectory func() (directoryServices, error),
	tagResolve api.TagResolve,
	timelineResolve page.TimelineResolve,
	hub *realtime.Hub,
	measurements *obs.Metrics,
	tracing *obs.Tracing,
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

	// contracts/patients.md and contracts/patient-photo.md. The stack is
	// resolved lazily (patientFamily), the same reason recordFamily is: the
	// repository needs the cursor codec, which is keyed from a secret the
	// migrations have only just created. Resolved here, ahead of everything
	// that reads it — wired()'s medication pages need it for the context
	// header and the redirect, and the account surface needs it so
	// registration can provision the self-record FR-005 requires.
	patientResolve, photoResolve := patientFamily(app, cfg, registry, measurements, tracing)

	// The real ones win. This is the only line each later group touches.
	served, err := wired(resolve, searchResolve, patientResolve, timelineResolve, hub)
	if err != nil {
		return nil, err
	}

	maps.Copy(table, served)

	// healthz and readyz need neither the kind registry nor the application:
	// only the build stamp, the start instant and the drain flag, all of which
	// the composition root already holds by the time it calls this.
	maps.Copy(table, api.HealthHandlers(health))

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
		Patients:         patientResolve,
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

	maps.Copy(table, patientOps)

	photoOps, err := api.PatientPhotoHandlers(photoResolve)
	if err != nil {
		return nil, err
	}

	maps.Copy(table, photoOps)

	activePatientOps, err := api.ActivePatientHandlers(patientResolve)
	if err != nil {
		return nil, err
	}

	maps.Copy(table, activePatientOps)

	patientPages, err := page.PatientPages(patientDeps)
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

	// The directory: practitioners and facilities, contracts/practitioners.md
	// and contracts/facilities.md's ten operations. Wired here, alongside
	// accounts, because both need the application: a repository needs the
	// cursor codec, and the codec is keyed from a secret the migrations have
	// only just created — see directoryFamily.
	practitionerResolve := api.PractitionerResolve(func() (*practitionersvc.Service, error) {
		services, resolveErr := resolveDirectory()

		return services.Practitioner, resolveErr
	})

	facilityResolve := api.FacilityResolve(func() (*facilitysvc.Service, error) {
		services, resolveErr := resolveDirectory()

		return services.Facility, resolveErr
	})

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

	maps.Copy(table, practitionerOps)
	maps.Copy(table, facilityOps)

	// contracts/pages.md P3-P6, the same four resolvers as the JSON operations
	// above.
	practitionerPages, err := page.PractitionerHandlers(practitionerResolve, facilityResolve)
	if err != nil {
		return nil, err
	}

	facilityPages, err := page.FacilityHandlers(facilityResolve)
	if err != nil {
		return nil, err
	}

	maps.Copy(table, practitionerPages)
	maps.Copy(table, facilityPages)

	// contracts/tags.md, US7: the account's own tag vocabulary. tagResolve
	// is recordFamily's own resolver — the tag service comes out of the same
	// registerKinds call that wires the tag checker into the registry, so it
	// is not built again here.
	tagForms, err := page.NewTagForms(tagResolve)
	if err != nil {
		return nil, err
	}

	tagOps, err := api.TagHandlers(api.TagDeps{Resolve: tagResolve, Forms: tagForms})
	if err != nil {
		return nil, err
	}

	maps.Copy(table, tagOps)

	tagPages, err := page.TagHandlers(tagResolve)
	if err != nil {
		return nil, err
	}

	maps.Copy(table, tagPages)

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
func wired(
	resolve api.Resolve, searchResolve api.SearchResolve, patients api.PatientResolve,
	timelineResolve page.TimelineResolve, hub *realtime.Hub,
) (httproute.Handlers, error) {
	table := make(httproute.Handlers)

	records, err := api.Handlers(resolve)
	if err != nil {
		return nil, err
	}

	pages, err := page.Handlers(resolve, patients)
	if err != nil {
		return nil, err
	}

	immunizationPages, err := page.ImmunizationHandlers(resolve, patients)
	if err != nil {
		return nil, err
	}

	insurancePages, err := page.InsuranceHandlers(resolve, patients)
	if err != nil {
		return nil, err
	}

	injuryPages, err := page.InjuryHandlers(resolve, patients)
	if err != nil {
		return nil, err
	}

	equipmentPages, err := page.EquipmentHandlers(resolve, patients)
	if err != nil {
		return nil, err
	}

	symptomPages, err := page.SymptomHandlers(resolve, patients)
	if err != nil {
		return nil, err
	}

	vitalsPages, err := page.VitalsHandlers(resolve, patients)
	if err != nil {
		return nil, err
	}

	encounterPages, err := page.EncounterHandlers(resolve, patients)
	if err != nil {
		return nil, err
	}

	procedurePages, err := page.ProcedureHandlers(resolve, patients)
	if err != nil {
		return nil, err
	}

	treatmentPages, err := page.TreatmentHandlers(resolve, patients)
	if err != nil {
		return nil, err
	}

	familyMemberPages, err := page.FamilyMemberHandlers(resolve, patients)
	if err != nil {
		return nil, err
	}

	streams, err := stream.Handlers(stream.Deps{Resolve: resolve, Hub: hub})
	if err != nil {
		return nil, err
	}

	searchOps, err := api.SearchHandlers(searchResolve)
	if err != nil {
		return nil, err
	}

	searchPages, err := page.SearchHandlers(searchResolve, patients)
	if err != nil {
		return nil, err
	}

	timelinePages, err := page.TimelineHandlers(timelineResolve, patients)
	if err != nil {
		return nil, err
	}

	maps.Copy(table, records)
	maps.Copy(table, pages)
	maps.Copy(table, immunizationPages)
	maps.Copy(table, injuryPages)
	maps.Copy(table, insurancePages)
	maps.Copy(table, equipmentPages)
	maps.Copy(table, symptomPages)
	maps.Copy(table, vitalsPages)
	maps.Copy(table, encounterPages)
	maps.Copy(table, procedurePages)
	maps.Copy(table, treatmentPages)
	maps.Copy(table, familyMemberPages)
	maps.Copy(table, timelinePages)
	maps.Copy(table, streams)
	maps.Copy(table, searchOps)
	maps.Copy(table, searchPages)

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

// vitalsUnitSystemOf mirrors unitSystemOf for internal/service/vitals, which
// declares its own UnitSystemOf type rather than importing internal/web/api's
// (internal/service must not depend on internal/web).
func vitalsUnitSystemOf(accounts *serviceidentity.Service) vitals.UnitSystemOf {
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
//
// registry is the same *records.Registry recordFamily registers every kind
// into: the chart's per-kind counts (contracts/patient-chart.md) walk it
// rather than switching on kind, and this is what lets a request-time read of
// it be correct regardless of which of the two families' OnceValues runs
// first — the boot gate forces both before anything serves.
func patientFamily(
	app core.App, cfg config.Config, registry *records.Registry, measurements *obs.Metrics, tracing *obs.Tracing,
) (api.PatientResolve, api.PatientPhotoResolve) {
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

		repository.SetTracer(obs.NewSpanTracer(tracing.TracerProvider(), "store.patients"))

		authorizer, err := accessservice.New(owners, accessservice.WithPatients(repository, auditor))
		if err != nil {
			return patientStack{}, err
		}

		photos, err := patientstore.NewPhotoStore(app, cfg.Files.PhotoThumbs)
		if err != nil {
			return patientStack{}, err
		}

		photos.SetMetrics(measurements)

		activePatient, err := patientstore.NewActivePatientRepo(app)
		if err != nil {
			return patientStack{}, err
		}

		counter, err := records.NewCounter(registry, func(ctx context.Context, collection, patientID string) (int, error) {
			return store.CountByPatient(ctx, app, collection, patientID)
		})
		if err != nil {
			return patientStack{}, err
		}

		activity, err := auditservice.NewRecentActivity(trail)
		if err != nil {
			return patientStack{}, err
		}

		service, err := patient.New(repository, photos, authorizer, activePatient, auditor, counter, activity)
		if err != nil {
			return patientStack{}, err
		}

		service.SetMetrics(measurements)
		service.SetTracer(obs.NewSpanTracer(tracing.TracerProvider(), "service.patient"))

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
//
// recordFamilyResult is the once-built tuple api.Resolve, api.TagResolve,
// api.SearchResolve and page.TimelineResolve each read a part of:
// sync.OnceValues would need four separate calls to build the handler, the
// tag service, the search service and the timeline service otherwise, and
// registerKinds wires the tag checker into the registry before any kind
// registers (FR-064) — calling it more than once would build a second tag
// service the checker never sees.
type recordFamilyResult struct {
	handler  *records.Handler
	tags     *tagsvc.Service
	search   *searchsvc.Service
	kinds    []kind.Kind
	timeline *timelinesvc.Service
}

func recordFamily(
	app core.App, registry *records.Registry, hub *realtime.Hub,
) (api.Resolve, api.TagResolve, api.SearchResolve, page.TimelineResolve) {
	once := sync.OnceValues(func() (recordFamilyResult, error) {
		tags, timelineReader, err := registerKinds(app, registry, hub)
		if err != nil {
			return recordFamilyResult{}, err
		}

		search, kinds, err := buildSearchService(app, registry, tags)
		if err != nil {
			return recordFamilyResult{}, err
		}

		timeline, err := timelinesvc.New(registry, timelineReader)
		if err != nil {
			return recordFamilyResult{}, err
		}

		return recordFamilyResult{
			handler:  records.NewHandler(registry),
			tags:     tags,
			search:   search,
			kinds:    kinds,
			timeline: timeline,
		}, nil
	})

	resolve := func() (*records.Handler, error) {
		result, err := once()

		return result.handler, err
	}

	tagResolve := func() (*tagsvc.Service, error) {
		result, err := once()

		return result.tags, err
	}

	searchResolve := func() (*searchsvc.Service, []kind.Kind, error) {
		result, err := once()

		return result.search, result.kinds, err
	}

	timelineResolve := func() (*timelinesvc.Service, error) {
		result, err := once()

		return result.timeline, err
	}

	return resolve, tagResolve, searchResolve, timelineResolve
}

// buildSearchService wires US8's read side, once registerKinds has finished:
// registry.Kinds() is only complete afterwards, and it is what a grouped
// search's default kind selection and group order both read. tags is the
// same tag service registerKinds already built — search's own `?tags=`
// ownership check (T164-T177 follow-up) reuses it rather than building a
// second one.
func buildSearchService(app core.App, registry *records.Registry, tags *tagsvc.Service) (*searchsvc.Service, []kind.Kind, error) {
	secret, err := store.CursorSecret(app, "")
	if err != nil {
		return nil, nil, err
	}

	cursors, err := store.NewCursorCodec(secret)
	if err != nil {
		return nil, nil, err
	}

	owners, err := store.NewOwners(app)
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

	patientOwners, err := store.NewPatientOwners(app)
	if err != nil {
		return nil, nil, err
	}

	authorizer, err := accessservice.New(owners, accessservice.WithPatients(patientOwners, auditor))
	if err != nil {
		return nil, nil, err
	}

	searchRepo, err := searchstore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	service, err := searchsvc.NewService(searchRepo, searchRepo, authorizer, tags)
	if err != nil {
		return nil, nil, err
	}

	return service, registry.Kinds(), nil
}

// registerKinds is the extension point phases 002 through 006 add a kind to.
// One call per kind, seven consumers wired by it, and no route.
func registerKinds(app core.App, registry *records.Registry, hub *realtime.Hub) (*tagsvc.Service, timelinesvc.Reader, error) {
	secret, err := store.CursorSecret(app, "")
	if err != nil {
		return nil, nil, err
	}

	cursors, err := store.NewCursorCodec(secret)
	if err != nil {
		return nil, nil, err
	}

	owners, err := store.NewOwners(app)
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

	patientOwners, err := store.NewPatientOwners(app)
	if err != nil {
		return nil, nil, err
	}

	authorizer, err := accessservice.New(owners, accessservice.WithPatients(patientOwners, auditor))
	if err != nil {
		return nil, nil, err
	}

	views, err := page.NewMedicationViews()
	if err != nil {
		return nil, nil, err
	}

	repository, err := medicationstore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	searchRepo, err := searchstore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	indexer, err := searchsvc.NewIndexer(searchRepo)
	if err != nil {
		return nil, nil, err
	}

	registry.SetIndexer(indexer)
	registry.SetSearchReader(searchRepo)

	// The tag checker is wired before any kind registers: Registry.add wraps
	// each kind's Service in the decorator that validates a Patch's tag ids
	// against it (FR-064), so a kind registered before this line would carry
	// no such check. taggables is a closure over the registry rather than a
	// fixed list because the registry is not fully populated yet — every
	// kind below still has to register — and by the time usage_count's own
	// query runs, it will be.
	tagRepository, err := tagstore.New(app, cursors, func() []string {
		var collections []string
		for _, k := range registry.Kinds() {
			collections = append(collections, k.Collection())
		}

		return collections
	})
	if err != nil {
		return nil, nil, err
	}

	tagService, err := tagsvc.New(tagRepository, tagRepository, tagRepository, tagsvc.DefaultAuthorizer, auditor)
	if err != nil {
		return nil, nil, err
	}

	registry.SetTagChecker(tagService)

	if medicationRegisterErr := medication.Register(registry, medication.Wiring{
		Repository:   repository,
		Authorizer:   authorizer,
		Codec:        api.MedicationCodec{},
		Schema:       api.MedicationSchema(),
		Views:        views,
		SearchFields: api.MedicationSearchFields,
		Basis:        api.MedicationBasis,
	}); medicationRegisterErr != nil {
		return nil, nil, medicationRegisterErr
	}

	insuranceViews, err := page.NewInsuranceViews()
	if err != nil {
		return nil, nil, err
	}

	insuranceRepository, err := insurancestore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	err = insurance.Register(registry, insurance.Wiring{
		Repository:   insuranceRepository,
		Authorizer:   authorizer,
		Codec:        api.InsuranceCodec{},
		Schema:       api.InsuranceSchema(),
		Views:        insuranceViews,
		SearchFields: api.InsuranceSearchFields,
		Basis:        api.InsuranceBasis,
	})
	if err != nil {
		return nil, nil, err
	}

	equipmentViews, err := page.NewEquipmentViews()
	if err != nil {
		return nil, nil, err
	}

	equipmentRepository, err := equipmentstore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	if err = equipment.Register(registry, equipment.Wiring{
		Repository:   equipmentRepository,
		Authorizer:   authorizer,
		Codec:        api.EquipmentCodec{},
		Schema:       api.EquipmentSchema(),
		Views:        equipmentViews,
		SearchFields: api.EquipmentSearchFields,
		Basis:        api.EquipmentBasis,
	}); err != nil {
		return nil, nil, err
	}

	allergyViews, err := page.NewAllergyViews()
	if err != nil {
		return nil, nil, err
	}

	allergyRepo, err := allergystore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	if err = kinds.RegisterAllergy(registry, kinds.AllergyWiring{
		Repository:   allergyRepo,
		Authorizer:   authorizer,
		Codec:        api.AllergyCodec{},
		Schema:       api.AllergySchema(),
		Views:        allergyViews,
		SearchFields: api.AllergySearchFields,
		Basis:        api.AllergyBasis,
	}); err != nil {
		return nil, nil, err
	}

	conditionViews, err := page.NewConditionViews()
	if err != nil {
		return nil, nil, err
	}

	conditionRepo, err := conditionstore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	if err = kinds.RegisterCondition(registry, kinds.ConditionWiring{
		Repository:   conditionRepo,
		Authorizer:   authorizer,
		Codec:        api.ConditionCodec{},
		Schema:       api.ConditionSchema(),
		Views:        conditionViews,
		SearchFields: api.ConditionSearchFields,
		Basis:        api.ConditionBasis,
	}); err != nil {
		return nil, nil, err
	}

	emergencyContactViews, err := page.NewEmergencyContactViews()
	if err != nil {
		return nil, nil, err
	}

	emergencyContactRepo, err := emergencycontactstore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	if err = kinds.RegisterEmergencyContact(registry, kinds.EmergencyContactWiring{
		Repository:   emergencyContactRepo,
		Authorizer:   authorizer,
		Codec:        api.EmergencyContactCodec{},
		Schema:       api.EmergencyContactSchema(),
		Views:        emergencyContactViews,
		SearchFields: api.EmergencyContactSearchFields,
		Basis:        api.EmergencyContactBasis,
	}); err != nil {
		return nil, nil, err
	}

	immunizationRepo, err := storeimmunization.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	immunizationViews, err := page.NewImmunizationViews()
	if err != nil {
		return nil, nil, err
	}

	if err = kinds.Register(registry, kinds.Wiring{
		Repository:   immunizationRepo,
		Authorizer:   authorizer,
		Codec:        api.ImmunizationCodec{},
		Schema:       api.ImmunizationSchema(),
		Views:        immunizationViews,
		SearchFields: api.ImmunizationSearchFields,
		Basis:        api.ImmunizationBasis,
	}); err != nil {
		return nil, nil, err
	}

	injuryRepo, err := storeinjury.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	injuryViews, err := page.NewInjuryViews()
	if err != nil {
		return nil, nil, err
	}

	if err = kinds.RegisterInjury(registry, kinds.InjuryWiring{
		Repository:   injuryRepo,
		Authorizer:   authorizer,
		Codec:        api.InjuryCodec{},
		Schema:       api.InjurySchema(),
		Views:        injuryViews,
		SearchFields: api.InjurySearchFields,
		Basis:        api.InjuryBasis,
	}); err != nil {
		return nil, nil, err
	}

	identityRepo, err := storeidentity.NewRepository(app)
	if err != nil {
		return nil, nil, err
	}

	identityAuth, err := storeidentity.NewAuthenticator(app)
	if err != nil {
		return nil, nil, err
	}

	mailer, err := pb.NewMailer(app)
	if err != nil {
		return nil, nil, err
	}

	identityService, err := serviceidentity.New(serviceidentity.Config{
		Repository:    identityRepo,
		Authenticator: identityAuth,
		Mailer:        mailer,
		Auditor:       auditor,
		Clock:         serviceidentity.SystemClock{},
	})
	if err != nil {
		return nil, nil, err
	}

	symptomViews, err := page.NewSymptomViews()
	if err != nil {
		return nil, nil, err
	}

	symptomRepo, err := symptomstore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	if symptomRegisterErr := symptom.Register(registry, symptom.Wiring{
		Repository:   symptomRepo,
		Authorizer:   authorizer,
		Codec:        api.SymptomCodec{},
		Schema:       api.SymptomSchema(),
		Views:        symptomViews,
		SearchFields: api.SymptomSearchFields,
		Basis:        api.SymptomBasis,
	}); symptomRegisterErr != nil {
		return nil, nil, symptomRegisterErr
	}

	vitalsViews, err := page.NewVitalsViews()
	if err != nil {
		return nil, nil, err
	}

	vitalsRepo, err := vitalsstore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	if vitalsRegisterErr := vitals.Register(registry, vitals.Wiring{
		Repository:   vitalsRepo,
		Authorizer:   authorizer,
		Codec:        api.VitalsCodec{},
		UnitSystemOf: vitalsUnitSystemOf(identityService),
		Schema:       api.VitalsSchema(),
		Views:        vitalsViews,
		SearchFields: api.VitalsSearchFields,
		Basis:        api.VitalsBasis,
	}); vitalsRegisterErr != nil {
		return nil, nil, vitalsRegisterErr
	}

	encounterViews, err := page.NewEncounterViews()
	if err != nil {
		return nil, nil, err
	}

	encounterRepo, err := encounterstore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	if err = encounter.Register(registry, encounter.Wiring{
		Repository:   encounterRepo,
		Authorizer:   authorizer,
		Codec:        api.EncounterCodec{},
		Schema:       api.EncounterSchema(),
		Views:        encounterViews,
		SearchFields: api.EncounterSearchFields,
		Basis:        api.EncounterBasis,
	}); err != nil {
		return nil, nil, err
	}

	procedureViews, err := page.NewProcedureViews()
	if err != nil {
		return nil, nil, err
	}

	procedureRepo, err := procedurestore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	if err = procedure.Register(registry, procedure.Wiring{
		Repository:   procedureRepo,
		Authorizer:   authorizer,
		Codec:        api.ProcedureCodec{},
		Schema:       api.ProcedureSchema(),
		Views:        procedureViews,
		SearchFields: api.ProcedureSearchFields,
		Basis:        api.ProcedureBasis,
	}); err != nil {
		return nil, nil, err
	}

	treatmentViews, err := page.NewTreatmentViews()
	if err != nil {
		return nil, nil, err
	}

	treatmentRepo, err := treatmentstore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	if err = treatment.Register(registry, treatment.Wiring{
		Repository:   treatmentRepo,
		Authorizer:   authorizer,
		Codec:        api.TreatmentCodec{},
		Schema:       api.TreatmentSchema(),
		Views:        treatmentViews,
		SearchFields: api.TreatmentSearchFields,
		Basis:        api.TreatmentBasis,
	}); err != nil {
		return nil, nil, err
	}

	familyMemberViews, err := page.NewFamilyMemberViews()
	if err != nil {
		return nil, nil, err
	}

	familyMemberRepo, err := familymemberstore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	if familyMemberRegisterErr := familymember.Register(registry, familymember.Wiring{
		Repository:   familyMemberRepo,
		Authorizer:   authorizer,
		Codec:        api.FamilyMemberCodec{},
		Schema:       api.FamilyMemberSchema(),
		Views:        familyMemberViews,
		SearchFields: api.FamilyMemberSearchFields,
		Basis:        api.FamilyMemberBasis,
	}); familyMemberRegisterErr != nil {
		return nil, nil, familyMemberRegisterErr
	}

	// FR-036's three rows, written by the post-commit hooks and by no handler
	// (research D-21). Bound after the kinds are registered, so it audits
	// exactly what this build serves and nothing else.
	if recordAuditErr := pb.BindRecordAudit(app, pb.RecordAudit{
		Trail:   auditor,
		Kinds:   registry.Kinds(),
		Actor:   web.ActorFrom,
		Request: obs.CorrelationID,
	}); recordAuditErr != nil {
		return nil, nil, recordAuditErr
	}

	// Patients are not a kind.Kind (research D-05), so their rows are bound
	// separately rather than through registry.Kinds().
	if patientAuditErr := pb.BindPatientAudit(app, pb.PatientAudit{
		Trail:   auditor,
		Actor:   web.ActorFrom,
		Request: obs.CorrelationID,
	}); patientAuditErr != nil {
		return nil, nil, patientAuditErr
	}

	// FR-036's sign-in rows, from OnRecordAuthRequest and never from a handler
	// (research D-14): PocketBase's native auth route stays reachable, so a
	// handler-side audit would leave one of the two paths to a session
	// unrecorded.
	if authAuditErr := pb.BindAuthAudit(app, pb.AuthAudit{
		Trail:   auditor,
		Request: obs.CorrelationID,
	}); authAuditErr != nil {
		return nil, nil, authAuditErr
	}

	// contracts/streams.md's publisher, bound to the same three post-commit
	// hooks and to the same kinds: a live view of a kind this build does not
	// serve is a live view of nothing.
	if streamErr := pb.BindRecordStream(app, pb.RecordStream{Hub: hub, Kinds: registry.Kinds()}); streamErr != nil {
		return nil, nil, streamErr
	}

	// US9's timeline reads the same cursor codec every other repository
	// registered above does.
	timelineReader, err := timelinestore.New(app, cursors)
	if err != nil {
		return nil, nil, err
	}

	return tagService, timelineReader, nil
}

// directoryServices is the two services practitioners.md and facilities.md's
// ten operations are built on.
type directoryServices struct {
	Practitioner *practitionersvc.Service
	Facility     *facilitysvc.Service
}

// directoryFamily resolves both directory services, once, on first use —
// recordFamily's own mechanism, for the same reason: a repository needs the
// cursor codec, and the codec is keyed from a secret the migrations have only
// just created (store.CursorSecret), which does not exist until apis.Serve's
// OnServe has run.
func directoryFamily(app core.App, measurements *obs.Metrics) func() (directoryServices, error) {
	return sync.OnceValues(func() (directoryServices, error) {
		return registerDirectory(app, measurements)
	})
}

// registerDirectory builds both directory services and binds their audit
// hooks. One call, no route: practitioners and facilities are not a
// kind.Kind (research D-05), so neither goes through registerKinds.
func registerDirectory(app core.App, measurements *obs.Metrics) (directoryServices, error) {
	secret, err := store.CursorSecret(app, "")
	if err != nil {
		return directoryServices{}, err
	}

	cursors, err := store.NewCursorCodec(secret)
	if err != nil {
		return directoryServices{}, err
	}

	trail, err := auditstore.New(app)
	if err != nil {
		return directoryServices{}, err
	}

	auditor, err := auditservice.New(trail, auditservice.WithRequestID(obs.CorrelationID))
	if err != nil {
		return directoryServices{}, err
	}

	practitionerRepo, err := practitionerstore.New(app, cursors)
	if err != nil {
		return directoryServices{}, err
	}

	practitionerService, err := practitionersvc.New(practitionerRepo, practitionersvc.DefaultAuthorizer, auditor)
	if err != nil {
		return directoryServices{}, err
	}

	practitionerService.SetMetrics(measurements)

	facilityRepo, err := facilitystore.New(app, cursors)
	if err != nil {
		return directoryServices{}, err
	}

	facilityService, err := facilitysvc.New(facilityRepo, facilitysvc.NewAuthorizer(), auditor)
	if err != nil {
		return directoryServices{}, err
	}

	facilityService.SetMetrics(measurements)

	if err := pb.BindDirectoryAudit(app, pb.DirectoryAudit{
		Trail: auditor,
		Collections: map[string]audit.TargetKind{
			"practitioners": audit.TargetKindPractitioner,
			"facilities":    audit.TargetKindFacility,
		},
		Actor:   web.ActorFrom,
		Request: obs.CorrelationID,
	}); err != nil {
		return directoryServices{}, err
	}

	return directoryServices{Practitioner: practitionerService, Facility: facilityService}, nil
}

// directoryOperations is the ten operation ids practitioners.md and
// facilities.md publish, for unimplemented() to mark done alongside the other
// groups operations() assembles outside wired().
func directoryOperations() []string {
	return []string{
		api.OpListPractitioners, api.OpCreatePractitioner, api.OpGetPractitioner,
		api.OpUpdatePractitioner, api.OpDeletePractitioner,
		api.OpListFacilities, api.OpCreateFacility, api.OpGetFacility,
		api.OpUpdateFacility, api.OpDeleteFacility,
		page.OpPractitionerListPage, page.OpPractitionerDetailPage,
		page.OpFacilityListPage, page.OpFacilityDetailPage,
		api.OpListTags, api.OpCreateTag, api.OpUpdateTag, api.OpDeleteTag,
		page.OpTagsPage,
	}
}

// unimplemented lists, sorted, the operations still answered by the stub.
//
// It resolves nothing: the handler table's shape is decided by which groups
// have landed, not by whether an instance could build one, so the resolver
// handed in here is one that is never called.
func unimplemented() []string {
	implemented, err := wired(
		func() (*records.Handler, error) { return nil, nil },
		func() (*searchsvc.Service, []kind.Kind, error) { return nil, nil, nil },
		func() (*patient.Service, error) { return nil, nil },
		func() (*timelinesvc.Service, error) { return nil, nil },
		realtime.New(),
	)
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

	for _, opID := range directoryOperations() {
		implemented[opID] = nil
	}

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
