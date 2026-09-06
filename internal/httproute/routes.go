package httproute

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/testsupport/seed"
	"medikube/internal/web/views/shell"
)

// THE declarative table.
//
// PocketBase's router keeps its routes in an unexported field and Go 1.27's
// http.ServeMux still has no pattern-enumeration API, so the inventory cannot
// be recovered from the running process after the fact. This file is the only
// place it exists: api/openapi.json, `medikube routes` and the Playwright
// target list all read it, and none of them has a second source to drift
// against (reconciliation C15, research D-09).
//
// The five verbs, the /api/v1 base and the no-trailing-slash rule are
// contracts/README.md's; the nine pages, their landmarks and their smoke URLs
// are contracts/pages.md's. Both are transcribed here without interpretation —
// a landmark string is a Playwright selector, so changing one is a breaking
// change to the gate and belongs in the contract first.

// The MediKube API base. Every operation of this phase hangs off it.
const apiBase = "/api/v1"

// The PocketBase auth collection whose native routes stay reachable. Spelled
// here rather than imported, following the house pattern of one unexported
// const per package that names it.
const usersCollection = "users"

// The two token pages cannot smoke a real token: a reset token lives thirty
// minutes and a confirmation token twenty-four hours, so any committed fixture
// is expired before CI runs, and minting one for the gate would be test-only
// code in a security path. Both pages answer this token with 200 and the
// "this link is no longer usable" state, which is what FR-074 requires anyway
// and is the most likely real-world visit (contracts/pages.md).
const expiredTokenForSmoke = "expired-token-for-smoke"

// notFoundSmokeURL produces the 404 error view. It is asserted against the
// table in smoketargets_test.go, so a later phase that registers a route here
// breaks the gate loudly instead of quietly re-pointing it at a real page.
const notFoundSmokeURL = "/not-found-for-smoke"

const settingsPath = "/settings"

// statusViewVariant answers the one SmokeVariants entry a status view's own
// kind catalogues (contracts/pages.md §3.5): a page whose kind carries no
// catalogue entry panics, so a kind added to the catalogue with no page to
// carry it — or a status view page assembled without reading the catalogue —
// fails at boot rather than shipping unsmoked (T183a).
func statusViewVariant(k kind.Kind) []string {
	view, found := records.StatusViewFor(k)
	if !found {
		panic(fmt.Sprintf("httproute: %s carries no entry in records.StatusViews", k))
	}

	return []string{view.SmokeURL(seed.AccountAPatientSelfID)}
}

// streamMiddlewares is built once rather than per call to table(), because two
// calls returning two equal-but-distinct handler pointers would make the
// inventory and the registry differ by identity while agreeing on everything
// anybody reads. routes_test.go compares the two tables by value and is what
// found that.
var streamMiddlewares = []*hook.Handler[*core.RequestEvent]{apis.SkipSuccessActivityLog()}

// New returns the registry for this build: every row of the table paired with
// the handler that serves it.
//
// It reports both halves of a mismatch — a row nothing serves and a handler
// naming no row — because either one means the inventory and the application
// have parted company, and that is the single failure this package exists to
// prevent. cmd/medikube treats the error as a boot failure.
func New(handlers Handlers) (*Registry, error) {
	registry := Empty()

	var problems []error
	served := make(map[string]struct{}, len(handlers))

	for _, route := range table() {
		if route.Kind == KindExternal {
			registry.Document(route)

			continue
		}

		handler, wired := handlers[route.OpID]
		if !wired {
			problems = append(problems, fmt.Errorf("%s (%s) has no handler", route.OpID, route.Pattern()))

			continue
		}

		served[route.OpID] = struct{}{}
		registry.Handle(route, handler)
	}

	// Sorted, because ranging a map is not: an error message that reorders
	// itself between runs is one nobody can diff.
	var stray []string
	for opID := range handlers {
		if _, matched := served[opID]; !matched {
			stray = append(stray, opID)
		}
	}

	slices.Sort(stray)

	for _, opID := range stray {
		problems = append(problems, fmt.Errorf("a handler is wired under %q, which is not a route MediKube serves", opID))
	}

	for _, view := range errorViews() {
		registry.DescribeErrorView(view)
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("wire the MediKube route table: %w", errors.Join(problems...))
	}

	return registry, nil
}

// Inventory returns the table described and nothing served. It is what
// `medikube routes`, the OpenAPI generator and the browser gate's target list
// read: all three want the inventory and none of them answers a request. The
// registry it returns cannot Bind, which is exactly what Bind refuses.
func Inventory() *Registry {
	registry := Empty()

	for _, route := range table() {
		registry.describe(route)
	}

	for _, view := range errorViews() {
		registry.DescribeErrorView(view)
	}

	return registry
}

