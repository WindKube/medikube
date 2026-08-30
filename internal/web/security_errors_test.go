package web

import (
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
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

// testRoutes is the response menagerie both of the tests below drive: one of
// every shape a handler in this application can answer in.
func testRoutes() testsupport.ServeBinder {
	return binder(func(se *core.ServeEvent) {
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
		// What is asserted here is the middleware order, and a stream is the
		// response most likely to escape it: it sets its own headers, flushes
		// immediately, and stays open.
		se.Router.Route(http.MethodGet, "/x/stream", func(e *core.RequestEvent) error {
			e.Response.Header().Set("Content-Type", "text/event-stream")
			e.Response.Header().Set("Cache-Control", "no-cache")
			e.Response.WriteHeader(http.StatusOK)
			_, err := e.Response.Write([]byte("event: datastar-patch-elements\ndata: elements <p id=\"x\"></p>\n\n"))

			return err
		})
	})
}

// This repository has already shipped two bugs of exactly this shape. The first
// was a 404 that lost four security headers because a middleware short-circuited
// ahead of them, and the missing headers told an anonymous caller which of two
// 404s they had hit. The second is the reason the last four rows exist and the
// reason this no longer runs on tests.ApiScenario: a CORS preflight and a
// ServeMux path-normalising redirect are answered before any router middleware
// is entered, so every OPTIONS response and every `//`, `/./` or `/../` request
// left the process carrying none of this set at all.
//
// ApiScenario cannot catch either. It calls apis.NewRouter, which does not bind
// apis.CORS — only apis.Serve does — and it drives the built mux directly, with
// ServeEvent.Server nil, so nothing installed outside the mux runs. The harness
// is testsupport.NewEdgeHandler, which builds the handler the way apis.Serve
// builds it.
//
// The assertion is the whole set on every kind of response, not a few names on
// the happy path.
func TestEveryKindOfResponseCarriesTheWholeSecurityHeaderSet(t *testing.T) {
	t.Parallel()

	handler := served(t, discardLogger(), testRoutes())

	cases := []struct {
		name    string
		method  string
		target  string
		headers []string
		status  int
	}{
		{name: "a plain success", method: http.MethodGet, target: "/x/ok", status: http.StatusNoContent},
		{name: "a rejected field", method: http.MethodGet, target: "/x/invalid", status: http.StatusUnprocessableEntity},
		{name: "a recovered panic", method: http.MethodGet, target: "/x/panic", status: http.StatusInternalServerError},
		{name: "a router-level refusal", method: http.MethodGet, target: "/x/guarded", status: http.StatusUnauthorized},
		{name: "a path nobody registered", method: http.MethodGet, target: "/x/nothing-here", status: http.StatusNotFound},
		{name: "a route the lockdown closes", method: http.MethodGet, target: "/api/collections/users/records", status: http.StatusNotFound},
		{name: "an event stream", method: http.MethodGet, target: "/x/stream", status: http.StatusOK},
		{
			name:   "a CORS preflight",
			method: http.MethodOptions,
			target: "/x/ok",
			headers: []string{
				"Origin", "https://evil.example",
				"Access-Control-Request-Method", http.MethodGet,
			},
			status: http.StatusNoContent,
		},
		{name: "a plain OPTIONS with no Origin", method: http.MethodOptions, target: "/x/ok", status: http.StatusNoContent},
		{name: "a ServeMux path-clean redirect", method: http.MethodGet, target: "/x//ok", status: http.StatusTemporaryRedirect},
		{name: "a dot-segment redirect", method: http.MethodGet, target: "/x/nested/../ok", status: http.StatusTemporaryRedirect},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			res := call(t, handler, one.method, one.target, one.headers...)
			t.Cleanup(func() { _ = res.Body.Close() })

			require.Equal(t, one.status, res.StatusCode, "the case did not produce the response it names")

			assert.Equal(t, expectedSecurityHeaders(), securityOnly(res.Header),
				"%s is missing part of the set", one.name)

			assert.NotEmpty(t, res.Header.Get(obs.CorrelationHeader),
				"%s carries no correlation id, so the person who hit it cannot quote one back (FR-054)", one.name)
		})
	}
}

// FR-053 and Principle VI: one request, one line. Both directions, because the
// mechanism has two halves that can each fail on its own — the outermost
// wrapper writing a second line for a request obs.RequestLogger already
// recorded, and neither of them writing one for a request the router never saw.
//
// The second half is what the 307 row is. Before the wrapper existed, eight
// requests produced six log lines and the two missing ones were an anonymous
// caller's: an unbounded request class that left no operational record.
func TestOneRequestIsExactlyOneLogLineWhicheverSideOfTheRouterAnswers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		method string
		target string
		status int
		path   string
	}{
		{"a request the router handles", http.MethodGet, "/x/ok", http.StatusNoContent, "/x/ok"},
		{"a request the router never sees", http.MethodGet, "/x//ok", http.StatusTemporaryRedirect, "/x//ok"},
		{"a preflight answered by the CORS middleware", http.MethodOptions, "/x/ok", http.StatusNoContent, "/x/ok"},
		{"a path nobody registered", http.MethodGet, "/x/nothing-here", http.StatusNotFound, "/x/nothing-here"},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			var sink logSink

			handler := served(t, logTo(&sink), testRoutes())

			res := call(t, handler, one.method, one.target)
			t.Cleanup(func() { _ = res.Body.Close() })
			require.Equal(t, one.status, res.StatusCode)

			lines := sink.lines()
			require.Len(t, lines, 1, "one request must be one line, and this one was %d:\n%s",
				len(lines), strings.Join(lines, "\n"))

			assert.Contains(t, lines[0], `"msg":"http_request"`)
			assert.Contains(t, lines[0], `"path":"`+one.path+`"`)
			assert.Contains(t, lines[0], `"status":`+strconv.Itoa(one.status))
			assert.Contains(t, lines[0], `"request_id":"`+res.Header.Get(obs.CorrelationHeader)+`"`,
				"the line and the response name different ids for the same request")
		})
	}
}

