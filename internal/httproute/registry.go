package httproute

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// RouteKind classifies a route by who serves it and which gate checks it. The
// field it fills is spelled Kind (research D-09); the type is not, because
// medikube/internal/domain/kind is imported alongside it and one `Kind` in a
// file is enough.
type RouteKind string

const (
	// KindAPI is a JSON operation under /api/v1. It appears in
	// api/openapi.json and the Principle IX gate asserts it in both
	// directions.
	KindAPI RouteKind = "api"
	// KindStream is a Server-Sent Events endpoint. It is documented in
	// api/openapi.json too, as a text/event-stream response, because FR-064
	// covers "every operation in its public interface" and an SSE endpoint is
	// one.
	KindStream RouteKind = "stream"
	// KindPage is server-rendered HTML. Every page is a browser-gate target,
	// which is why a page with no Landmark or no SmokeURL cannot be
	// registered at all.
	KindPage RouteKind = "page"
	// KindAsset is an embedded static file. It is neither documented nor
	// smoked: an asset has no landmark and no DTO.
	KindAsset RouteKind = "asset"
	// KindExternal is a PocketBase-native path MediKube deliberately leaves
	// reachable. PocketBase serves it, so Bind must not: registering it here
	// would shadow the real handler. It is recorded so the Principle IX gate
	// does not flag it and so nobody believes it was closed
	// (contracts/README.md).
	KindExternal RouteKind = "external"
)

// Auth is the session a caller needs. For a page it is also what the browser
// gate uses to decide which stored session state to visit with.
type Auth string

const (
	AuthPublic Auth = "public"
	AuthUser   Auth = "user"
	AuthAdmin  Auth = "admin"
)

// Handler serves one route. It is PocketBase's own handler signature, so a
// method value from internal/web assigns to it with no conversion.
type Handler func(e *core.RequestEvent) error

// Handlers is the seam the later groups fill: one entry per route, keyed by
// OpID. internal/web/api, internal/web/page and internal/web/stream each
// contribute their own map and the composition root merges them, so a handler
// arrives without anyone editing routes.go and New reports both halves of a
// mismatch.
type Handlers map[string]Handler

// Route is one row of the inventory: everything the router, the OpenAPI
// generator, `medikube routes` and the browser gate need, declared once.
type Route struct {
	// OpID is the route's stable identity. For an api, stream or external
	// route it is the OpenAPI operationId that api/openapi.json is asserted
	// against; for a page it is the page's name. It is also the key a handler
	// is wired under, so two routes cannot share one.
	OpID string

	Method string
	Path   string
	Kind   RouteKind
	Auth   Auth

	// Summary is the one-line description `medikube routes` prints and the
	// OpenAPI operation carries. A route nobody described is a route nobody
	// can review.
	Summary string

	// Landmark is the ARIA selector the browser gate asserts inside <main>,
	// verbatim from contracts/pages.md. Pages only.
	Landmark string

	// SmokeURL is the concrete URL the gate opens, with every path parameter
	// already bound. Pages only. contracts/pages.md's two token pages carry a
	// deliberately invalid token here: a seeded one would be expired by the
	// time CI ran, and the expired-link state is what FR-074 requires anyway.
	SmokeURL string

	// SmokeVariants is US9's addition (contracts/pages.md §3.5): additional
	// concrete URLs on THIS route the browser gate must also visit, each
	// already bound — no unbound "{param}" — exactly like SmokeURL. It is how
	// the seven status views enter the gate without becoming seven more page
	// routes (research L2): they are query strings on an already-registered
	// route, and this is where that route says so. Pages only, and never
	// counted toward the page total `medikube routes` reports.
	SmokeVariants []string

	// Middlewares are bound to this route alone, at the moment it is
	// registered.
	//
	// It exists because a middleware cannot be bound by the handler it wraps:
	// by the time a handler runs, the chain that would have contained it has
	// already been assembled. contracts/streams.md requires the SSE stream to
	// be registered with apis.SkipSuccessActivityLog(), which is a
	// *hook.Handler and therefore has exactly one place it can go — here.
	//
	// Every entry must carry an Id. PocketBase appends an anonymous handler
	// and never replaces it (tools/hook/hook.go), so an unnamed middleware
	// cannot be removed, cannot be replaced and cannot be asserted — which
	// would make this column a thing the inventory records and nothing checks.
	Middlewares []*hook.Handler[*core.RequestEvent]

	// Unbind names middlewares bound further out that must not run on this
	// route.
	//
	// PocketBase's rate limiter has no per-rule exclusion — internal/platform/pb's
	// RateLimitRules says so in as many words — so the stream's exemption from
	// it is an Unbind on the route. router.Route.Unbind also adds the id to an
	// exclude list, which is what makes it reach a middleware bound on the
	// parent group rather than on the route itself.
	Unbind []string
}

