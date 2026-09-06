package httproute_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
	"medikube/internal/platform/pb"
)

// T093. Handle(spec, handler) is one indivisible call: there is no way to
// register a route without describing it and no way to describe one without
// registering it, because the route table cannot be recovered from PocketBase's
// router afterwards — RouterGroup.children is unexported and Go 1.27's
// http.ServeMux still has no pattern-enumeration API (research D-09).

// serveEvent builds the event Registry.Bind is handed, without an app.
//
// core.ServeEvent carries an App, but nothing in Bind reads it: the router is a
// generic wrapper over http.ServeMux and its ErrorHandler resolves an ApiError
// with no help from the application. Building the event by hand rather than
// through tests.NewTestApp keeps this suite at milliseconds and, more
// importantly, keeps it honest about what Bind actually depends on.
func serveEvent(t *testing.T) *core.ServeEvent {
	t.Helper()

	se := new(core.ServeEvent)
	se.Router = router.NewRouter(func(w http.ResponseWriter, r *http.Request) (*core.RequestEvent, router.EventCleanupFunc) {
		e := new(core.RequestEvent)
		e.Response = w
		e.Request = r

		return e, nil
	})

	return se
}

// serve binds the registry and returns a live mux, so a test can assert what a
// request actually reaches rather than what the table says it should.
func serve(t *testing.T, registry *httproute.Registry) http.Handler {
	t.Helper()

	se := serveEvent(t)
	require.NoError(t, registry.Bind(se))

	mux, err := se.Router.BuildMux()
	require.NoError(t, err)

	return mux
}

func get(t *testing.T, mux http.Handler, target string) *http.Response {
	t.Helper()

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))

	return recorder.Result()
}

// panicMessage runs f and returns the value it panicked with, as a string. It
// fails the test when f returns normally, because every caller here is
// asserting that a malformed registration is refused.
func panicMessage(t *testing.T, f func()) string {
	t.Helper()

	var recovered any

	func() {
		defer func() { recovered = recover() }()
		f()
	}()

	require.NotNil(t, recovered, "the registration was accepted; it must panic")

	return fmt.Sprint(recovered)
}

// samplePage and sampleAPI are the two shapes every test below starts from.
// They are deliberately not MediKube routes: this file tests the mechanism, and
// routes_test.go tests the table.
func samplePage() httproute.Route {
	return httproute.Route{
		OpID:     "samplePage",
		Method:   http.MethodGet,
		Path:     "/sample",
		Kind:     httproute.KindPage,
		Auth:     httproute.AuthPublic,
		Summary:  "a page that exists only in this test",
		Landmark: `region[name="Sample"]`,
		SmokeURL: "/sample",
	}
}

func sampleAPI() httproute.Route {
	return httproute.Route{
		OpID:    "sampleOperation",
		Method:  http.MethodGet,
		Path:    "/api/v1/sample",
		Kind:    httproute.KindAPI,
		Auth:    httproute.AuthPublic,
		Summary: "an operation that exists only in this test",
	}
}

func TestHandleRegistersAndDescribesInOneCall(t *testing.T) {
	t.Parallel()

	registry := httproute.Empty()

	reached := make(map[string]bool)
	handler := func(name string) httproute.Handler {
		return func(e *core.RequestEvent) error {
			reached[name] = true

			return e.NoContent(http.StatusNoContent)
		}
	}

	page, api := samplePage(), sampleAPI()
	registry.Handle(page, handler(page.OpID))
	registry.Handle(api, handler(api.OpID))

	t.Run("the description is what was handed in", func(t *testing.T) {
		assert.Equal(t, []httproute.Route{page, api}, registry.Routes())
	})

	t.Run("the same call bound the handler", func(t *testing.T) {
		mux := serve(t, registry)

		require.Equal(t, http.StatusNoContent, get(t, mux, page.Path).StatusCode)
		require.Equal(t, http.StatusNoContent, get(t, mux, api.Path).StatusCode)

		assert.True(t, reached[page.OpID], "the page route was described but not served")
		assert.True(t, reached[api.OpID], "the api route was described but not served")
	})
}

func TestRoutesReturnsACopy(t *testing.T) {
	t.Parallel()

	registry := httproute.Empty()
	registry.Handle(samplePage(), func(e *core.RequestEvent) error { return nil })

	routes := registry.Routes()
	require.Len(t, routes, 1)
	routes[0].Path = "/mutated"

	assert.Equal(t, "/sample", registry.Routes()[0].Path,
		"the inventory is read by the CLI, the OpenAPI generator and the smoke list; one of them must not be able to edit it for the others")
}

