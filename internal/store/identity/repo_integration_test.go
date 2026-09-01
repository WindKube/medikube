package identity_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	domainidentity "medikube/internal/domain/identity"
	service "medikube/internal/service/identity"
	"medikube/internal/service/identity/identitytest"
	"medikube/internal/store"
	pbidentity "medikube/internal/store/identity"

	// The migrations register themselves from their own init and
	// tests.NewTestApp runs core.AppMigrations against the instance. Without
	// this import every case below would run against PocketBase's stock schema
	// — no `role`, no `disabled_at`, and no idx_users_email_lower, which is the
	// index half of this file exists to hold in place.
	_ "medikube/internal/store/migrations"
)

// harness is one instance and the two implementations under test.
//
// A new one per test rather than one shared: the counting seam below reads a
// number that a second test's sign-in would move, and the rotation cases assert
// about tokens minted for accounts only that case created.
type harness struct {
	app  *tests.TestApp
	repo *pbidentity.Repository
	auth *pbidentity.Authenticator
}

func newHarness(t *testing.T) harness {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	repo, err := pbidentity.NewRepository(app)
	require.NoError(t, err)

	auth, err := pbidentity.NewAuthenticator(app)
	require.NoError(t, err)

	return harness{app: app, repo: repo, auth: auth}
}

func (h harness) draft(email, name string) domainidentity.User {
	return domainidentity.User{
		Email:      email,
		Name:       name,
		Role:       domainidentity.DefaultRole,
		UnitSystem: domainidentity.DefaultUnitSystem,
		Locale:     domainidentity.DefaultLocale,
		DateFormat: domainidentity.DefaultDateFormat,
		Theme:      domainidentity.DefaultTheme,
	}
}

func (h harness) create(t *testing.T, email, name string) domainidentity.User {
	t.Helper()

	stored, err := h.repo.Create(t.Context(), h.draft(email, name), identitytest.Password)
	require.NoError(t, err)

	return stored
}

// authToken is what a signed-in browser carries, minted the way the edge mints
// it — from the record, through the collection's own secret.
func (h harness) authToken(t *testing.T, userID string) string {
	t.Helper()

	record, err := h.app.FindRecordById(store.AccountCollection, userID)
	require.NoError(t, err)

	token, err := record.NewAuthToken()
	require.NoError(t, err)

	return token
}

// live is the identical question internal/web/stream/session.go asks on every
// event and every heartbeat: would a request bearing this token still be
// authenticated? Nothing else in this file decides whether a session is over.
func (h harness) live(token string) error {
	_, err := h.app.FindAuthRecordByToken(token, core.TokenTypeAuth)

	return err
}

// mintLink is identitytest.Implementation.Token against a real instance.
//
// The tokens are PocketBase's own, and they have to be: a reset token is signed
// with `record.TokenKey() + collection.PasswordResetToken.Secret`, which is the
// whole mechanism the rotation cases in the contract suite observe. A stubbed
// minter would make three of them pass having asserted nothing.
func mintLink(t *testing.T, app core.App) func(service.TokenPurpose, string) (string, error) {
	t.Helper()

	return func(purpose service.TokenPurpose, userID string) (string, error) {
		record, err := app.FindRecordById(store.AccountCollection, userID)
		if err != nil {
			return "", err
		}

		switch purpose {
		case service.TokenPasswordReset:
			return record.NewPasswordResetToken()
		case service.TokenEmailConfirmation:
			return record.NewVerificationToken()
		default:
			return "", domain.ErrNotFound
		}
	}
}

// TestThePocketBaseIdentityStorePassesTheSameContractTheFakeDoes is T191.
//
// It is the same suite internal/service/identity runs against the in-memory
// fake, and running it twice is the point: every case here is a place the two
// could disagree while each one's own tests stayed green — the case-insensitive
// uniqueness of an address, whether a refused credential comes back with an
// account attached, whether a password change spends an outstanding link.
func TestThePocketBaseIdentityStorePassesTheSameContractTheFakeDoes(t *testing.T) {
	t.Parallel()

	identitytest.RunRepositoryContract(t, func(t *testing.T) identitytest.Implementation {
		t.Helper()

		h := newHarness(t)

		return identitytest.Implementation{
			Repository:    h.repo,
			Authenticator: h.auth,
			Token:         mintLink(t, h.app),
		}
	})
}

