package identity_test

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/service/identity"
	"medikube/internal/service/identity/identitytest"
)

const requestID = "req-identity-service"

type harness struct {
	service       *identity.Service
	repository    *identitytest.Repository
	authenticator *identitytest.Authenticator
	mailer        *identitytest.Mailer
	auditor       *identitytest.Auditor
	account       domainidentity.User
}

// newHarness wires the service against the fakes with registration CLOSED,
// which is the operator default (FR-002) and therefore the state a test has to
// opt out of rather than into.
func newHarness(t *testing.T) harness { return wire(t, false) }

func newOpenHarness(t *testing.T) harness { return wire(t, true) }

// newOpenHarnessWithLocalePredicate is registration-open plus a
// SupportedLocale, for T035's Register-locale tests: those need both an open
// door and a catalogue to fall back against.
func newOpenHarnessWithLocalePredicate(t *testing.T, predicate func(string) bool) harness {
	t.Helper()

	repository := identitytest.NewRepository()
	authenticator := identitytest.NewAuthenticator(repository)
	mailer := identitytest.NewMailer()
	auditor := identitytest.NewAuditor()

	service, err := identity.New(identity.Config{
		Repository:       repository,
		Authenticator:    authenticator,
		Mailer:           mailer,
		Auditor:          auditor,
		Clock:            identitytest.NewClock(),
		RegistrationOpen: true,
		SupportedLocale:  predicate,
	})
	require.NoError(t, err)

	return harness{service: service, repository: repository, authenticator: authenticator, mailer: mailer, auditor: auditor}
}

func wire(t *testing.T, registrationOpen bool) harness {
	t.Helper()

	repository := identitytest.NewRepository()
	authenticator := identitytest.NewAuthenticator(repository)
	mailer := identitytest.NewMailer()
	auditor := identitytest.NewAuditor()

	service, err := identity.New(identity.Config{
		Repository:       repository,
		Authenticator:    authenticator,
		Mailer:           mailer,
		Auditor:          auditor,
		Clock:            identitytest.NewClock(),
		RegistrationOpen: registrationOpen,
	})
	require.NoError(t, err)

	account, err := repository.Create(t.Context(), domainidentity.User{
		Email:      identitytest.Email,
		Name:       identitytest.Name,
		Role:       domainidentity.DefaultRole,
		UnitSystem: domainidentity.DefaultUnitSystem,
		Locale:     domainidentity.DefaultLocale,
		DateFormat: domainidentity.DefaultDateFormat,
		Theme:      domainidentity.DefaultTheme,
	}, identitytest.Password)
	require.NoError(t, err)

	repository.Forget()
	authenticator.Forget()

	return harness{
		service:       service,
		repository:    repository,
		authenticator: authenticator,
		mailer:        mailer,
		auditor:       auditor,
		account:       account,
	}
}

func (h harness) actor() access.Actor {
	return access.Actor{UserID: h.account.ID, RequestID: requestID}
}

// only is the one audit row a call was expected to write, with the constants
// every identity row shares asserted once here rather than in every case.
func (h harness) only(t *testing.T, action audit.Action) audit.Event {
	t.Helper()

	events := h.auditor.Events()
	require.Len(t, events, 1, "the call wrote %d audit rows: %v", len(events), h.auditor.Actions())

	event := events[0]
	assert.Equal(t, action, event.Action)
	assert.Equal(t, audit.TargetKindUser, event.TargetKind, "an identity row is about a user (FR-036)")
	assert.Equal(t, audit.ActorKindUser, event.ActorKind)
	assert.Equal(t, requestID, event.RequestID)
	assert.NoError(t, event.Validate(), "the row the service builds is one the store would refuse")

	return event
}

// TestNewRefusesAnIncompleteService. A nil port is a service that would panic on
// its first request, after the process has been accepting traffic for however
// long it took somebody to reach it.
func TestNewRefusesAnIncompleteService(t *testing.T) {
	t.Parallel()

	complete := identity.Config{
		Repository:    identitytest.NewRepository(),
		Authenticator: identitytest.NewAuthenticator(identitytest.NewRepository()),
		Mailer:        identitytest.NewMailer(),
		Auditor:       identitytest.NewAuditor(),
		Clock:         identitytest.NewClock(),
	}

	require.NotNil(t, must(t, complete), "the complete configuration was itself refused")

	cases := map[string]func(*identity.Config){
		"no repository":    func(c *identity.Config) { c.Repository = nil },
		"no authenticator": func(c *identity.Config) { c.Authenticator = nil },
		"no mailer":        func(c *identity.Config) { c.Mailer = nil },
		"no auditor":       func(c *identity.Config) { c.Auditor = nil },
		"no clock":         func(c *identity.Config) { c.Clock = nil },
	}

	for name, remove := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			incomplete := complete
			remove(&incomplete)

			service, err := identity.New(incomplete)
			require.Error(t, err)
			assert.Nil(t, service, "an incomplete service was returned beside the error")
		})
	}
}

func must(t *testing.T, cfg identity.Config) *identity.Service {
	t.Helper()

	service, err := identity.New(cfg)
	require.NoError(t, err)

	return service
}

