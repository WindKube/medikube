package api_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/platform/pb"
	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/apitest"
)

// T238, FR-035: no general-purpose browsing or bulk-extraction facility is
// reachable by an ordinary signed-in person.
//
// This is NOT an aggregation of the tests that already exist, and it was
// written that way on purpose. The brief offered a pointer test tying
// internal/platform/pb/lockdown_test.go:45, :87 and internal/store/filter_test.go
// together; a test whose body is a reference to three other tests asserts
// nothing, cannot fail for the reason it names, and would keep passing if all
// three were deleted. So this file asserts the parts of FR-035 that those three
// do not reach, and says below exactly which part each of them already owns:
//
//   - lockdown_test.go proves the record subtree and /api/batch answer 404 in
//     internal/platform/pb's own harness, comparing against PocketBase's
//     genuine 404. What it cannot see is the ASSEMBLED application: at the real
//     edge that 404 passes through web.Errors and becomes MediKube's envelope,
//     which is different bytes reaching a different comparison. Part one below.
//   - filter_test.go proves no filter-DSL string is written outside
//     internal/store. That is a source fact about what MediKube spells. It says
//     nothing about what an ATTACKER can spell: a `?filter=` parameter honoured
//     by a handler, or a `?q=` value concatenated into an expression, would both
//     pass it. Parts two and three below.
//   - nothing at all covers the surfaces PocketBase serves that the lockdown
//     does not close — /api/logs, /api/settings, /api/backups, /api/crons and
//     the collection administration API. Every one of them is a general-purpose
//     browsing or bulk-extraction facility and only the collection rules stand
//     between them and an ordinary account. Part one below.

// pocketbaseBrowsingSurface is one of PocketBase's own facilities that
// MediKube neither serves nor closes: it is left to PocketBase's superuser
// rules. The reason is what an ordinary account would get if the rule ever
// loosened, which is what makes each row worth a request.
type pocketbaseBrowsingSurface struct {
	method string

	// pattern is how the router registered it, asserted with HasRoute so a row
	// naming a route PocketBase has stopped serving fails instead of passing
	// against the catch-all.
	pattern string

	// url is the concrete address to send.
	url string

	// superuserReads marks a surface a superuser may safely be pointed at from
	// a test — a read with no side effect. It is the anti-vacuity control and
	// not a nicety: "an ordinary account is refused" is equally satisfied by a
	// surface that refuses everybody, one that is broken, and one that no
	// longer does anything, and only a caller who IS answered can tell those
	// apart from a facility that is genuinely there and genuinely closed.
	//
	// The writes are deliberately without one. POST /api/backups would put a
	// copy of the whole database on disk and POST /api/settings/test/s3 would
	// open an outbound connection with the instance's stored credentials —
	// both are exactly the things this file exists to keep shut, and a test
	// that performed them to prove they work would be absurd.
	superuserReads bool

	reason string
}

