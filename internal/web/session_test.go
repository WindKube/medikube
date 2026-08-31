package web

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
	"medikube/internal/obs"
	"medikube/internal/platform/pb"
	"medikube/internal/testsupport"
)

// T199. The cookie is the whole of a browser's credential, so every attribute
// of it is a control and a missing one is that control gone: without HttpOnly
// any script reads the token, without SameSite a third-party site rides it,
// without Secure it crosses the wire in the clear, without Path it is absent
// from half the application, and without Max-Age it dies with the tab.
//
// The assertions are made twice on purpose — once against the struct, once
// against the header a browser actually receives — because those are different
// things: net/http renders MaxAge 0 as no Max-Age at all, and a cookie whose
// Go value looks right can still reach the browser missing the attribute.

// usersCollection is PocketBase's own auth collection, spelled here for the one
// assertion that reads the collection's token lifetime back.
const usersCollection = "users"

// anonymous is what the probe route answers when nothing authenticated the
// request. It is a body rather than a status because a request that is merely
// anonymous is not refused by the probe — which is the entire failure mode this
// file is about: nothing errors, nothing logs, everybody is a guest.
const anonymous = "anonymous"

func TestTheSessionCookieCarriesEveryAttributeThatMakesItACredential(t *testing.T) {
	t.Parallel()

	const token = "a-pocketbase-auth-token"

	cookie := SessionCookie{TTL: 168 * time.Hour, Secure: true}.New(token)

	assert.Equal(t, "medikube_session", SessionCookieName,
		"the cookie name is what a browser matches on, so renaming it signs everybody out")
	assert.Equal(t, SessionCookieName, cookie.Name)
	assert.Equal(t, token, cookie.Value)
	assert.True(t, cookie.HttpOnly, "the token is readable by any script on the page")
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite, "the cookie rides a cross-site request")
	assert.True(t, cookie.Secure, "the session token crosses the wire in the clear")
	assert.Equal(t, "/", cookie.Path, "the cookie is absent from the routes it is not scoped to")
	assert.Equal(t, int((168 * time.Hour).Seconds()), cookie.MaxAge)

	// The same five, as the browser receives them. `Secure` and `HttpOnly` are
	// bare attributes, so a contains check is the whole of what is on the wire.
	rendered := cookie.String()
	for _, attribute := range []string{
		"medikube_session=" + token,
		"Path=/",
		"Max-Age=604800",
		"HttpOnly",
		"Secure",
		"SameSite=Lax",
	} {
		assert.Containsf(t, rendered, attribute, "the Set-Cookie header a browser receives is missing %s: %s", attribute, rendered)
	}
}

// The clear is the same cookie emptied, because a browser replaces a cookie by
// name and path. A clear that changed either would leave the original one in
// place, and the person would walk away still carrying it.
func TestClearingTheSessionCookieEmptiesItAndExpiresItImmediately(t *testing.T) {
	t.Parallel()

	cookies := SessionCookie{TTL: time.Hour, Secure: true}
	cleared := cookies.Cleared()
	rendered := cleared.String()

	assert.Empty(t, cleared.Value)
	assert.Equal(t, SessionCookieName, cleared.Name)
	assert.Equal(t, cookies.New("x").Path, cleared.Path)
	assert.Equal(t, cookies.New("x").SameSite, cleared.SameSite)
	assert.Equal(t, cookies.New("x").Secure, cleared.Secure)
	assert.True(t, cleared.HttpOnly)

	// contracts/auth.md spells the logout header out: `medikube_session=;
	// Max-Age=0`. net/http renders a negative MaxAge that way and a zero one as
	// no Max-Age at all, which would leave the cookie where it was.
	assert.Contains(t, rendered, "Max-Age=0", rendered)
	assert.NotContains(t, rendered, "Max-Age=0=", rendered)
}