// TestTheZeroConfigurationIsClosed. FR-002's default is closed, and a
// composition root that forgets to say must get the safe answer rather than an
// instance that accepts accounts from strangers.
func TestTheZeroConfigurationIsClosed(t *testing.T) {
	t.Parallel()

	assert.False(t, newHarness(t).service.RegistrationOpen())
}

func TestRegisterCreatesTheAccountAndRecordsIt(t *testing.T) {
	t.Parallel()

	h := newOpenHarness(t)

	created, err := h.service.Register(t.Context(), access.Anonymous(requestID), identity.Registration{
		Email:    "  new@example.test  ",
		Name:     "  New Person  ",
		Password: identitytest.Password,
	})
	require.NoError(t, err)

	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "new@example.test", created.Email, "the stored address kept the spaces around it")
	assert.Equal(t, "New Person", created.Name)
	assert.Equal(t, domainidentity.DefaultRole, created.Role)
	assert.Equal(t, domainidentity.DefaultUnitSystem, created.UnitSystem)
	assert.Equal(t, domainidentity.DefaultLocale, created.Locale)
	assert.Equal(t, domainidentity.DefaultDateFormat, created.DateFormat)
	assert.Equal(t, domainidentity.DefaultTheme, created.Theme)
	assert.False(t, created.EmailConfirmed, "registering confirmed the address by itself (FR-075)")

	event := h.only(t, audit.ActionCreate)
	assert.Equal(t, created.ID, event.TargetID)
	assert.Equal(t, created.ID, event.ActorID, "the account that was just created is who created it")
}

// TestRegisterAppliesTheSubmittedLocale is FR-004 and T035: a registration
// carrying a locale the instance ships stores it on the new account rather
// than DefaultLocale.
func TestRegisterAppliesTheSubmittedLocale(t *testing.T) {
	t.Parallel()

	h := newOpenHarnessWithLocalePredicate(t, func(locale string) bool { return locale == "pl" })

	created, err := h.service.Register(t.Context(), access.Anonymous(requestID), identity.Registration{
		Email:    "new@example.test",
		Name:     "New Person",
		Password: identitytest.Password,
		Locale:   "pl",
	})
	require.NoError(t, err)
	assert.Equal(t, "pl", created.Locale)
}

// TestRegisterFallsBackToDefaultLocale is D-10: an empty locale, or one the
// instance does not ship a catalogue for, falls back to
// identity.DefaultLocale rather than refusing the sign-up.
func TestRegisterFallsBackToDefaultLocale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		locale string
	}{
		{"empty", ""},
		{"unsupported", "xx"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newOpenHarnessWithLocalePredicate(t, func(locale string) bool { return locale == "pl" })

			created, err := h.service.Register(t.Context(), access.Anonymous(requestID), identity.Registration{
				Email:    test.name + "@example.test",
				Name:     "New Person",
				Password: identitytest.Password,
				Locale:   test.locale,
			})
			require.NoError(t, err)
			assert.Equal(t, domainidentity.DefaultLocale, created.Locale)
		})
	}
}

// TestRegistrationIsRefusedWhenTheOperatorHasClosedIt is FR-002, and the second
// half is the half that matters: a refused registration leaves nothing behind.
func TestRegistrationIsRefusedWhenTheOperatorHasClosedIt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.Register(t.Context(), access.Anonymous(requestID), identity.Registration{
		Email:    "stranger@example.test",
		Name:     "A Stranger",
		Password: identitytest.Password,
	})

	require.ErrorIs(t, err, identity.ErrRegistrationClosed)
	assert.ErrorIs(t, err, domain.ErrForbidden,
		"an edge that has not learned the specific code would answer 500 rather than a refusal")
	assert.Empty(t, h.repository.Writes(), "a closed instance created an account anyway")
	assert.Empty(t, h.auditor.Events())
}

// TestAClosedInstanceRefusesBeforeItLooksAtTheSubmission. A closed instance
// that answered `422 invalid email` for one body and `403 registration_closed`
// for another would be running its validator for anonymous callers, and would
// say which addresses parse.
func TestAClosedInstanceRefusesBeforeItLooksAtTheSubmission(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.service.Register(t.Context(), access.Anonymous(requestID), identity.Registration{})

	require.ErrorIs(t, err, identity.ErrRegistrationClosed)

	var invalid *domain.ValidationError
	assert.NotErrorAs(t, err, &invalid, "the closed instance validated the submission and said so")
	assert.Empty(t, h.repository.Calls(), "the closed instance went to the store")
}

// TestRegisterRefusesAnAddressThatAlreadyHasAnAccount is FR-003. The refusal is
// a conflict and the message names no address: "that address cannot be used"
// rather than "amara@x.test is already registered", because the second confirms
// to an anonymous caller that a specific person has an account here (D16).
func TestRegisterRefusesAnAddressThatAlreadyHasAnAccount(t *testing.T) {
	t.Parallel()

	h := newOpenHarness(t)

	for _, spelling := range []string{identitytest.Email, strings.ToUpper(identitytest.Email)} {
		_, err := h.service.Register(t.Context(), access.Anonymous(requestID), identity.Registration{
			Email:    spelling,
			Name:     "An Impostor",
			Password: identitytest.Password,
		})

		require.ErrorIsf(t, err, domain.ErrConflict, "%s was accepted as a second account", spelling)
		assert.NotContains(t, err.Error(), identitytest.Email,
			"the refusal carries the address, which reaches the log stream")
	}
}

