package pb

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// ServeHookID names the OnServe handler, so binding twice replaces rather than
// appends. PocketBase's hook.Bind only replaces when an id is supplied
// (tools/hook/hook.go:65-104); an anonymous handler accumulates forever, which
// is a real defect in apis/extensions.go and not one to reproduce here.
const ServeHookID = "medikubeServe"

// ServeHookPriority runs MediKube's binding ahead of PocketBase's own OnServe
// handlers — the cron starter at 999 and the admin-UI extensions at 9999 — so
// the middleware chain is settled before anything reads it.
const ServeHookPriority = -1000

// RouteBinder is the seam internal/httproute.Registry plugs into. It is
// declared here, by the consumer, so that the registry satisfies it by existing
// and nobody has to edit this file to wire it up.
type RouteBinder interface {
	Bind(se *core.ServeEvent) error
}

// ServeOptions is what the composition root decides.
type ServeOptions struct {
	// WriteTimeout replaces PocketBase's hardcoded five minutes.
	//
	// Zero — the default — removes the server-wide write deadline entirely,
	// and that is deliberate. Any positive value is a silent cap on every
	// Server-Sent Events stream and every large download, and it fails in the
	// worst possible way: the connection dies mid-write, the browser
	// reconnect-loops, and every test shorter than the cap passes.
	WriteTimeout time.Duration

	// Middlewares are MediKube's own, bound on the root router. The request
	// logger belongs here: this file unbinds PocketBase's activity logger and
	// something has to take its place.
	Middlewares []*hook.Handler[*core.RequestEvent]

	// Routes is MediKube's route table. Nil is valid — a build that serves
	// nothing but the platform is a build.
	Routes RouteBinder
}

// BindServe installs MediKube's OnServe binding: the write timeout, the
// middleware order, the lockdown, and the route table.
//
// Everything here has to happen inside OnServe rather than before it, because
// PocketBase assigns the ServeEvent its server at apis/serve.go:212 and builds
// the mux and the listener inside the terminal function at :218-250 — so an
// OnServe handler is the last moment at which either is still changeable, and
// the first at which both exist.
func BindServe(app core.App, opts ServeOptions) {
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Id:       ServeHookID,
		Priority: ServeHookPriority,
		Func: func(se *core.ServeEvent) error {
			// tests.ApiScenario builds the event by hand and sets only App and
			// Router (tests/api.go:192-195), so this is nil in every
			// scenario-based HTTP test in the repository.
			if se.Server != nil {
				se.Server.WriteTimeout = opts.WriteTimeout
			}

			// PocketBase's first-run installer is a superuser creation flow
			// reachable without credentials, and it makes a freshly seeded
			// instance non-deterministic for the browser gate — the first
			// navigation lands on an installer instead of the application
			// (research D-06).
			se.InstallerFunc = nil

			// PocketBase's activity logger writes the full request URI into a
			// second log store, and a query string is where a search for a
			// medication name ends up (FR-038, Principle VI). MediKube's own
			// request logger records the path only, and arrives through
			// opts.Middlewares.
			se.Router.Unbind(apis.DefaultActivityLoggerMiddlewareId)

			for _, middleware := range opts.Middlewares {
				se.Router.Bind(middleware)
			}

			// On the root router, not on a sub-group: RouterGroup.children is
			// unexported, so a middleware bound to a group obtained by calling
			// Group() a second time lands on a new, empty group and applies to
			// nothing at all.
			se.Router.Bind(Lockdown())

			if opts.Routes != nil {
				if err := opts.Routes.Bind(se); err != nil {
					return fmt.Errorf("bind the MediKube route table: %w", err)
				}
			}

			// After opts.Routes.Bind, never before it. A RouteBinder can call
			// se.Router.Unbind(LockdownMiddlewareID) -- deliberately or by
			// reaching for a group -- and RouterGroup.Unbind strips the handler
			// with no error and no log line. Asserted ahead of that call the
			// check passes and the instance boots serving PocketBase's record
			// API to anonymous callers.
			if err := AssertLockedRoutesRegistered(se.Router); err != nil {
				return err
			}

			if err := AssertLockdownBound(se.Router); err != nil {
				return err
			}

			return se.Next()
		},
	})
}