// MiddlewareIDs is what this route binds, in order. It is what a test asserts
// against: the alternative is reading them back off PocketBase's router, whose
// RouterGroup keeps its children in an unexported field.
func (r Route) MiddlewareIDs() []string {
	ids := make([]string, 0, len(r.Middlewares))
	for _, middleware := range r.Middlewares {
		ids = append(ids, middleware.Id)
	}

	return ids
}

// Pattern is the method and path as Go's ServeMux spells them, which is also
// how PocketBase's router and its own rate limiter identify a route.
func (r Route) Pattern() string { return r.Method + " " + r.Path }

// ErrorViewName identifies one of contracts/pages.md's three error views.
type ErrorViewName string

// ErrorView is what the application renders instead of a route. It has no
// method and no path on purpose: a 404 is produced by a path that matches
// nothing, a 403 by a session-required page reached without a session and a 500
// by whatever failed — none of the three is a route anybody could register.
// They are declared here anyway, because the browser gate covers them and the
// gate's list is this package.
type ErrorView struct {
	Name     ErrorViewName
	Status   int
	Landmark string
	// Auth is the session state the gate visits with. The sign-in-required
	// view is reached by opening a session-required page with no session, so
	// its Auth is public.
	Auth Auth

	// SmokeURL is the URL that produces this view. Exclusive with
	// Unreachable, and exactly one of the two is required.
	SmokeURL string

	// Unreachable records why no URL in a shipped build produces this view.
	// It exists for exactly one case — the 500 view, which would need a route
	// that deliberately fails, and a build that ships one has a worse problem
	// than an unsmoked error page. Stating the reason in the table is what
	// keeps that omission reviewable instead of invisible; contracts/pages.md
	// already runs deliberately broken builds as negative controls and this
	// view belongs to that family.
	Unreachable string
}

// SmokeTarget is one row of the browser gate's list. e2e/routes.ts derives it
// from `medikube routes --json` at Playwright's collection phase, so a target
// that stops being produced is a page that stops being checked.
type SmokeTarget struct {
	Name     string
	URL      string
	Status   int
	Landmark string
	Auth     Auth
}

// Registry is the one place a route is both registered and described.
//
// It is not safe for concurrent registration and does not need to be:
// everything is declared at startup, from one goroutine, before anything
// listens.
type Registry struct {
	routes   []Route
	views    []ErrorView
	handlers map[string]Handler

	opIDs    map[string]struct{}
	patterns map[string]string
	viewName map[ErrorViewName]struct{}
}

// Empty returns a registry with nothing in it. New and Inventory build on it,
// and a test that exercises registration directly starts here.
func Empty() *Registry {
	return &Registry{
		handlers: make(map[string]Handler),
		opIDs:    make(map[string]struct{}),
		patterns: make(map[string]string),
		viewName: make(map[ErrorViewName]struct{}),
	}
}