// TestRegisterReportsEveryOffendingFieldAtOnce is FR-027 across two validators —
// the account's and the password's. A person fixing one problem per round trip
// because the server only mentioned one is four wasted attempts to say what it
// knew at the first.
func TestRegisterReportsEveryOffendingFieldAtOnce(t *testing.T) {
	t.Parallel()

	h := newOpenHarness(t)

	_, err := h.service.Register(t.Context(), access.Anonymous(requestID), identity.Registration{
		Email:    "not-an-address",
		Name:     "   ",
		Password: "short",
	})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)

	fields := make([]string, 0, len(invalid.Fields))
	for _, field := range invalid.Fields {
		fields = append(fields, field.Field)
	}

	assert.ElementsMatch(t, []string{"email", "name", domainidentity.FieldPassword}, fields)
	assert.Empty(t, h.repository.Writes())
}

// TestARegistrationCanCarryNothingButTheFourThingsFRoneAsksFor is FR-012
// enforced by shape rather than by a check somebody can forget: there is no
// member on the type a role or an account status could arrive in, so a caller
// inside this process cannot promote itself either. Locale is the one
// addition FR-004 asks for.
func TestARegistrationCanCarryNothingButTheFourThingsFRoneAsksFor(t *testing.T) {
	t.Parallel()

	assert.ElementsMatch(t, []string{"Email", "Name", "Password", "Locale"}, membersOf(reflect.TypeOf(identity.Registration{})))
}

// TestAProfileCanCarryNothingButTheFiveThingsFRelevenAsksFor. Same enforcement,
// the other end of the account's life (FR-011, FR-012, contracts/account.md).
func TestAProfileCanCarryNothingButTheFiveThingsFRelevenAsksFor(t *testing.T) {
	t.Parallel()

	assert.ElementsMatch(t,
		[]string{"Name", "UnitSystem", "Locale", "DateFormat", "Theme"},
		membersOf(reflect.TypeOf(identity.Profile{})))
}

func membersOf(structure reflect.Type) []string {
	found := make([]string, 0, structure.NumField())
	for i := range structure.NumField() {
		found = append(found, structure.Field(i).Name)
	}

	return found
}

func TestUpdateProfileChangesTheFiveThingsAPersonMayChangeAndRecordsIt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	name, locale := "Renamed Person", "en-GB"
	units, format, theme :=
		domainidentity.UnitSystemImperial, domainidentity.DateFormatDMY, domainidentity.ThemeDark

	updated, err := h.service.UpdateProfile(t.Context(), h.actor(), identity.Profile{
		Name:       &name,
		UnitSystem: &units,
		Locale:     &locale,
		DateFormat: &format,
		Theme:      &theme,
	})
	require.NoError(t, err)

	assert.Equal(t, name, updated.Name)
	assert.Equal(t, units, updated.UnitSystem)
	assert.Equal(t, locale, updated.Locale)
	assert.Equal(t, format, updated.DateFormat)
	assert.Equal(t, theme, updated.Theme)

	stored, exists := h.repository.Stored(h.account.ID)
	require.True(t, exists)
	assert.Equal(t, name, stored.Name)

	event := h.only(t, audit.ActionUpdate)
	assert.Equal(t, h.account.ID, event.TargetID)
	assert.Equal(t, h.account.ID, event.ActorID)
}

// TestUpdateProfileLeavesTheFieldsFRtwelveForbidsAlone. The patch has no member
// for any of them, so this is what the stored record looks like after the only
// path a person has to it — and it is asserted against an account that is an
// admin, has a confirmed address and is out of service, so that every one of
// the four has a value there is something to lose.
func TestUpdateProfileLeavesTheFieldsFRtwelveForbidsAlone(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	privileged := h.account
	privileged.Role = domainidentity.RoleAdmin
	privileged.EmailConfirmed = true

	seeded, err := h.repository.Update(t.Context(), privileged)
	require.NoError(t, err)
	h.repository.Forget()

	name := "Renamed"

	updated, err := h.service.UpdateProfile(t.Context(), h.actor(), identity.Profile{Name: &name})
	require.NoError(t, err)

	assert.Equal(t, name, updated.Name)
	assert.Equal(t, domainidentity.RoleAdmin, updated.Role, "the profile change moved the permission tier")
	assert.Equal(t, seeded.Email, updated.Email, "the profile change moved the sign-in address")
	assert.True(t, updated.EmailConfirmed, "the profile change moved the confirmed state")
	assert.Equal(t, seeded.CreatedAt.UTC(), updated.CreatedAt.UTC())
}

// TestUpdateProfileRefusesAValueOutsideAPublishedVocabulary, and writes nothing.
func TestUpdateProfileRefusesAValueOutsideAPublishedVocabulary(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	theme := domainidentity.Theme("neon")

	_, err := h.service.UpdateProfile(t.Context(), h.actor(), identity.Profile{Theme: &theme})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Len(t, invalid.Fields, 1)
	assert.Equal(t, "theme", invalid.Fields[0].Field)
	assert.Equal(t, domain.CodeInvalidValue, invalid.Fields[0].Code)
	assert.Empty(t, h.repository.Writes(), "the refused change was written anyway")
	assert.Empty(t, h.auditor.Events())
}

