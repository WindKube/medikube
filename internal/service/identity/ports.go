package identity

import (
	"context"
	"time"

	"medikube/internal/domain/audit"
	"medikube/internal/domain/identity"
)

// TokenPurpose is what a link's token is good for. It is a parameter of Redeem
// rather than a method per purpose because the refusal has to be the same
// refusal for all of them, and two methods are two places for one of them to
// grow a different error.
//
// A token minted for one purpose and presented for another is refused exactly
// as a forged one: the difference is only interesting to somebody probing.
type TokenPurpose string

const (
	// TokenPasswordReset carries the link in a recovery message (FR-074). It
	// lives 30 minutes — PocketBase's collection default, confirmed against
	// v0.40.1 rather than assumed (contracts/auth.md, T223o).
	TokenPasswordReset TokenPurpose = "password_reset"

	// TokenEmailConfirmation carries the link that proves a person controls the
	// address on their account (FR-075). It lives 24 hours.
	TokenEmailConfirmation TokenPurpose = "email_confirmation"
)

// Profile is a change a person may make to their own account: the display name
// and the four preferences FR-011 enumerates, and nothing else.
//
// The absent members are the requirement. There is no Email, no Role, no
// EmailConfirmed and no DisabledAt here, so FR-012 is enforced by shape rather
// than by a check a handler could forget — a caller inside this process cannot
// promote itself, because the type it would have to promote itself through has
// no member to say so in.
//
// Pointers because "leave this alone" and "set this" are different
// instructions, exactly as they are on a medication patch.
type Profile struct {
	Name       *string
	UnitSystem *identity.UnitSystem
	Locale     *string
	DateFormat *identity.DateFormat
	Theme      *identity.Theme
}

// Credentials is one sign-in attempt as it reaches the service. It is a value
// and not two parameters so that no call site can transpose them.
type Credentials struct {
	Email    string
	Password string
}

// Registration is one sign-up. It carries no role and no status for the same
// reason Profile does not (FR-012, contracts/auth.md).
type Registration struct {
	Email    string
	Name     string
	Password string
}

// Repository is the account storage seam, declared by the consumer
// (Principle II). Implemented by internal/store/identity against PocketBase and
// by identitytest's in-memory fake; both pass identitytest.RunRepositoryContract.
//
// Five methods, which is plan.md's interface-segregation cap.
type Repository interface {
	// Create stores the account AND its first credential in one act.
	//
	// The password is a parameter here rather than a second call to the
	// Authenticator because FR-003 requires that no partially created account
	// survives a failure: an account written by one call and given a password
	// by another is an account that exists with no way in if the second call
	// fails. identity.User carries no password by design (research D-13), so
	// the credential travels beside it.
	//
	// An address that already has an account — in any letter case, which the
	// LOWER(email) unique index decides — is domain.ErrConflict.
	Create(ctx context.Context, draft identity.User, password string) (identity.User, error)

	// Get answers domain.ErrNotFound for an id with no account.
	Get(ctx context.Context, id string) (identity.User, error)

	// FindByEmail resolves an address to its account, case-insensitively, and
	// answers domain.ErrNotFound when there is none.
	//
	// It is reached on the refusal paths only — attributing a failed sign-in to
	// the account somebody aimed at, and deciding whether there is anybody to
	// send a recovery message to. Neither answer reaches a caller.
	FindByEmail(ctx context.Context, email string) (identity.User, error)

	// Update writes the account as supplied and returns it as stored.
	//
	// It writes what it is given, including the members a request may not set,
	// because a repository that decided which fields were writable would be a
	// second authorization rule in the layer that is meant to have none. The
	// service is what reads the stored account first and carries the address,
	// the role, the confirmation and the disabled instant over from it; the
	// test that deletes that step is what proves it.
	Update(ctx context.Context, user identity.User) (identity.User, error)

	// Delete removes the account permanently (FR-014). Every medication
	// recorded under it goes with it, by the cascade on `medications.owner`;
	// the audit trail does not, by the deliberate absence of one on
	// `audit_events.actor` (research D-22).
	Delete(ctx context.Context, id string) error
}

