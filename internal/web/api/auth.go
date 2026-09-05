package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/httproute"
	"medikube/internal/obs"
	"medikube/internal/service/identity"
	"medikube/internal/web"
)

// The operation ids of contracts/auth.md's nine operations. They are the keys
// the route table joins handlers on and the operationIds the Principle IX gate
// reads out of api/openapi.json, so they are constants here and matched against
// the registry by auth_test.go rather than spelled twice.
const (
	OpGetAuthConfig            = "getAuthConfig"
	OpRegister                 = "register"
	OpLogin                    = "login"
	OpRefreshSession           = "refreshSession"
	OpLogout                   = "logout"
	OpRequestPasswordReset     = "requestPasswordReset"
	OpConfirmPasswordReset     = "confirmPasswordReset"
	OpRequestEmailVerification = "requestEmailVerification"
	OpConfirmEmailVerification = "confirmEmailVerification"
)

// ErrNoAccounts is a build whose auth and account operations were wired without
// the service that decides them.
var ErrNoAccounts = errors.New("api: the auth and account operations were wired without an identity service")

// Counter reports how many records the signed-in account owns, for the
// deletion confirmation and the overview (FR-013, contracts/account.md).
//
// It takes an actor and no id, so there is no other account it could be asked
// about. NewCounter resolves it through the record family, which means the
// count passes the same authorization checkpoint every record read does rather
// than being a second, unguarded query over the same table.
type Counter func(ctx context.Context, actor access.Actor) (MeCounts, error)

// MailConfigured reports whether this instance can send outgoing mail.
//
// FR-076 makes an unconfigured instance a specified refusal rather than a
// silent failure: PocketBase's mailer falls back to a local `sendmail` binary
// the distroless image does not contain, and the native routes send through
// routine.FireAndForget, so nothing about the failure reaches the caller. The
// handlers therefore ask BEFORE the service is called — asked afterwards it
// would be 503 for a registered address and 202 for an unregistered one, which
// is FR-073's oracle wearing a different status code.
type MailConfigured func() bool

// Deps is what the auth and account handlers are wired with.
type Deps struct {
	// Accounts is the identity service. It is the concrete type rather than an
	// interface because the interface would have thirteen methods, four times
	// plan.md's segregation cap, and would be satisfied by exactly one value.
	Accounts *identity.Service

	// Sessions is the ONE way a caller ends up holding a token.
	Sessions *SessionWriter

	Counts Counter

	Mail MailConfigured

	// SelfRecord provisions the one patient FR-005 guarantees for every
	// account, right after Register creates it. Nil is tolerated so a build
	// that wires no patient stack at all (a test harness exercising only
	// auth) still boots; register skips provisioning rather than panicking.
	SelfRecord SelfRecordFunc
}

type authHandlers struct {
	deps Deps

	// me is getMe's registered path. The Location a registration answers with
	// is read back from the route table rather than composed, so the address a
	// client is sent to is by construction the address the router serves.
	me string
}

// AuthHandlers is contracts/auth.md's nine operations.
func AuthHandlers(deps Deps) (httproute.Handlers, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}

	path, err := routePath(OpGetMe)
	if err != nil {
		return nil, err
	}

	h := &authHandlers{deps: deps, me: path}

	return httproute.Handlers{
		OpGetAuthConfig:            h.config,
		OpRegister:                 web.WithActor(h.register),
		OpLogin:                    web.WithActor(h.login),
		OpRefreshSession:           web.WithActor(h.refresh),
		OpLogout:                   web.WithActor(h.logout),
		OpRequestPasswordReset:     web.WithActor(h.requestPasswordReset),
		OpConfirmPasswordReset:     web.WithActor(h.confirmPasswordReset),
		OpRequestEmailVerification: web.WithActor(h.requestVerification),
		OpConfirmEmailVerification: web.WithActor(h.confirmVerification),
	}, nil
}

func (d Deps) validate() error {
	var missing []string

	if d.Accounts == nil {
		missing = append(missing, "an identity service")
	}

	if d.Sessions == nil {
		missing = append(missing, "a session writer")
	}

	if d.Counts == nil {
		missing = append(missing, "a record counter")
	}

	if d.Mail == nil {
		missing = append(missing, "a way to tell whether this instance can send mail")
	}

	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("%w: it has no %s", ErrNoAccounts, joinPhrases(missing))
}

// config is the public description of what this instance allows.
//
// It is answered without reading anything: the registration switch is
// configuration and the password rules are the domain's published value, so
// there is nothing here that could vary with who is asking or with what the
// instance holds.
func (h *authHandlers) config(e *core.RequestEvent) error {
	return web.WriteJSON(e, http.StatusOK, NewAuthConfig(h.deps.Accounts.RegistrationOpen()))
}