// TestTwoAddressesDifferingOnlyInCaseAreOneAccount is FR-003 against the index
// that decides it.
//
// idx_users_email_lower is a UNIQUE index on LOWER(email), and it is the whole
// enforcement: PocketBase's own idx on `email` is a byte comparison, and
// FindAuthRecordByEmail only searches case-insensitively when the single-column
// unique index on `email` carries COLLATE NOCASE — which the stock one does
// not. Measured against v0.40.1: FindAuthRecordByEmail("AMARA@…") answers
// "no rows" for an account stored as "amara@…". Reading the address through
// that call rather than through LOWER() would make every spelling below a
// second account.
func TestTwoAddressesDifferingOnlyInCaseAreOneAccount(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	// Registered in MIXED case, which is what a person types and what the
	// account therefore keeps. A fixture stored in lower case would be found by
	// a lookup that folded only the address it was handed, and the column's own
	// LOWER() — the half the unique index is built on — would never run.
	stored := h.create(t, "Amara.Okonkwo@Example.Test", "Amara Okonkwo")

	spellings := []struct {
		name  string
		email string
	}{
		{name: "the address as registered", email: "Amara.Okonkwo@Example.Test"},
		{name: "all lower case", email: "amara.okonkwo@example.test"},
		{name: "a capitalised local part", email: "Amara.okonkwo@example.test"},
		{name: "a capitalised domain", email: "amara.okonkwo@Example.Test"},
		{name: "shouted", email: "AMARA.OKONKWO@EXAMPLE.TEST"},
	}

	for _, spelling := range spellings {
		t.Run(spelling.name, func(t *testing.T) {
			found, err := h.repo.FindByEmail(t.Context(), spelling.email)
			require.NoError(t, err, "the address resolved to no account")
			assert.Equal(t, stored.ID, found.ID)

			_, err = h.repo.Create(t.Context(), h.draft(spelling.email, "Impostor"), identitytest.Password)
			assert.ErrorIs(t, err, domain.ErrConflict, "a second account was created for one address")

			signedIn, err := h.auth.Authenticate(t.Context(), spelling.email, identitytest.Password)
			require.NoError(t, err, "the account could not be signed in to by this spelling")
			assert.Equal(t, stored.ID, signedIn.ID)
		})
	}

	total, err := h.app.CountRecords(store.AccountCollection)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total, "more than one account exists for one address")
}

// TestTheIndexThatMakesThemOneAccountIsOnTheCollection is the layer underneath
// the case above, asserted on its own.
//
// The two are independent by design. Delete the LOWER() lookup and the case
// above fails while this passes; drop the index from the migration and this
// fails while a single-process lookup could still look right. FR-003 needs
// both: the lookup is what one request sees, and the index is what two
// simultaneous registrations of one address collide on.
func TestTheIndexThatMakesThemOneAccountIsOnTheCollection(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	collection, err := h.app.FindCollectionByNameOrId(store.AccountCollection)
	require.NoError(t, err)

	var found bool

	for _, index := range collection.Indexes {
		lowered := strings.ToLower(index)
		if strings.Contains(lowered, "unique") && strings.Contains(lowered, "lower(email)") {
			found = true
		}
	}

	assert.True(t, found,
		"no UNIQUE index on LOWER(email): two simultaneous registrations of one address would both succeed, indexes=%v",
		collection.Indexes)
}