// table is the inventory itself: contracts/README.md's twenty-two operations,
// contracts/pages.md's nine pages, and the PocketBase-native paths that stay
// reachable.
func table() []Route {
	routes := make([]Route, 0, 46)
	routes = append(routes, authRoutes()...)
	routes = append(routes, accountRoutes()...)
	routes = append(routes, recordRoutes()...)
	routes = append(routes, patientRoutes()...)
	routes = append(routes, directoryRoutes()...)
	routes = append(routes, tagRoutes()...)
	routes = append(routes, healthRoutes()...)
	routes = append(routes, pageRoutes()...)
	routes = append(routes, patientPageRoutes()...)
	routes = append(routes, searchRoutes()...)
	routes = append(routes, assetRoutes()...)
	routes = append(routes, externalRoutes()...)

	return routes
}

// The two embedded assets every page's head links (contracts/pages.md's
// shell). Neither is a page: an asset has no landmark and no smoke URL, and
// contracts/pages.md's zero-failed-requests assertion is what actually proves
// both are reachable, on every page, at every viewport.
func assetRoutes() []Route {
	return []Route{
		{
			OpID: "assetAppCSS", Method: http.MethodGet, Path: shell.AppCSSHref,
			Kind: KindAsset, Auth: AuthPublic,
			Summary: "The compiled Tailwind stylesheet every page links.",
		},
		{
			OpID: "assetDatastarJS", Method: http.MethodGet, Path: shell.DatastarJSHref,
			Kind: KindAsset, Auth: AuthPublic,
			Summary: "The vendored Datastar v1.0.2 browser runtime every page loads as a module script.",
		},
	}
}

// contracts/auth.md. Nine operations, five of them public because a person who
// cannot sign in is the only person who needs them.
func authRoutes() []Route {
	base := apiBase + "/auth"

	return []Route{
		{
			OpID: "getAuthConfig", Method: http.MethodGet, Path: base + "/config",
			Kind: KindAPI, Auth: AuthPublic,
			Summary: "What this instance allows: whether registration is open and whether it can send mail.",
		},
		{
			OpID: "register", Method: http.MethodPost, Path: base + "/register",
			Kind: KindAPI, Auth: AuthPublic,
			Summary: "Create an account. 403 registration_closed when registration is disabled, which is the default.",
		},
		{
			OpID: "login", Method: http.MethodPost, Path: base + "/login",
			Kind: KindAPI, Auth: AuthPublic,
			Summary: "Exchange an address and a password for a session.",
		},
		{
			OpID: "refreshSession", Method: http.MethodPost, Path: base + "/refresh",
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Extend the current session.",
		},
		{
			OpID: "logout", Method: http.MethodPost, Path: base + "/logout",
			Kind: KindAPI, Auth: AuthUser,
			Summary: "End the current session.",
		},
		{
			OpID: "requestPasswordReset", Method: http.MethodPost, Path: base + "/password-reset",
			Kind: KindAPI, Auth: AuthPublic,
			Summary: "Send a recovery link. Answers identically whether or not the address has an account (FR-073).",
		},
		{
			OpID: "confirmPasswordReset", Method: http.MethodPost, Path: base + "/password-reset/confirm",
			Kind: KindAPI, Auth: AuthPublic,
			Summary: "Set a new password from a recovery token.",
		},
		{
			// Public would let a caller enumerate addresses; this one resends
			// to the signed-in account's own address and takes none from the
			// caller (contracts/README.md).
			OpID: "requestEmailVerification", Method: http.MethodPost, Path: base + "/verify-email",
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Resend the confirmation to the signed-in account's own address.",
		},
		{
			OpID: "confirmEmailVerification", Method: http.MethodPost, Path: base + "/verify-email/confirm",
			Kind: KindAPI, Auth: AuthPublic,
			Summary: "Confirm an address from a confirmation token.",
		},
	}
}

// contracts/account.md. Nesting is at most one level deep, which is why the
// password lives at /me/password and not somewhere deeper.
func accountRoutes() []Route {
	base := apiBase + "/me"

	return []Route{
		{
			OpID: "getMe", Method: http.MethodGet, Path: base,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "The signed-in account: address, display name and preferences.",
		},
		{
			OpID: "updateMe", Method: http.MethodPatch, Path: base,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Change the display name or the preferences.",
		},
		{
			OpID: "deleteMe", Method: http.MethodDelete, Path: base,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Delete the account and everything it owns, in one transaction. There is no soft delete.",
		},
		{
			OpID: "changePassword", Method: http.MethodPut, Path: base + "/password",
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Replace the password and rotate the token key, which ends every other session.",
		},
		{
			// contracts/active-patient.md. A whole-value replace, hence PUT,
			// collapsing upstream's four routes into one (SHARED-DESIGN
			// §2.3 route 12). Never consulted for authorization (FR-015).
			OpID: "setActivePatient", Method: http.MethodPut, Path: base + "/active-patient",
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Set or clear the person in view. Authorized against the target before the pointer is written.",
		},
	}
}