func TestRoutesAndSmokeTargetsAgree(t *testing.T) {
	t.Parallel()

	registry := httproute.Empty()

	page, api := samplePage(), sampleAPI()
	registry.Handle(page, func(e *core.RequestEvent) error { return nil })
	registry.Handle(api, func(e *core.RequestEvent) error { return nil })
	registry.Document(httproute.Route{
		OpID:    "sampleExternal",
		Method:  http.MethodPost,
		Path:    "/api/collections/users/auth-with-password",
		Kind:    httproute.KindExternal,
		Auth:    httproute.AuthPublic,
		Summary: "a documented external that MediKube does not serve",
	})

	targets := registry.SmokeTargets()

	require.Len(t, targets, 1, "exactly the one page; an api route and an external are not browser targets")
	assert.Equal(t, httproute.SmokeTarget{
		Name:     page.OpID,
		URL:      page.SmokeURL,
		Status:   http.StatusOK,
		Landmark: page.Landmark,
		Auth:     page.Auth,
	}, targets[0])
}

// FR-067. The panic is the strongest link in the Principle VIII chain: a page
// without a landmark or without a smoke URL cannot boot, so it cannot ship, so
// it cannot escape the browser gate.
func TestRegisteringAPageWithoutALandmarkOrASmokeURLPanics(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*httproute.Route)
		message string
	}{
		{
			name:    "no landmark",
			mutate:  func(r *httproute.Route) { r.Landmark = "" },
			message: "no Landmark",
		},
		{
			name:    "no smoke URL",
			mutate:  func(r *httproute.Route) { r.SmokeURL = "" },
			message: "no SmokeURL",
		},
		{
			name:    "a smoke URL with an unbound parameter",
			mutate:  func(r *httproute.Route) { r.SmokeURL = "/sample/{id}" },
			message: "unbound parameter",
		},
		{
			name:    "a relative smoke URL",
			mutate:  func(r *httproute.Route) { r.SmokeURL = "sample" },
			message: "SmokeURL",
		},
		{
			name:    "a smoke variant with an unbound parameter",
			mutate:  func(r *httproute.Route) { r.SmokeVariants = []string{"/sample/{id}"} },
			message: "unbound parameter",
		},
		{
			name:    "a relative smoke variant",
			mutate:  func(r *httproute.Route) { r.SmokeVariants = []string{"sample"} },
			message: "SmokeURL",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			route := samplePage()
			testCase.mutate(&route)

			message := panicMessage(t, func() {
				httproute.Empty().Handle(route, func(e *core.RequestEvent) error { return nil })
			})

			assert.Contains(t, message, testCase.message)
			assert.Contains(t, message, route.OpID, "the panic must name the offending route")
		})
	}
}