// register creates the account and signs the person in (FR-001).
//
// The 201 carries Location and the session cookie. It does not carry the token:
// Session has no member for one (research D-15).
func (h *authHandlers) register(e *core.RequestEvent, actor access.Actor) error {
	// BEFORE the body is so much as parsed, and that is the point rather than
	// an economy: a closed instance that answered 422 `unknown_field` for one
	// document and 403 for another would be running its decoder — and, one
	// layer down, its validator and its uniqueness check — for anonymous
	// callers on an instance that accepts nobody. Every registration a closed
	// instance receives is answered with the same bytes.
	//
	// It is not the enforcement. The service refuses a closed registration
	// itself, before it looks at the submission, and its own test asserts that
	// directly — so deleting this line loses the uniformity and not the
	// refusal, and deleting the service's loses the refusal and not this.
	if !h.deps.Accounts.RegistrationOpen() {
		return refused(identity.ErrRegistrationClosed)
	}

	var body RegisterRequest
	if err := web.Decode(e, &body); err != nil {
		return err
	}

	user, err := h.deps.Accounts.Register(e.Request.Context(), actor, identity.Registration{
		Email:    body.Email,
		Name:     body.Name,
		Password: body.Password,
	})
	if err != nil {
		return refused(err)
	}

	if h.deps.SelfRecord != nil {
		if _, selfRecordErr := h.deps.SelfRecord(e.Request.Context(), user.ID, body.Name); selfRecordErr != nil {
			return selfRecordErr
		}
	}

	me, err := h.render(e, actor, user)
	if err != nil {
		return err
	}

	intent := signedIn(http.StatusCreated, me)
	intent.location = h.me

	return h.deps.Sessions.Issue(e, user.ID, core.MFAMethodPassword, intent)
}

// login exchanges an address and a password for a session (FR-005).
//
// Everything that makes the three refusals one refusal is BELOW this handler:
// the identical error value, the identical message and the bcrypt comparison an
// address with no account still pays for are all the service's and the
// authenticator's. What this handler must not do is add a distinction of its
// own, which is why every failure leaves through one return.
func (h *authHandlers) login(e *core.RequestEvent, actor access.Actor) error {
	var body LoginRequest
	if err := web.Decode(e, &body); err != nil {
		return err
	}

	user, err := h.deps.Accounts.SignIn(e.Request.Context(), actor, identity.Credentials{
		Email:    body.Email,
		Password: body.Password,
	})
	if err != nil {
		return refused(err)
	}

	me, err := h.render(e, actor, user)
	if err != nil {
		return err
	}

	return h.deps.Sessions.Issue(e, user.ID, core.MFAMethodPassword, signedIn(http.StatusOK, me))
}

// refresh exchanges the current token for a fresh one (FR-008).
//
// The authMethod is the EMPTY STRING and that is load-bearing: the sign-in
// audit row is discriminated on core.MFAMethodPassword, so a renewal that
// announced itself as a password sign-in would write a second `login` row every
// time a browser extended its session (research D-14).
//
// It goes through Me first, so an account an operator has taken out of service
// cannot renew its way past the disabling: PocketBase's token validation
// evaluates no collection rule and never looks at `disabled_at`.
func (h *authHandlers) refresh(e *core.RequestEvent, actor access.Actor) error {
	user, err := h.deps.Accounts.Me(e.Request.Context(), actor)
	if err != nil {
		return refused(err)
	}

	me, err := h.render(e, actor, user)
	if err != nil {
		return err
	}

	return h.deps.Sessions.Issue(e, user.ID, "", signedIn(http.StatusOK, me))
}

// logout ends the session, and every session the account still had open
// (FR-007).
//
// The cookie is cleared after the rotation and never instead of it. A clear on
// its own hands the person a browser that has forgotten a credential that still
// works everywhere else.
func (h *authHandlers) logout(e *core.RequestEvent, actor access.Actor) error {
	if err := h.deps.Accounts.SignOut(e.Request.Context(), actor); err != nil {
		return refused(err)
	}

	h.deps.Sessions.Clear(e)
	e.Response.Header().Set("Cache-Control", accountCacheControl)

	return e.NoContent(http.StatusNoContent)
}

// requestPasswordReset asks for a recovery message for one address (FR-073).
//
// ONE EXIT, AND THE ANSWER IS BUILT BEFORE THE ACCOUNT IS CONSULTED. There is
// no early return on the branch only an unregistered address reaches, and no
// branch on the way out: the acknowledgement the service hands back is the same
// value for every address, and the same constructor writes it either way.
//
// The service's error is LOGGED AND NOT ANSWERED. It names a mail or store
// failure the operator needs; surfacing it would say that the address has an
// account, since an address without one has nothing to fail at.
func (h *authHandlers) requestPasswordReset(e *core.RequestEvent, actor access.Actor) error {
	var body PasswordResetRequest
	if err := web.Decode(e, &body); err != nil {
		return err
	}

	if err := h.mailable(); err != nil {
		return err
	}

	acknowledgement, failure := h.deps.Accounts.RequestPasswordReset(e.Request.Context(), actor, body.Email)
	obs.Report(e, failure)

	return web.WriteJSON(e, http.StatusAccepted, Acknowledgement{Status: acknowledgement.Status})
}