// contracts/records.md. Six operations serve every clinical kind, now and in
// every later phase: phase 003 registers thirteen more kinds and adds zero
// routes. {kind} is the path parameter, never a kind's own spelling — the
// generic handler resolves it through kind.FromSegment and answers 404 for
// anything undeclared.
func recordRoutes() []Route {
	collection := apiBase + "/records"
	ofKind := collection + "/{kind}"
	one := ofKind + "/{id}"

	return []Route{
		{
			OpID: "listRecords", Method: http.MethodGet, Path: collection,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Every record the signed-in account owns, across kinds, cursor-paginated.",
		},
		{
			OpID: "listRecordsOfKind", Method: http.MethodGet, Path: ofKind,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "One kind's records for the signed-in account, cursor-paginated.",
		},
		{
			OpID: "createRecord", Method: http.MethodPost, Path: ofKind,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Create a record. The owner comes from the session, never from the body.",
		},
		{
			OpID: "getRecord", Method: http.MethodGet, Path: one,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "One record. Another account's id answers 404, byte-identical to an id that never existed.",
		},
		{
			OpID: "updateRecord", Method: http.MethodPatch, Path: one,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Partial update. If-Match is required; a mismatch is 412 carrying the current representation.",
		},
		{
			OpID: "deleteRecord", Method: http.MethodDelete, Path: one,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Delete a record. If-Match is required.",
		},
		{
			// Documented in api/openapi.json as a text/event-stream response:
			// FR-064 covers every operation in the public interface and an SSE
			// endpoint is one (contracts/README.md, contracts/streams.md).
			OpID: "streamRecords", Method: http.MethodGet, Path: apiBase + "/streams/records",
			Kind: KindStream, Auth: AuthUser,
			Summary: "The Datastar element stream. Re-authorised per event; publishes ids, never bodies.",
			// contracts/streams.md: registered with the activity-log skip and
			// exempt from the rate limiter. Both are decisions only the
			// registration can make — a handler cannot bind a middleware that
			// wraps it, and PocketBase's limiter has no per-rule exclusion, so
			// the exemption is an Unbind of the limiter on this route.
			//
			// The skip is bound even though internal/platform/pb unbinds
			// PocketBase's activity logger wholesale, and deliberately: it is
			// what the contract specifies, it costs one struct, and the day the
			// logger is bound back this route does not start writing request
			// URIs into a second store.
			Middlewares: streamMiddlewares,
			// A stream is one request that lasts an hour. Counting it against
			// a 300-per-10-seconds budget is meaningless; being cut off by it
			// on a reconnect storm is not.
			Unbind: []string{apis.DefaultRateLimitMiddlewareId},
		},
	}
}

// contracts/patients.md and contracts/patient-photo.md. Patients are not a
// kind.Kind (research D-05, the anchor rather than a record kind), so these
// seven are their own routes rather than a kind registered with recordRoutes'
// generic six.
func patientRoutes() []Route {
	collection := apiBase + "/patients"
	one := collection + "/{patientId}"
	photo := one + "/photo"

	return []Route{
		{
			OpID: "listPatients", Method: http.MethodGet, Path: collection,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Every person the signed-in account owns. total and owned_count are unconditional.",
		},
		{
			OpID: "createPatient", Method: http.MethodPost, Path: collection,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Add a person. The owner comes from the session, never from the body.",
		},
		{
			OpID: "getPatient", Method: http.MethodGet, Path: one,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "One person. Another account's id answers 404, byte-identical to an id that never existed.",
		},
		{
			OpID: "updatePatient", Method: http.MethodPatch, Path: one,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Partial update. If-Match is required; a mismatch is 412 carrying the current representation.",
		},
		{
			OpID: "deletePatient", Method: http.MethodDelete, Path: one,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Permanent. A self-record is refused with 409: closing the account is what removes it.",
		},
		{
			OpID: "getPatientChart", Method: http.MethodGet, Path: one + "/summary",
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Per-kind counts and the last ten activity entries. Never cached, never an ETag.",
		},
		{
			OpID: "putPatientPhoto", Method: http.MethodPut, Path: photo,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Replace the one photograph. The type is decided from its content, never its name.",
		},
		{
			OpID: "getPatientPhoto", Method: http.MethodGet, Path: photo,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "The photograph's bytes, streamed only after the request is authorized. Never a PocketBase file token.",
		},
		{
			OpID: "deletePatientPhoto", Method: http.MethodDelete, Path: photo,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Remove the photograph and its thumbnails. Idempotent.",
		},
	}
}

// contracts/pages.md P1 and P2. Registered separately from pageRoutes'
// medication pair because both need the plural this package would otherwise
// have to spell twice: patients are not a kind.Kind, so there is no
// kind.Patient.Segment() to read it back from.
func patientPageRoutes() []Route {
	list := "/patients"
	detail := list + "/{patientId}"

	return []Route{
		{
			OpID: "patientListPage", Method: http.MethodGet, Path: list,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "The people this account keeps records for, with its empty state inside the landmark.",
			Landmark: `region[name="Patients"]`, SmokeURL: list,
		},
		{
			OpID: "patientDetailPage", Method: http.MethodGet, Path: detail,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "One person's own chart header.",
			Landmark: `region[name="Patient chart"]`, SmokeURL: list + "/" + seed.AccountAPatientSelfID,
		},
	}
}