// TestUpdateProfileChecksTheLocaleAgainstTheConfiguredPredicate, T017: the
// membership check is a fake predicate here, never i18n.IsSupported — this
// package imports no such thing.
func TestUpdateProfileChecksTheLocaleAgainstTheConfiguredPredicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		predicate func(string) bool
		wantErr   bool
	}{
		{name: "the predicate accepts the locale", predicate: func(string) bool { return true }},
		{name: "the predicate rejects the locale", predicate: func(string) bool { return false }, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := wireWithLocalePredicate(t, test.predicate)

			locale := "xx"
			_, err := h.service.UpdateProfile(t.Context(), h.actor(), identity.Profile{Locale: &locale})

			if !test.wantErr {
				require.NoError(t, err)

				return
			}

			var invalid *domain.ValidationError
			require.ErrorAs(t, err, &invalid)
			require.Len(t, invalid.Fields, 1)
			assert.Equal(t, "locale", invalid.Fields[0].Field)
			assert.Equal(t, domain.CodeInvalidValue, invalid.Fields[0].Code)
			assert.Empty(t, h.repository.Writes(), "the refused change was written anyway")
		})
	}
}

// TestUpdateProfileWithNoConfiguredPredicateAcceptsAnyFormedLocale: a harness
// wired with no SupportedLocale at all — every other test in this file — must
// keep accepting whatever identity.User.Validate's own format rule lets
// through, so this package's many other UpdateProfile tests do not have to
// know about a catalogue they never asked about.
func TestUpdateProfileWithNoConfiguredPredicateAcceptsAnyFormedLocale(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	locale := "xx"
	updated, err := h.service.UpdateProfile(t.Context(), h.actor(), identity.Profile{Locale: &locale})
	require.NoError(t, err)
	assert.Equal(t, locale, updated.Locale)
}

func wireWithLocalePredicate(t *testing.T, predicate func(string) bool) harness {
	t.Helper()

	repository := identitytest.NewRepository()
	authenticator := identitytest.NewAuthenticator(repository)
	mailer := identitytest.NewMailer()
	auditor := identitytest.NewAuditor()

	service, err := identity.New(identity.Config{
		Repository:      repository,
		Authenticator:   authenticator,
		Mailer:          mailer,
		Auditor:         auditor,
		Clock:           identitytest.NewClock(),
		SupportedLocale: predicate,
	})
	require.NoError(t, err)

	account, err := repository.Create(t.Context(), domainidentity.User{
		Email:      identitytest.Email,
		Name:       identitytest.Name,
		Role:       domainidentity.DefaultRole,
		UnitSystem: domainidentity.DefaultUnitSystem,
		Locale:     domainidentity.DefaultLocale,
		DateFormat: domainidentity.DefaultDateFormat,
		Theme:      domainidentity.DefaultTheme,
	}, identitytest.Password)
	require.NoError(t, err)

	repository.Forget()
	authenticator.Forget()

	return harness{
		service:       service,
		repository:    repository,
		authenticator: authenticator,
		mailer:        mailer,
		auditor:       auditor,
		account:       account,
	}
}

func TestChangePasswordReplacesTheCredentialAndRecordsIt(t *testing.T) {
	t.Parallel()

	const replacement = "a-second-perfectly-ordinary-passphrase"

	h := newHarness(t)

	require.NoError(t, h.service.ChangePassword(t.Context(), h.actor(), identitytest.Password, replacement))

	require.NoError(t, h.authenticator.Verify(t.Context(), h.account.ID, replacement))
	assert.ErrorIs(t, h.authenticator.Verify(t.Context(), h.account.ID, identitytest.Password),
		domain.ErrUnauthenticated, "the password that was replaced still works")

	event := h.only(t, audit.ActionPasswordChange)
	assert.Equal(t, h.account.ID, event.TargetID)
}

// TestAPasswordChangeEndsEverySessionIssuedBeforeIt is FR-010 at the layer that
// triggers it. The rotation is one write inside SetPassword, and what this
// asserts is that ChangePassword has no other way to a new password: a service
// that wrote the credential through the repository would leave every stolen
// session alive and nothing downstream would say so.
//
// It is observed through a link minted before the change, because a link is
// signed with the same record key a session token is (research D-16). T200
// asserts the same rotation against a real instance and a real token.
func TestAPasswordChangeEndsEverySessionIssuedBeforeIt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	before, err := h.authenticator.Token(identity.TokenPasswordReset, h.account.ID)
	require.NoError(t, err)

	_, err = h.authenticator.Redeem(t.Context(), identity.TokenPasswordReset, before)
	require.NoError(t, err, "the link did not resolve before the change, so the assertion below is vacuous")

	require.NoError(t, h.service.ChangePassword(
		t.Context(), h.actor(), identitytest.Password, "a-second-perfectly-ordinary-passphrase"))

	assert.Contains(t, h.authenticator.Calls(), "set_password",
		"the password was changed through some path other than the one that rotates the record key")

	_, err = h.authenticator.Redeem(t.Context(), identity.TokenPasswordReset, before)
	assert.ErrorIs(t, err, identity.ErrInvalidToken,
		"a credential issued before the password change still resolves")
}

