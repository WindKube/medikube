package identity

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	domainidentity "medikube/internal/domain/identity"
	service "medikube/internal/service/identity"
	"medikube/internal/store"
)

// DummyPasswordHash is the credential no account has, and the whole of research
// D-17 in this package.
//
// An address with no account is refused only after a bcrypt comparison against
// this has been paid for, so that "no such account" and "wrong password" cost
// comparable time. Measured on this codebase against v0.40.1, the naive shape —
// look up, miss, return — answers an unknown address in 206µs and a wrong
// password in 69.8ms: a 339× difference behind two byte-identical response
// bodies, which is an account-existence oracle that survives every assertion
// about the body.
//
// It is a FIXED hash and not a sampled row, which is where it departs from
// PocketBase's own dummyPasswordCheck (apis/record_auth_with_password.go:125).
// That one picks any existing record and RETURNS WITHOUT COMPARING when the
// query finds none, so an instance with no accounts — a fresh one, the moment
// registration matters most — has the full oracle back. It is also unexported,
// so it could not be reused in any case. A constant has no branch to take.
//
// Its work factor must equal the collection's, or the two comparisons stop
// costing the same and the equalisation quietly becomes decorative; the
// repository's integration test asserts they agree, because nothing else would
// notice.
//
//nolint:gosec // a bcrypt hash of a value no account can hold, which is the point
const DummyPasswordHash = "$2a$10$GTMhxTt4lWq/6htfbSDeDeJg2nwwwyw2So8MYcvHo9420khZcE6Ra"

// dummyPassword is the plain value DummyPasswordHash was generated from, kept
// beside it so the pair can be ASSERTED rather than assumed.
//
// A dummy comparison that has quietly stopped comparing anything still costs
// one call and one count, so a test that counts calls cannot tell it from one
// that works — that is the difference between "the branch exists" and "the
// branch does the work", and only a comparison asked to SUCCEED separates them.
// auth_internal_test.go is where that happens.
//
// It is not a secret. There is no account behind the hash, and every caller
// discards the result, so knowing the value buys nothing.
//
//nolint:gosec // the plaintext of a hash no account has
const dummyPassword = "\x00medikube: the credential no account has"

// Authenticator is the credential seam against PocketBase: hashing, comparison,
// token minting and token-key rotation are all PocketBase's, and this is the
// whole of what MediKube asks of them (research D-13, D-16).
//
// No method returns a token. The session a caller ends up holding is minted at
// the edge, from the saved record, through apis.RecordAuthResponse — which is
// what fires OnRecordAuthRequest and therefore what gets a sign-in audited
// through PocketBase's native route as well as MediKube's (research D-14).
type Authenticator struct {
	app core.App

	// The counting seam T202 asserts through. Atomics rather than a mutex
	// because they are read from tests while requests are in flight and are
	// never read in the same breath as a decision.
	comparisons atomic.Int64
	dummies     atomic.Int64
}

var _ service.Authenticator = (*Authenticator)(nil)

func NewAuthenticator(app core.App) (*Authenticator, error) {
	if app == nil {
		return nil, errors.New("identity: the authenticator is wired with no application")
	}

	return &Authenticator{app: app}, nil
}

// Comparisons is how many times a supplied password has been compared against a
// stored hash, the dummy included.
//
// It exists so that a test can assert the anti-enumeration mechanism rather
// than time it: every refusal costs exactly one comparison, and a count is
// deterministic where a latency is not (T202, Constitution VIII).
func (a *Authenticator) Comparisons() int { return int(a.comparisons.Load()) }

// DummyComparisons is how many of those were against DummyPasswordHash — the
// comparison an address with no account still pays for.
func (a *Authenticator) DummyComparisons() int { return int(a.dummies.Load()) }

// Forget resets both counters, so a test can seed an account and still count
// only what the call under test did.
func (a *Authenticator) Forget() {
	a.comparisons.Store(0)
	a.dummies.Store(0)
}