// contracts/health.md. Both public and both deliberately incurious: an operator
// learns whether the instance is healthy and nothing about the data.
func healthRoutes() []Route {
	return []Route{
		{
			OpID: "healthz", Method: http.MethodGet, Path: apiBase + "/healthz",
			Kind: KindAPI, Auth: AuthPublic,
			Summary: "Liveness. Answers as long as the process is running.",
		},
		{
			OpID: "readyz", Method: http.MethodGet, Path: apiBase + "/readyz",
			Kind: KindAPI, Auth: AuthPublic,
			Summary: "Readiness, including the database. What `medikube healthcheck` dials.",
		},
	}
}

// contracts/pages.md. The Landmark and SmokeURL columns are why Handle panics:
// a page missing either cannot be registered, so the binary cannot boot, so an
// unsmokeable page cannot ship (FR-067).
func pageRoutes() []Route {
	// The plural is declared once, in internal/domain/kind, and read back
	// here. The page path, the collection and the {kind} segment of the API
	// cannot drift apart because none of them is spelled twice (research
	// D-05).
	list := "/" + kind.Medication.Segment()
	detail := list + "/{id}"
	allergyList := "/" + kind.Allergy.Segment()
	conditionList := "/" + kind.Condition.Segment()
	emergencyContactList := "/" + kind.EmergencyContact.Segment()

	insuranceList := "/" + kind.Insurance.Segment()
	insuranceDetail := insuranceList + "/{id}"

	equipmentList := "/" + kind.Equipment.Segment()
	equipmentDetail := equipmentList + "/{id}"

	encounterList := "/" + kind.Encounter.Segment()
	encounterDetail := encounterList + "/{id}"

	procedureList := "/" + kind.Procedure.Segment()
	procedureDetail := procedureList + "/{id}"

	treatmentList := "/" + kind.Treatment.Segment()
	treatmentDetail := treatmentList + "/{id}"

	return []Route{
		{
			OpID: "loginPage", Method: http.MethodGet, Path: "/login",
			Kind: KindPage, Auth: AuthPublic,
			Summary:  "Sign in.",
			Landmark: `form[name="Sign in"]`, SmokeURL: "/login",
		},
		{
			// Registered unconditionally and rendering 404 when registration
			// is closed. A page that disappears under some configurations is a
			// page the inventory gate cannot check (contracts/pages.md).
			OpID: "registerPage", Method: http.MethodGet, Path: "/register",
			Kind: KindPage, Auth: AuthPublic,
			Summary:  "Create an account. Renders the closed-registration explanation when registration is disabled.",
			Landmark: `form[name="Create account"]`, SmokeURL: "/register",
		},
		{
			OpID: "overviewPage", Method: http.MethodGet, Path: "/",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "The application root.",
			Landmark: `region[name="Overview"]`, SmokeURL: "/",
		},
		{
			// The smoke URL names a patient (research D-13, FR-015): every
			// list over patient-scoped data requires `?patient=`, and the
			// seeded self-record is what the gate has standing to open.
			OpID: "medicationListPage", Method: http.MethodGet, Path: list,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "The record list, with its empty state inside the landmark rather than instead of it.",
			Landmark: `region[name="Medications"]`, SmokeURL: list + "?patient=" + seed.AccountAPatientSelfID,
			SmokeVariants: statusViewVariant(kind.Medication),
		},
		{
			// The smoke URL is the seeded partial-data row: a name, a state
			// and every optional field empty. It reads the seeder's own
			// constant so the gate cannot be pointed at a row the seed stopped
			// writing (data-model §6).
			OpID: "medicationDetailPage", Method: http.MethodGet, Path: detail,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "One record, every value it holds and the time it last changed.",
			Landmark: `article[name="Medication"]`, SmokeURL: list + "/" + seed.NameOnlyID,
		},
		{
			OpID: "allergyListPage", Method: http.MethodGet, Path: allergyList,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "US1's allergy list, with its empty state inside the landmark rather than instead of it.",
			Landmark: kind.Allergy.ListLandmark(), SmokeURL: allergyList + "?patient=" + seed.AccountAPatientSelfID,
			SmokeVariants: statusViewVariant(kind.Allergy),
		},
		{
			OpID: "allergyDetailPage", Method: http.MethodGet, Path: allergyList + "/{id}",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "US1's allergy detail: every value it holds and the time it last changed.",
			Landmark: kind.Allergy.DetailLandmark(), SmokeURL: allergyList + "/" + seed.CriticalAllergyID,
		},
		{
			OpID: "conditionListPage", Method: http.MethodGet, Path: conditionList,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "US1's condition list, with its empty state inside the landmark rather than instead of it.",
			Landmark: kind.Condition.ListLandmark(), SmokeURL: conditionList + "?patient=" + seed.AccountAPatientSelfID,
			SmokeVariants: statusViewVariant(kind.Condition),
		},
		{
			OpID: "conditionDetailPage", Method: http.MethodGet, Path: conditionList + "/{id}",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "US1's condition detail: every value it holds and the time it last changed.",
			Landmark: kind.Condition.DetailLandmark(), SmokeURL: conditionList + "/" + seed.ResolvedConditionID,
		},
		{
			OpID: "emergencyContactListPage", Method: http.MethodGet, Path: emergencyContactList,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "US1's emergency contact list, with its empty state inside the landmark rather than instead of it.",
			Landmark: kind.EmergencyContact.ListLandmark(), SmokeURL: emergencyContactList + "?patient=" + seed.AccountAPatientSelfID,
		},
		{
			OpID: "emergencyContactDetailPage", Method: http.MethodGet, Path: emergencyContactList + "/{id}",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "US1's emergency contact detail: every value it holds and the time it last changed.",
			Landmark: kind.EmergencyContact.DetailLandmark(), SmokeURL: emergencyContactList + "/" + seed.PrimaryContactID,
		},
		{
			// The smoke URL points at the patient whose /immunizations is
			// seeded empty on purpose (T116, contracts/pages.md §5): the
			// empty-state path needs a gate that actually reaches it.
			OpID: "immunizationListPage", Method: http.MethodGet, Path: "/" + kind.Immunization.Segment(),
			Kind: KindPage, Auth: AuthUser,
			Summary:  "The record list, with its empty state inside the landmark rather than instead of it.",
			Landmark: `region[name="Vaccinations"]`, SmokeURL: "/" + kind.Immunization.Segment() + "?patient=" + seed.AccountAPatientParentID,
		},
		{
			OpID: "immunizationDetailPage", Method: http.MethodGet, Path: "/" + kind.Immunization.Segment() + "/{id}",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "One record, every value it holds and the time it last changed.",
			Landmark: `article[name="Vaccination"]`, SmokeURL: "/" + kind.Immunization.Segment() + "/" + seed.ImmunizationSampleID,
		},
		{
			OpID: "injuryListPage", Method: http.MethodGet, Path: "/" + kind.Injury.Segment(),
			Kind: KindPage, Auth: AuthUser,
			Summary:  "The record list, with its empty state inside the landmark rather than instead of it.",
			Landmark: `region[name="Injuries"]`, SmokeURL: "/" + kind.Injury.Segment() + "?patient=" + seed.AccountAPatientSelfID,
			SmokeVariants: statusViewVariant(kind.Injury),
		},
		{
			OpID: "injuryDetailPage", Method: http.MethodGet, Path: "/" + kind.Injury.Segment() + "/{id}",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "One record, every value it holds and the time it last changed.",
			Landmark: `article[name="Injury"]`, SmokeURL: "/" + kind.Injury.Segment() + "/mkinjamara00001",
		},
		{
			OpID: "insuranceListPage", Method: http.MethodGet, Path: insuranceList,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "US5's insurance list, with its empty state inside the landmark rather than instead of it.",
			Landmark: `region[name="Insurance"]`, SmokeURL: insuranceList + "?patient=" + seed.AccountAPatientSelfID,
			SmokeVariants: statusViewVariant(kind.Insurance),
		},
		{
			OpID: "insuranceDetailPage", Method: http.MethodGet, Path: insuranceDetail,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "One policy, every value it holds and the time it last changed.",
			Landmark: `article[name="Insurance"]`, SmokeURL: insuranceList + "/" + seed.InsurancePrimaryID,
		},
		{
			OpID: "equipmentListPage", Method: http.MethodGet, Path: equipmentList,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "US5's equipment list, with its empty state inside the landmark rather than instead of it.",
			Landmark: `region[name="Equipment"]`, SmokeURL: equipmentList + "?patient=" + seed.AccountAPatientSelfID,
			SmokeVariants: statusViewVariant(kind.Equipment),
		},
		{
			OpID: "equipmentDetailPage", Method: http.MethodGet, Path: equipmentDetail,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "One piece of equipment, every value it holds and the time it last changed.",
			Landmark: `article[name="Equipment"]`, SmokeURL: equipmentList + "/" + seed.EquipmentOverdueID,
		},
		{
			OpID: "symptomListPage", Method: http.MethodGet, Path: "/" + kind.Symptom.Segment(),
			Kind: KindPage, Auth: AuthUser,
			Summary:  "The symptom episode list, with its empty state inside the landmark rather than instead of it.",
			Landmark: `region[name="Symptoms"]`, SmokeURL: "/" + kind.Symptom.Segment() + "?patient=" + seed.AccountAPatientSelfID,
		},
		{
			OpID: "symptomDetailPage", Method: http.MethodGet, Path: "/" + kind.Symptom.Segment() + "/{id}",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "One symptom episode, every value it holds and the aggregate over its own name.",
			Landmark: `article[name="Symptom episode"]`, SmokeURL: "/" + kind.Symptom.Segment() + "/" + seed.SymptomHeadacheOne,
		},
		{
			OpID: "measurementsListPage", Method: http.MethodGet, Path: "/" + kind.Vitals.Segment(),
			Kind: KindPage, Auth: AuthUser,
			Summary:  "The measurement set list, with its empty state inside the landmark rather than instead of it.",
			Landmark: `region[name="Measurements"]`, SmokeURL: "/" + kind.Vitals.Segment() + "?patient=" + seed.AccountAPatientSelfID,
		},
		{
			OpID: "measurementsDetailPage", Method: http.MethodGet, Path: "/" + kind.Vitals.Segment() + "/{id}",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "One measurement set, only the values recorded and bmi when both height and weight are present.",
			Landmark: `article[name="Measurement set"]`, SmokeURL: "/" + kind.Vitals.Segment() + "/" + seed.VitalsOne,
		},
		{
			OpID: "encounterListPage", Method: http.MethodGet, Path: encounterList,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "The encounter list, with its empty state inside the landmark rather than instead of it.",
			Landmark: `region[name="Encounters"]`, SmokeURL: encounterList + "?patient=" + seed.AccountAPatientSelfID,
		},
		{
			OpID: "encounterDetailPage", Method: http.MethodGet, Path: encounterDetail,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "One encounter, every value it holds and the time it last changed.",
			Landmark: `article[name="Encounter"]`, SmokeURL: encounterList + "/" + seed.EncounterNameOnlyID,
		},
		{
			OpID: "procedureListPage", Method: http.MethodGet, Path: procedureList,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "The procedure list, with its empty state inside the landmark rather than instead of it.",
			Landmark: `region[name="Procedures"]`, SmokeURL: procedureList + "?patient=" + seed.AccountAPatientSelfID,
			SmokeVariants: statusViewVariant(kind.Procedure),
		},
		{
			OpID: "procedureDetailPage", Method: http.MethodGet, Path: procedureDetail,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "One procedure, every value it holds and the time it last changed.",
			Landmark: `article[name="Procedure"]`, SmokeURL: procedureList + "/" + seed.ProcedureNameOnlyID,
		},
		{
			OpID: "treatmentListPage", Method: http.MethodGet, Path: treatmentList,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "The treatment list, with its empty state inside the landmark rather than instead of it.",
			Landmark: `region[name="Treatments"]`, SmokeURL: treatmentList + "?patient=" + seed.AccountAPatientSelfID,
		},
		{
			OpID: "treatmentDetailPage", Method: http.MethodGet, Path: treatmentDetail,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "One treatment, every value it holds and the time it last changed.",
			Landmark: `article[name="Treatment"]`, SmokeURL: treatmentList + "/" + seed.TreatmentNameOnlyID,
		},
		{
			OpID: "familyHistoryListPage", Method: http.MethodGet, Path: "/" + kind.FamilyMember.Segment(),
			Kind: KindPage, Auth: AuthUser,
			Summary:  "The family history list, empty on account A's self patient so the empty state is exercised.",
			Landmark: `region[name="Family history"]`, SmokeURL: "/" + kind.FamilyMember.Segment() + "?patient=" + seed.AccountAPatientSelfID,
		},
		{
			OpID: "familyHistoryDetailPage", Method: http.MethodGet, Path: "/" + kind.FamilyMember.Segment() + "/{id}",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "One relative, every condition recorded against them.",
			Landmark: `article[name="Relative"]`, SmokeURL: "/" + kind.FamilyMember.Segment() + "/" + seed.FamilyMemberGrandmotherID,
		},
		{
			OpID: "practitionerListPage", Method: http.MethodGet, Path: "/practitioners",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "contracts/pages.md P3: the account's practitioner directory.",
			Landmark: `region[name="Practitioners"]`, SmokeURL: "/practitioners",
		},
		{
			OpID: "practitionerDetailPage", Method: http.MethodGet, Path: "/practitioners/{id}",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "contracts/pages.md P4: one practitioner, with usage.",
			Landmark: `article[name="Practitioner"]`, SmokeURL: "/practitioners/" + seed.AccountAPractitionerID,
		},
		{
			OpID: "facilityListPage", Method: http.MethodGet, Path: "/facilities",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "contracts/pages.md P5: the account's directory of places of care.",
			Landmark: `region[name="Facilities"]`, SmokeURL: "/facilities",
		},
		{
			OpID: "facilityDetailPage", Method: http.MethodGet, Path: "/facilities/{id}",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "contracts/pages.md P6: one facility, with usage.",
			Landmark: `article[name="Facility"]`, SmokeURL: "/facilities/" + seed.AccountAFacilityPracticeID,
		},
		{
			OpID: "tagsPage", Method: http.MethodGet, Path: "/tags",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "contracts/pages.md: the tag manager — create, rename, recolour, delete, with usage counts and a delete confirmation naming how many records carry the tag.",
			Landmark: `region[name="Tags"]`, SmokeURL: "/tags",
		},
		{
			// /timeline requires ?patient=, same as /search: without one it
			// renders the explicit "choose a person" state rather than
			// guessing (FR-070, US8-3, US9).
			OpID: "timelinePage", Method: http.MethodGet, Path: "/timeline",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "contracts/pages.md §3: one chronological view across every kind, narrowable by kind, date range and tag.",
			Landmark: `region[name="Timeline"]`, SmokeURL: "/timeline?patient=" + seed.AccountAPatientSelfID,
		},
		{
			OpID: "settingsPage", Method: http.MethodGet, Path: settingsPath,
			Kind: KindPage, Auth: AuthUser,
			Summary:  "Display name, preferences, address confirmation and the danger zone.",
			Landmark: `region[name="Settings"]`, SmokeURL: settingsPath,
		},
		{
			OpID: "forgotPasswordPage", Method: http.MethodGet, Path: "/forgot-password",
			Kind: KindPage, Auth: AuthPublic,
			Summary:  "Ask for a recovery link.",
			Landmark: `form[name="Reset password"]`, SmokeURL: "/forgot-password",
		},
		{
			OpID: "resetPasswordPage", Method: http.MethodGet, Path: "/reset-password/{token}",
			Kind: KindPage, Auth: AuthPublic,
			Summary:  "Choose a new password from a recovery link. An expired link answers 200 with the ask-again state.",
			Landmark: `form[name="Choose a new password"]`, SmokeURL: "/reset-password/" + expiredTokenForSmoke,
		},
		{
			OpID: "verifyEmailPage", Method: http.MethodGet, Path: "/verify-email/{token}",
			Kind: KindPage, Auth: AuthPublic,
			Summary:  "Confirm an address from a confirmation link. An expired link answers 200 with the ask-again state.",
			Landmark: `region[name="Email confirmation"]`, SmokeURL: "/verify-email/" + expiredTokenForSmoke,
		},
	}
}