// TestAPasswordChangeRefusalDoesNotSayWhichHalfFailed is T197 and FR-009.
//
// The three refusals are compared as whole values, so a message, a field name
// or a code that differed between them would fail. One message for both halves
// is the requirement; the rules themselves are published by
// GET /api/v1/auth/config, so a person is told what a password must look like
// without having to fail at one.
func TestAPasswordChangeRefusalDoesNotSayWhichHalfFailed(t *testing.T) {
	t.Parallel()

	cases := map[string]struct{ current, next string }{
		"the current password is wrong":        {"not-the-password-on-the-account", "a-second-perfectly-ordinary-passphrase"},
		"the current password is absent":       {"", "a-second-perfectly-ordinary-passphrase"},
		"the new password fails the rules":     {identitytest.Password, "short"},
		"the new password is the address":      {identitytest.Password, identitytest.Email},
		"both the current and the new are bad": {"not-the-password-on-the-account", "short"},
	}

	refusals := make(map[string]string, len(cases))

	for name, attempt := range cases {
		h := newHarness(t)

		err := h.service.ChangePassword(t.Context(), h.actor(), attempt.current, attempt.next)

		var invalid *domain.ValidationError
		require.ErrorAsf(t, err, &invalid, "%s was not refused as a validation failure", name)

		refusals[name] = string(mustMarshal(t, invalid))

		assert.Empty(t, h.repository.Writes(), "%s changed the password anyway", name)
		assert.Empty(t, h.auditor.Events(), "%s recorded a password change that did not happen", name)
		require.NoError(t, h.authenticator.Verify(t.Context(), h.account.ID, identitytest.Password),
			"%s replaced the credential", name)
	}

	var first string
	for _, refusal := range refusals {
		first = refusal

		break
	}

	for name, refusal := range refusals {
		assert.Equalf(t, first, refusal,
			"%s is answered differently, so a caller can read which half it got right", name)
	}
}

// TestAPasswordThatCouldNotBeCheckedIsNotARefusal. A store that did not answer
// has refused nobody, and answering "wrong password" on the strength of a
// failed read would tell somebody their password had changed.
func TestAPasswordThatCouldNotBeCheckedIsNotARefusal(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.authenticator.Fail(io.ErrUnexpectedEOF)

	err := h.service.ChangePassword(
		t.Context(), h.actor(), identitytest.Password, "a-second-perfectly-ordinary-passphrase")

	require.Error(t, err)

	var invalid *domain.ValidationError
	assert.NotErrorAs(t, err, &invalid, "a failed check was answered as the person's mistake")
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestDeleteAccountRemovesTheAccountAndRecordsIt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	require.NoError(t, h.service.DeleteAccount(
		t.Context(), h.actor(), identitytest.Password, domainidentity.DeleteConfirmationPhrase))

	_, exists := h.repository.Stored(h.account.ID)
	assert.False(t, exists)

	event := h.only(t, audit.ActionAccountDelete)
	assert.Equal(t, h.account.ID, event.TargetID,
		"the row does not name the account, so nothing survives to say whose deletion it was")
}

// TestDeleteAccountRefusesAnythingButTheExactPhrase is FR-013 and T193. An
// irreversible act asks for a deliberate one in return: a phrase that is
// trimmed or folded is a phrase somebody types by accident.
func TestDeleteAccountRefusesAnythingButTheExactPhrase(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"lower case":        strings.ToLower(domainidentity.DeleteConfirmationPhrase),
		"a trailing space":  domainidentity.DeleteConfirmationPhrase + " ",
		"a leading space":   " " + domainidentity.DeleteConfirmationPhrase,
		"empty":             "",
		"a near miss":       "DELETE MY ACCOUNTS",
		"the words in yes":  "yes",
		"an inner run of  ": "DELETE  MY ACCOUNT",
	}

	for name, phrase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			err := h.service.DeleteAccount(t.Context(), h.actor(), identitytest.Password, phrase)

			var invalid *domain.ValidationError
			require.ErrorAs(t, err, &invalid)
			require.Len(t, invalid.Fields, 1)
			assert.Equal(t, "confirmation", invalid.Fields[0].Field)
			assert.Equal(t, "mismatch", invalid.Fields[0].Code)

			_, exists := h.repository.Stored(h.account.ID)
			assert.True(t, exists, "the account was deleted on a confirmation nobody typed")
			assert.Empty(t, h.auditor.Events())
		})
	}
}

// TestDeleteAccountReportsAWrongPasswordAndAWrongPhraseTogether. Both are the
// caller's own account and neither is an oracle, so FR-027's "every offending
// field at once" applies here as it does to any other form.
func TestDeleteAccountReportsAWrongPasswordAndAWrongPhraseTogether(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	err := h.service.DeleteAccount(t.Context(), h.actor(), "not-the-password-on-the-account", "no")

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)

	fields := make([]string, 0, len(invalid.Fields))
	for _, field := range invalid.Fields {
		fields = append(fields, field.Field)
	}

	assert.ElementsMatch(t, []string{"confirmation", "password"}, fields)

	_, exists := h.repository.Stored(h.account.ID)
	assert.True(t, exists)
}

