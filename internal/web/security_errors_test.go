package web

import (
	"io"
	"net/http"
	"regexp"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/obs"
	"medikube/internal/platform/pb"
	"medikube/internal/testsupport"
)

// securityOnly keeps the headers the middleware is responsible for and drops
// the ones that legitimately differ per response — the content type, the
// length, the correlation id, the entity tag.
func securityOnly(header http.Header) map[string]string {
	whole := headerSet(header)
	kept := make(map[string]string, len(whole))

	for name := range expectedSecurityHeaders() {
		if value, present := whole[name]; present {
			kept[name] = value
		}
	}

	return kept
}

var requestIDPattern = regexp.MustCompile(`"request_id":"[^"]*"`)

// This repository has already shipped one bug of exactly this shape: a 404 that
// lost four security headers because a middleware short-circuited ahead of
// them, and the missing headers told an anonymous caller which of two 404s they
// had hit. The assertion is the whole set on every kind of response, not a few
// names on the happy path.
func TestEveryKindOfResponseCarriesTheWholeSecurityHeaderSet(t *testing.T) {
	t.Parallel()

	factory := testsupport.NewAppFactory(
		middleware(obs.RequestLogger(discardLogger()), Errors(nil), pb.Lockdown()),
		SecurityBinder{},
		binder(func(se *core.ServeEvent) {
			se.Router.Route(http.MethodGet, "/x/ok", func(e *core.RequestEvent) error {
				return e.NoContent(http.StatusNoContent)
			})
			se.Router.Route(http.MethodGet, "/x/panic", func(e *core.RequestEvent) error {
				panic("a nil map write")
			})
			se.Router.Route(http.MethodGet, "/x/invalid", func(e *core.RequestEvent) error {
				var invalid domain.ValidationError
				invalid.Add("name", domain.CodeRequired, "a name is required")

				return invalid.OrNil()
			})
			se.Router.Route(http.MethodGet, "/x/guarded", func(e *core.RequestEvent) error {
				return e.NoContent(http.StatusNoContent)
			}).Bind(apis.RequireAuth())
			// A stand-in for the SSE route, which lives in internal/web/stream.
			// What is asserted here is the middleware order, and a stream is
			// the response most likely to escape it: it sets its own headers,
			// flushes immediately, and stays open.
			se.Router.Route(http.MethodGet, "/x/stream", func(e *core.RequestEvent) error {
				e.Response.Header().Set("Content-Type", "text/event-stream")
				e.Response.Header().Set("Cache-Control", "no-cache")
				e.Response.WriteHeader(http.StatusOK)
				_, err := e.Response.Write([]byte("event: datastar-patch-elements\ndata: elements <p id=\"x\"></p>\n\n"))

				return err
			})
		}),
	)

	cases := []struct {
		name   string
		url    string
		status int
	}{
		{"a plain success", "/x/ok", http.StatusNoContent},
		{"a rejected field", "/x/invalid", http.StatusUnprocessableEntity},
		{"a recovered panic", "/x/panic", http.StatusInternalServerError},
		{"a router-level refusal", "/x/guarded", http.StatusUnauthorized},
		{"a path nobody registered", "/x/nothing-here", http.StatusNotFound},
		{"a route the lockdown closes", "/api/collections/users/records", http.StatusNotFound},
		{"an event stream", "/x/stream", http.StatusOK},
	}

	seen := map[string]map[string]string{}

	for _, one := range cases {
		var got map[string]string

		scenario := tests.ApiScenario{
			Name:           one.name,
			Method:         http.MethodGet,
			URL:            one.url,
			ExpectedStatus: one.status,
			TestAppFactory: factory,
			AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
				got = securityOnly(res.Header)
			},
		}

		if one.status != http.StatusNoContent {
			scenario.ExpectedContent = []string{""}
		}

		scenario.Test(t)

		assert.Equalf(t, expectedSecurityHeaders(), got, "%s is missing part of the set", one.name)

		seen[one.name] = got
	}

	require.Len(t, seen, len(cases), "a case did not run, so its response was never inspected")
}

// The oracle FR-033 closes one level up the path: a 404 for a route the
// lockdown closes and a 404 for a path that does not exist must be the same
// response, headers included. One header that differs tells an anonymous caller
// which of the two they hit.
func TestALockedRouteAndAnUnknownPathAreIndistinguishable(t *testing.T) {
	t.Parallel()

	factory := testsupport.NewAppFactory(
		middleware(obs.RequestLogger(discardLogger()), Errors(nil), pb.Lockdown()),
		SecurityBinder{},
	)

	answers := map[string]struct {
		headers map[string]string
		body    string
	}{}

	for name, url := range map[string]string{
		"a route the lockdown closes": "/api/collections/users/records",
		"a path nobody registered":    "/no-such-path-at-all",
	} {
		var headers map[string]string
		var body string

		scenario := tests.ApiScenario{
			Name:            name,
			Method:          http.MethodGet,
			URL:             url,
			ExpectedStatus:  http.StatusNotFound,
			ExpectedContent: []string{`"code":"not_found"`},
			TestAppFactory:  factory,
			AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
				headers = headerSet(res.Header)
				delete(headers, obs.CorrelationHeader)
				delete(headers, "Date")

				raw, err := io.ReadAll(res.Body)
				require.NoError(t, err)
				body = requestIDPattern.ReplaceAllString(string(raw), `"request_id":"…"`)
			},
		}
		scenario.Test(t)

		answers[name] = struct {
			headers map[string]string
			body    string
		}{headers, body}
	}

	locked := answers["a route the lockdown closes"]
	unknown := answers["a path nobody registered"]

	assert.Equal(t, unknown.headers, locked.headers,
		"a header differs between the two 404s, which tells an anonymous caller which one they hit")
	assert.Equal(t, unknown.body, locked.body,
		"the two 404s differ by more than the correlation id")
}

// The rate limiter, the body limit and the lockdown all answer before any
// MediKube handler runs. The security middleware has to be outside all of them,
// which is what the priority says and what this asserts through a real chain.
func TestTheSecurityMiddlewareRunsOutsideEveryShortCircuit(t *testing.T) {
	t.Parallel()

	assert.Less(t, SecurityHeadersMiddlewarePriority, pb.LockdownPriority,
		"the lockdown short-circuits ahead of the security headers, so a locked route answers without them")
	assert.Less(t, SecurityHeadersMiddlewarePriority, apis.DefaultRateLimitMiddlewarePriority,
		"a rate-limited response would carry no security headers")
}