// contracts/search.md and contracts/pages.md §3: US8's one API operation and
// its one page. Both require `?patient=`; the page renders an explicit
// "choose a person" state rather than falling back to the active patient
// when it is absent (FR-070, US8-3) — the smoke URL below deliberately
// carries no `?q=`, which is the "before a term is entered" empty state
// contracts/pages.md §5 seeds on purpose.
func searchRoutes() []Route {
	return []Route{
		{
			OpID: "search", Method: http.MethodGet, Path: apiBase + "/search",
			Kind: KindAPI, Auth: AuthUser,
			Summary: "One search across a named person's records of every kind, grouped and paged per kind.",
		},
		{
			OpID: "searchPage", Method: http.MethodGet, Path: "/search",
			Kind: KindPage, Auth: AuthUser,
			Summary:  "contracts/pages.md P: one search over a named person's whole chart.",
			Landmark: `search`, SmokeURL: "/search?patient=" + seed.AccountAPatientSelfID,
		},
	}
}

// contracts/README.md, "Documented PocketBase-native paths that stay public".
//
// PocketBase serves every one of these, so Bind skips them. They are recorded
// because the lockdown is scoped to the record subtree precisely so they
// survive, and a reachable path nobody wrote down is a path somebody discovers
// by accident.
func externalRoutes() []Route {
	native := "/api/collections/"
	users := native + usersCollection
	superusers := native + "_superusers"

	return []Route{
		{
			OpID: "nativeAdminUI", Method: http.MethodGet, Path: "/_/{path...}",
			Kind: KindExternal, Auth: AuthAdmin,
			Summary: "The PocketBase superuser admin UI. It ships in production, hardened: mandatory MFA, mandatory IP allowlist, every session audited (constitution VII).",
		},
		{
			OpID: "nativeSuperuserAuthWithPassword", Method: http.MethodPost, Path: superusers + "/auth-with-password",
			Kind: KindExternal, Auth: AuthAdmin,
			Summary: "The admin UI's own authentication.",
		},
		{
			OpID: "nativeSuperuserAuthRefresh", Method: http.MethodPost, Path: superusers + "/auth-refresh",
			Kind: KindExternal, Auth: AuthAdmin,
			Summary: "The admin UI's own session refresh.",
		},
		{
			OpID: "nativeSuperuserAuthMethods", Method: http.MethodGet, Path: superusers + "/auth-methods",
			Kind: KindExternal, Auth: AuthAdmin,
			Summary: "The admin UI's own auth-method discovery.",
		},
		{
			OpID: "nativeUserAuthWithPassword", Method: http.MethodPost, Path: users + "/auth-with-password",
			Kind: KindExternal, Auth: AuthPublic,
			Summary: "PocketBase-native sign-in. MediKube's /api/v1/auth/login is the supported path; both are audited, because the row is written from OnRecordAuthRequest rather than from a handler (research D-14).",
		},
		{
			OpID: "nativeUserAuthRefresh", Method: http.MethodPost, Path: users + "/auth-refresh",
			Kind: KindExternal, Auth: AuthUser,
			Summary: "PocketBase-native session refresh.",
		},
		{
			OpID: "nativeUserAuthMethods", Method: http.MethodGet, Path: users + "/auth-methods",
			Kind: KindExternal, Auth: AuthPublic,
			Summary: "PocketBase-native auth-method discovery.",
		},
		{
			OpID: "nativeUserRequestPasswordReset", Method: http.MethodPost, Path: users + "/request-password-reset",
			Kind: KindExternal, Auth: AuthPublic,
			Summary: "PocketBase-native recovery request. Same token rules as MediKube's, because it is the same code MediKube's handler calls.",
		},
		{
			OpID: "nativeUserConfirmPasswordReset", Method: http.MethodPost, Path: users + "/confirm-password-reset",
			Kind: KindExternal, Auth: AuthPublic,
			Summary: "PocketBase-native recovery confirmation.",
		},
		{
			OpID: "nativeUserRequestVerification", Method: http.MethodPost, Path: users + "/request-verification",
			Kind: KindExternal, Auth: AuthPublic,
			Summary: "PocketBase-native confirmation request.",
		},
		{
			OpID: "nativeUserConfirmVerification", Method: http.MethodPost, Path: users + "/confirm-verification",
			Kind: KindExternal, Auth: AuthPublic,
			Summary: "PocketBase-native confirmation.",
		},
	}
}