func pocketbaseBrowsingSurfaces() []pocketbaseBrowsingSurface {
	return []pocketbaseBrowsingSurface{
		{
			method: http.MethodGet, pattern: "/api/collections", url: "/api/collections", superuserReads: true,
			reason: "the collection administration API: every collection, every field, every rule. It is the map an " +
				"attacker would read before deciding what to ask for",
		},
		{
			method: http.MethodGet, pattern: "/api/collections/meta/scaffolds", url: "/api/collections/meta/scaffolds", superuserReads: true,
			reason: "the schema scaffolds behind the admin UI's collection editor",
		},
		{
			method: http.MethodGet, pattern: "/api/logs", url: "/api/logs", superuserReads: true,
			reason: "PocketBase's own request log store. internal/platform/pb unbinds the activity logger so nothing " +
				"writes to it, but this is the route that would read a request URI back out if anything did (FR-038)",
		},
		{
			method: http.MethodGet, pattern: "/api/logs/stats", url: "/api/logs/stats", superuserReads: true,
			reason: "aggregates over the same store",
		},
		{
			method: http.MethodGet, pattern: "/api/settings", url: "/api/settings", superuserReads: true,
			reason: "instance settings, including the mail credentials and the S3 keys",
		},
		{
			method: http.MethodGet, pattern: "/api/backups", url: "/api/backups", superuserReads: true,
			reason: "the backup listing. The download route beside it serves an entire SQLite database as one file, " +
				"which is the largest bulk extraction this application could possibly offer",
		},
		{
			method: http.MethodPost, pattern: "/api/backups", url: "/api/backups",
			reason: "creating a backup on demand is the first half of that same extraction",
		},
		{
			method: http.MethodGet, pattern: "/api/crons", url: "/api/crons", superuserReads: true,
			reason: "the scheduled-job listing, which names the retention purge (T241)",
		},
		{
			method: http.MethodPost, pattern: "/api/crons/{id}", url: "/api/crons/__pbDBOptimize__",
			reason: "running a scheduled job on demand. It is NOT in pb.LockedRoutes, so a superuser can trigger the " +
				"audit retention purge whenever they like — which is a decision nobody has written down and is worth " +
				"one, but it must at the very least be closed to an ordinary account",
		},
		{
			method: http.MethodPost, pattern: "/api/settings/test/s3", url: "/api/settings/test/s3",
			reason: "makes the instance open an outbound connection with its stored credentials",
		},
	}
}

// TestNoPocketBaseBrowsingSurfaceAnswersAnOrdinaryAccount is part one.
func TestNoPocketBaseBrowsingSurfaceAnswersAnOrdinaryAccount(t *testing.T) {
	t.Parallel()

	served := newServedInstance(t, testsupport.AccountAEmail)

	ordinary := served.caller
	anonymous := ordinary.anonymous()

	superuser := &caller{
		t:       t,
		app:     ordinary.app,
		handler: ordinary.handler,
		token:   testsupport.AuthToken(t, ordinary.app, core.CollectionNameSuperusers, testsupport.SuperuserEmail),
	}

	surfaces := pocketbaseBrowsingSurfaces()

	var controlled int

	for _, surface := range surfaces {
		if surface.superuserReads {
			controlled++
		}

		t.Run(surface.method+" "+surface.pattern, func(t *testing.T) {
			// The row has to name a route that exists, or its refusal is the
			// router's catch-all and proves nothing about the surface.
			require.Truef(t, served.router.HasRoute(surface.method, surface.pattern),
				"%s %s is written down here as a surface to keep closed (%s) and PocketBase no longer registers it: "+
					"the answer below would be the catch-all", surface.method, surface.pattern, surface.reason)

			for _, who := range []struct {
				name string
				as   *caller
			}{
				{name: "an ordinary signed-in account", as: ordinary},
				{name: "nobody", as: anonymous},
			} {
				answer := who.as.do(surface.method, surface.url, bodyFor(surface.method), nil)

				assert.GreaterOrEqualf(t, answer.Status, http.StatusBadRequest,
					"%s answers %s: %s (%s)", surface.pattern, who.name, answer.Body, surface.reason)

				assertDisclosesNothing(t, surface.pattern+" to "+who.name, answer.Body)
			}

			if !surface.superuserReads {
				return
			}

			answer := superuser.do(surface.method, surface.url, bodyFor(surface.method), nil)

			require.Lessf(t, answer.Status, http.StatusBadRequest,
				"%s refuses a superuser too, so it is broken or gone rather than closed, and the two refusals above "+
					"prove nothing about it: %s", surface.pattern, answer.Body)
		})
	}

	require.Greater(t, controlled, 5,
		"almost no row carries a superuser control, so this table cannot tell a closed facility from an absent one")
	require.Greater(t, len(surfaces), 8,
		"the surface list has shrunk; PocketBase serves more general-purpose facilities than this is asking about")
}