// Issue and Clear are what a handler calls, and they have to put the cookie on
// the response exactly once. A sign-in that set two Set-Cookie headers, or none,
// would pass every assertion made against the *http.Cookie value alone.
func TestIssuingAndClearingWriteExactlyOneSetCookieHeader(t *testing.T) {
	t.Parallel()

	cookies := SessionCookie{TTL: time.Hour, Secure: true}

	issued, issuedRecorder := event(t, http.MethodPost, "/api/v1/auth/login")
	cookies.Issue(issued, "a-pocketbase-auth-token")

	cleared, clearedRecorder := event(t, http.MethodPost, "/api/v1/auth/logout")
	cookies.Clear(cleared)

	issuedHeaders := issuedRecorder.Result().Header.Values("Set-Cookie")
	clearedHeaders := clearedRecorder.Result().Header.Values("Set-Cookie")

	require.Len(t, issuedHeaders, 1)
	require.Len(t, clearedHeaders, 1)

	assert.Equal(t, cookies.New("a-pocketbase-auth-token").String(), issuedHeaders[0])
	assert.Equal(t, cookies.Cleared().String(), clearedHeaders[0])
	assert.Contains(t, issuedHeaders[0], "HttpOnly")
	assert.Contains(t, clearedHeaders[0], "Max-Age=0")
}

// Secure is decided by the public URL and by nothing else, and it fails closed
// (research D-15). An operator serving a medical instance over plain http to a
// real hostname gets a browser that refuses to send the cookie — which is a
// deployment that visibly does not work rather than one that quietly ships
// session tokens in the clear.
func TestOnlyALoopbackPublicURLGivesUpTheSecureAttribute(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		publicURL string
		secure    bool
	}{
		{name: "the development default", publicURL: "http://localhost:8090", secure: false},
		{name: "loopback by address", publicURL: "http://127.0.0.1:8090", secure: false},
		{name: "loopback over IPv6", publicURL: "http://[::1]:8090", secure: false},
		{name: "loopback served over TLS is still TLS", publicURL: "https://localhost:8090", secure: true},
		{name: "a real deployment", publicURL: "https://medikube.example.test", secure: true},
		{name: "a real hostname over plain http", publicURL: "http://medikube.example.test", secure: true},
		{name: "a private address is not a loopback one", publicURL: "http://10.0.0.7:8090", secure: true},
		{name: "unset", publicURL: "", secure: true},
		{name: "unparseable", publicURL: "://not-a-url", secure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.secure, NewSessionCookie(time.Hour, tc.publicURL).Secure)
			assert.Equal(t, tc.secure, NewSessionCookie(time.Hour, tc.publicURL).New("token").Secure)
		})
	}
}

// The cookie's Max-Age and the token's own lifetime are one number, taken from
// MEDIKUBE_AUTH_SESSION_TTL at both ends: internal/platform/pb's applySessionTTL
// writes it onto the users collection, and this writes it onto the cookie.
//
// They are asserted together because the two failures are silent and opposite.
// A cookie that outlives its token is a browser that believes it is signed in
// and is refused on every request; a cookie that dies first signs people out
// while their session is still good.
func TestTheCookieAndTheTokenExpireOnTheSameNumber(t *testing.T) {
	t.Parallel()

	const ttl = 42 * time.Hour

	app := testsupport.NewApp(t)
	require.NoError(t, pb.ApplySettings(app, config.Config{Auth: config.AuthConfig{SessionTTL: ttl}}))

	users, err := app.FindCollectionByNameOrId(usersCollection)
	require.NoError(t, err)

	cookie := NewSessionCookie(ttl, "https://medikube.example.test").New("token")

	require.Equal(t, int64((42 * time.Hour).Seconds()), users.AuthToken.Duration,
		"the configured lifetime did not reach the collection, so the comparison below proves nothing")
	assert.Equal(t, users.AuthToken.Duration, int64(cookie.MaxAge))
}

// The number, and the reason it is that number. -1021 is one step before
// PocketBase's loadAuthToken, and the behavioural half of this file is what
// happens at -1019 instead.
func TestTheSessionMiddlewareIsBoundBeforeTheTokenIsLoaded(t *testing.T) {
	t.Parallel()

	assert.Equal(t, -1021, SessionMiddlewarePriority)
	assert.Equal(t, apis.DefaultLoadAuthTokenMiddlewarePriority-1, SessionMiddlewarePriority,
		"PocketBase moved loadAuthToken; the cookie has to move with it")
	assert.Less(t, SessionMiddlewarePriority, apis.DefaultLoadAuthTokenMiddlewarePriority)
	assert.Less(t, SessionMiddlewarePriority, ActorMiddlewarePriority)

	handler := Sessions()
	assert.Equal(t, SessionMiddlewarePriority, handler.Priority)
	assert.Equal(t, SessionMiddlewareID, handler.Id,
		"an unnamed middleware cannot be replaced or reordered, and binding twice appends a second one")
}