// TestTheDeletionRowIsWrittenBeforeTheAccountIsGone. `audit_events.actor` does
// not cascade, so the row outlives the account — but only if it was written
// while the account still existed. A trail that cannot be written must stop the
// deletion rather than lose the only evidence it happened (research D-22).
func TestTheDeletionRowIsWrittenBeforeTheAccountIsGone(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.auditor.Fail(io.ErrClosedPipe)

	err := h.service.DeleteAccount(
		t.Context(), h.actor(), identitytest.Password, domainidentity.DeleteConfirmationPhrase)

	require.ErrorIs(t, err, io.ErrClosedPipe)

	_, exists := h.repository.Stored(h.account.ID)
	assert.True(t, exists, "the account was deleted and nothing recorded that it had been")
	assert.NotContains(t, h.repository.Writes(), "delete")
}

// TestAnUnknownAddressAndAWrongPasswordAreOneRefusal is FR-005 at the service.
// The errors are compared as whole strings because this is what reaches the one
// log stream: two messages that differ say which addresses are registered to
// anybody who can read it.
func TestAnUnknownAddressAndAWrongPasswordAreOneRefusal(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	disabled := h.account
	disabled.Email = "out-of-service@example.test"
	disabled.DisabledAt = time.Date(2026, time.March, 3, 9, 0, 0, 0, time.UTC)
	disabled.ID = ""

	stored, err := h.repository.Create(t.Context(), disabled, identitytest.Password)
	require.NoError(t, err)
	h.repository.Forget()

	attempts := map[string]identity.Credentials{
		"an address with no account":  {Email: identitytest.StrangerEmail, Password: identitytest.Password},
		"the wrong password":          {Email: identitytest.Email, Password: "not-the-password-on-the-account"},
		"an account out of service":   {Email: stored.Email, Password: identitytest.Password},
		"an empty address":            {Email: "", Password: identitytest.Password},
		"an address that is not one":  {Email: "not-an-address", Password: identitytest.Password},
		"an account and no password ": {Email: identitytest.Email, Password: ""},
	}

	messages := make(map[string]string, len(attempts))

	for name, credentials := range attempts {
		user, err := h.service.SignIn(t.Context(), access.Anonymous(requestID), credentials)

		require.ErrorIsf(t, err, domain.ErrUnauthenticated, "%s was not refused", name)
		assert.Emptyf(t, user.ID, "%s came back with an account attached", name)

		messages[name] = err.Error()
	}

	for name, message := range messages {
		assert.Equalf(t, messages["an address with no account"], message,
			"%s reads differently from an address with no account", name)
		assert.NotContainsf(t, message, identitytest.Email, "%s carries the address into the log", name)
	}
}

// TestEverySignInRefusalCostsOneComparison is T202's mechanism at this layer,
// and it is a count rather than a clock: the latency is not deterministic and
// the count is (Constitution VIII, research D-17).
//
// An address with no account must still pay for the comparison a wrong password
// pays for, or the refusal can be told apart by how long it took however
// identical the body is.
func TestEverySignInRefusalCostsOneComparison(t *testing.T) {
	t.Parallel()

	cases := map[string]identity.Credentials{
		"an address with no account": {Email: identitytest.StrangerEmail, Password: identitytest.Password},
		"the wrong password":         {Email: identitytest.Email, Password: "not-the-password-on-the-account"},
	}

	for name, credentials := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			_, err := h.service.SignIn(t.Context(), access.Anonymous(requestID), credentials)
			require.ErrorIs(t, err, domain.ErrUnauthenticated)

			assert.Equal(t, 1, h.authenticator.Comparisons(),
				"the refusal cost a different number of comparisons than the other one, which is measurable")
		})
	}

	unknown := newHarness(t)
	_, err := unknown.service.SignIn(t.Context(), access.Anonymous(requestID), cases["an address with no account"])
	require.Error(t, err)

	assert.Equal(t, 1, unknown.authenticator.DummyComparisons(),
		"the address with no account skipped the fixed dummy comparison, which is the whole of the defence")
}

// TestASuccessfulSignInWritesNoLoginRow, because the hook does (research D-14).
//
// PocketBase's own auth route stays reachable, so a `login` row written here
// would leave every sign-in through that route unaudited while looking exactly
// like coverage. The row is written from OnRecordAuthRequest, which fires for
// both paths (T205), and this is what stops anybody quietly adding a second.
func TestASuccessfulSignInWritesNoLoginRow(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	user, err := h.service.SignIn(t.Context(), access.Anonymous(requestID), identity.Credentials{
		Email:    identitytest.Email,
		Password: identitytest.Password,
	})
	require.NoError(t, err)
	assert.Equal(t, h.account.ID, user.ID)

	assert.Empty(t, h.auditor.Actions(),
		"the service wrote a sign-in row, so PocketBase's own auth route is silently unaudited")
}

