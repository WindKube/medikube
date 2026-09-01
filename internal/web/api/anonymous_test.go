package api_test

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	pbrouter "github.com/pocketbase/pocketbase/tools/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/obs"
	"medikube/internal/testsupport"
	"medikube/internal/web/apitest"
)

// T227, FR-034.
//
// Driven by the route table, never by a list written here. internal/web/page/served_test.go
// is why: its predecessor walked a curated list of pages, and that is exactly
// how three finished-looking pages shipped answering 501 while being declared,
// smoked and named in the browser gate's targets. A route that is added to
// httproute's table and forgotten here would be a route nothing ever asked to
// refuse an anonymous caller.
//
// FR-034 names "the sign-in page, the sign-up page, the recovery and
// confirmation pages and the liveness and readiness signals". The task text
// calls that "the public five"; the table's AuthPublic column holds THIRTEEN
// routes today, six of them PocketBase-native, and the column is what the
// router enforces (httproute.Registry.Bind binds apis.RequireAuth from it). So
// the table is what this walks, and the count is measured below rather than
// asserted from the task's prose.

// anonymousParameters binds every path parameter the route table declares.
//
// It is keyed by the parameter rather than by the route, so a new route made of
// parameters that are already here is covered the moment it is declared. A
// route carrying a parameter that is NOT here fails the walk by name, which is
// the loud half: a silent skip is how a curated list rots.
func anonymousParameters(recordID string) map[string]string {
	return map[string]string{
		// Never the plural spelled by hand: internal/domain/kind declares it
		// once and the AST guard bites a second spelling.
		"{kind}":       kind.Medication.Segment(),
		"{id}":         recordID,
		"{recordId}":   recordID,
		"{collection}": kind.Medication.Collection(),
		"{token}":      "not-a-token-this-instance-ever-issued",
		"{filename}":   "attachment.pdf",
		// The admin UI's catch-all. Empty is the UI's own root, which is the
		// URL a person actually opens.
		"{path...}": "",
	}
}

var pathParameter = regexp.MustCompile(`\{[^}]*\}`)

// anonymousExempt is the one door out: a route the table does not mark public
// and that nevertheless answers an anonymous caller. The value is the reason
// and the task that owns it, keyed by the exact OpID — never a prefix, because
// internal/store/filter_test.go:697 records a real bug where a prefix exemption
// covered four more things than anybody meant.
//
// It is checked in both directions. A route listed here that starts refusing is
// a stale entry and fails, so the table cannot outlive what it excuses.
var anonymousExempt = map[string]anonymousException{
	"overviewPage": {
		Answers: http.StatusNotImplemented,
		Reason: "T264, phase 6 (US4): the overview page has no handler yet, so every caller — signed in or not — gets " +
			"the stub and there is no refusal here to assert. internal/web/page/served_test.go carries the same " +
			"exemption for the same reason and is what fails when the page lands",
	},
	"nativeSuperuserAuthMethods": {
		Answers: http.StatusOK,
		Reason: "contracts/README.md: PocketBase's auth-method discovery for the superuser collection. It publishes " +
			"which authentication methods THIS INSTANCE is configured for — password on, oauth2 off, mfa off — and " +
			"nothing about any account or any record. The admin UI reads it before anybody has signed in, so a session " +
			"cannot be required for it; the users-collection twin of it is declared AuthPublic for the same reason",
	},
}

// anonymousException is one excused route. The status is part of the exemption
// and not a footnote: it is what makes the entry self-cancelling. overviewPage
// is excused because it answers the 501 stub, so the day it answers anything
// else the excuse fails and somebody has to write the refusal down instead.
type anonymousException struct {
	Answers int
	Reason  string
}