// TestEverySignInRefusalCostsExactlyOneComparison is T202's mechanism, asserted
// at the seam that performs it.
//
// An address with no account must pay for a bcrypt comparison against a fixed
// dummy hash before it is refused. Without it the two refusals are identical in
// body and 339× apart in latency (measured on this codebase against v0.40.1:
// 206µs versus 69.8ms), which is an account-existence oracle wearing an
// identical response.
//
// It is asserted through a counter and never through a clock. The mechanism is
// deterministic; the latency is not, and Constitution VIII forbids a gate
// assertion that can flap.
func TestEverySignInRefusalCostsExactlyOneComparison(t *testing.T) {
	t.Parallel()

	const knownEmail = "known@example.test"

	cases := []struct {
		name     string
		email    string
		password string
		refused  bool
		dummies  int
	}{
		{
			name:     "an address with no account",
			email:    "nobody@example.test",
			password: identitytest.Password,
			refused:  true,
			dummies:  1,
		},
		{
			name:     "an account with another password",
			email:    knownEmail,
			password: "not-the-password-on-the-account",
			refused:  true,
			dummies:  0,
		},
		{
			name:     "the account's own password",
			email:    knownEmail,
			password: identitytest.Password,
			refused:  false,
			dummies:  0,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.create(t, knownEmail, "Known")
			h.auth.Forget()

			_, err := h.auth.Authenticate(t.Context(), testCase.email, testCase.password)

			if testCase.refused {
				require.ErrorIs(t, err, domain.ErrUnauthenticated)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, 1, h.auth.Comparisons(),
				"a sign-in that did not cost exactly one comparison is one an attacker can time")
			assert.Equal(t, testCase.dummies, h.auth.DummyComparisons())
		})
	}
}

// TestEveryPasswordCheckOnAKnownAccountCostsOneComparisonToo is the same
// property for the operations that ask a signed-in person to prove it is still
// them (FR-009, FR-013). An id with no account is the one that has to pay.
func TestEveryPasswordCheckOnAKnownAccountCostsOneComparisonToo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		account bool
		correct bool
		dummies int
	}{
		{name: "an id with no account", dummies: 1},
		{name: "an account with another password", account: true},
		{name: "the account's own password", account: true, correct: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			stored := h.create(t, "verifying@example.test", "Verifying")

			userID := "nosuchaccount01"
			if testCase.account {
				userID = stored.ID
			}

			password := "not-the-password-on-the-account"
			if testCase.correct {
				password = identitytest.Password
			}

			h.auth.Forget()

			err := h.auth.Verify(t.Context(), userID, password)

			if testCase.correct {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, domain.ErrUnauthenticated)
			}

			assert.Equal(t, 1, h.auth.Comparisons())
			assert.Equal(t, testCase.dummies, h.auth.DummyComparisons())
		})
	}
}

// TestTheDummyHashCostsWhatEveryRealHashCosts is the drift guard on the
// equalisation.
//
// The dummy is a fixed hash, so its bcrypt cost is fixed at the moment it was
// written. A real credential's cost comes from the collection's PasswordField,
// which an operator or a PocketBase default could move. If they part company
// the two comparisons stop taking comparable time and the equalisation silently
// becomes decorative — an assertion on the count would still pass, because the
// comparison still happens, and only this notices.
func TestTheDummyHashCostsWhatEveryRealHashCosts(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	stored := h.create(t, "costed@example.test", "Costed")

	record, err := h.app.FindRecordById(store.AccountCollection, stored.ID)
	require.NoError(t, err)

	credential := bcryptCost(t, record.GetString(core.FieldNamePassword+":hash"))
	dummy := bcryptCost(t, pbidentity.DummyPasswordHash)

	assert.Equal(t, credential, dummy,
		"the fixed dummy hash costs %d rounds and a real credential costs %d, so the refusals no longer take comparable time",
		dummy, credential)
}

// bcryptCost reads the work factor out of a modular-crypt bcrypt string:
// `$2<minor>$<cost>$<salt+digest>`. Parsed rather than read with
// golang.org/x/crypto/bcrypt so that the assertion costs no dependency
// promotion for one integer.
func bcryptCost(t *testing.T, hash string) int {
	t.Helper()

	parts := strings.Split(hash, "$")
	require.Lenf(t, parts, 4, "%q is not a bcrypt hash", hash)
	require.Equal(t, "", parts[0])
	require.Truef(t, strings.HasPrefix(parts[1], "2"), "%q is not a bcrypt hash", hash)

	cost, err := strconv.Atoi(parts[2])
	require.NoError(t, err)

	return cost
}

