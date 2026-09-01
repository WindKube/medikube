package api_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
)

// T192. contracts/auth.md's five session operations, driven through the whole
// edge: the middleware order, the error envelope, the cookie and the audit
// trail included. Every assertion here is about the WIRE — the status, the
// body, the headers — because that is the only thing a client and an attacker
// can both see.

const (
	newAccountEmail = "dara@example.test"
	newAccountName  = "Dara Ferreira"
	//nolint:gosec // a test credential, not one
	newAccountPassword = "a-perfectly-fine-password"
)

func body(pairs ...string) string {
	members := make([]string, 0, len(pairs)/2)
	for index := 0; index+1 < len(pairs); index += 2 {
		members = append(members, `"`+pairs[index]+`":`+pairs[index+1])
	}

	return "{" + strings.Join(members, ",") + "}"
}

func quoted(value string) string { return `"` + value + `"` }

// The password rules the API publishes are the same values the validator
// enforces, read from one source (FR-004). A second copy of the numbers here
// would be a document that could disagree with the code it describes.
func TestTheInstancePublishesWhatItAllowsAndNothingAboutAnybody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rig  func(*testing.T) *rig
		open bool
	}{
		{name: "closed, which is the default", rig: func(t *testing.T) *rig { return newRig(t) }, open: false},
		{name: "opened by the operator", rig: openRig, open: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			answer := test.rig(t).anonymous().get(authConfigURL)
			require.Equal(t, http.StatusOK, answer.Status, answer.Body)

			var config authConfigDTO
			answer.decode(t, &config)

			assert.Equal(t, test.open, config.RegistrationOpen)

			rules := domainidentity.PublishedPasswordRules()
			assert.Equal(t, rules.MinLength, config.PasswordRules.MinLength)
			assert.Equal(t, rules.MaxLength, config.PasswordRules.MaxLength)
			assert.Equal(t, rules.RejectsEmail, config.PasswordRules.RejectsEmail)
			assert.Equal(t, rules.RejectsName, config.PasswordRules.RejectsName)

			// It is served to a caller with no session, so it must say nothing
			// about who is registered here.
			for _, disclosure := range []string{
				testsupport.AccountAEmail, testsupport.AccountAName, testsupport.AccountAID,
			} {
				assert.NotContainsf(t, answer.Body, disclosure,
					"the public configuration named %q", disclosure)
			}
		})
	}
}

// FR-001: an account is created from an address, a display name and a password.
// Nothing else is required and nothing else is accepted.
func TestRegisteringCreatesAnAccountAndSignsThePersonIn(t *testing.T) {
	t.Parallel()

	instance := openRig(t)

	answer := instance.anonymous().post(registerURL, body(
		"email", quoted(newAccountEmail),
		"name", quoted(newAccountName),
		"password", quoted(newAccountPassword),
	))

	require.Equal(t, http.StatusCreated, answer.Status, answer.Body)
	assert.Equal(t, "/api/v1/me", answer.Header.Get("Location"))

	session := answer.session(t)
	assert.Equal(t, newAccountEmail, session.User.Email)
	assert.Equal(t, newAccountName, session.User.Name)
	assert.Equal(t, string(domainidentity.DefaultRole), session.User.Role)
	assert.Equal(t, string(domainidentity.DefaultTheme), session.User.Theme)
	assert.False(t, session.User.EmailConfirmed, "a new address is not confirmed by asserting it")
	assert.Zero(t, session.User.Counts[kind.Medication.Segment()])
	assert.NotEmpty(t, session.ExpiresAt)

	// The one member the response must NOT carry. research D-15: the token is
	// the HttpOnly cookie and nothing else, because the content security policy
	// grants 'unsafe-eval' and an injected expression that could read it would.
	assert.NotContains(t, answer.Body, `"token"`,
		"the session body carries a token, which any script on the page can read")
	assert.NotContains(t, answer.Body, `"disabled_at"`)

	cookie := answer.sessionCookie(t)
	require.NotNil(t, cookie, "a registration that signs nobody in is not one")
	assert.True(t, cookie.HttpOnly)
	assert.NotEmpty(t, cookie.Value)

	// The cookie ALONE is a session. This is the browser's whole credential, so
	// a test that also sent a bearer token would prove nothing about it.
	me := instance.anonymous().cookieGet(meURL, cookie.Value)
	require.Equal(t, http.StatusOK, me.Status, me.Body)
	assert.Equal(t, session.User.ID, me.me(t).ID)
}

