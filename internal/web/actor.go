package web

import (
	"context"
	"errors"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"medikube/internal/domain/access"
	"medikube/internal/domain/identity"
	"medikube/internal/obs"
)

// ActorMiddlewareID names the handler so it can be reordered by name.
const ActorMiddlewareID = "medikubeActor"

// ActorMiddlewarePriority runs the actor immediately after PocketBase's
// loadAuthToken, which is what populates e.Auth (apis/middlewares.go:42). Bound
// before it, every request would build an anonymous actor and every
// authenticated caller would be refused — silently, because an anonymous actor
// is a legitimate value.
const ActorMiddlewarePriority = apis.DefaultLoadAuthTokenMiddlewarePriority + 1

// userRoleField is the column the `users` collection carries MediKube's
// application tier in, added by
// internal/store/migrations/1756100100_users_profile.go. internal/store keeps
// its own copy unexported; a getter on a column that does not exist returns the
// zero value with nowhere to report it, which is why store.AssertMappedFields
// refuses to boot against a schema this spelling does not match.
const userRoleField = "role"

// ErrNoActor is a route wired without the actor middleware.
//
// It is an internal failure and not the caller's fault: nothing they sent could
// have caused it and nothing they change will fix it, so it is a 500 with a
// line in the log rather than a refusal in their language.
var ErrNoActor = errors.New("web: the request carries no actor, so the actor middleware was not bound on this route")

type actorKey struct{}

// Actors returns the middleware that turns e.Auth into an access.Actor and puts
// it on the request context.
//
// The actor is derived from the token and from nothing else. There is no
// ?owner= and no ?user= anywhere in the API and neither write DTO has an owner
// member (FR-032) — but a middleware that read a header would bypass both, so
// this one reads e.Auth and the correlation id and nothing at all from the
// request.
func Actors() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id:       ActorMiddlewareID,
		Priority: ActorMiddlewarePriority,
		Func: func(e *core.RequestEvent) error {
			ctx := context.WithValue(e.Request.Context(), actorKey{}, actorOf(e))
			e.Request = e.Request.WithContext(ctx)

			return e.Next()
		},
	}
}

// ActorFrom returns the actor ctx carries and whether there is one.
//
// It reports absence rather than inventing a guest. An invented guest is
// indistinguishable from a real one, so a route that lost its middleware would
// answer every request as anonymous instead of failing — and the day somebody
// writes an authorization check that reads a field rather than calling
// Authenticated(), the zero actor decides it.
func ActorFrom(ctx context.Context) (access.Actor, bool) {
	actor, ok := ctx.Value(actorKey{}).(access.Actor)

	return actor, ok
}

// ActorHandler is a handler that has been given the actor, which is every
// handler that decides anything.
type ActorHandler func(e *core.RequestEvent, actor access.Actor) error

// WithActor adapts an ActorHandler to the router's signature and fails closed.
//
// This is what makes "a handler that never sees an actor cannot be reached"
// structural rather than a convention: the actor is a parameter, so a handler
// cannot forget to look it up, and a route wired without the middleware answers
// 500 with the body never running.
func WithActor(handler ActorHandler) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		actor, present := ActorFrom(e.Request.Context())
		if !present {
			return ErrNoActor
		}

		return handler(e, actor)
	}
}

func actorOf(e *core.RequestEvent) access.Actor {
	requestID := obs.CorrelationID(e.Request.Context())

	if e.Auth == nil {
		return access.Anonymous(requestID)
	}

	actor := access.Actor{
		UserID:      e.Auth.Id,
		IsSuperuser: e.Auth.IsSuperuser(),
		RequestID:   requestID,
	}

	// A PocketBase superuser is the break-glass credential and not a MediKube
	// role at all (data-model §1, FR-040). Its record lives in another
	// collection and has no `role` column; reading one would return the empty
	// string anyway, and asking is what makes that deliberate.
	if !actor.IsSuperuser {
		actor.Role = identity.Role(e.Auth.GetString(userRoleField))
	}

	return actor
}