// anonymousNotServedHere names a route the table declares that THIS BUILD does
// not register, so its answer proves nothing about a refusal.
//
// This map is the antidote to a test that is true by absence. PocketBase only
// binds the admin UI when its dist directory is embedded (apis/extensions.go:20
// returns early when ui.DistDirFS is nil), so under the test harness /_/ is
// answered by the router's catch-all — a 404 that looks exactly like a refusal
// and is nothing of the kind. Asserting it here would be asserting that a route
// which does not exist is closed.
var anonymousNotServedHere = map[string]string{
	"nativeAdminUI": "apis/extensions.go:20 — PocketBase binds /_/{path...} inside OnServe only when ui.DistDirFS is " +
		"non-nil, and the admin UI's dist is not embedded in a test build. The 404 here is the catch-all, not a " +
		"refusal. The deployed admin UI is covered by constitution VII's mandatory MFA, address allowlist and session " +
		"auditing (internal/platform/pb/hooks_admin_session_test.go), not by this file",
}

// servedInstance is an assembled MediKube together with the router that serves
// it, so a route's answer can be read alongside whether the route exists at all.
type servedInstance struct {
	caller *caller
	router *pbrouter.Router[*core.RequestEvent]
}

// newServedInstance captures se.Router after the whole OnServe chain has run.
//
// After, and that is the point: PocketBase binds the admin UI at priority 9999
// and MediKube's own table at -1000, so a router read before se.Next() returns
// is missing whatever binds later. Reading it here rather than building a
// second router is what makes HasRoute answer for the router that actually
// serves these requests, rather than for one assembled alongside it.
func newServedInstance(t *testing.T, email string) *servedInstance {
	t.Helper()

	instance := apitest.New(t)

	served := new(servedInstance)

	instance.App.OnServe().BindFunc(func(se *core.ServeEvent) error {
		err := se.Next()
		served.router = se.Router

		return err
	})

	handler := testsupport.NewEdgeHandler(t, instance.App)

	token := ""
	if email != "" {
		token = testsupport.UserToken(t, instance.App, email)
	}

	served.caller = &caller{t: t, app: instance.App, handler: handler, token: token}

	require.NotNil(t, served.router, "the OnServe chain never reached the observer, so nothing was captured")

	return served
}

// bound is the path as the router registered it.
//
// httproute.bindPath is unexported and this is its one case: Go 1.22's ServeMux
// reads "GET /" as a prefix pattern matching every unmatched GET, so the
// application root is registered in its exact-match form. The inventory keeps
// "/" because that is the URL contracts/pages.md declares.
func bound(path string) string {
	if path == "/" {
		return "/{$}"
	}

	return path
}

// registeredPattern is the pattern the router actually holds for a path.
//
// The inventory records a PocketBase-native route by the concrete URL it
// serves — /api/collections/users/auth-refresh — because that is the address a
// client uses and the address contracts/README.md publishes. PocketBase
// registers the whole family once, under a collection parameter
// (apis/record_auth.go:20 groups them all under /collections/{collection}), so
// asking the router about the concrete spelling answers no for a route that is
// very much registered.
//
// This is a rule rather than a per-route table on purpose: a later phase that
// documents another native path under a collection needs nothing here.
func registeredPattern(path string) string {
	const under = "/api/collections/"

	if !strings.HasPrefix(path, under) {
		return path
	}

	rest := strings.TrimPrefix(path, under)

	collection, tail, hasTail := strings.Cut(rest, "/")
	if !hasTail || collection == "" {
		return path
	}

	return under + "{collection}/" + tail
}

// bind fills a route's path parameters, failing by name on one nothing binds.
func bind(t *testing.T, path string, values map[string]string) string {
	t.Helper()

	for _, parameter := range pathParameter.FindAllString(path, -1) {
		value, declared := values[parameter]
		require.Truef(t, declared,
			"the route table declares path parameter %s and nothing here binds it, so %s would be requested with the "+
				"parameter still in the URL and refused for the wrong reason", parameter, path)

		path = strings.Replace(path, parameter, value, 1)
	}

	// A wildcard bound to nothing leaves a trailing slash — /_/{path...}
	// becomes /_/ — which is the URL a person opens. The application root is
	// the one path that legitimately IS a slash and must survive the trim.
	if trimmed := strings.TrimSuffix(path, "/"); trimmed != "" {
		return trimmed
	}

	return path
}

// anonymousBody is what a write is sent with. It is a valid empty JSON object
// on purpose: a malformed body would be refused by the body decoder, which
// satisfies "the anonymous caller did not succeed" while asserting nothing at
// all about authentication.
func anonymousBody(method string) string {
	if method == http.MethodGet || method == http.MethodDelete {
		return ""
	}

	return `{}`
}