// FR-003: the LOWER(email) unique index makes Amara@… and amara@… the same
// address, so the second attempt is refused whatever case it is typed in — and
// the refusal names no address.
func TestRegisteringAnAddressThatAlreadyHasAnAccountIsRefused(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		address string
	}{
		{name: "as registered", address: testsupport.AccountAEmail},
		{name: "shouted", address: strings.ToUpper(testsupport.AccountAEmail)},
		{name: "mixed", address: "Amara@Example.Test"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			instance := openRig(t)

			answer := instance.anonymous().post(registerURL, body(
				"email", quoted(test.address),
				"name", quoted(newAccountName),
				"password", quoted(newAccountPassword),
			))

			require.Equal(t, http.StatusConflict, answer.Status, answer.Body)
			assert.Equal(t, web.CodeConflict, answer.envelope(t).Error.Code)

			// "That address cannot be used" and never "amara@example.test is
			// already registered": the second confirms to an anonymous caller
			// that a specific person has an account on this instance, which on
			// a self-hosted medical server is a disclosure.
			assert.NotContains(t, strings.ToLower(answer.Body), strings.ToLower(test.address))
			assert.NotContains(t, strings.ToLower(answer.Body), "registered")
			assert.NotContains(t, strings.ToLower(answer.Body), "exists")
		})
	}
}

// FR-012 by shape. `role` is not a member of the request, so it is refused by
// the decoder — and the account, if one were created, would still be a user.
func TestARegistrationCannotNameItsOwnRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		member string
		value  string
	}{
		{name: "role", member: "role", value: quoted(string(domainidentity.RoleAdmin))},
		{name: "verified", member: "verified", value: "true"},
		{name: "disabled_at", member: "disabled_at", value: quoted("")},
		{name: "an unknown member", member: "is_admin", value: "true"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			instance := openRig(t)

			answer := instance.anonymous().post(registerURL, body(
				"email", quoted(newAccountEmail),
				"name", quoted(newAccountName),
				"password", quoted(newAccountPassword),
				test.member, test.value,
			))

			require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
			assert.Equal(t, [][2]string{{test.member, domain.CodeUnknownField}},
				answer.envelope(t).fieldCodes())

			assert.False(t, instance.accountExists(t, newAccountEmail),
				"a refused registration left an account behind")
		})
	}
}

// FR-027: every offending field at once, not one per round trip.
func TestARegistrationReportsEveryProblemAtOnce(t *testing.T) {
	t.Parallel()

	instance := openRig(t)

	answer := instance.anonymous().post(registerURL, body(
		"email", quoted("not-an-address"),
		"name", quoted("  "),
		"password", quoted("short"),
	))

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)

	fields := make([]string, 0, 3)
	for _, reported := range answer.envelope(t).Error.Fields {
		fields = append(fields, reported.Field)
	}

	assert.ElementsMatch(t, []string{"email", "name", domainidentity.FieldPassword}, fields)
	assert.False(t, instance.accountExists(t, "not-an-address"))
}

// contracts/auth.md's Edge Case, asserted against stored data: two simultaneous
// registrations of one address leave exactly one account. The losing side hits
// the unique index, and the whole operation is one save, so there is no
// half-created account to survive.
func TestTwoSimultaneousRegistrationsOfOneAddressLeaveOneAccount(t *testing.T) {
	t.Parallel()

	instance := openRig(t)

	var (
		wait      sync.WaitGroup
		mutex     sync.Mutex
		statuses  []int
		attempts  = 4
		submitted = body(
			"email", quoted(newAccountEmail),
			"name", quoted(newAccountName),
			"password", quoted(newAccountPassword),
		)
	)

	wait.Add(attempts)

	for range attempts {
		go func() {
			defer wait.Done()

			answer := instance.anonymous().post(registerURL, submitted)

			mutex.Lock()
			defer mutex.Unlock()

			statuses = append(statuses, answer.Status)
		}()
	}

	wait.Wait()

	created := 0

	for _, status := range statuses {
		if status == http.StatusCreated {
			created++
		} else {
			assert.Equal(t, http.StatusConflict, status,
				"a losing registration answered with something other than a conflict")
		}
	}

	assert.Equal(t, 1, created, "the address was created more than once")

	records, err := instance.instance.App.FindAllRecords(usersCollection)
	require.NoError(t, err)

	matching := 0

	for _, record := range records {
		if strings.EqualFold(record.Email(), newAccountEmail) {
			matching++
		}
	}

	assert.Equal(t, 1, matching, "the account collection holds more than one row for one address")
}