// TestEveryRefusedSignInIsRecordedWithoutTheAddress is FR-006 and FR-077. The
// row names the account somebody aimed at and never the string they typed:
// writing that would put a real person's address — possibly a stranger's,
// possibly a typo of one — into a two-year medical audit trail.
func TestEveryRefusedSignInIsRecordedWithoutTheAddress(t *testing.T) {
	t.Parallel()

	t.Run("an address with an account", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		_, err := h.service.SignIn(t.Context(), access.Anonymous(requestID), identity.Credentials{
			Email:    identitytest.Email,
			Password: "not-the-password-on-the-account",
		})
		require.Error(t, err)

		event := h.only(t, audit.ActionLoginFailed)
		assert.Equal(t, h.account.ID, event.TargetID)
		assert.Empty(t, event.ActorID, "an anonymous attempt was attributed to the account it was aimed at")
	})

	t.Run("an address with none", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		_, err := h.service.SignIn(t.Context(), access.Anonymous(requestID), identity.Credentials{
			Email:    identitytest.StrangerEmail,
			Password: identitytest.Password,
		})
		require.Error(t, err)

		event := h.only(t, audit.ActionLoginFailed)
		assert.Empty(t, event.TargetID, "there is no account to name, so the row names none")

		for _, field := range []string{event.TargetID, event.ActorID, event.RequestID} {
			assert.NotContains(t, field, "@", "an address reached the audit trail")
		}
	})
}

// TestAnAuthenticatorThatAnswersWithNoAccountAndNoErrorHasRefused. The account
// is read and not only the error: an implementation that returned a zero user
// and nil would otherwise sign in an empty id, and the edge would mint a token
// for it.
func TestAnAuthenticatorThatAnswersWithNoAccountAndNoErrorHasRefused(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	service, err := identity.New(identity.Config{
		Repository:    h.repository,
		Authenticator: hollowAuthenticator{},
		Mailer:        h.mailer,
		Auditor:       h.auditor,
		Clock:         identitytest.NewClock(),
	})
	require.NoError(t, err)

	user, err := service.SignIn(t.Context(), access.Anonymous(requestID), identity.Credentials{
		Email:    identitytest.Email,
		Password: identitytest.Password,
	})

	require.ErrorIs(t, err, domain.ErrUnauthenticated)
	assert.Empty(t, user.ID)
}

// hollowAuthenticator answers every question with a zero value and no error,
// which is what a half-written implementation looks like.
type hollowAuthenticator struct{}

func (hollowAuthenticator) Authenticate(context.Context, string, string) (domainidentity.User, error) {
	return domainidentity.User{}, nil
}
func (hollowAuthenticator) Verify(context.Context, string, string) error      { return nil }
func (hollowAuthenticator) SetPassword(context.Context, string, string) error { return nil }
func (hollowAuthenticator) EndSessions(context.Context, string) error         { return nil }
func (hollowAuthenticator) Redeem(context.Context, identity.TokenPurpose, string) (domainidentity.User, error) {
	return domainidentity.User{}, nil
}

// TestACredentialThatCouldNotBeCheckedIsNotARefusal. A database that did not
// answer has refused nobody; reporting it as a wrong password would fill the
// trail with failures nobody attempted and hide the outage behind a 401.
func TestACredentialThatCouldNotBeCheckedIsNotARefusal(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.authenticator.Fail(io.ErrUnexpectedEOF)

	_, err := h.service.SignIn(t.Context(), access.Anonymous(requestID), identity.Credentials{
		Email:    identitytest.Email,
		Password: identitytest.Password,
	})

	require.Error(t, err)
	assert.NotErrorIs(t, err, domain.ErrUnauthenticated)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.Empty(t, h.auditor.Events(), "an outage was recorded as somebody's failed sign-in")
}

// TestSignOutEndsEverySessionAndRecordsIt is FR-007: the ended session must not
// be usable again from anywhere it was still open, which is one rotation of the
// record's key rather than a revocation list (research D-16).
func TestSignOutEndsEverySessionAndRecordsIt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	before, err := h.authenticator.Token(identity.TokenPasswordReset, h.account.ID)
	require.NoError(t, err)

	require.NoError(t, h.service.SignOut(t.Context(), h.actor()))

	assert.Contains(t, h.authenticator.Calls(), "end_sessions")

	_, err = h.authenticator.Redeem(t.Context(), identity.TokenPasswordReset, before)
	assert.ErrorIs(t, err, identity.ErrInvalidToken, "a credential issued before the sign-out still resolves")

	event := h.only(t, audit.ActionLogout)
	assert.Equal(t, h.account.ID, event.TargetID)
	assert.Equal(t, h.account.ID, event.ActorID)
}

// TestSignOutEndsTheSessionEvenWhenTheTrailCannotBeWritten. The rotation
// happens first on purpose: a trail that cannot be written must leave the
// session ended rather than leave it open, and the unwritten row still reaches
// the caller and the log as an error.
func TestSignOutEndsTheSessionEvenWhenTheTrailCannotBeWritten(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.auditor.Fail(io.ErrClosedPipe)

	before, err := h.authenticator.Token(identity.TokenPasswordReset, h.account.ID)
	require.NoError(t, err)

	require.ErrorIs(t, h.service.SignOut(t.Context(), h.actor()), io.ErrClosedPipe)

	_, err = h.authenticator.Redeem(t.Context(), identity.TokenPasswordReset, before)
	assert.ErrorIs(t, err, identity.ErrInvalidToken, "the session outlived the sign-out because a row would not write")
}

