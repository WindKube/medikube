package identitytest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"medikube/internal/domain"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/service/identity"
)

// Implementation is one pairing of the two ports a contract run exercises.
//
// They are supplied together and not separately because they are one thing
// underneath: the account a Repository writes is the account an Authenticator
// checks a password against, and a suite that let a test pair a fake repository
// with a real authenticator would be asserting about a combination that never
// ships.
type Implementation struct {
	Repository    identity.Repository
	Authenticator identity.Authenticator

	// Token mints a link token of one purpose for one account, as the mailer
	// would. The suite cannot mint its own: a real token is signed with the
	// instance's own secret and with the record's own key, which is the whole
	// mechanism the rotation cases below observe.
	Token func(purpose identity.TokenPurpose, userID string) (string, error)
}

// Factory builds one empty implementation, per test. Per test and not per run:
// a suite that shared one between two cases would have the second reading rows
// the first left, and the uniqueness assertions would depend on the order the
// methods happen to run in.
type Factory func(t *testing.T) Implementation

// RunRepositoryContract is the contract every identity.Repository and
// identity.Authenticator pair passes — the in-memory fakes here and the
// PocketBase implementation in internal/store/identity, against a real
// instance.
//
// It is one suite run twice rather than two suites, because the failure it
// exists to catch is the one where the fake and the store agree about
// everything the service does and disagree about the case-insensitive
// uniqueness of an address, about whether a refused credential returns an
// account anyway, or about whether a password change spends a recovery link.
// Every one of those is invisible to a test written against either
// implementation alone.
func RunRepositoryContract(t *testing.T, newImplementation Factory) {
	t.Helper()

	require.NotNil(t, newImplementation, "the contract has no implementation to run against")

	suite.Run(t, &repositoryContract{newImplementation: newImplementation})
}

type repositoryContract struct {
	suite.Suite

	newImplementation Factory
	repository        identity.Repository
	authenticator     identity.Authenticator
	token             func(identity.TokenPurpose, string) (string, error)
}

func (c *repositoryContract) SetupTest() {
	implementation := c.newImplementation(c.T())

	c.Require().NotNil(implementation.Repository, "the factory returned no repository")
	c.Require().NotNil(implementation.Authenticator, "the factory returned no authenticator")
	c.Require().NotNil(implementation.Token, "the factory returned no way to mint a link token, so no rotation case can run")

	c.repository, c.authenticator, c.token =
		implementation.Repository, implementation.Authenticator, implementation.Token
}

func (c *repositoryContract) ctx() context.Context { return c.T().Context() }