// Handle registers a route and describes it in one indivisible call.
//
// It panics on every malformed declaration, because the route table is static
// data read at startup and a defect in it is programmer error with nobody left
// to return an error to. The page checks are the load-bearing ones: a page with
// no landmark or no smoke URL cannot boot, so it cannot ship, so it cannot
// escape the browser gate (FR-067).
func (r *Registry) Handle(route Route, handler Handler) {
	if route.Kind == KindExternal {
		panic(fmt.Sprintf("httproute: %s is external: PocketBase serves it and binding it here would shadow the real handler — use Document", identify(route)))
	}

	if handler == nil {
		panic(fmt.Sprintf("httproute: %s was registered with a nil handler", identify(route)))
	}

	r.describe(route)
	r.handlers[route.OpID] = handler
}

// Document records a PocketBase-native path that MediKube deliberately leaves
// reachable. There is nothing to bind — PocketBase already serves it — but it
// is part of the inventory, so the Principle IX gate does not flag it and so
// nobody discovers it by accident (contracts/README.md).
func (r *Registry) Document(route Route) {
	if route.Kind != KindExternal {
		panic(fmt.Sprintf("httproute: %s is not external: a route MediKube serves is registered and described together — use Handle", identify(route)))
	}

	r.describe(route)
}

// DescribeErrorView declares one of the three error views. It carries no
// handler: an error view is rendered by whatever failed, not by a route.
func (r *Registry) DescribeErrorView(view ErrorView) {
	if view.Name == "" {
		panic("httproute: an error view was declared with no Name")
	}

	if _, duplicate := r.viewName[view.Name]; duplicate {
		panic(fmt.Sprintf("httproute: error view %s is declared twice", view.Name))
	}

	if view.Landmark == "" {
		panic(fmt.Sprintf("httproute: error view %s declares no Landmark, so the browser gate would assert nothing about it", view.Name))
	}

	if view.Status < http.StatusBadRequest || view.Status > 599 {
		panic(fmt.Sprintf("httproute: error view %s declares status %d, which is not an error status", view.Name, view.Status))
	}

	if !validAuth(view.Auth) {
		panic(fmt.Sprintf("httproute: error view %s declares auth %q, which is not public, user or admin", view.Name, view.Auth))
	}

	switch {
	case view.SmokeURL == "" && view.Unreachable == "":
		panic(fmt.Sprintf("httproute: error view %s declares neither a SmokeURL nor an Unreachable reason; one of the two is required so a view cannot leave the browser gate unremarked", view.Name))
	case view.SmokeURL != "" && view.Unreachable != "":
		panic(fmt.Sprintf("httproute: error view %s declares both a SmokeURL and an Unreachable reason; the two are exclusive", view.Name))
	case view.SmokeURL != "":
		assertOpenable(fmt.Sprintf("error view %s", view.Name), view.SmokeURL)
	}

	r.viewName[view.Name] = struct{}{}
	r.views = append(r.views, view)
}

// Routes returns the inventory in registration order. It is a copy: `medikube
// routes`, the OpenAPI generator and the browser gate all read it and none of
// them may edit it for the others.
func (r *Registry) Routes() []Route { return slices.Clone(r.routes) }

// ErrorViews returns the declared error views in declaration order, as a copy.
func (r *Registry) ErrorViews() []ErrorView { return slices.Clone(r.views) }

// SmokeTargets returns exactly the registered pages and the error views that a
// URL can produce, and nothing else.
func (r *Registry) SmokeTargets() []SmokeTarget {
	targets := make([]SmokeTarget, 0, len(r.routes)+len(r.views))

	for _, route := range r.routes {
		if route.Kind != KindPage {
			continue
		}

		targets = append(targets, SmokeTarget{
			Name:     route.OpID,
			URL:      route.SmokeURL,
			Status:   http.StatusOK,
			Landmark: route.Landmark,
			Auth:     route.Auth,
		})
	}

	for _, view := range r.views {
		if view.SmokeURL == "" {
			continue
		}

		targets = append(targets, SmokeTarget{
			Name:     string(view.Name),
			URL:      view.SmokeURL,
			Status:   view.Status,
			Landmark: view.Landmark,
			Auth:     view.Auth,
		})
	}

	return targets
}