func TestHandleRefusesAMalformedRoute(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		route   httproute.Route
		message string
	}{
		{
			name:    "no OpID",
			route:   httproute.Route{Method: http.MethodGet, Path: "/x", Kind: httproute.KindAPI, Auth: httproute.AuthPublic, Summary: "x"},
			message: "no OpID",
		},
		{
			name:    "no summary",
			route:   httproute.Route{OpID: "x", Method: http.MethodGet, Path: "/x", Kind: httproute.KindAPI, Auth: httproute.AuthPublic},
			message: "no Summary",
		},
		{
			name:    "an unknown kind",
			route:   httproute.Route{OpID: "x", Method: http.MethodGet, Path: "/x", Kind: "widget", Auth: httproute.AuthPublic, Summary: "x"},
			message: "kind",
		},
		{
			name:    "an unknown auth",
			route:   httproute.Route{OpID: "x", Method: http.MethodGet, Path: "/x", Kind: httproute.KindAPI, Auth: "root", Summary: "x"},
			message: "auth",
		},
		{
			name:    "an unknown method",
			route:   httproute.Route{OpID: "x", Method: "FETCH", Path: "/x", Kind: httproute.KindAPI, Auth: httproute.AuthPublic, Summary: "x"},
			message: "method",
		},
		{
			name:    "a lowercase method",
			route:   httproute.Route{OpID: "x", Method: "get", Path: "/x", Kind: httproute.KindAPI, Auth: httproute.AuthPublic, Summary: "x"},
			message: "method",
		},
		{
			name:    "a relative path",
			route:   httproute.Route{OpID: "x", Method: http.MethodGet, Path: "x", Kind: httproute.KindAPI, Auth: httproute.AuthPublic, Summary: "x"},
			message: "Path",
		},
		{
			// contracts/README.md: PocketBase has done no trailing-slash
			// normalisation since v0.23, so /api/v1/records/ and
			// /api/v1/records are two different routes.
			name:    "a trailing slash",
			route:   httproute.Route{OpID: "x", Method: http.MethodGet, Path: "/api/v1/records/", Kind: httproute.KindAPI, Auth: httproute.AuthPublic, Summary: "x"},
			message: "trailing slash",
		},
		{
			name:    "an external, which is documented rather than served",
			route:   httproute.Route{OpID: "x", Method: http.MethodGet, Path: "/x", Kind: httproute.KindExternal, Auth: httproute.AuthPublic, Summary: "x"},
			message: "Document",
		},
		{
			name:    "a landmark on something that is not a page",
			route:   httproute.Route{OpID: "x", Method: http.MethodGet, Path: "/x", Kind: httproute.KindAPI, Auth: httproute.AuthPublic, Summary: "x", Landmark: `region[name="X"]`},
			message: "Landmark",
		},
		{
			name:    "a smoke URL on something that is not a page",
			route:   httproute.Route{OpID: "x", Method: http.MethodGet, Path: "/x", Kind: httproute.KindAPI, Auth: httproute.AuthPublic, Summary: "x", SmokeURL: "/x"},
			message: "SmokeURL",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			message := panicMessage(t, func() {
				httproute.Empty().Handle(testCase.route, func(e *core.RequestEvent) error { return nil })
			})

			assert.Contains(t, message, testCase.message)
		})
	}
}

func TestHandleRefusesANilHandler(t *testing.T) {
	t.Parallel()

	message := panicMessage(t, func() { httproute.Empty().Handle(sampleAPI(), nil) })

	assert.Contains(t, message, "nil handler")
}

func TestDocumentIsForExternalsOnly(t *testing.T) {
	t.Parallel()

	message := panicMessage(t, func() { httproute.Empty().Document(sampleAPI()) })

	assert.Contains(t, message, "Handle")
}

// The five nil API rules and the -1009 lockdown are PocketBase's to enforce on
// the native routes; MediKube documents them so the Principle IX gate does not
// flag them, and binding one here would shadow the real handler with a 404.
func TestBindSkipsExternals(t *testing.T) {
	t.Parallel()

	registry := httproute.Empty()
	external := httproute.Route{
		OpID:    "sampleExternal",
		Method:  http.MethodGet,
		Path:    "/api/collections/users/auth-methods",
		Kind:    httproute.KindExternal,
		Auth:    httproute.AuthPublic,
		Summary: "a documented external that PocketBase serves",
	}
	registry.Document(external)

	se := serveEvent(t)
	require.NoError(t, registry.Bind(se))

	assert.False(t, se.Router.HasRoute(external.Method, external.Path),
		"binding a documented external would shadow PocketBase's own handler")
	assert.Contains(t, registry.Routes(), external, "it is still part of the inventory")
}

// FR-034 and the universal authorization rule: a route the table calls `user`
// is a route the router refuses anonymously. Leaving Auth as prose that only a
// handler honours is the drift this package exists to prevent.
func TestBindRequiresAuthOnEveryNonPublicAPIRoute(t *testing.T) {
	t.Parallel()

	registry := httproute.Empty()

	reached := false
	guarded := sampleAPI()
	guarded.OpID = "guardedOperation"
	guarded.Path = "/api/v1/guarded"
	guarded.Auth = httproute.AuthUser
	registry.Handle(guarded, func(e *core.RequestEvent) error {
		reached = true

		return e.NoContent(http.StatusNoContent)
	})

	public := sampleAPI()
	registry.Handle(public, func(e *core.RequestEvent) error { return e.NoContent(http.StatusNoContent) })

	mux := serve(t, registry)

	assert.Equal(t, http.StatusUnauthorized, get(t, mux, guarded.Path).StatusCode)
	assert.False(t, reached, "the handler ran for an anonymous caller")
	assert.Equal(t, http.StatusNoContent, get(t, mux, public.Path).StatusCode)
}