// Authenticate resolves an address and a password to the account they belong
// to (FR-005).
//
// THE TWO REFUSALS ARE ONE REFUSAL, in three respects that each have their own
// guard: the same error value, the same error text — this string reaches the
// one log stream, and two spellings would tell an operator's log which
// addresses are registered — and comparable time, through the dummy comparison
// above.
//
// A credential that could NOT be checked is none of them. It returns the
// failure, unwrapped by domain.ErrUnauthenticated, so that a database outage
// cannot be read as a wrong password; the service refuses to sign anybody in on
// it either way.
//
// A disabled account authenticates here. Whether it may be used is policy, and
// policy is the service's.
func (a *Authenticator) Authenticate(
	ctx context.Context,
	email, password string,
) (domainidentity.User, error) {
	record, err := byEmail(ctx, a.app, email)

	switch {
	case err == nil:
	case errors.Is(err, domain.ErrNotFound):
		// The equalisation, and the one line research D-17 turns on.
		a.compareDummy(password)

		return domainidentity.User{}, refusedCredential()
	default:
		return domainidentity.User{}, fmt.Errorf("identity: the credential could not be checked: %w", err)
	}

	if !a.compare(storedHash(record), password) {
		return domainidentity.User{}, refusedCredential()
	}

	return store.UserFromRecord(record)
}

// Verify checks one known account's own password, for the operations that ask a
// signed-in person to prove it is still them (FR-009, FR-013).
//
// An id with no account pays for the same comparison and is answered by the
// same refusal. The caller is already authenticated, so there is no address to
// enumerate — but the refusal still reaches the log stream, and one shape of
// refusal is one fewer thing to keep in step.
func (a *Authenticator) Verify(ctx context.Context, userID, password string) error {
	record, err := byID(ctx, a.app, userID)

	switch {
	case err == nil:
	case errors.Is(err, domain.ErrNotFound):
		a.compareDummy(password)

		return refusedCredential()
	default:
		return fmt.Errorf("identity: the credential could not be checked: %w", err)
	}

	if !a.compare(storedHash(record), password) {
		return refusedCredential()
	}

	return nil
}

// SetPassword replaces the credential and, in the same write, ends every
// session issued before it (FR-010, research D-16).
//
// THE ROTATION IS NOT PERFORMED HERE, AND THAT IS DELIBERATE. PocketBase's
// onRecordSaveExecute re-randomises `tokenKey` when the saved record's password
// changed and its key did not (core/record_model.go:1448), which is one write
// rather than two and therefore cannot half-happen. A second rotation on this
// side would not make it safer — it would make a PocketBase regression
// invisible, because the guard would be passing on MediKube's own line.
//
// What this method must not do is defeat it: writing the hash through a raw
// UPDATE, or through any path that does not reach onRecordSaveExecute, leaves
// every outstanding token valid and nothing downstream says so. That is what
// the repository's integration test deletes to watch the suite go red.
func (a *Authenticator) SetPassword(ctx context.Context, userID, password string) error {
	record, err := byID(ctx, a.app, userID)
	if err != nil {
		return err
	}

	record.SetPassword(password)

	if saveErr := a.app.SaveWithContext(ctx, record); saveErr != nil {
		return writeFailure("changing a password", saveErr)
	}

	return nil
}

// EndSessions rotates the token key without touching the password, which is
// what makes signing out on a phone sign out the laptop (FR-007).
//
// This rotation IS MediKube's, and it has to be: nothing about a sign-out
// changes the password, so PocketBase's automatic refresh never fires. The
// credential must survive it — a sign-out that reset somebody's password would
// lock them out of their own medical records.
//
// RefreshTokenKey sets the field's autogenerate modifier, whose setter runs
// synchronously (core/collection_model.go:1005), so the new key is on the
// record before the save and the save is the only statement.
func (a *Authenticator) EndSessions(ctx context.Context, userID string) error {
	record, err := byID(ctx, a.app, userID)
	if err != nil {
		return err
	}

	record.RefreshTokenKey()

	if saveErr := a.app.SaveWithContext(ctx, record); saveErr != nil {
		return writeFailure("ending the sessions of an account", saveErr)
	}

	return nil
}

// linkTokens maps MediKube's two published purposes onto PocketBase's token
// types. A purpose with no type is not a link this instance issues, and is
// refused exactly as a forged one.
var linkTokens = map[service.TokenPurpose]string{
	service.TokenPasswordReset:     core.TokenTypePasswordReset,
	service.TokenEmailConfirmation: core.TokenTypeVerification,
}

