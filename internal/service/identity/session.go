package identity

import (
	"context"
	"errors"
	"fmt"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/identity"
)

// SignIn resolves an address and a password to the account they belong to
// (FR-005).
//
// It returns the account and NOT a session. Minting the token is the edge's,
// through PocketBase's own auth response, because that is what fires
// OnRecordAuthRequest — the hook that writes the `login` audit row for
// MediKube's route AND for PocketBase's native one, which stays reachable
// (research D-13, D-14). A `login` row written here would leave the native path
// silently unaudited, and would look exactly like coverage.
//
// THREE REFUSALS, ONE ANSWER. An address with no account, an account with
// another password, and an account an operator has taken out of service are the
// same error value with the same wrapped sentinel. Telling the third apart —
// "your account is disabled" — tells an attacker the address is registered,
// which on a self-hosted medical instance is the disclosure FR-005 exists to
// prevent.
func (s *Service) SignIn(ctx context.Context, actor access.Actor, credentials Credentials) (identity.User, error) {
	user, err := s.authenticator.Authenticate(ctx, credentials.Email, credentials.Password)

	switch {
	case err != nil && !errors.Is(err, domain.ErrUnauthenticated):
		// A credential that could not be checked has refused nobody, and
		// answering "wrong password" on the strength of a database being down
		// would fill the trail with failures nobody attempted.
		return identity.User{}, fmt.Errorf("identity: the credential could not be checked: %w", err)

	// The account is read and not merely the error. An authenticator that
	// answered with no error and no account would otherwise be signing in
	// nobody — an empty id the edge would mint a token for.
	case err == nil && user.ID != "" && !user.IsDisabled():
		return user, nil

	default:
		return identity.User{}, s.refuse(ctx, actor, credentials.Email)
	}
}

// SignOut ends the session, and every other session the account had open
// (FR-007).
//
// It needs no stored account: the only session it can end is the caller's own,
// and an account in a state that would refuse a profile read must still be able
// to sign out of it.
//
// The rotation happens BEFORE the audit write, so a trail that cannot be
// written leaves the session ended rather than leaving it open. The unwritten
// row still reaches the caller, the log and Sentry as an error.
func (s *Service) SignOut(ctx context.Context, actor access.Actor) error {
	if err := authorize(actor); err != nil {
		return err
	}

	if err := s.authenticator.EndSessions(ctx, actor.UserID); err != nil {
		return fmt.Errorf("identity: the session was not ended: %w", err)
	}

	return s.record(ctx, actor, event{action: audit.ActionLogout, target: actor.UserID})
}

// refuse is the one refusal every failed sign-in leaves through, and the one
// place a `login_failed` row is written.
//
// The row names the account somebody aimed at and NEVER the address they typed
// (contracts/auth.md). `target_id` is the account id when the address has one
// and empty when it does not: writing the typed string would put a real
// person's address — possibly a stranger's, possibly a typo of one — into a
// two-year medical audit trail.
//
// The lookup happens on this path only. It costs the same on both branches, and
// both branches have already paid for a bcrypt comparison inside Authenticate,
// so it adds no difference between an address with an account and one without.
func (s *Service) refuse(ctx context.Context, actor access.Actor, email string) error {
	refusal := fmt.Errorf("identity: that address and password do not match an account: %w", domain.ErrUnauthenticated)

	var aimedAt string

	switch account, err := s.repository.FindByEmail(ctx, email); {
	case err == nil:
		aimedAt = account.ID
	case errors.Is(err, domain.ErrNotFound):
	default:
		return errors.Join(refusal, fmt.Errorf("identity: the refused sign-in could not be attributed: %w", err))
	}

	if err := s.record(ctx, actor, event{action: audit.ActionLoginFailed, target: aimedAt}); err != nil {
		// Joined, not returned alone. The caller still gets the refusal — a 500
		// where every other failure is a 401 would be an oracle by itself.
		return errors.Join(refusal, err)
	}

	return refusal
}