// TestMeIsTheAccountBehindTheSession.
func TestMeIsTheAccountBehindTheSession(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	me, err := h.service.Me(t.Context(), h.actor())
	require.NoError(t, err)

	assert.Equal(t, h.account.ID, me.ID)
	assert.Equal(t, identitytest.Email, me.Email)
	assert.Empty(t, h.auditor.Events(), "reading one's own profile is not an event")
}

// TestAnAccountAnOperatorTookOutOfServiceReachesNothing. data-model §1 makes a
// non-zero `disabled_at` refuse a sign-in; PocketBase's token validation never
// looks at the column, so without this the account stays fully usable until the
// token expires — up to the whole configured session lifetime.
func TestAnAccountAnOperatorTookOutOfServiceReachesNothing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	disabled := h.account
	disabled.DisabledAt = time.Date(2026, time.April, 4, 9, 0, 0, 0, time.UTC)
	_, err := h.repository.Update(t.Context(), disabled)
	require.NoError(t, err)

	_, err = h.service.Me(t.Context(), h.actor())
	assert.ErrorIs(t, err, domain.ErrUnauthenticated)

	name := "Renamed"
	_, err = h.service.UpdateProfile(t.Context(), h.actor(), identity.Profile{Name: &name})
	assert.ErrorIs(t, err, domain.ErrUnauthenticated)
}

// TestEveryAccountOperationRefusesACallerWithNoMediKubeAccount is deliberately
// structural rather than a check of the methods that exist today.
//
// The walk is driven from the method set, and a method nobody has classified
// fails the test rather than passing by omission — which is the only version of
// this worth having, because the method added next year is the one that will
// forget.
func TestEveryAccountOperationRefusesACallerWithNoMediKubeAccount(t *testing.T) {
	t.Parallel()

	// What each method requires of its caller. A method missing from here is an
	// offence below: "this method needs no account" and "nobody has decided"
	// produce the same green tick otherwise.
	needsAnAccount := map[string]bool{
		"Me":                   true,
		"UpdateProfile":        true,
		"ChangePassword":       true,
		"DeleteAccount":        true,
		"SignOut":              true,
		"RequestVerification":  true,
		"Register":             false,
		"SignIn":               false,
		"RequestPasswordReset": false,
		"ConfirmPasswordReset": false,
		"ConfirmVerification":  false,
		"RegistrationOpen":     false,
	}

	serviceType := reflect.TypeOf(&identity.Service{})
	require.GreaterOrEqual(t, serviceType.NumMethod(), 12,
		"the walk found almost no methods; it is not looking at the service it thinks it is")

	callers := map[string]access.Actor{
		"an anonymous caller": access.Anonymous(requestID),
		"a superuser session": {UserID: "mksuperadmin001", IsSuperuser: true, RequestID: requestID},
	}

	for i := range serviceType.NumMethod() {
		method := serviceType.Method(i)

		t.Run(method.Name, func(t *testing.T) {
			t.Parallel()

			required, classified := needsAnAccount[method.Name]
			require.Truef(t, classified,
				"%s is not classified: say whether it needs a MediKube account and assert it here", method.Name)

			if !required {
				return
			}

			for name, caller := range callers {
				h := newHarness(t)

				results := call(t, h.service, method, caller)

				assert.ErrorIsf(t, errorOf(t, results), domain.ErrUnauthenticated,
					"%s answered %s as something other than a caller with no account", method.Name, name)
				assert.Emptyf(t, h.repository.Writes(),
					"%s wrote to the store for %s", method.Name, name)
				assert.Emptyf(t, h.auditor.Events(),
					"%s recorded a row for %s", method.Name, name)
			}
		})
	}
}

// call invokes a method with a context, the supplied actor and the zero value
// of everything else. The zero values are the point: they are refused by
// validation, and validation runs after the caller is checked — so a method
// that reached them at all has already passed the assertion above.
func call(t *testing.T, service *identity.Service, method reflect.Method, actor access.Actor) []reflect.Value {
	t.Helper()

	args := make([]reflect.Value, 0, method.Type.NumIn())
	args = append(args, reflect.ValueOf(service))

	for i := 1; i < method.Type.NumIn(); i++ {
		switch in := method.Type.In(i); {
		case in == reflect.TypeOf((*context.Context)(nil)).Elem():
			args = append(args, reflect.ValueOf(t.Context()))
		case in == reflect.TypeOf(access.Actor{}):
			args = append(args, reflect.ValueOf(actor))
		default:
			args = append(args, reflect.New(in).Elem())
		}
	}

	return method.Func.Call(args)
}

func errorOf(t *testing.T, results []reflect.Value) error {
	t.Helper()

	require.NotEmpty(t, results, "the method returned nothing at all")

	last := results[len(results)-1]
	require.True(t, last.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()),
		"the method's last result is not an error")

	if last.IsNil() {
		return nil
	}

	err, ok := last.Interface().(error)
	require.True(t, ok)

	return err
}

// mustMarshal renders a refusal as the bytes a client would receive, so that a
// message, a field name, a code or their order differing between two refusals
// is a failure rather than something a struct comparison might forgive.
func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()

	encoded, err := json.Marshal(value)
	require.NoError(t, err)

	return encoded
}