// TestAPasswordChangeEndsEverySessionTheAccountHadOpen is FR-010 and the first
// half of T217.
//
// The rotation is PocketBase's: onRecordSaveExecute re-randomises `tokenKey`
// when the password changed and the key did not (core/record_model.go:1448).
// MediKube does not perform a second one — it goes through Record.SetPassword
// and Save, which is what triggers it. The failure this guards is MediKube
// DEFEATING that: a raw UPDATE of the hash, or a write that never reaches
// onRecordSaveExecute, leaves every stolen session alive and nothing else says
// so.
//
// "Ended" is asked here in exactly the terms internal/web/stream/session.go
// asks it in — FindAuthRecordByToken against core.TokenTypeAuth — so the
// rotation and the stream's per-event check cannot end up reading different
// fields.
func TestAPasswordChangeEndsEverySessionTheAccountHadOpen(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	stored := h.create(t, "changing@example.test", "Changing")

	phone := h.authToken(t, stored.ID)
	laptop := h.authToken(t, stored.ID)

	require.NoError(t, h.live(phone), "a freshly minted session was already over")
	require.NoError(t, h.live(laptop))

	require.NoError(t, h.auth.SetPassword(t.Context(), stored.ID,
		"a-second-perfectly-ordinary-passphrase"))

	assert.Error(t, h.live(phone), "the session the password was changed from is still usable")
	assert.Error(t, h.live(laptop), "another device's session survived the password change")

	after := h.authToken(t, stored.ID)
	assert.NoError(t, h.live(after), "the account cannot open a session with its new password")
}

// TestASignOutEndsEverySessionTheAccountHadOpen is FR-007 and the second half
// of T217.
//
// This rotation is MediKube's, and it has to be: nothing about a sign-out
// changes the password, so PocketBase's automatic refresh does not fire. The
// password must still work afterwards — a sign-out that quietly reset somebody's
// credential would lock them out of their own medical records.
func TestASignOutEndsEverySessionTheAccountHadOpen(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	stored := h.create(t, "leaving@example.test", "Leaving")

	phone := h.authToken(t, stored.ID)
	laptop := h.authToken(t, stored.ID)

	require.NoError(t, h.live(phone))
	require.NoError(t, h.live(laptop))

	require.NoError(t, h.auth.EndSessions(t.Context(), stored.ID))

	assert.Error(t, h.live(phone), "signing out on one device left it signed in")
	assert.Error(t, h.live(laptop), "signing out on the phone left the laptop signed in (FR-007)")

	signedIn, err := h.auth.Authenticate(t.Context(), "leaving@example.test", identitytest.Password)
	require.NoError(t, err, "signing out changed the credential")
	assert.Equal(t, stored.ID, signedIn.ID)
}

// TestChangingAPreferenceDoesNotSignAnybodyOut is the negative half, and it is
// the one that would be missed.
//
// The same save path carries a theme change and a password change. If the
// rotation were reached for every write — by MediKube calling RefreshTokenKey
// unconditionally, or by the mapper writing the password column on an update —
// every profile change would sign the person out of every device, and every
// test above would still pass.
func TestChangingAPreferenceDoesNotSignAnybodyOut(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	stored := h.create(t, "settling-in@example.test", "Settling In")

	open := h.authToken(t, stored.ID)
	require.NoError(t, h.live(open))

	stored.Theme = domainidentity.ThemeDark
	stored.Name = "Settled"

	updated, err := h.repo.Update(t.Context(), stored)
	require.NoError(t, err)
	require.Equal(t, domainidentity.ThemeDark, updated.Theme)

	assert.NoError(t, h.live(open), "changing a preference signed the person out of every device")
}

