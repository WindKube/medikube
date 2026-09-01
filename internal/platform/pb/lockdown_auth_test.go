package pb_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
)

// authRoute is one of PocketBase's collection-scoped authentication endpoints.
//
// They share the /api/collections/ prefix with the record-CRUD subtree, which is
// why the lockdown discriminates on the router's own matched pattern instead of
// on a path prefix. A prefix match one segment too greedy takes all of these
// with it — and every phase after this one is built on them.
type authRoute struct {
	name string
	// want is the status an anonymous caller gets on an untouched PocketBase.
	// Anything other than the same status under the lockdown is the lockdown
	// having eaten a route it must not touch; a 404 in particular means the
	// discriminator is wrong.
	//
	// wantSuperusers overrides it where the two collections genuinely differ:
	// `_superusers` has neither OAuth2 nor one-time codes configured, so those
	// three routes answer 403 "not configured" before reaching the validation
	// that gives `users` a 400. Different status, same conclusion — reachable.
	want           int
	wantSuperusers int
	method         string
	path           string
	body           string
}

// The thirteen collection-scoped auth routes of apis/record_auth.go:20-68.
//
// The task text says "fourteen … including the superusers equivalents" and then
// lists eleven. Both counts are wrong for v0.40.1 and the correction matters:
// there is no separate /api/admins route family any more — `_superusers` is an
// ordinary collection name flowing through these same {collection} patterns —
// and the list omits request-otp and auth-with-otp, which exist. So the honest
// shape is thirteen patterns run against BOTH collections, twenty-six
// assertions, plus the two global oauth2-redirect routes.
func collectionAuthRoutes() []authRoute {
	return []authRoute{
		{name: "auth-methods", method: http.MethodGet, path: "/auth-methods", want: http.StatusOK},
		{name: "auth-refresh", method: http.MethodPost, path: "/auth-refresh", body: "{}", want: http.StatusUnauthorized},
		{name: "auth-with-password", method: http.MethodPost, path: "/auth-with-password", body: "{}", want: http.StatusBadRequest},
		{name: "auth-with-oauth2", method: http.MethodPost, path: "/auth-with-oauth2", body: "{}", want: http.StatusBadRequest, wantSuperusers: http.StatusForbidden},
		{name: "request-otp", method: http.MethodPost, path: "/request-otp", body: "{}", want: http.StatusBadRequest, wantSuperusers: http.StatusForbidden},
		{name: "auth-with-otp", method: http.MethodPost, path: "/auth-with-otp", body: "{}", want: http.StatusBadRequest, wantSuperusers: http.StatusForbidden},
		{name: "request-password-reset", method: http.MethodPost, path: "/request-password-reset", body: "{}", want: http.StatusBadRequest},
		{name: "confirm-password-reset", method: http.MethodPost, path: "/confirm-password-reset", body: "{}", want: http.StatusBadRequest},
		{name: "request-verification", method: http.MethodPost, path: "/request-verification", body: "{}", want: http.StatusBadRequest},
		{name: "confirm-verification", method: http.MethodPost, path: "/confirm-verification", body: "{}", want: http.StatusBadRequest},
		{name: "request-email-change", method: http.MethodPost, path: "/request-email-change", body: "{}", want: http.StatusUnauthorized},
		{name: "confirm-email-change", method: http.MethodPost, path: "/confirm-email-change", body: "{}", want: http.StatusBadRequest},
		{name: "impersonate", method: http.MethodPost, path: "/impersonate/" + probeRecordID, body: "{}", want: http.StatusUnauthorized},
	}
}

func TestEveryPocketBaseAuthRouteStaysReachable(t *testing.T) {
	t.Parallel()

	h := newHarness(t, bindMediKubeServe)

	for _, collection := range []string{"users", core.CollectionNameSuperusers} {
		for _, route := range collectionAuthRoutes() {
			t.Run(collection+"/"+route.name, func(t *testing.T) {
				want := route.want
				if collection == core.CollectionNameSuperusers && route.wantSuperusers != 0 {
					want = route.wantSuperusers
				}

				target := "/api/collections/" + collection + route.path
				res := h.do(t, route.method, target, "", route.body)

				assert.NotEqualf(t, http.StatusNotFound, res.Status,
					"%s %s was swallowed by the lockdown", route.method, target)
				assert.Equalf(t, want, res.Status,
					"%s %s must behave exactly as it does without the lockdown: %s", route.method, target, res.Body)
			})
		}
	}
}

// The two global OAuth2 routes (apis/record_auth.go:12-18). They carry no
// collection segment at all, so they are the control case for a discriminator
// that keys on the collection path value instead of the pattern.
func TestTheGlobalOAuth2RedirectRoutesStayReachable(t *testing.T) {
	t.Parallel()

	h := newHarness(t, bindMediKubeServe)

	for _, tc := range []struct {
		method string
		want   int
	}{
		{method: http.MethodGet, want: http.StatusTemporaryRedirect},
		{method: http.MethodPost, want: http.StatusSeeOther},
	} {
		t.Run(tc.method, func(t *testing.T) {
			res := h.do(t, tc.method, "/api/oauth2-redirect", "", "")

			assert.Equal(t, tc.want, res.Status)
		})
	}
}

// The rest of the /api/collections surface: collection administration. All of
// it is superuser-only already, so an anonymous caller gets 401 — and 401, not
// 404, is the proof that the lockdown did not claim it. If any of these turns
// into a 404 the discriminator has become a prefix match.
func TestCollectionAdministrationRoutesAreNotSweptUp(t *testing.T) {
	t.Parallel()

	h := newHarness(t, bindMediKubeServe)

	for _, req := range []request{
		{http.MethodGet, "/api/collections", ""},
		{http.MethodPost, "/api/collections", "{}"},
		{http.MethodGet, "/api/collections/users", ""},
		{http.MethodPatch, "/api/collections/users", "{}"},
		{http.MethodDelete, "/api/collections/users", ""},
		{http.MethodDelete, "/api/collections/users/truncate", ""},
		{http.MethodPut, "/api/collections/import", "{}"},
		{http.MethodGet, "/api/collections/meta/scaffolds", ""},
		{http.MethodGet, "/api/collections/meta/oauth2-providers", ""},
		{http.MethodPost, "/api/collections/meta/dry-run-view", "{}"},
	} {
		t.Run(req.method+" "+req.target, func(t *testing.T) {
			res := h.do(t, req.method, req.target, "", req.body)

			assert.Equal(t, http.StatusUnauthorized, res.Status,
				"a 404 here means the lockdown matched on the /api/collections/ prefix rather than on the route")
		})
	}
}
