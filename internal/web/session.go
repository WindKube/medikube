package web

import (
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// SessionCookieName is the one cookie MediKube sets, and it carries the
// PocketBase auth token (research D-15).
const SessionCookieName = "medikube_session"

// SessionMiddlewareID names the handler so that binding it twice replaces
// rather than appends, and so that a test can rebind it at another priority
// without ending up with two.
const SessionMiddlewareID = "medikubeSessionCookie"

// SessionMiddlewarePriority is -1021, one step BEFORE PocketBase's
// loadAuthToken at -1020, and the whole trick is in that one step.
//
// Hook priorities run low first. loadAuthToken reads the Authorization header
// and nothing else (apis/middlewares.go:186-220), so the cookie has to become
// that header before it runs. Bound after it — at the actor's -1019, say —
// nothing errors and nothing logs: every cookie-carrying request is simply
// anonymous, which is a legitimate value everywhere downstream, so the
// application answers 401 to every browser while every curl-with-a-bearer-token
// test still passes. session_test.go asserts both halves of that table.
//
// Derived rather than spelled, so that a PocketBase upgrade that moves
// loadAuthToken moves this with it; the literal -1021 is asserted separately,
// which is what turns such a move into a failing test rather than a silent
// reordering.
const SessionMiddlewarePriority = apis.DefaultLoadAuthTokenMiddlewarePriority - 1

// authorizationHeader is the header loadAuthToken reads, spelled here and
// re-read by internal/web/stream's session port. Both have to agree: a stream
// re-checks its session by re-parsing the token out of this header
// (stream/session.go's bearer), so a middleware that authenticated by any other
// route — assigning e.Auth, stashing the token in the context — would leave
// every cookie-authenticated stream unable to find a token at all. Browsers are
// exactly the cookie users, so that failure is a browser-only outage with a
// green API suite.
const authorizationHeader = "Authorization"

// Sessions returns the middleware that turns the session cookie into the
// bearer token the rest of the stack already understands.
//
// After it, nothing downstream needs to know a cookie exists: e.Auth,
// apis.RequireAuth, the superuser check, the lockdown and the stream's
// re-check all behave identically for a browser and for curl.
//
// An Authorization header that is already present wins. A caller that sent one
// asked to be that caller, and silently overriding it with whatever cookie the
// browser happened to hold would make the effective identity depend on
// middleware order rather than on the request.
func Sessions() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id:       SessionMiddlewareID,
		Priority: SessionMiddlewarePriority,
		Func: func(e *core.RequestEvent) error {
			if e.Request.Header.Get(authorizationHeader) == "" {
				if cookie, err := e.Request.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
					// The value is copied unvalidated on purpose: validating it
					// here would be a second opinion about what a valid session
					// is, and the whole point is that there is exactly one —
					// PocketBase's, applied to this request exactly as it is
					// applied to an API client's. A forged, expired or revoked
					// cookie therefore arrives anonymous rather than refused,
					// which is what an absent credential looks like everywhere
					// else.
					e.Request.Header.Set(authorizationHeader, cookie.Value)
				}
			}

			return e.Next()
		},
	}
}

// SessionCookie mints and clears the cookie for one instance.
//
// It is a value rather than a package function because two of the attributes
// are the operator's: how long a session lasts, and whether the public URL is
// one a browser will send a Secure cookie to. The name, the path, HttpOnly and
// SameSite are not configurable and never will be.
type SessionCookie struct {
	// TTL is MEDIKUBE_AUTH_SESSION_TTL, the same duration
	// internal/platform/pb's applySessionTTL writes onto the users collection's
	// AuthToken. The two are one number on purpose: a cookie that outlives the
	// token inside it is a browser that believes it is signed in and is refused
	// on every request, and a cookie that dies first signs people out early.
	TTL time.Duration

	// Secure is set unless the public URL is a loopback address, which is the
	// only deployment where a browser will not send a Secure cookie and the
	// only one where that is not a downgrade.
	Secure bool
}

// NewSessionCookie decides the two configurable attributes from the
// configuration the composition root already holds.
func NewSessionCookie(ttl time.Duration, publicURL string) SessionCookie {
	return SessionCookie{TTL: ttl, Secure: secureCookies(publicURL)}
}

// New builds the Set-Cookie a successful sign-in, registration or refresh
// answers with.
//
// The gosec exemption is for Secure alone, and it is not a waiver of the
// attribute: G124 wants a literal `true`, and this one is decided per
// deployment by secureCookies. HttpOnly, SameSite, Path and Max-Age are
// asserted attribute by attribute in session_test.go — against the rendered
// header, which is what a browser is given — and each of them fails the suite
// when it is removed.
//
//nolint:gosec // Secure is configuration, not a constant; the attributes are asserted in session_test.go
func (c SessionCookie) New(token string) *http.Cookie {
	return &http.Cookie{
		Name:  SessionCookieName,
		Value: token,
		// Every route, because a plain navigation to any of them is how a
		// person reaches this application.
		Path: "/",
		// The credential is unreachable from JavaScript, which matters more
		// than usual here: the content security policy grants 'unsafe-eval'
		// for Datastar's expression compiler (research D-35), so an injected
		// expression that could read the token would have it.
		HttpOnly: true,
		// Lax rather than Strict so that following a link into MediKube from a
		// recovery message or a bookmark lands signed in. There is no
		// cross-site form post to protect: form-action 'self' is in the policy
		// and every mutation is a same-origin fetch.
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Secure,
		MaxAge:   int(c.TTL.Seconds()),
	}
}

// Cleared builds the Set-Cookie that ends the browser's half of a sign-out:
// the same cookie, empty, with Max-Age=0.
//
// Every other attribute is repeated deliberately. A browser replaces a cookie
// by name, path and domain, so a clear that dropped Path would leave the
// original one sitting under /, and the person would still be carrying a
// credential the server has already rotated away from.
//
//nolint:gosec // G124 follows the *http.Cookie rather than its attributes; see New
func (c SessionCookie) Cleared() *http.Cookie {
	cleared := c.New("")
	// net/http renders a negative MaxAge as `Max-Age=0`, which is the header
	// contracts/auth.md specifies; zero would render no Max-Age at all and
	// leave the cookie in place.
	cleared.MaxAge = -1

	return cleared
}

// Issue writes the session cookie onto the response.
func (c SessionCookie) Issue(e *core.RequestEvent, token string) {
	http.SetCookie(e.Response, c.New(token))
}

// Clear writes the emptied session cookie onto the response.
//
// It is the browser half of a sign-out and never the whole of one: the session
// itself ends by rotating the record's token key, which is what makes it
// unusable from everywhere else it was open (FR-007). A clear on its own would
// hand the person a browser that has forgotten a credential that still works.
func (c SessionCookie) Clear(e *core.RequestEvent) {
	http.SetCookie(e.Response, c.Cleared())
}

// secureCookies reports whether the public URL is one a browser will send a
// Secure cookie to.
//
// It fails closed: anything that is not demonstrably a loopback address gets
// the attribute, including a public URL that is unparseable or plainly http.
// An operator serving a medical instance over plain http to a real hostname is
// a deployment that should visibly not work rather than one that quietly ships
// session tokens in the clear.
func secureCookies(publicURL string) bool {
	parsed, err := url.Parse(publicURL)
	if err != nil || parsed.Host == "" {
		return true
	}

	if parsed.Scheme != "http" {
		return true
	}

	host := parsed.Hostname()
	if host == "localhost" {
		return false
	}

	address := net.ParseIP(host)

	return address == nil || !address.IsLoopback()
}