// draft is a complete account: every column data-model §1 declares required,
// filled with the defaults registration applies.
func (c *repositoryContract) draft(email, name string) domainidentity.User {
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

func (c *repositoryContract) create(email, name, password string) domainidentity.User {
	c.T().Helper()

	stored, err := c.repository.Create(c.ctx(), c.draft(email, name), password)
	c.Require().NoError(err)

	return stored
}

// TestCreateStoresTheAccountAndMintsAnIdentity is the round trip. Every column
// is written and read back, because a mapper that drops one silently stores an
// account missing the preference the person chose.
func (c *repositoryContract) TestCreateStoresTheAccountAndMintsAnIdentity() {
	draft := c.draft("round-trip@example.test", "Round Trip")
	draft.UnitSystem = domainidentity.UnitSystemImperial
	draft.Locale = "en-GB"
	draft.DateFormat = domainidentity.DateFormatDMY
	draft.Theme = domainidentity.ThemeDark

	stored, err := c.repository.Create(c.ctx(), draft, Password)
	c.Require().NoError(err)

	c.Require().NotEmpty(stored.ID, "an account with no identity cannot be addressed again")
	c.Assert().False(stored.CreatedAt.IsZero())
	c.Assert().False(stored.UpdatedAt.IsZero())

	read, err := c.repository.Get(c.ctx(), stored.ID)
	c.Require().NoError(err)

	c.Assert().Equal(stored.ID, read.ID)
	c.Assert().Equal(draft.Email, read.Email)
	c.Assert().Equal(draft.Name, read.Name)
	c.Assert().Equal(domainidentity.DefaultRole, read.Role, "a new account is never anything but a user (FR-012)")
	c.Assert().Equal(draft.UnitSystem, read.UnitSystem)
	c.Assert().Equal(draft.Locale, read.Locale)
	c.Assert().Equal(draft.DateFormat, read.DateFormat)
	c.Assert().Equal(draft.Theme, read.Theme)
	c.Assert().False(read.IsDisabled(), "a new account arrives out of service")
	c.Assert().False(read.EmailConfirmed, "a new account's address is confirmed by a message, never by registering")
}

// TestCreateRefusesASecondAccountForOneAddress is FR-003.
func (c *repositoryContract) TestCreateRefusesASecondAccountForOneAddress() {
	c.create("taken@example.test", "First", Password)

	_, err := c.repository.Create(c.ctx(), c.draft("taken@example.test", "Second"), Password)

	c.Require().ErrorIs(err, domain.ErrConflict)
}

// TestAddressesDifferingOnlyInCaseAreOneAddress is the half of FR-003 that
// depends on a storage-layer index rather than on any code — idx_users_email_lower
// — which is exactly why it is in the contract rather than in one
// implementation's own test. A fake that compared byte for byte would let every
// service test above it pass while the real instance refused.
func (c *repositoryContract) TestAddressesDifferingOnlyInCaseAreOneAddress() {
	stored := c.create("amara@example.test", "Amara", Password)

	_, err := c.repository.Create(c.ctx(), c.draft("Amara@Example.Test", "Impostor"), Password)
	c.Require().ErrorIs(err, domain.ErrConflict)

	found, err := c.repository.FindByEmail(c.ctx(), "AMARA@EXAMPLE.TEST")
	c.Require().NoError(err)
	c.Assert().Equal(stored.ID, found.ID, "the same address in another case resolved to no account")
}

func (c *repositoryContract) TestGetOfAnIdentityThatNeverExistedIsNotFound() {
	_, err := c.repository.Get(c.ctx(), "nosuchaccount01")

	c.Require().ErrorIs(err, domain.ErrNotFound)
}

func (c *repositoryContract) TestFindByEmailForAnAddressWithNoAccountIsNotFound() {
	_, err := c.repository.FindByEmail(c.ctx(), StrangerEmail)

	c.Require().ErrorIs(err, domain.ErrNotFound)
}

// TestUpdateWritesTheAccountAndReturnsItAsStored.
func (c *repositoryContract) TestUpdateWritesTheAccountAndReturnsItAsStored() {
	stored := c.create("changeable@example.test", "Before", Password)

	changed := stored
	changed.Name = "After"
	changed.Theme = domainidentity.ThemeLight
	changed.EmailConfirmed = true

	updated, err := c.repository.Update(c.ctx(), changed)
	c.Require().NoError(err)

	c.Assert().Equal("After", updated.Name)
	c.Assert().Equal(domainidentity.ThemeLight, updated.Theme)
	c.Assert().True(updated.EmailConfirmed)

	read, err := c.repository.Get(c.ctx(), stored.ID)
	c.Require().NoError(err)

	c.Assert().Equal("After", read.Name)
	c.Assert().Equal(domainidentity.ThemeLight, read.Theme)
	c.Assert().True(read.EmailConfirmed, "the confirmed state a service set did not survive the write")
	c.Assert().Equal(stored.CreatedAt.UTC(), read.CreatedAt.UTC(), "the creation instant moved under an update")
}

func (c *repositoryContract) TestUpdateOfAnIdentityThatNeverExistedIsNotFound() {
	absent := c.draft("absent@example.test", "Absent")
	absent.ID = "nosuchaccount01"

	_, err := c.repository.Update(c.ctx(), absent)

	c.Require().ErrorIs(err, domain.ErrNotFound)
}

// TestDeleteRemovesTheAccount. FR-014: permanent, and the credential goes with
// it — a deleted account that could still be authenticated is the failure
// SC-012 exists to rule out.
func (c *repositoryContract) TestDeleteRemovesTheAccount() {
	stored := c.create("departing@example.test", "Departing", Password)

	c.Require().NoError(c.repository.Delete(c.ctx(), stored.ID))

	_, err := c.repository.Get(c.ctx(), stored.ID)
	c.Assert().ErrorIs(err, domain.ErrNotFound)

	_, err = c.repository.FindByEmail(c.ctx(), "departing@example.test")
	c.Assert().ErrorIs(err, domain.ErrNotFound)

	_, err = c.authenticator.Authenticate(c.ctx(), "departing@example.test", Password)
	c.Assert().ErrorIs(err, domain.ErrUnauthenticated, "a deleted account's credentials still sign in")
}

func (c *repositoryContract) TestDeleteOfAnIdentityThatNeverExistedIsNotFound() {
	c.Require().ErrorIs(c.repository.Delete(c.ctx(), "nosuchaccount01"), domain.ErrNotFound)
}

// TestEveryRefusalIsOneOfTheDomainSentinels. The service maps what it gets from
// here onto a status by errors.Is: a repository returning a bare driver error
// for a missing account produces a 500 where the contract says 404.
func (c *repositoryContract) TestEveryRefusalIsOneOfTheDomainSentinels() {
	_, err := c.repository.Get(c.ctx(), "nosuchaccount01")

	c.Require().Error(err)
	c.Assert().True(
		errors.Is(err, domain.ErrNotFound) ||
			errors.Is(err, domain.ErrConflict) ||
			errors.Is(err, domain.ErrUnauthenticated),
		"a refusal that matches no sentinel reaches the client as an internal error")
}

// TestAuthenticateResolvesTheAccountForItsOwnPassword.
func (c *repositoryContract) TestAuthenticateResolvesTheAccountForItsOwnPassword() {
	stored := c.create("signing-in@example.test", "Signing In", Password)

	authenticated, err := c.authenticator.Authenticate(c.ctx(), "signing-in@example.test", Password)
	c.Require().NoError(err)

	c.Assert().Equal(stored.ID, authenticated.ID)
	c.Assert().Equal(stored.Email, authenticated.Email)
}

// TestAWrongPasswordAndAnUnknownAddressAreOneRefusal is FR-005 at the port.
//
// The error TEXT is compared and not only the sentinel, because this string is
// what reaches the one log stream: two messages that differ tell an operator's
// log — and anyone who can read it — which addresses are registered, which is
// the disclosure the identical response body exists to prevent.
func (c *repositoryContract) TestAWrongPasswordAndAnUnknownAddressAreOneRefusal() {
	c.create("known@example.test", "Known", Password)

	wrongPassword, known := c.authenticator.Authenticate(c.ctx(), "known@example.test", "not-the-password-on-the-account")
	unknownAddress, unknown := c.authenticator.Authenticate(c.ctx(), StrangerEmail, Password)

	c.Require().ErrorIs(known, domain.ErrUnauthenticated)
	c.Require().ErrorIs(unknown, domain.ErrUnauthenticated)
	c.Assert().Equal(known.Error(), unknown.Error(),
		"the two refusals read differently, so the log stream says which addresses have accounts")

	// Fail-closed on both. A partially filled account beside an error is what a
	// caller reads when it forgets to check the error, and here that caller
	// would be signing somebody in.
	c.Assert().Empty(wrongPassword.ID, "a refused credential came back with an account attached")
	c.Assert().Empty(unknownAddress.ID)
}

// TestADisabledAccountAuthenticatesAndIsRefusedAbove. The credential either
// matches or it does not; whether the account may be used is policy, and the
// service is where policy lives (data-model §1, FR-005).
//
// It is pinned here because the alternative fails silently: an implementation
// that refused a disabled account itself would make the service's own check
// unreachable, and the test of that check would pass having exercised nothing.
func (c *repositoryContract) TestADisabledAccountAuthenticatesAndIsRefusedAbove() {
	draft := c.draft("out-of-service@example.test", "Out Of Service")
	draft.DisabledAt = time.Date(2026, time.February, 2, 12, 0, 0, 0, time.UTC)

	stored, err := c.repository.Create(c.ctx(), draft, Password)
	c.Require().NoError(err)
	c.Require().True(stored.IsDisabled(), "the disabled instant was dropped on the way in")

	authenticated, err := c.authenticator.Authenticate(c.ctx(), "out-of-service@example.test", Password)
	c.Require().NoError(err)
	c.Assert().Equal(stored.ID, authenticated.ID)
	c.Assert().True(authenticated.IsDisabled(), "the service cannot refuse what it is not told")
}

// TestVerifyChecksOneAccountsOwnPassword, and answers an id with no account
// exactly as a wrong password: the operations that call it are already
// authenticated, and a different answer would still be a difference somebody
// could read out of a log.
func (c *repositoryContract) TestVerifyChecksOneAccountsOwnPassword() {
	stored := c.create("verifying@example.test", "Verifying", Password)

	c.Require().NoError(c.authenticator.Verify(c.ctx(), stored.ID, Password))

	wrong := c.authenticator.Verify(c.ctx(), stored.ID, "not-the-password-on-the-account")
	c.Require().ErrorIs(wrong, domain.ErrUnauthenticated)

	absent := c.authenticator.Verify(c.ctx(), "nosuchaccount01", Password)
	c.Require().ErrorIs(absent, domain.ErrUnauthenticated)
	c.Assert().Equal(wrong.Error(), absent.Error())
}

// TestSetPasswordReplacesTheCredential.
func (c *repositoryContract) TestSetPasswordReplacesTheCredential() {
	const replacement = "a-second-perfectly-ordinary-passphrase"

	stored := c.create("changing@example.test", "Changing", Password)

	c.Require().NoError(c.authenticator.SetPassword(c.ctx(), stored.ID, replacement))

	_, err := c.authenticator.Authenticate(c.ctx(), "changing@example.test", Password)
	c.Assert().ErrorIs(err, domain.ErrUnauthenticated, "the password that was replaced still signs in")

	authenticated, err := c.authenticator.Authenticate(c.ctx(), "changing@example.test", replacement)
	c.Require().NoError(err)
	c.Assert().Equal(stored.ID, authenticated.ID)
}

// TestSetPasswordSpendsEveryLinkMintedBeforeIt is how the token-key rotation
// becomes observable through this port at all (FR-010, FR-074, research D-16).
//
// A recovery link is signed with the record's key, so a password change moves
// the key out from under every link that was outstanding — which is the same
// single write that ends every session issued before it. There is no separate
// "was it rotated" question to ask: this IS the question.
func (c *repositoryContract) TestSetPasswordSpendsEveryLinkMintedBeforeIt() {
	stored := c.create("spending@example.test", "Spending", Password)

	token, err := c.token(identity.TokenPasswordReset, stored.ID)
	c.Require().NoError(err)

	redeemed, err := c.authenticator.Redeem(c.ctx(), identity.TokenPasswordReset, token)
	c.Require().NoError(err, "a freshly minted link did not resolve, so the case below would pass vacuously")
	c.Require().Equal(stored.ID, redeemed.ID)

	c.Require().NoError(c.authenticator.SetPassword(c.ctx(), stored.ID, "a-third-perfectly-ordinary-passphrase"))

	_, err = c.authenticator.Redeem(c.ctx(), identity.TokenPasswordReset, token)
	c.Assert().ErrorIs(err, identity.ErrInvalidToken, "a link minted before a password change is still usable")
}

// TestEndSessionsSpendsEveryLinkMintedBeforeIt is the same rotation reached
// without a new password, which is what a sign-out does (FR-007).
func (c *repositoryContract) TestEndSessionsSpendsEveryLinkMintedBeforeIt() {
	stored := c.create("ending@example.test", "Ending", Password)

	token, err := c.token(identity.TokenPasswordReset, stored.ID)
	c.Require().NoError(err)

	c.Require().NoError(c.authenticator.EndSessions(c.ctx(), stored.ID))

	_, err = c.authenticator.Redeem(c.ctx(), identity.TokenPasswordReset, token)
	c.Assert().ErrorIs(err, identity.ErrInvalidToken)
}

// TestRedeemResolvesEachPurposeToItsAccount.
func (c *repositoryContract) TestRedeemResolvesEachPurposeToItsAccount() {
	stored := c.create("redeeming@example.test", "Redeeming", Password)

	for _, purpose := range []identity.TokenPurpose{identity.TokenPasswordReset, identity.TokenEmailConfirmation} {
		token, err := c.token(purpose, stored.ID)
		c.Require().NoError(err)

		redeemed, err := c.authenticator.Redeem(c.ctx(), purpose, token)
		c.Require().NoError(err)
		c.Assert().Equal(stored.ID, redeemed.ID)
		c.Assert().Equal(stored.Email, redeemed.Email)
	}
}

// TestEveryUnusableLinkIsOneRefusal. Expired, spent, minted for another purpose
// and never minted at all are the same error with the same text: telling them
// apart tells an attacker which tokens once existed (FR-074, contracts/auth.md).
func (c *repositoryContract) TestEveryUnusableLinkIsOneRefusal() {
	stored := c.create("refusing@example.test", "Refusing", Password)

	reset, err := c.token(identity.TokenPasswordReset, stored.ID)
	c.Require().NoError(err)

	spent, err := c.token(identity.TokenPasswordReset, stored.ID)
	c.Require().NoError(err)
	c.Require().NoError(c.authenticator.SetPassword(c.ctx(), stored.ID, "a-fourth-perfectly-ordinary-passphrase"))

	refusals := map[string]string{
		"minted for another purpose": reset,
		"never minted at all":        "not-a-token-this-instance-issued",
		"already spent":              spent,
	}

	messages := make([]string, 0, len(refusals))

	for name, token := range refusals {
		purpose := identity.TokenPasswordReset
		if name == "minted for another purpose" {
			purpose = identity.TokenEmailConfirmation
		}

		_, err := c.authenticator.Redeem(c.ctx(), purpose, token)

		c.Require().ErrorIsf(err, identity.ErrInvalidToken, "a link %s was not refused as one refusal", name)

		messages = append(messages, err.Error())
	}

	for _, message := range messages {
		c.Assert().Equal(messages[0], message,
			"the refusals read differently, so which links once existed can be read out of the difference")
	}
}