// TestEveryRouteTheTableDoesNotPublishRefusesAnAnonymousCaller is FR-034 over
// the whole inventory.
func TestEveryRouteTheTableDoesNotPublishRefusesAnAnonymousCaller(t *testing.T) {
	t.Parallel()

	served := newServedInstance(t, "")
	anonymous := served.caller

	parameters := anonymousParameters(testsupport.NameOnlyMedicationID)

	// What an anonymous caller must never be told, whatever the status: the id
	// of a real record, the drug it names, and the address of a real account.
	// A refusal asserted on its status alone is a refusal that could carry the
	// whole record in its body.
	secrets := []string{
		testsupport.NameOnlyMedicationID,
		"Paracetamol",
		testsupport.AccountAEmail,
		testsupport.AccountAID,
	}

	var public, refused, exempt, absent int

	hitExempt := make(map[string]bool, len(anonymousExempt))
	hitAbsent := make(map[string]bool, len(anonymousNotServedHere))

	for _, route := range httproute.Inventory().Routes() {
		url := bind(t, route.Path, parameters)

		if route.Auth == httproute.AuthPublic {
			public++

			continue
		}

		if reason, missing := anonymousNotServedHere[route.OpID]; missing {
			absent++
			hitAbsent[route.OpID] = true

			t.Run(route.OpID+" is not served by this build", func(t *testing.T) {
				assert.Falsef(t, served.router.HasRoute(route.Method, registeredPattern(bound(route.Path))),
					"%s is written off as unserved (%s) and this build registers it after all: "+
						"strike it out of anonymousNotServedHere and assert its refusal", route.OpID, reason)
			})

			continue
		}

		t.Run(route.OpID, func(t *testing.T) {
			// Registration first, and it is not a formality. A route nothing
			// registers is answered by the router's catch-all with a 404 that
			// is indistinguishable from a refusal — so without this the whole
			// walk could pass by asserting that absent routes are closed.
			require.Truef(t, served.router.HasRoute(route.Method, registeredPattern(bound(route.Path))),
				"%s (%s) is declared in the route table and this build does not register it, so its answer below is "+
					"the catch-all rather than a refusal", route.OpID, route.Pattern())

			answer := anonymous.do(route.Method, url, anonymousBody(route.Method), nil)

			if excuse, excused := anonymousExempt[route.OpID]; excused {
				hitExempt[route.OpID] = true

				assert.Equalf(t, excuse.Answers, answer.Status,
					"%s is exempt (%s) and no longer answers %d: strike it out of anonymousExempt and assert its refusal",
					route.OpID, excuse.Reason, excuse.Answers)
			} else {
				assert.GreaterOrEqualf(t, answer.Status, http.StatusBadRequest,
					"%s (%s) is declared %s and answered an anonymous caller: %s",
					route.OpID, route.Pattern(), route.Auth, answer.Body)
			}

			for _, secret := range secrets {
				assert.NotContainsf(t, answer.Body, secret,
					"%s discloses %q to an anonymous caller", route.OpID, secret)
			}
		})

		if _, excused := anonymousExempt[route.OpID]; excused {
			exempt++
		} else {
			refused++
		}
	}

	for opID, excuse := range anonymousExempt {
		assert.Truef(t, hitExempt[opID], "%s is exempt (%s) and is not a route in the table: strike it out", opID, excuse.Reason)
	}

	for opID, reason := range anonymousNotServedHere {
		assert.Truef(t, hitAbsent[opID], "%s is written off as unserved (%s) and is not a route in the table: strike it out",
			opID, reason)
	}

	// The guard on the guard. The refused count is the one that matters: an
	// exemption table that grew until it covered the table, or a walk that
	// stopped finding routes, would otherwise pass this test having asserted
	// nothing.
	require.Greater(t, refused, 15, "almost nothing is being refused; the walk or the exemption table has eaten the gate")
	require.Greater(t, public, 10, "the table has stopped declaring public routes, so this is walking something else")
	require.LessOrEqual(t, exempt, 2, "anonymousExempt has become an off switch rather than a door")
	require.LessOrEqual(t, absent, 1, "more of the table is unserved than served, so this asserts almost nothing")
}