// Bind is the only place a MediKube route reaches PocketBase's router, and it
// is what satisfies pb.RouteBinder.
//
// pb.BindServe calls it inside OnServe and runs the lockdown's boot assertions
// afterwards, so a binder that unbound or shadowed the lockdown refuses to
// start.
func (r *Registry) Bind(se *core.ServeEvent) error {
	var problems []error

	for _, route := range r.routes {
		if route.Kind == KindExternal {
			continue
		}

		if r.handlers[route.OpID] == nil {
			problems = append(problems, fmt.Errorf("%s (%s) has no handler", route.OpID, route.Pattern()))

			continue
		}

		if se.Router.HasRoute(route.Method, bindPath(route.Path)) {
			problems = append(problems, fmt.Errorf("%s: %s is already registered on this router", route.OpID, route.Pattern()))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("bind the MediKube route table: %w", errors.Join(problems...))
	}

	for _, route := range r.routes {
		if route.Kind == KindExternal {
			continue
		}

		bound := se.Router.Route(route.Method, bindPath(route.Path), r.handlers[route.OpID])

		// The Auth column is enforced, not merely printed. A session-required
		// PAGE is the one exception: contracts/pages.md renders the sign-in
		// prompt at 403 inside the full shell, which a router-level 401 would
		// pre-empt, so that decision stays with the page handler.
		if route.Kind != KindPage && route.Auth != AuthPublic {
			bound.Bind(apis.RequireAuth())
		}

		// Bind before Unbind, never the other way round: router.Route.Bind
		// clears an id from the exclude list, so an Unbind followed by a Bind
		// of the same id would silently re-admit the middleware the table said
		// to keep off this route.
		if len(route.Middlewares) > 0 {
			bound.Bind(route.Middlewares...)
		}

		if len(route.Unbind) > 0 {
			bound.Unbind(route.Unbind...)
		}
	}

	return nil
}

// describe is the half Handle and Document share.
func (r *Registry) describe(route Route) {
	if route.OpID == "" {
		panic(fmt.Sprintf("httproute: %s declares no OpID; it is the identity the OpenAPI gate and the handler wiring join on", route.Pattern()))
	}

	if _, duplicate := r.opIDs[route.OpID]; duplicate {
		panic(fmt.Sprintf("httproute: %s is registered twice", route.OpID))
	}

	if route.Summary == "" {
		panic(fmt.Sprintf("httproute: %s declares no Summary, which `medikube routes` prints and the OpenAPI operation carries", identify(route)))
	}

	if !validKind(route.Kind) {
		panic(fmt.Sprintf("httproute: %s declares kind %q, which is not api, stream, page, asset or external", identify(route), route.Kind))
	}

	if !validAuth(route.Auth) {
		panic(fmt.Sprintf("httproute: %s declares auth %q, which is not public, user or admin", identify(route), route.Auth))
	}

	if !validMethod(route.Method) {
		panic(fmt.Sprintf("httproute: %s declares method %q; contracts/README.md's verbs are GET, POST, PATCH, PUT and DELETE, uppercase", identify(route), route.Method))
	}

	if !strings.HasPrefix(route.Path, "/") {
		panic(fmt.Sprintf("httproute: %s declares Path %q, which is not absolute", identify(route), route.Path))
	}

	// PocketBase has done no trailing-slash normalisation since v0.23, so
	// /api/v1/records/ and /api/v1/records are two different routes and one of
	// them is a route nobody meant to publish. The application root is the one
	// path that legitimately ends in a slash.
	if route.Path != "/" && strings.HasSuffix(route.Path, "/") {
		panic(fmt.Sprintf("httproute: %s declares Path %q with a trailing slash", identify(route), route.Path))
	}

	if owner, taken := r.patterns[route.Pattern()]; taken {
		panic(fmt.Sprintf("httproute: %s and %s both claim %s", owner, route.OpID, route.Pattern()))
	}

	describeMiddlewares(route)

	if route.Kind == KindPage {
		r.describePage(route)
	} else {
		if route.Landmark != "" {
			panic(fmt.Sprintf("httproute: %s is not a page but declares a Landmark; the browser gate would assert an ARIA role against a response that renders none", identify(route)))
		}

		if route.SmokeURL != "" {
			panic(fmt.Sprintf("httproute: %s is not a page but declares a SmokeURL; only pages are browser-gate targets", identify(route)))
		}

		if len(route.SmokeVariants) > 0 {
			panic(fmt.Sprintf("httproute: %s is not a page but declares SmokeVariants; only pages are browser-gate targets", identify(route)))
		}
	}

	r.opIDs[route.OpID] = struct{}{}
	r.patterns[route.Pattern()] = route.OpID
	r.routes = append(r.routes, route)
}

// describeMiddlewares refuses a per-route middleware that cannot be named.
//
// An anonymous handler is appended rather than replaced, cannot be unbound and
// cannot be read back, so a route carrying one records an intention nothing
// verifies. An empty Unbind id is the same defect from the other side:
// router.Route.Unbind skips it silently.
func describeMiddlewares(route Route) {
	for index, middleware := range route.Middlewares {
		if middleware == nil {
			panic(fmt.Sprintf("httproute: %s declares a nil middleware at position %d", identify(route), index))
		}

		if middleware.Id == "" {
			panic(fmt.Sprintf("httproute: %s binds an anonymous middleware at position %d; PocketBase appends rather than replaces one, and nothing can unbind or assert it", identify(route), index))
		}
	}

	for index, id := range route.Unbind {
		if id == "" {
			panic(fmt.Sprintf("httproute: %s unbinds an empty middleware id at position %d, which router.Route.Unbind skips in silence", identify(route), index))
		}
	}
}

// describePage is FR-067 made mechanical. plan.md calls this panic
// load-bearing: it is why a page cannot escape the browser gate.
func (r *Registry) describePage(route Route) {
	if route.Landmark == "" {
		panic(fmt.Sprintf("httproute: page %s declares no Landmark, so the browser gate could not assert what it renders (FR-067)", route.OpID))
	}

	if route.SmokeURL == "" {
		panic(fmt.Sprintf("httproute: page %s declares no SmokeURL, so the browser gate would never open it (FR-067)", route.OpID))
	}

	assertOpenable("page "+route.OpID, route.SmokeURL)

	for _, variant := range route.SmokeVariants {
		assertOpenable("page "+route.OpID+"'s smoke variant", variant)
	}
}

func assertOpenable(who, smokeURL string) {
	if !strings.HasPrefix(smokeURL, "/") {
		panic(fmt.Sprintf("httproute: %s declares SmokeURL %q, which is not absolute", who, smokeURL))
	}

	if strings.ContainsAny(smokeURL, "{}") {
		panic(fmt.Sprintf("httproute: %s declares SmokeURL %q, which still has an unbound parameter in it", who, smokeURL))
	}
}

// bindPath is the trap this package exists to absorb. Go 1.22's ServeMux reads
// "GET /" as a prefix pattern matching every GET it does not match more
// specifically, so registering the overview page verbatim would swallow every
// unknown path and the 404 error view would never render. "{$}" is the
// exact-match form. The inventory keeps "/", because that is the path
// contracts/pages.md declares and the URL the gate opens.
func bindPath(path string) string {
	if path == "/" {
		return "/{$}"
	}

	return path
}

// identify names a route in a panic. Falling back to the pattern matters: the
// message for a missing OpID has no OpID to print.
func identify(route Route) string {
	if route.OpID == "" {
		return route.Pattern()
	}

	return route.OpID + " (" + route.Pattern() + ")"
}

func validKind(k RouteKind) bool {
	return slices.Contains([]RouteKind{KindAPI, KindStream, KindPage, KindAsset, KindExternal}, k)
}

func validAuth(a Auth) bool {
	return slices.Contains([]Auth{AuthPublic, AuthUser, AuthAdmin}, a)
}

// The five verbs of contracts/README.md, uppercase. Go's ServeMux matches the
// method literally, so a lowercase one silently matches nothing.
func validMethod(m string) bool {
	return slices.Contains([]string{
		http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete,
	}, m)
}
