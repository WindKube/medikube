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

// RequestPasswordReset asks for a recovery message for one address (FR-073).
//
// THE ANSWER IS BUILT BEFORE THE ACCOUNT IS LOOKED UP, and there is exactly one
// return statement in this method. That is the enumeration defence expressed as
// a shape rather than as a rule: the acknowledgement is already in hand when
// the branch runs, so there is nothing the branch could vary, and a later edit
// that wanted to answer "no such account" would have to add a second exit —
// which recovery_test.go reads out of the source and fails on.
//
// The returned error is NOT the caller's answer. It names a mail failure the
// operator needs in the log, and the acknowledgement beside it is still the
// answer to send: reporting the failure to the caller would say that the
// address has an account, since an address without one has nothing to fail at.
// The edge answers 202 with the acknowledgement and logs the error.
//
// An instance that cannot send mail at all is refused BEFORE this is reached,
// at the edge (FR-076, T223o) — for the same reason. Refused here it would be
// a 503 for a registered address and a 202 for an unregistered one, which is
// the oracle FR-073 closes wearing a different status code.
func (s *Service) RequestPasswordReset(
	ctx context.Context,
	_ access.Actor,
	email string,
) (identity.Acknowledgement, error) {
	acknowledgement := identity.AcknowledgeRecovery()

	var failure error

	switch account, err := s.repository.FindByEmail(ctx, email); {
	case err == nil:
		failure = s.mailer.SendPasswordReset(ctx, account.ID)
	case errors.Is(err, domain.ErrNotFound):
		// Nothing to send and nothing to say. A request writes no audit row
		// either: there is nothing yet to record about an account that may not
		// exist, and the only thing that could be recorded is the typed address
		// (contracts/auth.md).
	default:
		failure = fmt.Errorf("identity: the address could not be looked up: %w", err)
	}

	return acknowledgement, failure
}

// ConfirmPasswordReset sets a new password from a recovery link (FR-074).
//
// It writes the SAME `password_change` row a deliberate change writes. No new
// action value is introduced, so the vocabulary counts data-model §3 fixes and
// every later phase asserts are unchanged.
//
// The caller is not signed in by this: the password is proven again at
// /login, which is one more place it is shown to work.
func (s *Service) ConfirmPasswordReset(ctx context.Context, actor access.Actor, token, password string) error {
	user, err := s.redeem(ctx, TokenPasswordReset, token)
	if err != nil {
		return err
	}

	// The published rules, the same ones registration enforces (FR-004,
	// FR-074), and checked BEFORE the token is spent — a password the rules
	// refuse must leave the link usable, or one typo costs another round trip
	// through a mailbox.
	if err := identity.ValidatePassword(password, user.Email, user.Name); err != nil {
		return err
	}

	if err := s.authenticator.SetPassword(ctx, user.ID, password); err != nil {
		return fmt.Errorf("identity: the new password was not stored: %w", err)
	}

	return s.record(ctx, actor, event{
		action:  audit.ActionPasswordChange,
		actorID: user.ID,
		target:  user.ID,
	})
}

// RequestVerification sends the address-confirmation message again, to the
// signed-in account's own address (FR-075).
//
// It takes no address. The one it uses is on the record, because accepting one
// would let any signed-in caller aim MediKube's mailer at a stranger.
//
// An address that is already confirmed is answered exactly as one that is not,
// and no message is sent: there is nothing to disclose — the caller owns the
// account and can see the state on their own settings page — and nothing to
// fail at.
func (s *Service) RequestVerification(ctx context.Context, actor access.Actor) error {
	user, err := s.account(ctx, actor)
	if err != nil {
		return err
	}

	if user.EmailConfirmed {
		return nil
	}

	return s.mailer.SendVerification(ctx, user.ID)
}

// ConfirmVerification marks an address confirmed from the link sent to it
// (FR-075). Public: the person following the link may not be signed in on that
// device.
//
// THE SECOND USE IS REFUSED HERE AND NOWHERE ELSE. PocketBase does not
// invalidate a verification token when it is used — SetVerified rotates no
// token key, so a spent link stays resolvable for its full 24 hours (measured
// against v0.40.1: two consecutive confirmations both succeed). T223d requires
// a second use to be answered exactly as an expired one, so the already-
// confirmed account IS the used token, and this is the check that makes it so.
// Delete it and a recovered link works for a day.
//
// It writes `update` / `user` and not `email_change`, however obviously named
// that constant is: `email_change` is written by no phase in 001–006
// (data-model §3), and reaching for it would break the vocabulary counts later
// phases assert.
func (s *Service) ConfirmVerification(ctx context.Context, actor access.Actor, token string) error {
	user, err := s.redeem(ctx, TokenEmailConfirmation, token)
	if err != nil {
		return err
	}

	if user.EmailConfirmed {
		return ErrInvalidToken
	}

	user.EmailConfirmed = true

	updated, err := s.repository.Update(ctx, user)
	if err != nil {
		return err
	}

	return s.record(ctx, actor, event{
		action:  audit.ActionUpdate,
		actorID: updated.ID,
		target:  updated.ID,
	})
}

// redeem resolves a link's token, and is the one place the three refusals
// become one.
//
// A token that could not be checked — a database that did not answer — is NOT
// one of them. Folding an outage into `invalid_token` would tell somebody their
// link had expired on the strength of a failed read, and would hide the outage
// behind a 400 nobody investigates.
func (s *Service) redeem(ctx context.Context, purpose TokenPurpose, token string) (identity.User, error) {
	user, err := s.authenticator.Redeem(ctx, purpose, token)

	switch {
	case err == nil && user.ID != "":
		return user, nil
	case err == nil, errors.Is(err, ErrInvalidToken):
		// An authenticator that answered with no error and no account has
		// resolved nothing, and treating that as a grant would set a stranger's
		// password.
		return identity.User{}, ErrInvalidToken
	default:
		return identity.User{}, fmt.Errorf("identity: the link could not be checked: %w", err)
	}
}