// FR-005. THE assertion of this file: three different failures, one answer.
func TestAnUnknownAddressAWrongPasswordAndADisabledAccountAreOneRefusal(t *testing.T) {
	t.Parallel()

	instance := newRig(t)
	instance.disable(testsupport.AccountCEmail)

	attempts := []struct {
		name     string
		email    string
		password string
	}{
		{name: "an address with no account", email: "nobody@example.test", password: testsupport.Password},
		{name: "an account with another password", email: testsupport.AccountAEmail, password: "not-the-password"},
		{name: "an account taken out of service", email: testsupport.AccountCEmail, password: testsupport.Password},
	}

	answers := make([]response, 0, len(attempts))

	for _, attempt := range attempts {
		answer := instance.anonymous().post(loginURL, body(
			"email", quoted(attempt.email),
			"password", quoted(attempt.password),
		))

		require.Equalf(t, http.StatusUnauthorized, answer.Status, "%s: %s", attempt.name, answer.Body)
		assert.Equal(t, web.CodeUnauthenticated, answer.envelope(t).Error.Code)
		assert.Nil(t, answer.sessionCookie(t), "%s was answered with a session cookie", attempt.name)

		answers = append(answers, answer)
	}

	first := withoutCorrelationID(answers[0].Body)

	for index, answer := range answers[1:] {
		assert.Equalf(t, first, withoutCorrelationID(answer.Body),
			"%s answers differently from %s", attempts[index+1].name, attempts[0].name)
	}
}

// FR-005's other half: a sign-in that works hands back a session and nothing
// else, and the cookie alone carries it.
func TestASignInAnswersWithASessionAndNoToken(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	answer := instance.anonymous().post(loginURL, body(
		"email", quoted(testsupport.AccountAEmail),
		"password", quoted(testsupport.Password),
	))

	require.Equal(t, http.StatusOK, answer.Status, answer.Body)

	session := answer.session(t)
	assert.Equal(t, testsupport.AccountAID, session.User.ID)
	assert.Equal(t, testsupport.AccountAEmail, session.User.Email)
	assert.Equal(t, testsupport.AccountAMedicationCount, session.User.Counts[kind.Medication.Segment()])
	assert.NotContains(t, answer.Body, `"token"`)

	cookie := answer.sessionCookie(t)
	require.NotNil(t, cookie)
	assert.True(t, cookie.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)

	me := instance.anonymous().cookieGet(meURL, cookie.Value)
	require.Equal(t, http.StatusOK, me.Status, me.Body)
	assert.Equal(t, testsupport.AccountAID, me.me(t).ID)
}

// The address is folded before it is looked up, so the account somebody signs
// in to does not depend on how they typed it (FR-003).
func TestASignInResolvesOneAccountInAnyLetterCase(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	for _, spelling := range []string{
		testsupport.AccountAEmail,
		strings.ToUpper(testsupport.AccountAEmail),
		"Amara@Example.Test",
	} {
		answer := instance.anonymous().post(loginURL, body(
			"email", quoted(spelling),
			"password", quoted(testsupport.Password),
		))

		require.Equalf(t, http.StatusOK, answer.Status, "%s: %s", spelling, answer.Body)
		assert.Equal(t, testsupport.AccountAID, answer.session(t).User.ID)
	}
}

// FR-008. A refresh extends the session and writes a fresh cookie; it is not a
// sign-in and does not pretend to be one.
func TestRefreshingASessionIssuesAFreshCookieForTheSameAccount(t *testing.T) {
	t.Parallel()

	instance := newRig(t)
	signedIn := instance.as(testsupport.AccountAEmail)

	answer := signedIn.post(refreshURL, "")
	require.Equal(t, http.StatusOK, answer.Status, answer.Body)

	assert.Equal(t, testsupport.AccountAID, answer.session(t).User.ID)
	assert.NotContains(t, answer.Body, `"token"`)

	cookie := answer.sessionCookie(t)
	require.NotNil(t, cookie, "a refresh that re-sets nothing has extended nothing")

	me := instance.anonymous().cookieGet(meURL, cookie.Value)
	assert.Equal(t, http.StatusOK, me.Status, me.Body)
}

func TestRefreshingWithoutASessionIsRefused(t *testing.T) {
	t.Parallel()

	answer := newRig(t).anonymous().post(refreshURL, "")

	require.Equal(t, http.StatusUnauthorized, answer.Status, answer.Body)
	assert.Equal(t, web.CodeUnauthenticated, answer.envelope(t).Error.Code)
}

// An account an operator has taken out of service cannot renew its way past the
// disabling. PocketBase's token validation evaluates no collection rule and
// never reads `disabled_at`, so without MediKube's own check the account stays
// fully usable for the rest of its seven-day token.
func TestADisabledAccountCannotRefreshItsWayBackIn(t *testing.T) {
	t.Parallel()

	instance := newRig(t)
	signedIn := instance.as(testsupport.AccountAEmail)

	require.Equal(t, http.StatusOK, signedIn.post(refreshURL, "").Status)

	instance.disable(testsupport.AccountAEmail)

	answer := signedIn.post(refreshURL, "")
	assert.Equal(t, http.StatusUnauthorized, answer.Status, answer.Body)
	assert.Nil(t, answer.sessionCookie(t))
}