// confirmPasswordReset sets a new password from a recovery link (FR-074).
//
// The two typed passwords are compared before the token is spent, because a
// mismatch is the caller's own typing and costs nothing to answer; the token is
// then still good for the second attempt.
func (h *authHandlers) confirmPasswordReset(e *core.RequestEvent, actor access.Actor) error {
	var body PasswordResetConfirm
	if err := web.Decode(e, &body); err != nil {
		return err
	}

	if body.Password != body.PasswordConfirm {
		var invalid domain.ValidationError
		invalid.Add(MemberPasswordConfirm, identity.CodeMismatch, "the two passwords are not the same")

		return invalid.OrNil()
	}

	if err := h.deps.Accounts.ConfirmPasswordReset(e.Request.Context(), actor, body.Token, body.Password); err != nil {
		return refused(err)
	}

	return e.NoContent(http.StatusNoContent)
}

// requestVerification sends the confirmation message again, to the signed-in
// account's own address (FR-075).
//
// It reads no body at all. An address parameter would let any signed-in caller
// aim this instance's mailer at a stranger, so there is no shape for one to
// arrive in.
func (h *authHandlers) requestVerification(e *core.RequestEvent, actor access.Actor) error {
	if err := h.mailable(); err != nil {
		return err
	}

	if err := h.deps.Accounts.RequestVerification(e.Request.Context(), actor); err != nil {
		return refused(err)
	}

	return web.WriteJSON(e, http.StatusAccepted, Acknowledgement{Status: VerificationSent})
}

// confirmVerification marks an address confirmed from the link sent to it
// (FR-075). Public: the person following the link may not be signed in on that
// device.
func (h *authHandlers) confirmVerification(e *core.RequestEvent, actor access.Actor) error {
	var body EmailVerificationConfirm
	if err := web.Decode(e, &body); err != nil {
		return err
	}

	if err := h.deps.Accounts.ConfirmVerification(e.Request.Context(), actor, body.Token); err != nil {
		return refused(err)
	}

	return e.NoContent(http.StatusNoContent)
}

// mailable refuses an instance that cannot send mail (FR-076).
//
// Refused HERE and not in the service, and before the address is looked at: an
// instance with no outgoing mail cannot succeed, and FR-076 forbids accepting
// the request as though it had.
func (h *authHandlers) mailable() error {
	if h.deps.Mail() {
		return nil
	}

	return fmt.Errorf("api: this instance has no outgoing mail configured: %w", web.ErrMailUnconfigured)
}

// render builds the account body a session answers with.
//
// The counts are read through the record family, with the account's OWN actor:
// for a sign-in that is the caller, and for a registration it is the account
// that has only just come into existence — which the request could not have
// carried, because it was made by somebody with no session at all.
func (h *authHandlers) render(e *core.RequestEvent, actor access.Actor, user domainidentity.User) (Me, error) {
	counts, err := h.deps.Counts(e.Request.Context(), access.Actor{
		UserID:    user.ID,
		Role:      user.Role,
		RequestID: actor.RequestID,
	})
	if err != nil {
		return Me{}, err
	}

	return NewMe(user, counts), nil
}

// refused maps the identity service's two own conditions onto the published
// vocabulary.
//
// Everything else is already a domain sentinel and is left alone. Both of these
// are mapped rather than left to fall through because the fall-through is
// wrong in a different way for each: ErrRegistrationClosed wraps
// domain.ErrForbidden and would answer `forbidden` instead of
// `registration_closed`, and ErrInvalidToken wraps no sentinel at all and would
// answer 500 — deliberately loud, so a forgotten mapping is discovered rather
// than served.
func refused(err error) error {
	switch {
	case errors.Is(err, identity.ErrRegistrationClosed):
		return fmt.Errorf("%w: %w", web.ErrRegistrationClosed, err)
	case errors.Is(err, identity.ErrInvalidToken):
		return fmt.Errorf("%w: %w", web.ErrInvalidToken, err)
	default:
		return err
	}
}

// routePath recovers one operation's registered path from the inventory, so an
// address MediKube hands a client is by construction the one the router serves.
func routePath(opID string) (string, error) {
	for _, route := range httproute.Inventory().Routes() {
		if route.OpID == opID {
			return route.Path, nil
		}
	}

	return "", fmt.Errorf("api: %s is not in the route table, so nothing can be addressed to it", opID)
}

func joinPhrases(words []string) string {
	switch len(words) {
	case 1:
		return words[0]
	case 2:
		return words[0] + " and no " + words[1]
	default:
		return words[0] + ", no " + joinPhrases(words[1:])
	}
}