// contracts/pages.md, "The three error views". All three render inside the full
// shell, so a person who hits an error still has navigation, and none of them
// carries anything but the request id.
func errorViews() []ErrorView {
	return []ErrorView{
		{
			// The privacy view. FR-033 requires a request for another
			// account's record to be byte-identical to a request for one that
			// never existed, so this is what both produce.
			Name: "notFound", Status: http.StatusNotFound,
			Landmark: `region[name="Not found"]`,
			Auth:     AuthPublic,
			SmokeURL: notFoundSmokeURL,
		},
		{
			// Reached by opening a session-required page with no session,
			// which is why its Auth is public: the gate visits it signed out.
			// It renders the sign-in prompt rather than a 404, because the
			// existence of /settings is not information about anybody.
			Name: "signInRequired", Status: http.StatusForbidden,
			Landmark: `region[name="Sign in required"]`,
			Auth:     AuthPublic,
			SmokeURL: settingsPath,
		},
		{
			Name: "serverError", Status: http.StatusInternalServerError,
			Landmark: `region[name="Something went wrong"]`,
			Auth:     AuthPublic,
			Unreachable: "no URL in a shipped build produces a 500 on purpose, and a route that deliberately fails is a worse " +
				"defect than an unsmoked error page. It belongs to contracts/pages.md's negative-control family, alongside " +
				"the removed-landmark and console-error builds, and is covered by internal/web/page/errors_test.go (T230).",
		},
	}
}