// existenceProbe is one route asked the same question twice — once about
// something real and once about something that has never existed.
type existenceProbe struct {
	name string

	// what distinguishes the two requests, for the failure message.
	real, absent string
}

// TestAnAnonymousRefusalNeverSaysWhetherTheTargetExists is FR-034's second
// clause.
//
// The refusal is the same refusal whether the thing asked for is there or not.
// Otherwise an anonymous caller enumerates: the status alone would answer "is
// there a medication with this id", which is the question FR-033 spends the
// whole record surface refusing to answer for a signed-in stranger.
//
// The comparison is of the whole response — status, body and header set — with
// only the correlation id removed, exactly as testsupport.RunOwnershipMatrix
// compares a stranger's refusal with a genuine miss.
func TestAnAnonymousRefusalNeverSaysWhetherTheTargetExists(t *testing.T) {
	t.Parallel()

	served := newServedInstance(t, "")
	anonymous := served.caller

	probes := []existenceProbe{
		{
			name:   "a record on the JSON API",
			real:   recordURL(testsupport.NameOnlyMedicationID),
			absent: recordURL(missingID),
		},
		{
			name:   "a record on the rendered page",
			real:   "/" + kind.Medication.Segment() + "/" + testsupport.NameOnlyMedicationID,
			absent: "/" + kind.Medication.Segment() + "/" + missingID,
		},
		{
			// Which kinds this instance serves is not information an anonymous
			// caller is given either: phase 003 registers thirteen more, and a
			// difference here would enumerate them.
			name:   "a kind this instance serves",
			real:   collectionURL(),
			absent: unregisteredKindURL,
		},
		{
			name:   "a record of a kind this instance serves",
			real:   recordURL(testsupport.NameOnlyMedicationID),
			absent: unregisteredKindURL + "/" + testsupport.NameOnlyMedicationID,
		},
		{
			// The account surface, from the other side: a confirmation link
			// that was issued and one that never was.
			name:   "a confirmation token",
			real:   "/verify-email/" + strings.Repeat("a", 40),
			absent: "/verify-email/" + strings.Repeat("b", 40),
		},
	}

	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			t.Parallel()

			present := anonymous.get(probe.real)
			missing := anonymous.get(probe.absent)

			require.Equalf(t, missing.Status, present.Status,
				"%s answers %d and %s answers %d, so an anonymous caller learns which of the two is real",
				probe.real, present.Status, probe.absent, missing.Status)

			assert.Equal(t, withoutOwnCorrelationID(missing), withoutOwnCorrelationID(present),
				"the two answers differ in the body, so an anonymous caller learns which of the two is real")

			assert.Equal(t, headerNamesOf(missing), headerNamesOf(present),
				"the two answers carry different headers, so an anonymous caller learns which of the two is real "+
					"without reading a byte of either body")
		})
	}

	require.Greater(t, len(probes), 4, "the probe set has stopped covering both surfaces")
}

// withoutOwnCorrelationID removes the one member FR-033 permits two otherwise
// identical answers to differ in.
//
// It masks the id THIS response was given, taken from its own header, rather
// than matching a shape. The rendered error views carry it as a Reference
// inside the HTML and the JSON envelope carries it as a member, and a regular
// expression narrow enough for one is wrong for the other.
func withoutOwnCorrelationID(answer response) string {
	id := answer.Header.Get(obs.CorrelationHeader)
	if id == "" {
		return withoutCorrelationID(answer.Body)
	}

	return strings.ReplaceAll(withoutCorrelationID(answer.Body), id, "<correlation id>")
}

// headerNamesOf is the sorted header set with the one member FR-033 permits two
// otherwise identical responses to differ in removed by name.
func headerNamesOf(answer response) []string {
	names := make([]string, 0, len(answer.Header))

	for name := range answer.Header {
		if http.CanonicalHeaderKey(name) == http.CanonicalHeaderKey(obs.CorrelationHeader) {
			continue
		}

		names = append(names, name+": "+strings.Join(answer.Header.Values(name), ", "))
	}

	sort.Strings(names)

	return names
}