// THE TABLE THAT MATTERS. Bound one step later — at the actor's own priority,
// which is a plausible place to put it — the identical cookie authenticates
// nobody, and nothing anywhere says so.
func TestTheCookieAuthenticatesOnlyWhileItIsTranslatedBeforeTheTokenIsLoaded(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		priority int
		cookie   string
	}{
		{
			name:     "before loadAuthToken, which is where it is bound",
			priority: SessionMiddlewarePriority,
			cookie:   testsupport.AccountAID,
		},
		{
			name:     "after loadAuthToken, one step later",
			priority: apis.DefaultLoadAuthTokenMiddlewarePriority + 1,
			cookie:   anonymous,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler, app := cookieEdge(t, tc.priority)
			token := testsupport.UserToken(t, app, testsupport.AccountAEmail)

			assert.Equal(t, tc.cookie, whoami(t, handler, "Cookie", SessionCookieName+"="+token))

			// The control, and it is not decoration: at the wrong priority
			// every header-carrying caller still authenticates, which is why
			// an entire API suite stays green while every browser is signed
			// out.
			require.Equal(t, testsupport.AccountAID, whoami(t, handler, "Authorization", token),
				"the bearer token did not authenticate either, so this row is measuring a broken instance rather than the cookie")
		})
	}
}

// A caller that sent an Authorization header asked to be that caller. Letting
// whatever cookie the browser happens to hold win would make the effective
// identity depend on middleware order rather than on the request.
func TestAnExplicitTokenIsNotOverriddenByACookie(t *testing.T) {
	t.Parallel()

	handler, app := cookieEdge(t, SessionMiddlewarePriority)

	assert.Equal(t, testsupport.AccountBID, whoami(t, handler,
		"Authorization", testsupport.UserToken(t, app, testsupport.AccountBEmail),
		"Cookie", SessionCookieName+"="+testsupport.UserToken(t, app, testsupport.AccountAEmail),
	))
}

// The middleware copies the cookie and judges nothing. A forged, expired or
// revoked one has to arrive at PocketBase's own token check and be refused
// there, exactly as a forged bearer token is — a second opinion about what a
// valid session is would be a second thing to keep in agreement with the
// stream's re-check and with loadAuthToken.
func TestACookieThatIsNotATokenLeavesTheCallerAnonymous(t *testing.T) {
	t.Parallel()

	handler, app := cookieEdge(t, SessionMiddlewarePriority)
	token := testsupport.UserToken(t, app, testsupport.AccountAEmail)

	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "no cookie at all", value: ""},
		{name: "not a token", value: SessionCookieName + "=not-a-token"},
		{name: "an empty cookie", value: SessionCookieName + "="},
		{name: "somebody else's cookie name", value: "session=" + token},
		{name: "a truncated token", value: SessionCookieName + "=" + token[:len(token)-4]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			headers := []string{}
			if tc.value != "" {
				headers = append(headers, "Cookie", tc.value)
			}

			assert.Equal(t, anonymous, whoami(t, handler, headers...))
		})
	}
}

// cookieEdge is the production wiring with the session middleware bound at the
// priority under test: the request logger, the error envelope, the cookie
// translation, the actor, the security headers, and one route that reports who
// the chain decided the caller is.
func cookieEdge(t *testing.T, priority int) (http.Handler, *tests.TestApp) {
	t.Helper()

	app := testsupport.NewApp(t)

	sessions := Sessions()
	sessions.Priority = priority

	pb.BindServe(app, pb.ServeOptions{
		Middlewares: []*hook.Handler[*core.RequestEvent]{
			obs.RequestLogger(discardLogger()),
			Errors(nil),
			sessions,
			Actors(),
		},
		Routes: serveBinders{SecurityBinder{}, probe()},
	})

	return testsupport.NewEdgeHandler(t, app), app
}

// probe answers with the id the middleware chain authenticated, or with
// `anonymous`. It reads e.Auth rather than the actor so that the answer is
// PocketBase's own verdict on the credential and not MediKube's reading of it.
func probe() testsupport.ServeBinder {
	return route(http.MethodGet, "/probe", func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.String(http.StatusOK, anonymous)
		}

		return e.String(http.StatusOK, e.Auth.Id)
	})
}

func whoami(t *testing.T, handler http.Handler, headers ...string) string {
	t.Helper()

	response := call(t, handler, http.MethodGet, "/probe", headers...)
	defer func() { _ = response.Body.Close() }()

	require.Equal(t, http.StatusOK, response.StatusCode)

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return string(body)
}