// Authenticator is the credential seam: PocketBase owns hashing, token minting
// and token-key rotation, and this is the whole of what MediKube asks of it
// (research D-13, D-16).
//
// No method here returns a token. The session a caller ends up holding is minted
// at the edge, from the saved record, through apis.RecordAuthResponse — which is
// what fires OnRecordAuthRequest and therefore what gets a sign-in audited
// through PocketBase's native route as well as MediKube's (research D-14). A
// service that minted its own would leave the native path unaudited.
//
// Five methods, which is plan.md's interface-segregation cap.
type Authenticator interface {
	// Authenticate resolves an address and a password to the account they
	// belong to, and answers domain.ErrUnauthenticated otherwise.
	//
	// TWO PROPERTIES ARE LOAD-BEARING AND EACH HAS ITS OWN GUARD:
	//
	// The refusal is the same value for an address with no account and for an
	// account with another password, and the returned user is the ZERO value on
	// both. A partially filled user beside an error is what a caller reads when
	// it forgets to check the error, and here that caller would be signing
	// somebody in.
	//
	// The two refusals take comparable time. An address with no account still
	// costs a bcrypt comparison against a fixed dummy hash before the refusal
	// returns, because an identical body answered in microseconds rather than
	// tens of milliseconds is still an account-existence oracle (research D-17).
	// It is asserted through a counting seam on the comparer, never a clock
	// (T202) — the mechanism is deterministic and the latency is not.
	//
	// A disabled account authenticates here and is refused by the service. The
	// credential either matches or it does not; whether the account may be used
	// is policy, and policy is not this layer's.
	Authenticate(ctx context.Context, email, password string) (identity.User, error)

	// Verify checks one known account's own password, for the operations that
	// ask a signed-in person to prove it is still them (FR-009, FR-013). An id
	// with no account and a wrong password are the same refusal.
	Verify(ctx context.Context, userID, password string) error

	// SetPassword replaces the credential and rotates the record's token key in
	// the same write, which ends every session issued before it (FR-010,
	// research D-16). It is the ONE way a password changes: a path that wrote
	// the hash without going through here would leave every stolen session
	// alive, and nothing downstream would say so.
	SetPassword(ctx context.Context, userID, password string) error

	// EndSessions rotates the token key without changing the password, which is
	// what makes a sign-out end the session everywhere it was still open
	// (FR-007).
	EndSessions(ctx context.Context, userID string) error

	// Redeem resolves a link's token to the account it was minted for.
	//
	// Expired, already used and tampered with are ONE refusal — ErrInvalidToken
	// — because telling them apart tells somebody which tokens once existed.
	// "Already used" for a reset token is the token key having moved underneath
	// it, which is the same write that ended the sessions.
	Redeem(ctx context.Context, purpose TokenPurpose, token string) (identity.User, error)
}

// Mailer is the outgoing-message seam (T223j). Two methods, each taking an
// account id and nothing else: an address parameter would let a signed-in
// caller aim MediKube's mailer at a stranger, and the address is on the record
// already.
//
// The service never reaches mails.* itself. Rendering the message, minting the
// token it carries and sending it are all PocketBase's, behind this.
type Mailer interface {
	// SendPasswordReset sends the recovery message for one account (FR-073).
	SendPasswordReset(ctx context.Context, userID string) error

	// SendVerification sends the address-confirmation message (FR-075).
	SendVerification(ctx context.Context, userID string) error
}

// Auditor writes the trail. Identical in shape to the port the medication
// service declares, and satisfied by the same internal/service/audit writer.
type Auditor interface {
	Record(ctx context.Context, event audit.Event) error
}

// Clock is where every occurred_at comes from. It is a port rather than a call
// to time.Now so that the rows a test asserts are the rows the test chose, and
// so that "the audit row is written before the delete" is a statement about an
// order this package controls.
type Clock interface {
	Now() time.Time
}

// SystemClock is the real one, and the only implementation outside a test.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }
