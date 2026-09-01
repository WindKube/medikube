package stream

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// ErrSessionEnded is the session a stream was opened with, no longer usable:
// signed out, revoked by a password or email change, expired, or an account
// that is gone.
//
// It is the specified end of a stream rather than a failure to report. The loop
// closes the connection, the browser reconnects, and the reconnect is refused
// by the route's own authentication — which is what an ended session is
// supposed to look like from every direction (FR-007).
var ErrSessionEnded = errors.New("stream: the session this stream was opened with has ended")

// ErrNoSession is a stream that arrived carrying nothing re-checkable.
//
// It is an internal failure and never the caller's: the route declares AuthUser
// and PocketBase's loadAuthToken populates e.Auth from the Authorization header
// and from nothing else, so a request that authenticated and yet carries no
// token means the two have been wired apart. Refusing to open is the only safe
// answer — a stream that cannot re-check its identity is precisely the hole
// this file closes.
var ErrNoSession = errors.New("stream: the request carries no token to re-check, so the stream could not notice its session ending")

// Sessions turns the request a stream arrived on into something the subscriber
// loop can keep re-checking.
//
// It is a port so the in-package harness can revoke a session without a
// database. The production implementation is the zero value of this package's
// own type; a Deps that names none gets it.
type Sessions interface {
	Open(e *core.RequestEvent) (Session, error)
}

// Session is one signed-in identity, re-read for as long as the stream is open.
//
// The record checkpoint asks whether this actor may see this record. Nothing
// asked whether the actor is still signed in, and nothing could: web.WithActor
// builds the actor once, from the auth record, at subscribe. An ordinary
// request re-derives its actor from the token on every single call; a stream
// derived one once and then ran for an hour on it. FR-007 says an ended session
// "MUST NOT be usable again from anywhere it was still open", and an open
// stream is exactly somewhere it was still open — a token stolen for ten
// seconds bought an indefinite live feed of the victim's records, and changing
// the password did not close it.
type Session interface {
	// Live returns nil while the session is still usable and an error the
	// moment it is not.
	//
	// Every error ends the stream. ErrSessionEnded says the session ended,
	// which is routine; anything else says the check could not be made, which
	// a connection carrying a person's records must also stop for.
	Live(ctx context.Context) error
}

// tokenSessions re-validates the token the stream arrived on.
//
// The test it applies is deliberately the identical one PocketBase's
// loadAuthToken applies to every ordinary request (apis/middlewares.go:199):
// would a request bearing this token still be authenticated? That framing is
// what makes the answer unambiguous. Signature, expiry, the record's own token
// key — which PocketBase re-randomises on a password or email change
// (core/record_model.go:1449) — and the collection's auth secret are all inside
// it, so revocation, sign-out and expiry are one check rather than three, and
// the stream cannot end up with a more generous notion of "signed in" than the
// rest of the application has.
//
// Expiry being part of it is also the maximum stream lifetime. It is the
// configured session TTL (internal/platform/pb's applySessionTTL) rather than a
// second number invented here, noticed at worst one heartbeat late.
type tokenSessions struct{}

func (tokenSessions) Open(e *core.RequestEvent) (Session, error) {
	token := bearer(e.Request.Header.Get("Authorization"))
	if token == "" {
		return nil, ErrNoSession
	}

	if e.App == nil {
		return nil, fmt.Errorf("%w: there is no application to re-check it against", ErrNoSession)
	}

	return &tokenSession{app: e.App, token: token}, nil
}

type tokenSession struct {
	app   core.App
	token string
}

func (s *tokenSession) Live(context.Context) error {
	if _, err := s.app.FindAuthRecordByToken(s.token, core.TokenTypeAuth); err != nil {
		return fmt.Errorf("%w: %w", ErrSessionEnded, err)
	}

	return nil
}

// bearer reads the token out of an Authorization header exactly as PocketBase
// does, prefix optional (apis/middlewares.go:211). Reading it differently would
// leave a token the router accepted and this could not re-check.
func bearer(header string) string {
	if len(header) > 7 && strings.EqualFold(header[:7], "Bearer ") {
		return header[7:]
	}

	return header
}