// FR-007 in full: signing out on a phone signs out the laptop. The other
// session's next request is refused, and the browser's cookie is emptied.
func TestSigningOutEndsEverySessionTheAccountHadOpen(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	first := instance.as(testsupport.AccountAEmail)
	second := instance.as(testsupport.AccountAEmail)

	require.Equal(t, http.StatusOK, second.get(meURL).Status)

	answer := first.post(logoutURL, "")
	require.Equal(t, http.StatusNoContent, answer.Status, answer.Body)
	assert.Empty(t, answer.Body)

	cleared := answer.sessionCookie(t)
	require.NotNil(t, cleared, "a sign-out that leaves the cookie in place leaves the browser holding a credential")
	assert.Empty(t, cleared.Value)
	// The rendered header and not the parsed struct: net/http writes a negative
	// MaxAge as `Max-Age=0`, and reads `Max-Age=0` back as -1, so the struct
	// says one thing and the browser is given another. The browser's copy is
	// the one that decides whether the cookie is still there.
	assert.Contains(t, answer.Header.Get("Set-Cookie"), "Max-Age=0",
		"the cleared cookie does not expire immediately")

	assert.Equal(t, http.StatusUnauthorized, second.get(meURL).Status,
		"the other session survived a sign-out")
	assert.Equal(t, http.StatusUnauthorized, first.get(meURL).Status)
}

func TestSigningOutWithoutASessionIsRefused(t *testing.T) {
	t.Parallel()

	answer := newRig(t).anonymous().post(logoutURL, "")

	require.Equal(t, http.StatusUnauthorized, answer.Status, answer.Body)
	assert.Equal(t, web.CodeUnauthenticated, answer.envelope(t).Error.Code)
}

// A PocketBase superuser holds no MediKube account. It is the break-glass
// credential and not a role (data-model §1, FR-040), and MediKube's own routes
// are not a second way into somebody's data with it.
func TestASuperuserSessionReachesNoAccountOperation(t *testing.T) {
	t.Parallel()

	instance := newRig(t)
	admin := instance.superuser()

	for name, answer := range map[string]response{
		"getMe":          admin.get(meURL),
		"updateMe":       admin.do(http.MethodPatch, meURL, body("theme", quoted("dark")), nil),
		"refreshSession": admin.post(refreshURL, ""),
		"logout":         admin.post(logoutURL, ""),
	} {
		assert.Equalf(t, http.StatusUnauthorized, answer.Status, "%s answered %s", name, answer.Body)
	}
}

// The operation ids the route table joins on are the ones the handlers are
// registered under, and the ones api/openapi.json publishes. Spelled twice,
// they drift; asserted, they cannot.
func TestTheAccountSurfaceIsRegisteredUnderTheContractsOperationIds(t *testing.T) {
	t.Parallel()

	published := map[string]bool{}
	for _, opID := range api.AccountOperations() {
		published[opID] = false
	}

	for _, route := range httproute.Inventory().Routes() {
		if _, ours := published[route.OpID]; ours {
			published[route.OpID] = true
		}
	}

	for opID, registered := range published {
		assert.Truef(t, registered, "%s is served and is not a route MediKube publishes", opID)
	}

	assert.Len(t, published, 13, "contracts/README.md's inventory gives US2 thirteen operations")

	// And every one of them is actually reachable on this build: a route left
	// behind the 501 stub would answer in the same envelope as a real refusal.
	instance := newRig(t)
	for _, url := range []string{authConfigURL, registerURL, loginURL, refreshURL, logoutURL,
		passwordResetURL, confirmResetURL, verifyEmailURL, confirmVerifyURL, meURL, passwordURL} {
		answer := instance.anonymous().post(url, "{}")
		assert.NotEqualf(t, http.StatusNotImplemented, answer.Status, "%s is still a stub", url)
	}
}

// The seeded fixture is what every assertion above stands on, so it is asserted
// once here rather than assumed: apitest wires the account surface, and an
// instance whose identity stack failed to build would answer 501 rather than
// fail loudly.
func TestTheAccountSurfaceIsWiredOnEveryInstance(t *testing.T) {
	t.Parallel()

	instance := apitest.New(t)

	require.NotNil(t, instance.Accounts)
	require.NotNil(t, instance.Accounts.Authenticator)
	assert.False(t, instance.Accounts.Service.RegistrationOpen(),
		"the default instance opens registration, so FR-002's default is not what the tests exercise")
}
