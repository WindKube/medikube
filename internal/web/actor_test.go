package web

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	"medikube/internal/domain/identity"
	"medikube/internal/obs"
	"medikube/internal/testsupport"
)

// e.Auth is PocketBase's own auth record and is nil for a guest. The actor is
// built from it once per request and is the only thing derived from the token.
func TestTheAuthRecordBecomesAnActorInTheRequestContext(t *testing.T) {
	t.Parallel()

	app := testsupport.NewApp(t)
	token := testsupport.UserToken(t, app, testsupport.AccountAEmail)

	var seen access.Actor

	scenario := tests.ApiScenario{
		Name:           "a signed-in caller",
		Method:         http.MethodGet,
		URL:            "/x/who",
		Headers:        map[string]string{"Authorization": token},
		ExpectedStatus: http.StatusNoContent,
		TestAppFactory: testsupport.NewAppFactory(
			middleware(obs.RequestLogger(discardLogger()), Actors()),
			route(http.MethodGet, "/x/who", WithActor(func(e *core.RequestEvent, actor access.Actor) error {
				seen = actor

				return e.NoContent(http.StatusNoContent)
			})),
		),
	}
	scenario.Test(t)

	assert.Equal(t, testsupport.AccountAID, seen.UserID)
	assert.Equal(t, identity.RoleUser, seen.Role)
	assert.False(t, seen.IsSuperuser)
	assert.NotEmpty(t, seen.RequestID, "the actor carries no correlation id, so its audit row cannot be joined to the log")
	assert.True(t, seen.Authenticated())
}

// An unauthenticated request still has an actor, so that its refusal can be
// audited and correlated. It is a value and never a nil pointer, so no call
// site has to nil-check before authorizing.
func TestAGuestGetsTheAnonymousActorAndNotNothing(t *testing.T) {
	t.Parallel()

	var seen access.Actor
	var present bool

	scenario := tests.ApiScenario{
		Name:           "no credentials at all",
		Method:         http.MethodGet,
		URL:            "/x/who",
		ExpectedStatus: http.StatusNoContent,
		TestAppFactory: testsupport.NewAppFactory(
			middleware(obs.RequestLogger(discardLogger()), Actors()),
			route(http.MethodGet, "/x/who", func(e *core.RequestEvent) error {
				seen, present = ActorFrom(e.Request.Context())

				return e.NoContent(http.StatusNoContent)
			}),
		),
	}
	scenario.Test(t)

	assert.True(t, present, "a guest reached a handler with no actor at all")
	assert.False(t, seen.Authenticated())
	assert.Empty(t, seen.UserID)
	assert.NotEmpty(t, seen.RequestID)
}

// The structural half of T119: a handler that never sees an actor cannot be
// reached. WithActor fails closed, so a route wired without the middleware
// answers 500 and the handler body never runs — rather than running with the
// zero actor, which authorizes as nobody and is indistinguishable from a guest
// until the day somebody writes a check that reads a field instead of calling
// Authenticated().
func TestAHandlerWiredWithoutTheMiddlewareIsNotReached(t *testing.T) {
	t.Parallel()

	reached := false

	scenario := tests.ApiScenario{
		Name:            "the middleware was never bound",
		Method:          http.MethodGet,
		URL:             "/x/who",
		ExpectedStatus:  http.StatusInternalServerError,
		ExpectedContent: []string{`"code":"internal_error"`},
		TestAppFactory: testsupport.NewAppFactory(
			middleware(obs.RequestLogger(discardLogger()), Errors(nil)),
			route(http.MethodGet, "/x/who", WithActor(func(e *core.RequestEvent, _ access.Actor) error {
				reached = true

				return e.NoContent(http.StatusNoContent)
			})),
		),
	}
	scenario.Test(t)

	assert.False(t, reached, "the handler ran with no actor, so its authorization decided on the zero value")
}