// TestEveryLockedRouteIsAGenuineMissAtTheAssembledEdge is part one's other half.
//
// internal/platform/pb proves the lockdown answers PocketBase's own 404 in
// PocketBase's own harness. This is the same set of routes read off
// pb.LockedRoutes — never a list retyped here, so a route added to the lockdown
// is covered the day it is added — driven through the whole application, where
// that 404 has been through web.Errors and is MediKube's envelope. The bytes
// being compared are different bytes, and so is the comparison.
func TestEveryLockedRouteIsAGenuineMissAtTheAssembledEdge(t *testing.T) {
	t.Parallel()

	ordinary := newCaller(t)

	// A path that matches no route at all, answered by the router's catch-all.
	// Every locked route must be indistinguishable from it.
	genuine := ordinary.get("/api/this-route-has-never-existed")
	require.Equal(t, http.StatusNotFound, genuine.Status, genuine.Body)

	locked := pb.LockedRoutes()

	values := map[string]string{
		"{collection}": kind.Medication.Collection(),
		"{id}":         testsupport.NameOnlyMedicationID,
		"{recordId}":   testsupport.NameOnlyMedicationID,
		"{filename}":   "attachment.pdf",
	}

	for _, route := range locked {
		t.Run(route.Pattern(), func(t *testing.T) {
			url := bind(t, route.Path, values)

			answer := ordinary.do(route.Method, url, bodyFor(route.Method), nil)

			require.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
			assert.Equal(t, withoutOwnCorrelationID(genuine), withoutOwnCorrelationID(answer),
				"a locked route is distinguishable from a path that never existed, so an ordinary account can tell "+
					"which collections this instance keeps")
			assert.Equal(t, headerNamesOf(genuine), headerNamesOf(answer),
				"a locked route carries a different header set than a genuine miss")

			assertDisclosesNothing(t, route.Pattern(), answer.Body)
		})
	}

	require.Greater(t, len(locked), 8, "pb.LockedRoutes has shrunk, so the lockdown covers less than it did")
}

// queryLanguageParameters are PocketBase's own list parameters, none of which
// MediKube publishes. `filter` is the one that matters — it takes an arbitrary
// expression over any column of any collection, including `owner` — and the
// rest are here because a handler that read one of them would be a handler
// reading PocketBase's query vocabulary off the wire.
var queryLanguageParameters = map[string]string{
	"filter":      `owner != ""`,
	"expand":      "owner",
	"fields":      "*",
	"perPage":     "500",
	"page":        "2",
	"skipTotal":   "1",
	"sort":        "-owner",
	"$autoCancel": "false",
}

// TestNoPublishedRouteHonoursPocketBasesQueryLanguage is part two, driven by
// the route table rather than by a list of URLs.
//
// The claim is narrow and checkable: sending one of PocketBase's parameters
// either changes nothing at all or is refused. A parameter that is honoured
// would change the answer, and a parameter that is honoured is a general-purpose
// query facility whatever it is called.
func TestNoPublishedRouteHonoursPocketBasesQueryLanguage(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)

	// The stranger's rows exist on the same instance, which is the whole point:
	// a broadened query has something to reach.
	stranger := owner.as(testsupport.AccountBEmail)

	strangerIDs := idsOf(stranger.get(collectionURL() + "?limit=100").list(t))
	require.NotEmpty(t, strangerIDs, "the other account owns nothing, so a broadened answer would have nothing in it")

	var routes, probes int

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind != httproute.KindAPI || route.Auth != httproute.AuthUser || route.Method != http.MethodGet {
			continue
		}

		url := bind(t, route.Path, anonymousParameters(testsupport.NameOnlyMedicationID))

		plain := owner.get(url)
		require.Equalf(t, http.StatusOK, plain.Status, "%s: %s", route.OpID, plain.Body)

		routes++

		names := make([]string, 0, len(queryLanguageParameters))
		for name := range queryLanguageParameters {
			names = append(names, name)
		}

		sort.Strings(names)

		for _, name := range names {
			probes++

			t.Run(route.OpID+"?"+name, func(t *testing.T) {
				answer := owner.get(url + "?" + name + "=" + urlValue(queryLanguageParameters[name]))

				if answer.Status == http.StatusUnprocessableEntity {
					// Refused by name, which is the other acceptable outcome:
					// contracts/README.md refuses an unpublished `sort` rather
					// than ignoring it, because a silently dropped narrowing
					// produces a list that looks right and is not.
					assertDisclosesNothing(t, route.OpID+"?"+name, answer.Body)

					return
				}

				assert.Equalf(t, http.StatusOK, answer.Status, "%s?%s: %s", route.OpID, name, answer.Body)
				assert.Equalf(t, withoutOwnCorrelationID(plain), withoutOwnCorrelationID(answer),
					"%s answered %s differently, so it reads PocketBase's %s and MediKube publishes no such parameter",
					route.OpID, name, name)

				for _, id := range strangerIDs {
					assert.NotContainsf(t, answer.Body, id,
						"%s?%s reached another account's rows", route.OpID, name)
				}
			})
		}
	}

	// The guard on the guard. Both counters are literal: a walk that stopped
	// finding routes, or a parameter map that emptied itself, would otherwise
	// pass this test having sent nothing at all.
	require.Greater(t, routes, 2, "the route table has stopped declaring signed-in reads, so this walked nothing")
	require.Greater(t, probes, 20, "almost no parameter was tried")
}