// TestConfirmingAnAddressIsStoredAndDoesNotSignAnybodyOut. `verified` is the
// one column outside the profile that an update writes, and the mapper
// deliberately does not write it — internal/store's UserToRecord has a test
// asserting it cannot, so that no profile patch can confirm an address. The
// repository is where the confirmation lands, and confirming is not a
// credential change.
func TestConfirmingAnAddressIsStoredAndDoesNotSignAnybodyOut(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	stored := h.create(t, "confirming@example.test", "Confirming")
	require.False(t, stored.EmailConfirmed, "a new account arrived already confirmed")

	open := h.authToken(t, stored.ID)
	require.NoError(t, h.live(open))

	stored.EmailConfirmed = true

	updated, err := h.repo.Update(t.Context(), stored)
	require.NoError(t, err)
	assert.True(t, updated.EmailConfirmed)

	read, err := h.repo.Get(t.Context(), stored.ID)
	require.NoError(t, err)
	assert.True(t, read.EmailConfirmed, "the confirmation did not survive the write")

	assert.NoError(t, h.live(open), "confirming an address signed the person out")
}

// TestARegistrationCannotArriveAlreadyConfirmed. FR-075: an address is
// confirmed by following a message sent to it and never by claiming to have.
// The draft asks for both the confirmation and a role it may not have.
func TestARegistrationCannotArriveAlreadyConfirmed(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	draft := h.draft("optimistic@example.test", "Optimistic")
	draft.EmailConfirmed = true

	stored, err := h.repo.Create(t.Context(), draft, identitytest.Password)
	require.NoError(t, err)

	assert.False(t, stored.EmailConfirmed, "an account confirmed itself by asking")

	read, err := h.repo.Get(t.Context(), stored.ID)
	require.NoError(t, err)
	assert.False(t, read.EmailConfirmed)
}

// TestADisabledAccountIsStoredDisabledAndStillAuthenticates. The credential
// either matches or it does not; whether the account may be used is the
// service's to decide. A repository that refused it here would make the
// service's own check unreachable and its test vacuous.
func TestADisabledAccountIsStoredDisabledAndStillAuthenticates(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	draft := h.draft("out-of-service@example.test", "Out Of Service")
	draft.DisabledAt = time.Date(2026, time.February, 2, 12, 0, 0, 0, time.UTC)

	stored, err := h.repo.Create(t.Context(), draft, identitytest.Password)
	require.NoError(t, err)
	require.True(t, stored.IsDisabled(), "the disabled instant was dropped on the way in")

	signedIn, err := h.auth.Authenticate(t.Context(), "out-of-service@example.test", identitytest.Password)
	require.NoError(t, err)
	assert.True(t, signedIn.IsDisabled(), "the service cannot refuse what it is not told")
}

// TestALinkMintedForOneAccountIsRefusedForAnother, and a link minted against
// the superuser collection is refused outright.
//
// FindAuthRecordByToken resolves a token against whatever collection its claims
// name, so a `_superusers` reset token resolves perfectly well. Answering it
// with the account mapper's "not from the collection this mapper reads" would
// be a 500; it is a link that cannot be used, which is a 400, and it reads
// exactly like every other unusable link.
func TestASuperusersLinkIsNotAnAccountsLink(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	superuser, err := h.app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	require.NoError(t, err)

	record := core.NewRecord(superuser)
	record.SetEmail("admin@example.test")
	record.SetPassword("a-perfectly-ordinary-passphrase")
	require.NoError(t, h.app.Save(record))

	token, err := record.NewPasswordResetToken()
	require.NoError(t, err)

	redeemed, err := h.auth.Redeem(t.Context(), service.TokenPasswordReset, token)

	assert.ErrorIs(t, err, service.ErrInvalidToken)
	assert.Empty(t, redeemed.ID, "a superuser's recovery link resolved to an account")
}