// contracts/practitioners.md and contracts/facilities.md. Two account-owned
// directories, ten operations, neither a kind.Kind (research D-05): they are
// what a record's practitioner and pharmacy fields point at, not a record kind
// themselves.
func directoryRoutes() []Route {
	practitioners := apiBase + "/practitioners"
	onePractitioner := practitioners + "/{id}"

	facilities := apiBase + "/facilities"
	oneFacility := facilities + "/{id}"

	return []Route{
		{
			OpID: "listPractitioners", Method: http.MethodGet, Path: practitioners,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "The account's practitioner directory, cursor-paginated. Also the type-ahead behind every practitioner picker.",
		},
		{
			OpID: "createPractitioner", Method: http.MethodPost, Path: practitioners,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Add a practitioner to the account's directory.",
		},
		{
			OpID: "getPractitioner", Method: http.MethodGet, Path: onePractitioner,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "One practitioner, with usage. Another account's id answers 404.",
		},
		{
			OpID: "updatePractitioner", Method: http.MethodPatch, Path: onePractitioner,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Partial update. If-Match is required; a mismatch is 412 carrying the current representation.",
		},
		{
			OpID: "deletePractitioner", Method: http.MethodDelete, Path: onePractitioner,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Delete a practitioner. If-Match is required. Every referencing record survives with the reference cleared.",
		},
		{
			OpID: "listFacilities", Method: http.MethodGet, Path: facilities,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "The account's directory of places of care, cursor-paginated. Also the type-ahead behind every facility picker.",
		},
		{
			OpID: "createFacility", Method: http.MethodPost, Path: facilities,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Add a facility to the account's directory.",
		},
		{
			OpID: "getFacility", Method: http.MethodGet, Path: oneFacility,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "One facility, with usage. Another account's id answers 404.",
		},
		{
			OpID: "updateFacility", Method: http.MethodPatch, Path: oneFacility,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Partial update. If-Match is required; a mismatch is 412 carrying the current representation.",
		},
		{
			OpID: "deleteFacility", Method: http.MethodDelete, Path: oneFacility,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Delete a facility. If-Match is required. Every referencing record survives with the reference cleared.",
		},
	}
}

// contracts/tags.md. Tags belong to the account, not to a patient, and are
// not a kind.Kind: they are what a record's `tags` relation points at, the
// same reasoning directoryRoutes documents for practitioners and facilities.
func tagRoutes() []Route {
	tags := apiBase + "/tags"
	oneTag := tags + "/{id}"

	return []Route{
		{
			OpID: "listTags", Method: http.MethodGet, Path: tags,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "The account's own tags, cursor-paginated, with each tag's derived usage_count. Also the autocomplete every tag picker types against.",
		},
		{
			OpID: "createTag", Method: http.MethodPost, Path: tags,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Add a tag to the account's own vocabulary. 409 duplicate_name on a case-insensitive collision.",
		},
		{
			OpID: "updateTag", Method: http.MethodPatch, Path: oneTag,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Rename or recolour a tag. One row update: every carrier follows with no second write. No If-Match — a tag is not a clinical record.",
		},
		{
			OpID: "deleteTag", Method: http.MethodDelete, Path: oneTag,
			Kind: KindAPI, Auth: AuthUser,
			Summary: "Delete a tag. Every referencing record survives with the tag removed; none is destroyed.",
		},
	}
}
