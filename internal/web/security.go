package web

import (
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// SecurityHeadersMiddlewareID names MediKube's handler. It is deliberately NOT
// PocketBase's id: reusing that would re-include the id in a group it had been
// excluded from, and a rename upstream would silently un-replace MediKube's
// with PocketBase's rather than failing.
const SecurityHeadersMiddlewareID = "medikubeSecurityHeaders"

// SecurityHeadersMiddlewarePriority takes the slot PocketBase's own headers
// used, which is one step BEFORE the lockdown at -1009.
//
// The order is load-bearing and the lockdown's own comment explains why from
// the other side: the lockdown short-circuits, so every middleware bound after
// it is skipped for locked routes only. Headers written after it would be
// present on an unknown path and absent on a closed one, which tells an
// anonymous caller which 404 they hit.
const SecurityHeadersMiddlewarePriority = apis.DefaultSecurityHeadersMiddlewarePriority

// ContentSecurityPolicy is FR-042, exactly.
//
// `'unsafe-eval'` is the ONLY relaxed directive. Datastar's expression compiler
// is literally the Function constructor and its signal parser falls back to it,
// so without this every data-* expression on every page throws: the application
// does not degrade, it is entirely non-functional and it fills the console with
// violations. It is a genuine, permanent security trade-off, recorded as one
// (research D-35).
//
// What bounds it: `'unsafe-inline'` is never granted, so an injected <script>
// tag still does not run; every Datastar expression is server-authored templ
// output and expression text never comes from user input; `connect-src` and
// `form-action 'self'` block exfiltration; `object-src 'none'` and
// `frame-ancestors 'none'` close the classic bypasses.
//
// The three source directives beyond the six tasks.md T121 and
// contracts/pages.md list are not a weakening — they are what makes
// `default-src 'none'` survivable. The application serves its own stylesheet,
// its own vendored Datastar runtime and its own SSE stream from its own origin;
// with `default-src 'none'` and no `style-src`, `img-src` or `connect-src`,
// none of the three would load and contracts/pages.md's own assertion 6 — zero
// CSP violations — would fail on every route. research D-35 and plan.md:449
// name all nine directives and weaken `default-src` to `'self'` instead; this
// policy takes the strict half of each document.
const ContentSecurityPolicy = "default-src 'none'; " +
	"script-src 'self' 'unsafe-eval'; " +
	"style-src 'self'; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"object-src 'none'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

// StrictTransportSecurity is one year with subdomains and deliberately without
// `preload`: preload is an irreversible submission to a list every browser
// ships, and that is an operator's decision rather than a library's.
const StrictTransportSecurity = "max-age=31536000; includeSubDomains"

// ReferrerPolicy is no-referrer because a referrer carries the path, and a path
// in this application is /medications/{id} — an identifier for a person's
// medication, handed to whatever they click through to.
const ReferrerPolicy = "no-referrer"

// AdminUIPattern is the route PocketBase serves its superuser admin UI on
// (apis/serve.go:84). It is the router's own matched pattern, which is the same
// discriminator the lockdown uses and the only one that cannot fire on a path
// that was going to 404 anyway.
const AdminUIPattern = "GET /_/{path...}"

// SecurityHeaders returns MediKube's security-header middleware.
//
// Everything is written BEFORE the chain continues, so a middleware that
// short-circuits later — the lockdown, the rate limiter — still answers with
// the full set.
//
// PocketBase's X-XSS-Protection is deliberately not among them. The header is
// deprecated, every current browser ignores it, and the auditor it enabled was
// itself an XS-leak vector; setting it back would be reproducing a defect for
// the sake of a diff.
func SecurityHeaders() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id:       SecurityHeadersMiddlewareID,
		Priority: SecurityHeadersMiddlewarePriority,
		Func: func(e *core.RequestEvent) error {
			header := e.Response.Header()

			header.Set("X-Content-Type-Options", "nosniff")
			header.Set("X-Frame-Options", "DENY")
			header.Set("Cross-Origin-Opener-Policy", "same-origin")
			header.Set("Referrer-Policy", ReferrerPolicy)
			header.Set("Strict-Transport-Security", StrictTransportSecurity)

			// The one exemption, and it is not a hole: /_/ is a documented
			// external in the route registry, its existence is public, and
			// PocketBase applies its own hardened policy there only when the
			// header is empty (apis/serve.go:83-99). MediKube's policy forbids
			// the inline styles and the map tiles that UI loads, and
			// constitution VII keeps it in production as the break-glass
			// interface — a blank one is a door that does not open.
			if e.Request.Pattern != AdminUIPattern {
				header.Set("Content-Security-Policy", ContentSecurityPolicy)
			}

			return e.Next()
		},
	}
}

// SecurityBinder installs MediKube's security headers in place of PocketBase's.
//
// The unbind is the load-bearing half. PocketBase's securityHeaders sets
// X-Frame-Options with Set rather than set-if-empty, at the same priority
// (apis/middlewares.go:288-304), so a MediKube middleware bound alongside it
// under its own id loses the race and every page becomes framable by the
// instance itself. Removing it by id and binding MediKube's under a different
// one is explicit in both directions.
type SecurityBinder struct{}

// Bind satisfies both pb.RouteBinder and testsupport.ServeBinder. It registers
// and returns; continuing the chain belongs to whoever bound it.
func (SecurityBinder) Bind(se *core.ServeEvent) error {
	se.Router.Unbind(apis.DefaultSecurityHeadersMiddlewareId)
	se.Router.Bind(SecurityHeaders())

	return nil
}
