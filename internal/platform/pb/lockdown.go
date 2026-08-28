package pb

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"
)

// LockdownMiddlewareID names the handler so it can be found, reordered against
// or removed by id rather than by position.
const LockdownMiddlewareID = "medikubeLockdown"

// LockdownPriority puts the lockdown one step after PocketBase's security
// headers middleware, which is itself after loadAuthToken.
//
// The window is bounded on both sides and the number is derived, never typed:
// bound before loadAuthToken (-1020) e.Auth is still nil and the superuser
// carve-out cannot exist; bound after the CRUD handler it has already answered.
//
// Anywhere inside that window works for authorization. Only this end of it
// works for indistinguishability. The lockdown short-circuits, so every
// middleware bound after it is skipped -- and securityHeaders sits at -1010.
// At -1019 a locked route answered 404 with no X-Content-Type-Options, no
// X-Frame-Options, no X-Xss-Protection and no Cross-Origin-Opener-Policy, while
// a genuinely unknown path answered 404 carrying all four. One header told an
// anonymous caller which of the two it had hit, which is the route-existence
// disclosure this middleware exists to prevent. Running after securityHeaders
// makes the two byte-and-header identical.
//
// The cost is a superuser presenting a token from a non-allowlisted address:
// superuserIPsWhitelist (-1015) now answers 403 before the lockdown answers
// 404. That is visible only to someone already holding a valid superuser token.
const LockdownPriority = apis.DefaultSecurityHeadersMiddlewarePriority + 1

// ErrLockedRouteMissing reports that PocketBase no longer registers a route the
// lockdown closes.
var ErrLockedRouteMissing = errors.New("a route the lockdown closes is not registered by PocketBase")

// LockedRoute is one route the lockdown answers for.
type LockedRoute struct {
	Method string
	Path   string
}

// Pattern is the string Go's ServeMux matches on and therefore the string the
// middleware compares against.
func (r LockedRoute) Pattern() string {
	return r.Method + " " + r.Path
}

// LockedRoutes is the exact set of PocketBase routes MediKube closes.
//
// The discriminator is the router's own matched pattern — e.Request.Pattern,
// which Go's ServeMux has already populated by the time any middleware runs,
// because PocketBase registers the whole hook chain inside the matched route's
// handler (tools/router/router.go:131-152). PocketBase relies on the same fact
// itself at apis/middlewares_rate_limit.go:88.
//
// Every cheaper discriminator is wrong, and wrong in the direction that breaks
// the phases after this one:
//
//   - strings.HasPrefix(path, "/api/collections/") also takes all thirteen
//     authentication routes, collection administration and meta/scaffolds.
//   - PathValue("collection") != "" also takes the file download route and,
//     again, every auth route.
//   - a substring test for "/records" fires on paths that are not records
//     routes at all and would need its own tests to say so.
//
// The pattern set needs none of that: it is the router's own vocabulary, and an
// unmatched path carries the catch-all pattern "/", so the set can never fire
// on a request that was going to 404 anyway.
func LockedRoutes() []LockedRoute {
	return []LockedRoute{
		// The record-CRUD subtree (apis/record_crud.go:28-33). All five
		// methods; a lockdown that covers four is not a lockdown.
		{Method: "GET", Path: "/api/collections/{collection}/records"},
		{Method: "GET", Path: "/api/collections/{collection}/records/{id}"},
		{Method: "POST", Path: "/api/collections/{collection}/records"},
		{Method: "PATCH", Path: "/api/collections/{collection}/records/{id}"},
		{Method: "DELETE", Path: "/api/collections/{collection}/records/{id}"},

		// Batch is a second door into the same handler bodies: it calls
		// recordCreate/recordUpdate/recordDelete directly rather than through
		// the router (apis/batch.go:38-88), so the middleware cannot see those
		// sub-requests. Closing the door itself is what makes that irrelevant.
		// Left to its own handler it answers 403 "Batch requests are not
		// allowed." — a different status and a different sentence, which is a
		// disclosure. Settings().Batch.Enabled = false is the second half of
		// this; both are required.
		{Method: "POST", Path: "/api/batch"},

		// PocketBase's realtime endpoint is a second door to the same record
		// bodies: subscribe to a collection and its create/update payloads
		// arrive in full, record included. Only the POST carries the
		// subscription -- and only the POST can carry auth, because EventSource
		// cannot set headers, which is why the admin UI's GET is anonymous by
		// design (apis/realtime.go, realtimeSetSubscriptions). Closing the POST
		// leaves the GET stream connected and empty, so the admin UI still
		// works and an anonymous caller receives nothing.
		//
		// Under MediKube's schema the nil ListRule/ViewRule already stops this.
		// That is exactly the single control the lockdown exists to be
		// redundant with: record CRUD had two independent controls and realtime
		// had one.
		{Method: "POST", Path: "/api/realtime"},

		// PocketBase's file surface: /api/files/token mints a short-lived
		// credential for any signed-in caller, and the download route serves
		// the file. Neither applies MediKube's authorization. Files leave this
		// application through MediKube's own /api/v1 routes or not at all.
		{Method: "POST", Path: "/api/files/token"},
		{Method: "GET", Path: "/api/files/{collection}/{recordId}/{filename}"},
	}
}

