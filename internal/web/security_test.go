package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/obs"
	"medikube/internal/testsupport"
)

// The whole set, written out. This is the assertion: a header added to the
// middleware and not to this map is a change nobody reviewed, and a header
// dropped from the middleware is a protection nobody noticed leaving.
func expectedSecurityHeaders() map[string]string {
	return map[string]string{
		"Content-Security-Policy":    ContentSecurityPolicy,
		"Strict-Transport-Security":  StrictTransportSecurity,
		"Referrer-Policy":            ReferrerPolicy,
		"X-Content-Type-Options":     "nosniff",
		"X-Frame-Options":            "DENY",
		"Cross-Origin-Opener-Policy": "same-origin",
	}
}

// runSecurity drives the middleware over a bare event and returns exactly what
// it wrote, so the set can be compared rather than sampled.
func runSecurity(t *testing.T, pattern string) map[string]string {
	t.Helper()

	e, recorder := event(t, http.MethodGet, "/x")
	e.Request.Pattern = pattern

	require.NoError(t, SecurityHeaders().Func(e))

	return headerSet(recorder.Header())
}

func TestTheMiddlewareWritesExactlyTheDesignedHeaderSet(t *testing.T) {
	t.Parallel()

	assert.Equal(t, expectedSecurityHeaders(), runSecurity(t, ""),
		"the security header set changed; every response in the application carries this")
}

// FR-042 and contracts/pages.md. 'unsafe-eval' is the only relaxed directive,
// accepted deliberately and permanently for Datastar's expression evaluator —
// which is literally the Function constructor, so without it every data-*
// expression on every page throws and the application is not degraded but dead.
func TestTheContentSecurityPolicyIsExactlyAsDesigned(t *testing.T) {
	t.Parallel()

	directives := map[string]string{}

	for _, directive := range strings.Split(ContentSecurityPolicy, ";") {
		parts := strings.Fields(strings.TrimSpace(directive))
		require.NotEmpty(t, parts, "an empty directive in the policy")
		directives[parts[0]] = strings.Join(parts[1:], " ")
	}

	assert.Equal(t, map[string]string{
		"default-src":     "'none'",
		"script-src":      "'self' 'unsafe-eval'",
		"style-src":       "'self'",
		"img-src":         "'self' data:",
		"connect-src":     "'self'",
		"object-src":      "'none'",
		"frame-ancestors": "'none'",
		"base-uri":        "'self'",
		"form-action":     "'self'",
	}, directives)
}

// The two rules the 'unsafe-eval' trade-off rests on (research D-35). Both are
// properties of this string, so both are asserted here rather than left to
// review.
func TestUnsafeEvalIsTheOnlyRelaxationAndNoOriginOutsideTheInstanceIsAllowed(t *testing.T) {
	t.Parallel()

	assert.NotContains(t, ContentSecurityPolicy, "'unsafe-inline'",
		"an injected <script> tag now runs, which is exactly what banning Datastar's inline-script SDK family bought")
	assert.NotContains(t, ContentSecurityPolicy, "unsafe-hashes")
	assert.Equal(t, 1, strings.Count(ContentSecurityPolicy, "unsafe-"),
		"a second relaxation was added; 'unsafe-eval' is the only one FR-042 accepts")

	for _, directive := range strings.Split(ContentSecurityPolicy, ";") {
		for _, source := range strings.Fields(strings.TrimSpace(directive))[1:] {
			assert.Containsf(t, []string{"'none'", "'self'", "'unsafe-eval'", "data:"}, source,
				"%q names an origin outside the instance, and FR-042 forbids loading or embedding anything from one", source)
		}
	}

	// data: is on images alone. On script-src it would be a script source; on
	// connect-src it would be an exfiltration channel.
	assert.Equal(t, 1, strings.Count(ContentSecurityPolicy, "data:"))
	assert.Contains(t, ContentSecurityPolicy, "img-src 'self' data:")
}

// The application is served by the same origin that serves its stylesheet, its
// vendored Datastar runtime and its SSE stream. default-src 'none' with no
// style-src, no img-src and no connect-src would block all three — the
// stylesheet would not load, every fetch would be refused, and contracts/
// pages.md assertion 6 (zero CSP violations) would fail on every route.
//
// tasks.md T121 and contracts/pages.md list six directives and omit those
// three; research D-35 and plan.md:449 name all nine but weaken default-src to
// 'self'. This policy is the strict half of each: default-src 'none' from the
// first, and the three explicit allowances from the second.
func TestThePolicyStillAllowsTheApplicationToLoadItself(t *testing.T) {
	t.Parallel()

	for _, needed := range []string{"style-src 'self'", "connect-src 'self'", "img-src 'self'"} {
		assert.Containsf(t, ContentSecurityPolicy, needed,
			"%s is missing under default-src 'none', so the application cannot load its own assets", needed)
	}
}

// PocketBase's own securityHeaders sets X-Frame-Options: SAMEORIGIN with Set
// rather than set-if-empty (apis/middlewares.go:288-304), at the same priority.
// A MediKube middleware bound at -1011 with its own id therefore loses, and
// every page becomes framable by the instance itself.
func TestPocketBasesOwnSecurityHeadersAreReplacedRatherThanFoughtWith(t *testing.T) {
	t.Parallel()

	var got map[string]string

	scenario := tests.ApiScenario{
		Name:           "a plain 200 through the whole chain",
		Method:         http.MethodGet,
		URL:            "/x/ok",
		ExpectedStatus: http.StatusNoContent,
		TestAppFactory: testsupport.NewAppFactory(
			middleware(obs.RequestLogger(discardLogger()), Errors(nil)),
			SecurityBinder{},
			route(http.MethodGet, "/x/ok", func(e *core.RequestEvent) error {
				return e.NoContent(http.StatusNoContent)
			}),
		),
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			got = headerSet(res.Header)
		},
	}
	scenario.Test(t)

	for name, value := range expectedSecurityHeaders() {
		assert.Equalf(t, value, got[name], "%s did not survive the chain", name)
	}

	assert.NotContains(t, got, "X-Xss-Protection",
		"PocketBase's X-XSS-Protection survived; it is deprecated and the auditor it enabled is itself an XS-leak")
}