func TestTheWiringMistakeIsAnInternalFailureAndNotTheCallersFault(t *testing.T) {
	t.Parallel()

	e, _ := event(t, http.MethodGet, "/x")

	err := WithActor(func(*core.RequestEvent, access.Actor) error { return nil })(e)
	require.Error(t, err)

	status, code := Classify(err)
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, CodeInternal, code)
}

func TestActorFromReportsAbsenceRatherThanInventingAGuest(t *testing.T) {
	t.Parallel()

	_, present := ActorFrom(t.Context())
	assert.False(t, present,
		"a context with no actor answered with one, so the fail-closed guard above can never fire")
}

// A superuser is the break-glass credential and not a MediKube role at all. The
// flag and the role are different fields because FR-040 turns on them staying
// different.
func TestASuperuserIsFlaggedAndCarriesNoMediKubeRole(t *testing.T) {
	t.Parallel()

	app := testsupport.NewApp(t)
	token := testsupport.AuthToken(t, app, core.CollectionNameSuperusers, testsupport.SuperuserEmail)

	var seen access.Actor

	scenario := tests.ApiScenario{
		Name:           "a superuser",
		Method:         http.MethodGet,
		URL:            "/x/who",
		Headers:        map[string]string{"Authorization": token},
		ExpectedStatus: http.StatusNoContent,
		TestAppFactory: testsupport.NewAppFactory(
			middleware(obs.RequestLogger(discardLogger()), Actors()),
			route(http.MethodGet, "/x/who", WithActor(func(e *core.RequestEvent, actor access.Actor) error {
				seen = actor

				return e.NoContent(http.StatusNoContent)
			})),
		),
	}
	scenario.Test(t)

	assert.Equal(t, testsupport.SuperuserID, seen.UserID)
	assert.True(t, seen.IsSuperuser)
	assert.Empty(t, seen.Role, "a PocketBase superuser was given a MediKube application role")
}

// The actor is derived from the token and from nothing else. There is no
// ?owner= and no ?user= anywhere in the API, and a body member that named one
// would be refused by the decoder — but a middleware that read a header would
// bypass both.
func TestNothingTheCallerSuppliesReachesTheActor(t *testing.T) {
	t.Parallel()

	var seen access.Actor

	scenario := tests.ApiScenario{
		Name:   "a caller claiming to be somebody else",
		Method: http.MethodGet,
		URL:    "/x/who?owner=" + testsupport.AccountBID + "&user=" + testsupport.AccountBID,
		Headers: map[string]string{
			"X-User-Id":       testsupport.AccountBID,
			"X-Medikube-Role": string(identity.RoleAdmin),
		},
		ExpectedStatus: http.StatusNoContent,
		TestAppFactory: testsupport.NewAppFactory(
			middleware(obs.RequestLogger(discardLogger()), Actors()),
			route(http.MethodGet, "/x/who", WithActor(func(e *core.RequestEvent, actor access.Actor) error {
				seen = actor

				return e.NoContent(http.StatusNoContent)
			})),
		),
	}
	scenario.Test(t)

	assert.Empty(t, seen.UserID, "a client-supplied identifier became the actor")
	assert.Empty(t, seen.Role)
	assert.False(t, seen.IsSuperuser)
}

// The middleware must run after PocketBase's loadAuthToken, which is what
// populates e.Auth. Bound before it, every request would build an anonymous
// actor and every authenticated caller would be refused.
func TestTheActorMiddlewareRunsAfterTheTokenIsLoaded(t *testing.T) {
	t.Parallel()

	assert.Greater(t, ActorMiddlewarePriority, apis.DefaultLoadAuthTokenMiddlewarePriority,
		"the actor is built before e.Auth is populated, so every caller is anonymous")
	assert.Less(t, ActorMiddlewarePriority, apis.DefaultSecurityHeadersMiddlewarePriority,
		"the actor is built after the lockdown's window, which needs e.Auth for its superuser carve-out")
}