// TestALinkThatCouldNotBeCheckedIsNotAnInvalidLink.
//
// Folding an outage into ErrInvalidToken would tell somebody their link had
// expired on the strength of a failed read, and would hide the outage behind a
// 400 nobody investigates. The distinction is the same one internal/store's
// owner lookup makes, and for the same reason.
func TestALinkThatCouldNotBeCheckedIsNotAnInvalidLink(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	stored := h.create(t, "unreachable@example.test", "Unreachable")

	usable, err := mintLink(t, h.app)(service.TokenPasswordReset, stored.ID)
	require.NoError(t, err)

	const unusable = "not-a-token-this-instance-issued"

	// The same token is a usable link one moment and an unusable one the next,
	// so both have to be asked while nothing can answer.
	unreachable, cancel := context.WithCancel(t.Context())
	cancel()

	for name, token := range map[string]string{
		"a link that would have resolved":     usable,
		"a link that would have been refused": unusable,
	} {
		t.Run(name, func(t *testing.T) {
			_, judged := h.auth.Redeem(unreachable, service.TokenPasswordReset, token)

			require.Error(t, judged, "a link was judged while the database could not be asked about it")
			assert.NotErrorIs(t, judged, service.ErrInvalidToken,
				"a check that could not be made was reported as a link that cannot be used")
		})
	}

	// And with the database answering, the unusable one is exactly that.
	_, err = h.auth.Redeem(t.Context(), service.TokenPasswordReset, unusable)
	assert.ErrorIs(t, err, service.ErrInvalidToken)
}

// TestACancelledContextStopsEveryOperation, and stops it with the failure
// rather than with a refusal. A read that could not be made has found nothing
// and has refused nobody: reporting it as ErrNotFound would turn a dropped
// connection into "no such account", and reporting it as ErrUnauthenticated
// would turn one into a wrong password.
func TestACancelledContextStopsEveryOperation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	stored := h.create(t, "cancelled@example.test", "Cancelled")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	operations := map[string]func() error{
		"create": func() error {
			_, err := h.repo.Create(ctx, h.draft("later@example.test", "Later"), identitytest.Password)

			return err
		},
		"get": func() error {
			_, err := h.repo.Get(ctx, stored.ID)

			return err
		},
		"find by email": func() error {
			_, err := h.repo.FindByEmail(ctx, "cancelled@example.test")

			return err
		},
		"update": func() error {
			_, err := h.repo.Update(ctx, stored)

			return err
		},
		"delete": func() error {
			return h.repo.Delete(ctx, stored.ID)
		},
		"authenticate": func() error {
			_, err := h.auth.Authenticate(ctx, "cancelled@example.test", identitytest.Password)

			return err
		},
		"verify": func() error {
			return h.auth.Verify(ctx, stored.ID, identitytest.Password)
		},
		"set password": func() error {
			return h.auth.SetPassword(ctx, stored.ID, "a-second-perfectly-ordinary-passphrase")
		},
		"end sessions": func() error {
			return h.auth.EndSessions(ctx, stored.ID)
		},
		"redeem": func() error {
			_, err := h.auth.Redeem(ctx, service.TokenPasswordReset, "not-a-token-this-instance-issued")

			return err
		},
	}

	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			err := operation()

			require.Error(t, err, "the operation ran to completion on a cancelled request")
			assert.NotErrorIs(t, err, domain.ErrNotFound,
				"a read that could not be made was reported as no such account")
			assert.NotErrorIs(t, err, domain.ErrUnauthenticated,
				"a check that could not be made was reported as a refused credential")
		})
	}
}

// TestTheAccountIsStillThereAfterAFailedWrite. The refusal a caller sees has to
// be the refusal that happened: a create that lost the race for an address must
// leave the winner's account exactly as it was.
func TestAConflictLeavesTheAccountItCollidedWithUntouched(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	stored := h.create(t, "incumbent@example.test", "Incumbent")

	_, err := h.repo.Create(t.Context(), h.draft("INCUMBENT@example.test", "Challenger"), "a-different-passphrase-entirely")
	require.ErrorIs(t, err, domain.ErrConflict)

	read, err := h.repo.Get(t.Context(), stored.ID)
	require.NoError(t, err)
	assert.Equal(t, "Incumbent", read.Name)

	signedIn, err := h.auth.Authenticate(t.Context(), "incumbent@example.test", identitytest.Password)
	require.NoError(t, err, "the incumbent's credential was overwritten by a refused registration")
	assert.Equal(t, stored.ID, signedIn.ID)
}