// searchProbe is one `?q=` value and what it is allowed to match.
type searchProbe struct {
	name string

	// term is sent raw and is escaped by the caller, so what the handler
	// receives is exactly what is written here.
	term string

	// matches is whether this term is expected to find anything. Exactly one
	// probe sets it, and it is the anti-vacuity control: without it "the
	// injection found nothing" would also be satisfied by a search that finds
	// nothing at all, which is the same green for the opposite reason.
	matches bool
}

// TestTheSearchTermIsAValueAndNeverAnExpression is part three.
//
// `?q=` is the one parameter on the record surface whose value is free text.
// internal/store's Condition set has no operator that could hold an expression
// and internal/store/filter_test.go proves no DSL string is written outside
// that package — but neither of them is driven by what a caller can send, and
// the value still has to reach SQL as a bound parameter with its LIKE wildcards
// escaped. A `%` that arrived unescaped would match every row this account
// owns; one that arrived as a filter fragment would match every row anybody
// owns.
func TestTheSearchTermIsAValueAndNeverAnExpression(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	stranger := owner.as(testsupport.AccountBEmail)

	strangerIDs := idsOf(stranger.get(collectionURL() + "?limit=100").list(t))
	require.NotEmpty(t, strangerIDs)

	everything := owner.get(collectionURL() + "?limit=100").list(t)
	require.Len(t, everything.Items, testsupport.AccountAMedicationCount)

	probes := []searchProbe{
		{name: "a real substring", term: "Paracetamol", matches: true},
		{name: "the LIKE wildcard on its own", term: "%"},
		{name: "the LIKE single-character wildcard", term: "_"},
		{name: "a filter expression", term: `" || owner != "`},
		{name: "a filter expression in PocketBase's own spelling", term: `name != '' || owner != ''`},
		{name: "a closed quote and a tautology", term: `') OR (1=1`},
		{name: "an escape of the escape", term: `\%`},
	}

	var matching, empty int

	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			answer := owner.get(collectionURL() + "?limit=100&" + web.ParamSearch + "=" + urlValue(probe.term))

			require.Equal(t, http.StatusOK, answer.Status, answer.Body)

			found := answer.list(t)

			if probe.matches {
				assert.NotEmpty(t, found.Items,
					"the control term matches nothing, so every other row here is green because search is broken")
			} else {
				assert.Emptyf(t, found.Items,
					"%q matched %d of this account's rows, so it was not treated as a literal substring",
					probe.term, len(found.Items))
			}

			assert.LessOrEqual(t, len(found.Items), len(everything.Items),
				"a search term returned more than the unfiltered list, so it widened the query rather than narrowing it")

			for _, id := range strangerIDs {
				assert.NotContainsf(t, answer.Body, id, "%q reached another account's rows", probe.term)
			}
		})

		if probe.matches {
			matching++
		} else {
			empty++
		}
	}

	require.Positive(t, matching, "no control term is expected to match, so this cannot tell a safe search from a broken one")
	require.Greater(t, empty, 4, "almost nothing hostile is being sent")
}