var lockedPatterns = func() map[string]struct{} {
	routes := LockedRoutes()
	patterns := make(map[string]struct{}, len(routes))
	for _, r := range routes {
		patterns[r.Pattern()] = struct{}{}
	}

	return patterns
}()

// Lockdown returns the middleware that closes PocketBase's own record API.
//
// It answers 404 and not 403 deliberately. A 403 confirms that the thing asked
// for exists, and FR-033 requires a request for somebody else's record to be
// answered exactly as a request for one that has never existed. The body comes
// from PocketBase's own NewNotFoundError so the two are byte-identical without
// MediKube owning a string that has to stay in step.
//
// One consequence worth stating rather than discovering: a 404 returned here is
// ahead of the rate limiter at -1000, so these routes stop being rate limited.
// For the records subtree that changes nothing — apis/record_crud.go:28 already
// unbinds the limiter from that group — and for /api/batch it drops the default
// 3-per-second rule, which is an acceptable trade for a route that now costs a
// map lookup.
func Lockdown() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id:       LockdownMiddlewareID,
		Priority: LockdownPriority,
		Func: func(e *core.RequestEvent) error {
			if _, locked := lockedPatterns[e.Request.Pattern]; !locked {
				return e.Next()
			}

			// The admin UI is a superuser's only interface and it drives these
			// routes directly. A superuser bypasses every API rule by design,
			// which is why the constitution answers that with mandatory MFA,
			// an address allowlist and session auditing rather than with a
			// fourth mechanism here.
			if e.HasSuperuserAuth() {
				return e.Next()
			}

			return router.NewNotFoundError("", nil)
		},
	}
}

// AssertLockedRoutesRegistered fails when PocketBase no longer registers a
// route the lockdown closes.
//
// Without it, an upgrade that renames one of these patterns turns the lockdown
// into a no-op for that route: no error, no log line, just a working public API
// over somebody's medical records. This makes that a boot failure instead.
func AssertLockedRoutesRegistered(r *router.Router[*core.RequestEvent]) error {
	var missing []string

	for _, locked := range LockedRoutes() {
		if !r.HasRoute(locked.Method, locked.Path) {
			missing = append(missing, locked.Pattern())
		}
	}

	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("%w: %s", ErrLockedRouteMissing, strings.Join(missing, ", "))
}

// ErrLockdownUnbound reports that the lockdown middleware is not on the router.
var ErrLockdownUnbound = errors.New("the lockdown middleware is not bound")

// AssertLockdownBound fails when the lockdown is not on the root router at the
// priority it is supposed to hold.
//
// AssertLockedRoutesRegistered proves PocketBase still registers the routes;
// this proves MediKube still closes them. They are different failures: a
// RouteBinder that calls se.Router.Unbind(LockdownMiddlewareID), or that binds
// to a group and shadows it, leaves every route registered and every one of
// them open, with no error and no log line to say so.
func AssertLockdownBound(r *router.Router[*core.RequestEvent]) error {
	for _, m := range r.Middlewares {
		if m.Id != LockdownMiddlewareID {
			continue
		}

		if m.Priority != LockdownPriority {
			return fmt.Errorf("%w: bound at priority %d, not %d",
				ErrLockdownUnbound, m.Priority, LockdownPriority)
		}

		return nil
	}

	return fmt.Errorf("%w: no handler with id %q on the root router",
		ErrLockdownUnbound, LockdownMiddlewareID)
}