// One request, one id (FR-054). The wrapper mints it and the request logger has
// to take it back rather than mint a second: two ids for one request is an
// operator holding the id a person quoted and finding nothing.
func TestOneRequestCarriesOneCorrelationIDWhoeverMintedIt(t *testing.T) {
	t.Parallel()

	handler := served(t, discardLogger(), testRoutes())

	t.Run("an inbound id is honoured end to end", func(t *testing.T) {
		res := call(t, handler, http.MethodGet, "/x/ok", obs.CorrelationHeader, "from-the-proxy")
		t.Cleanup(func() { _ = res.Body.Close() })

		assert.Equal(t, "from-the-proxy", res.Header.Get(obs.CorrelationHeader))
	})

	t.Run("an inbound id is honoured on a response the router never sees", func(t *testing.T) {
		res := call(t, handler, http.MethodGet, "/x//ok", obs.CorrelationHeader, "from-the-proxy")
		t.Cleanup(func() { _ = res.Body.Close() })

		assert.Equal(t, "from-the-proxy", res.Header.Get(obs.CorrelationHeader))
	})

	t.Run("free text in the header is replaced rather than echoed", func(t *testing.T) {
		res := call(t, handler, http.MethodGet, "/x/ok", obs.CorrelationHeader, "a value with spaces and \"quotes\"")
		t.Cleanup(func() { _ = res.Body.Close() })

		got := res.Header.Get(obs.CorrelationHeader)
		assert.NotContains(t, got, " ")
		assert.Len(t, got, 32, "the replacement is a fresh 16-byte hex id")
	})
}

// The stream is a later phase, but the wrapper is in front of it now. A
// ResponseWriter that swallowed Flush would turn every SSE connection into a
// response the browser receives when the process closes it, and every test
// shorter than that passes.
func TestTheWrapperDoesNotBreakFlushing(t *testing.T) {
	t.Parallel()

	flushed := make(chan struct{}, 1)

	handler := served(t, discardLogger(), binder(func(se *core.ServeEvent) {
		se.Router.Route(http.MethodGet, "/x/flush", func(e *core.RequestEvent) error {
			e.Response.Header().Set("Content-Type", "text/event-stream")

			// Flushed BEFORE anything is written, which is what an SSE handler
			// does to get the header block out and the connection open. This is
			// PocketBase's own path and it reaches the wrapper through
			// http.ResponseController (tools/router/router.go:264): a wrapper
			// that does not treat it as the commit lets the headers go to the
			// client without the policy, and nothing afterwards can put it back.
			if err := e.Flush(); err != nil {
				return err
			}

			flusher, ok := e.Response.(http.Flusher)
			require.True(t, ok, "the response writer is no longer an http.Flusher")
			flusher.Flush()

			if _, err := e.Response.Write([]byte(": open\n\n")); err != nil {
				return err
			}

			close(flushed)

			return nil
		})
	}))

	res := call(t, handler, http.MethodGet, "/x/flush")
	t.Cleanup(func() { _ = res.Body.Close() })

	require.Equal(t, http.StatusOK, res.StatusCode)

	select {
	case <-flushed:
	default:
		require.Fail(t, "the handler never reached its flush")
	}

	assert.Equal(t, expectedSecurityHeaders(), securityOnly(res.Header),
		"a response whose handler only ever flushed lost part of the set")
}

// The one exemption has to survive the wrapper, and it is the reason the policy
// is filled in at commit rather than set before delegating. PocketBase's admin
// UI writes its own policy only when the header is empty (apis/serve.go:83-99),
// so a policy set eagerly out here would not be overridden by it — it would
// replace it, and constitution VII's break-glass interface would come up blank.
func TestTheAdminUIStillKeepsItsOwnPolicyThroughTheWrapper(t *testing.T) {
	t.Parallel()

	handler := served(t, discardLogger(), testRoutes())

	res := call(t, handler, http.MethodGet, "/_/")
	t.Cleanup(func() { _ = res.Body.Close() })

	require.Equal(t, http.StatusOK, res.StatusCode)

	policy := res.Header.Get("Content-Security-Policy")
	require.NotEmpty(t, policy, "the admin UI is served with no policy at all")
	assert.NotEqual(t, ContentSecurityPolicy, policy,
		"MediKube's policy reached the admin UI, whose own CSS, images and XHR it forbids")
	assert.Equal(t, pocketBaseAdminPolicy, policy,
		"the policy the admin UI wrote for itself did not survive the wrapper")

	for name, value := range expectedSecurityHeaders() {
		if name == "Content-Security-Policy" {
			continue
		}

		assert.Equalf(t, value, res.Header.Get(name), "%s was dropped along with the policy", name)
	}
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