// Redeem resolves a link's token to the account it was minted for.
//
// Expired, spent, minted for another purpose and never minted at all are ONE
// refusal with one message: telling them apart tells somebody which links once
// existed. "Spent" is not stored anywhere — a reset token is signed with the
// record's own key, so the same write that ended the sessions is what moved the
// key out from under it.
//
// A token that could NOT be checked is not one of them, and separating the two
// is why the failure path asks a second question. FindAuthRecordByToken reports
// a malformed token, a wrong signature, an expired one and a database that did
// not answer with errors nothing can tell apart; so when it fails, this asks
// the database whether it is answering at all. Folding an outage into
// ErrInvalidToken would tell somebody their link had expired on the strength of
// a failed read, and would hide the outage behind a 400 nobody investigates —
// the same distinction internal/store's owner lookup makes, for the same
// reason.
func (a *Authenticator) Redeem(
	ctx context.Context,
	purpose service.TokenPurpose,
	token string,
) (domainidentity.User, error) {
	if err := ctx.Err(); err != nil {
		return domainidentity.User{}, fmt.Errorf("identity: the link could not be checked: %w", err)
	}

	tokenType, published := linkTokens[purpose]
	if !published {
		return domainidentity.User{}, service.ErrInvalidToken
	}

	record, err := a.app.FindAuthRecordByToken(token, tokenType)
	if err != nil {
		if reachErr := a.reachable(ctx); reachErr != nil {
			return domainidentity.User{}, fmt.Errorf("identity: the link could not be checked: %w", reachErr)
		}

		return domainidentity.User{}, service.ErrInvalidToken
	}

	// A token's claims name the collection it was minted against, and
	// FindAuthRecordByToken resolves it there — so a _superusers recovery link
	// resolves perfectly well. It is not an account's link. Answering it with
	// the account mapper's "not from the collection this mapper reads" would be
	// a 500; it is a link that cannot be used, and it reads exactly like every
	// other one.
	if collection := record.Collection(); collection == nil || collection.Name != store.AccountCollection {
		return domainidentity.User{}, service.ErrInvalidToken
	}

	return store.UserFromRecord(record)
}

// reachable asks whether the database is answering, and is reached only when a
// link has already been refused. It costs one counting query on a path that is
// already a failure, and nothing at all on the ordinary one.
func (a *Authenticator) reachable(ctx context.Context) error {
	collection, err := accounts(a.app)
	if err != nil {
		return err
	}

	var accountCount int

	if rowErr := a.app.RecordQuery(collection).
		Select("count(*)").
		WithContext(ctx).
		Row(&accountCount); rowErr != nil {
		return fmt.Errorf("identity: the %s collection could not be read: %w", store.AccountCollection, rowErr)
	}

	return nil
}

// compare is the seam, and every refusal path in this file goes through it
// exactly once.
//
// IT HAS ONE PATH, AND THAT IS THE POINT. There is no dummy parameter here to
// branch on, because a branch is somewhere a later edit can put a `return
// false` — which would leave the call counted, the count asserted and the
// bcrypt gone, and the enumeration oracle back with every test still green.
// Which comparison this is, is the caller's business; what it costs is not.
//
// It runs PocketBase's own comparison — core.PasswordFieldValue.Validate is
// what Record.ValidatePassword calls — so the dummy and a real credential are
// not merely similar in cost, they are the same code over the same shape of
// input.
//
// Note that Record.SetRaw("password", "<a hash>") followed by ValidatePassword
// does NOT compare anything: SetRaw stores a plain string, ValidatePassword
// type-asserts for *PasswordFieldValue, and the assertion fails silently to
// false with no bcrypt at all. Measured against v0.40.1. That is the shape this
// avoids by holding the value directly.
func (a *Authenticator) compare(hash, supplied string) bool {
	a.comparisons.Add(1)

	return core.PasswordFieldValue{Hash: hash}.Validate(supplied)
}

// compareDummy is the comparison an address with no account pays for. It counts
// itself separately and then does exactly what every other comparison does.
func (a *Authenticator) compareDummy(supplied string) {
	a.dummies.Add(1)

	a.compare(DummyPasswordHash, supplied)
}

// storedHash reads the bcrypt string off a loaded record. PasswordField
// publishes it under the ":hash" getter; the plain value is empty on anything
// read from the database.
func storedHash(record *core.Record) string {
	return record.GetString(core.FieldNamePassword + ":hash")
}

// refusedCredential is the ONE refusal every failed check answers with. One
// function, so that no branch can grow a message of its own and no reader of
// the log stream can tell the two apart (FR-005).
func refusedCredential() error {
	return fmt.Errorf(
		"identity: that address and password do not match an account: %w",
		domain.ErrUnauthenticated,
	)
}