func TestTheMiddlewareOwnsTheSlotPocketBasesOwnHeadersUsed(t *testing.T) {
	t.Parallel()

	assert.Equal(t, apis.DefaultSecurityHeadersMiddlewarePriority, SecurityHeadersMiddlewarePriority,
		"MediKube's headers moved off PocketBase's slot")
	assert.NotEqual(t, apis.DefaultSecurityHeadersMiddlewareId, SecurityHeadersMiddlewareID,
		"reusing PocketBase's id re-includes it in an excluded group, and a rename upstream silently un-replaces it")
}

// The headers are written BEFORE the chain continues, and both halves of why
// are asserted here.
//
// A middleware bound later can short-circuit — the lockdown at -1009 returns
// 404 without calling e.Next() — and a handler can write the response, after
// which a header set is a header the client never sees, because net/http
// snapshots them at WriteHeader. Writing first is what makes the full set
// survive both.
//
// The chain is built by hand rather than driven through an app, because the
// property is about ordering alone.
func TestTheHeadersSurviveEverythingBoundAfterThem(t *testing.T) {
	t.Parallel()

	t.Run("a middleware that short-circuits", func(t *testing.T) {
		t.Parallel()

		e, recorder := event(t, http.MethodGet, "/x")

		chain := new(hook.Hook[*core.RequestEvent])
		chain.Bind(SecurityHeaders())
		chain.Bind(&hook.Handler[*core.RequestEvent]{
			Id: "aShortCircuitingMiddleware",
			// One step later, which is exactly where the lockdown sits.
			Priority: SecurityHeadersMiddlewarePriority + 1,
			Func: func(*core.RequestEvent) error {
				return router.NewNotFoundError("", nil)
			},
		})

		err := chain.Trigger(e, func(*core.RequestEvent) error {
			require.Fail(t, "the short-circuiting middleware called the next handler, so the case proves nothing")

			return nil
		})
		require.Error(t, err)

		assert.Equal(t, expectedSecurityHeaders(), headerSet(recorder.Header()),
			"a middleware that short-circuits after this one answers with no security headers at all")
	})

	t.Run("a handler that writes the response", func(t *testing.T) {
		t.Parallel()

		e, recorder := event(t, http.MethodGet, "/x")

		chain := new(hook.Hook[*core.RequestEvent])
		chain.Bind(SecurityHeaders())

		require.NoError(t, chain.Trigger(e, func(e *core.RequestEvent) error {
			return e.NoContent(http.StatusNoContent)
		}))

		// Result(), not Header(): net/http clones the header map at
		// WriteHeader, so this is what the client actually received rather than
		// what the map happens to hold afterwards.
		got := headerSet(recorder.Result().Header)
		for name, value := range expectedSecurityHeaders() {
			assert.Equalf(t, value, got[name], "%s was set after the response had gone", name)
		}
	})
}

// Constitution VII keeps the PocketBase superuser admin UI in production. It
// ships its own hardened CSP, applied only when the header is empty
// (apis/serve.go:83-99), and MediKube's policy forbids the inline styles and
// the map tiles that UI loads. A root middleware would win the race and leave
// the break-glass interface a blank page.
func TestTheAdminUIKeepsItsOwnContentSecurityPolicy(t *testing.T) {
	t.Parallel()

	written := runSecurity(t, AdminUIPattern)

	assert.NotContains(t, written, "Content-Security-Policy",
		"MediKube's policy was applied to the admin UI, whose own CSS, images and XHR it forbids")

	for name, value := range expectedSecurityHeaders() {
		if name == "Content-Security-Policy" {
			continue
		}

		assert.Equalf(t, value, written[name], "%s was dropped along with the policy", name)
	}
}

func TestNoOtherRouteIsExemptFromThePolicy(t *testing.T) {
	t.Parallel()

	for _, pattern := range []string{
		"",
		"/",
		"GET /api/v1/records/{kind}",
		"GET /_",
		"GET /_x/{path...}",
		"POST /_/{path...}",
	} {
		t.Run(pattern, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, runSecurity(t, pattern), "Content-Security-Policy",
				"a pattern other than the admin UI's was exempted from the policy")
		})
	}
}

// HSTS without preload is a deliberate choice: preload is an irreversible
// submission to a list browsers ship, and that is an operator's decision and
// not a library's.
func TestStrictTransportSecurityIsAYearAndDoesNotPreload(t *testing.T) {
	t.Parallel()

	assert.Contains(t, StrictTransportSecurity, "max-age=31536000")
	assert.Contains(t, StrictTransportSecurity, "includeSubDomains")
	assert.NotContains(t, StrictTransportSecurity, "preload")
}

// A referrer carries the path, and a path in this application is
// /medications/{id} — an identifier for a person's medication, sent to whatever
// they click through to.
func TestTheReferrerPolicyLeaksNothing(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "no-referrer", ReferrerPolicy)
}