// A session-required PAGE renders the sign-in prompt at 403 inside the full
// shell (contracts/pages.md E2), which a router-level 401 would pre-empt. So
// the page handler owns that decision and the registry binds nothing.
func TestBindDoesNotRequireAuthOnAPage(t *testing.T) {
	t.Parallel()

	registry := httproute.Empty()

	page := samplePage()
	page.Auth = httproute.AuthUser
	registry.Handle(page, func(e *core.RequestEvent) error { return e.NoContent(http.StatusNoContent) })

	assert.Equal(t, http.StatusNoContent, get(t, serve(t, registry), page.Path).StatusCode)
}

// Go 1.22's ServeMux treats "GET /" as a prefix pattern matching every GET the
// mux does not match more specifically, so registering the overview page at "/"
// verbatim swallows every unknown path and the 404 error view never renders.
// Bind registers the exact-match form; the inventory keeps "/" because that is
// the path contracts/pages.md P3 declares and the URL the gate visits.
func TestTheRootPageDoesNotSwallowUnknownPaths(t *testing.T) {
	t.Parallel()

	registry := httproute.Empty()

	root := samplePage()
	root.OpID = "rootPage"
	root.Path = "/"
	root.SmokeURL = "/"
	registry.Handle(root, func(e *core.RequestEvent) error { return e.NoContent(http.StatusNoContent) })

	mux := serve(t, registry)

	assert.Equal(t, http.StatusNoContent, get(t, mux, "/").StatusCode)
	assert.Equal(t, http.StatusNotFound, get(t, mux, "/no-such-page").StatusCode)
	assert.Equal(t, "/", registry.Routes()[0].Path, "the inventory keeps the declared path")
}

func TestBindRefusesARouteWithNoHandler(t *testing.T) {
	t.Parallel()

	err := httproute.Inventory().Bind(serveEvent(t))

	require.Error(t, err, "a described-but-unserved route must not reach a listening process")
	assert.Contains(t, err.Error(), "no handler")
}

func TestBindReportsAPatternSomethingElseAlreadyOwns(t *testing.T) {
	t.Parallel()

	registry := httproute.Empty()
	registry.Handle(sampleAPI(), func(e *core.RequestEvent) error { return nil })

	se := serveEvent(t)
	se.Router.GET(sampleAPI().Path, func(e *core.RequestEvent) error { return nil })

	err := registry.Bind(se)

	require.Error(t, err, "http.ServeMux panics on a duplicate pattern, far from here and with no route name in it")
	assert.Contains(t, err.Error(), sampleAPI().Path)
}

// The seam pb.ServeOptions.Routes declares. Asserting it at compile time is
// what keeps the two packages wired without either importing the other's
// concrete type.
var _ pb.RouteBinder = (*httproute.Registry)(nil)

// The whole table, through the real OnServe binding, against a real instance.
//
// pb.BindServe runs the lockdown's boot assertions AFTER this Bind, on purpose:
// a binder that unbound or shadowed the lockdown would otherwise boot serving
// PocketBase's record API to anonymous callers. This is the test that says the
// route table is not that binder.
func TestTheWholeTableBindsWithoutDisturbingTheLockdown(t *testing.T) {
	t.Parallel()

	handlers := make(httproute.Handlers)
	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindExternal {
			continue
		}

		handlers[route.OpID] = func(e *core.RequestEvent) error { return e.NoContent(http.StatusNoContent) }
	}

	registry, err := httproute.New(handlers)
	require.NoError(t, err)

	app, err := tests.NewTestApp()
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	pb.BindServe(app, pb.ServeOptions{Routes: registry})

	pbRouter, err := apis.NewRouter(app)
	require.NoError(t, err)

	se := new(core.ServeEvent)
	se.App = app
	se.Router = pbRouter

	require.NoError(t, app.OnServe().Trigger(se, func(e *core.ServeEvent) error { return nil }),
		"the lockdown's boot assertions run after Registry.Bind and refused the result")

	for _, route := range registry.Routes() {
		if route.Kind == httproute.KindExternal {
			assert.False(t, se.Router.HasRoute(route.Method, route.Path), "%s is PocketBase's to serve", route.OpID)

			continue
		}

		path := route.Path
		if path == "/" {
			path = "/{$}"
		}

		assert.True(t, se.Router.HasRoute(route.Method, path), "%s reached the inventory but not the router", route.OpID)
	}

	_, err = se.Router.BuildMux()
	require.NoError(t, err, "http.ServeMux refused the assembled pattern set")
}
