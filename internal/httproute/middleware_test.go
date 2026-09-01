package httproute_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
)

// The per-route middleware seam exists for one requirement — contracts/streams.md
// registers the SSE stream with apis.SkipSuccessActivityLog() and exempt from
// the rate limiter — and both halves are invisible from outside: PocketBase's
// RouterGroup keeps its children in an unexported field and Route.excludedMiddlewares
// is unexported too. So the assertions below are behavioural: a probe middleware
// with a known id is bound on the group, and the test asserts which requests it
// ran for.

const (
	probeGroupMiddlewareID = "probeGroup"
	probeRouteMiddlewareID = "probeRoute"
)

func probe(id string, ran *bool) *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id: id,
		Func: func(e *core.RequestEvent) error {
			*ran = true

			return e.Next()
		},
	}
}

// A middleware named in Route.Middlewares runs for that route and for no other,
// and a middleware named in Route.Unbind is skipped even though it was bound on
// the router the route hangs off. Both directions matter: a seam that bound
// everything everywhere would pass the first assertion alone.
func TestARouteBindsItsOwnMiddlewareAndSkipsOneBoundOnTheRouter(t *testing.T) {
	t.Parallel()

	var groupRan, routeRan bool

	registry := httproute.Empty()

	exempt := sampleAPI()
	exempt.OpID = "exemptOperation"
	exempt.Path = "/api/v1/exempt"
	exempt.Middlewares = []*hook.Handler[*core.RequestEvent]{probe(probeRouteMiddlewareID, &routeRan)}
	exempt.Unbind = []string{probeGroupMiddlewareID}
	registry.Handle(exempt, func(e *core.RequestEvent) error { return e.NoContent(http.StatusNoContent) })

	ordinary := sampleAPI()
	registry.Handle(ordinary, func(e *core.RequestEvent) error { return e.NoContent(http.StatusNoContent) })

	se := serveEvent(t)
	se.Router.Bind(probe(probeGroupMiddlewareID, &groupRan))
	require.NoError(t, registry.Bind(se))

	mux, err := se.Router.BuildMux()
	require.NoError(t, err)

	require.Equal(t, http.StatusNoContent, get(t, mux, ordinary.Path).StatusCode)
	assert.True(t, groupRan, "the router-level middleware did not run for a route that never unbound it — the probe proves nothing")
	assert.False(t, routeRan, "a middleware declared on one route ran for another")

	groupRan, routeRan = false, false

	require.Equal(t, http.StatusNoContent, get(t, mux, exempt.Path).StatusCode)
	assert.True(t, routeRan, "the route's own middleware was declared and never bound")
	assert.False(t, groupRan, "the route unbound the router-level middleware and it ran anyway")
}

// An anonymous middleware is appended and never replaced, cannot be unbound and
// cannot be read back, so a route carrying one records an intention nothing
// verifies. Same for an empty Unbind id, which router.Route.Unbind skips in
// silence.
func TestARouteCannotDeclareAMiddlewareNothingCanName(t *testing.T) {
	t.Parallel()

	for name, mutate := range map[string]func(*httproute.Route){
		"an anonymous middleware": func(r *httproute.Route) {
			r.Middlewares = []*hook.Handler[*core.RequestEvent]{{Func: func(e *core.RequestEvent) error { return e.Next() }}}
		},
		"a nil middleware": func(r *httproute.Route) {
			r.Middlewares = []*hook.Handler[*core.RequestEvent]{nil}
		},
		"an empty unbind id": func(r *httproute.Route) {
			r.Unbind = []string{""}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			route := sampleAPI()
			mutate(&route)

			assert.Panics(t, func() {
				httproute.Empty().Handle(route, func(e *core.RequestEvent) error { return nil })
			})
		})
	}
}

// contracts/streams.md, verbatim: "Registered with Bind(apis.SkipSuccessActivityLog())"
// and "exempted from the rate limiter". Neither is observable from a response,
// so the table is what carries them and this is what reads them back.
func TestTheStreamRouteSkipsTheActivityLogAndTheRateLimiter(t *testing.T) {
	t.Parallel()

	var stream httproute.Route

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindStream {
			stream = route
		}
	}

	require.NotEmpty(t, stream.OpID, "the route table declares no stream at all")

	assert.Contains(t, stream.MiddlewareIDs(), apis.DefaultSkipSuccessActivityLogMiddlewareId,
		"%s does not skip the success activity log, which writes the full request URI into a second store", stream.OpID)
	assert.Contains(t, stream.Unbind, apis.DefaultRateLimitMiddlewareId,
		"%s is inside the rate limiter: one stream is one request that lasts an hour, and the catch-all /api/ rule counts it", stream.OpID)
}