// TestNoSingleRequestExtractsMoreThanAPage is part four: the ceiling.
//
// A page bound is the difference between a leak being one request and being one
// request per hundred rows. It is asserted against an account holding several
// times MaxLimit, because a ceiling tested against twelve rows is a ceiling that
// was never reached.
func TestNoSingleRequestExtractsMoreThanAPage(t *testing.T) {
	t.Parallel()

	const bulk = 300

	instance := apitest.NewPopulated(t, testsupport.AccountAID, bulk)

	owner := &caller{
		t:       t,
		app:     instance.App,
		handler: testsupport.NewEdgeHandler(t, instance.App),
		token:   testsupport.UserToken(t, instance.App, testsupport.AccountAEmail),
	}

	require.Greater(t, bulk, 2*web.MaxLimit, "the account does not hold enough rows for a ceiling to be reached")

	t.Run("the default page is the published default", func(t *testing.T) {
		answer := owner.get(collectionURL())

		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Len(t, answer.list(t).Items, web.DefaultLimit)
	})

	t.Run("the ceiling is the published ceiling", func(t *testing.T) {
		answer := owner.get(collectionURL() + "?limit=" + itoa(web.MaxLimit))

		require.Equal(t, http.StatusOK, answer.Status, answer.Body)
		assert.Len(t, answer.list(t).Items, web.MaxLimit)
	})

	t.Run("one above it is refused rather than quietly reduced", func(t *testing.T) {
		answer := owner.get(collectionURL() + "?limit=" + itoa(web.MaxLimit+1))

		assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	})

	// The ways a caller would try to get round it, on both list routes.
	for _, url := range []string{collectionURL(), crossKindURL()} {
		for _, attempt := range []string{
			"?limit=100000",
			"?limit=-1",
			"?limit=0",
			"?perPage=1000",
			"?limit=" + itoa(web.MaxLimit) + "&perPage=1000",
			"?limit=" + itoa(web.MaxLimit) + "&page=1&skipTotal=1",
			"?limit=" + itoa(web.MaxLimit) + "," + itoa(bulk),
		} {
			t.Run(url+attempt, func(t *testing.T) {
				answer := owner.get(url + attempt)

				if answer.Status != http.StatusOK {
					assert.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)

					return
				}

				assert.LessOrEqualf(t, len(answer.list(t).Items), web.MaxLimit,
					"%s%s returned more than one page, so the ceiling is not a ceiling", url, attempt)
			})
		}
	}
}

// assertDisclosesNothing is the content half of every refusal above. A status
// code says the request was refused; this says the refusal was empty of the
// things the refusal exists to protect.
func assertDisclosesNothing(t *testing.T, where, body string) {
	t.Helper()

	for _, secret := range []string{
		testsupport.NameOnlyMedicationID,
		"Paracetamol",
		testsupport.AccountAEmail,
		testsupport.AccountAID,
	} {
		assert.NotContainsf(t, body, secret, "%s discloses %q", where, secret)
	}
}

// bodyFor is a valid empty JSON object for the verbs that take one. A malformed
// body would be refused by the decoder, which satisfies "the caller did not
// succeed" while asserting nothing about authorization.
func bodyFor(method string) string {
	if method == http.MethodGet || method == http.MethodDelete {
		return ""
	}

	return `{}`
}

// urlValue escapes one query parameter value.
func urlValue(raw string) string {
	return strings.NewReplacer(
		"%", "%25", " ", "%20", `"`, "%22", "'", "%27", "|", "%7C",
		"!", "%21", "=", "%3D", "(", "%28", ")", "%29", "*", "%2A",
		"\\", "%5C", "$", "%24", "&", "%26", "+", "%2B", "#", "%23",
	).Replace(raw)
}
