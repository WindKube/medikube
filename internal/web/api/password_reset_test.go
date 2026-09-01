package api_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/testsupport"
	"medikube/internal/web"
)

// T223c, FR-073 and FR-074. Password recovery over the wire.
//
// The whole of FR-073 is that a recovery form must not become an
// account-existence oracle: on a self-hosted medical instance the set of people
// who have accounts is itself sensitive, so "no such account" is a disclosure
// and so is any other difference a caller can read. The three 202 bodies below
// are compared byte for byte.
//
// FR-074's half is that expired, already used and tampered with are ONE
// refusal. Telling them apart tells an attacker which tokens once existed.

//nolint:gosec // a test credential
const recoveredPassword = "a-password-chosen-through-recovery"

func resetToken(t *testing.T, instance *rig, email string) string {
	t.Helper()

	record, err := instance.instance.App.FindAuthRecordByEmail(usersCollection, email)
	require.NoError(t, err)

	token, err := record.NewPasswordResetToken()
	require.NoError(t, err)

	return token
}

// FR-073. Three requests, three identical answers.
func TestEveryRecoveryRequestIsAnsweredIdentically(t *testing.T) {
	t.Parallel()

	instance := newRig(t).withMail(true)

	requests := []struct {
		name    string
		address string
	}{
		{name: "an address with an account", address: testsupport.AccountAEmail},
		{name: "an address with no account", address: "nobody@example.test"},
		{name: "something that is not an address at all", address: "]["},
		{name: "no address at all", address: ""},
	}

	answers := make([]response, 0, len(requests))

	for _, request := range requests {
		answer := instance.anonymous().post(passwordResetURL, body("email", quoted(request.address)))

		require.Equalf(t, http.StatusAccepted, answer.Status, "%s: %s", request.name, answer.Body)

		var acknowledgement acknowledgementDTO
		answer.decode(t, &acknowledgement)

		assert.Equal(t, domainidentity.RecoveryStatus, acknowledgement.Status)

		answers = append(answers, answer)
	}

	first := withoutCorrelationID(answers[0].Body)

	for index, answer := range answers[1:] {
		assert.Equalf(t, first, withoutCorrelationID(answer.Body),
			"%s is answered differently from %s", requests[index+1].name, requests[0].name)
	}

	// The wording is about the REQUEST and not about the account: "sent if
	// registered" is true whether or not anybody is registered, which is what
	// makes it safe to say to somebody fishing for the difference.
	assert.NotContains(t, answers[0].Body, testsupport.AccountAEmail)
}

// Only the address with an account has a message sent to it — which is
// invisible to the caller and is exactly the point: the difference is in what
// the SERVER does, never in what it says.
func TestOnlyARegisteredAddressIsSentAnything(t *testing.T) {
	t.Parallel()

	instance := newRig(t).withMail(true)

	require.Equal(t, http.StatusAccepted,
		instance.anonymous().post(passwordResetURL, body("email", quoted("nobody@example.test"))).Status)
	assert.Equal(t, 0, instance.instance.App.TestMailer.TotalSend(),
		"an address with no account had a message sent to it")

	require.Equal(t, http.StatusAccepted,
		instance.anonymous().post(passwordResetURL, body("email", quoted(testsupport.AccountAEmail))).Status)
	assert.Equal(t, 1, instance.instance.App.TestMailer.TotalSend(),
		"a registered address was not sent a recovery message")

	sent := instance.instance.App.TestMailer.LastMessage()
	require.Len(t, sent.To, 1)
	assert.Equal(t, testsupport.AccountAEmail, sent.To[0].Address)
}

// FR-076. An instance that cannot send mail refuses rather than accepting a
// request it cannot honour — and the refusal names no address, so it is not a
// 503-shaped oracle.
func TestAnInstanceThatCannotSendMailRefusesTheRequest(t *testing.T) {
	t.Parallel()

	instance := newRig(t).withMail(false)

	for _, address := range []string{testsupport.AccountAEmail, "nobody@example.test"} {
		answer := instance.anonymous().post(passwordResetURL, body("email", quoted(address)))

		require.Equal(t, http.StatusServiceUnavailable, answer.Status, answer.Body)
		assert.Equal(t, web.CodeMailUnconfigured, answer.envelope(t).Error.Code)
		assert.NotContains(t, answer.Body, address, "the refusal names the address it was given")
	}

	assert.Equal(t, 0, instance.instance.App.TestMailer.TotalSend(),
		"an instance with no outgoing mail queued a message anyway")
}

// FR-074. A valid link sets the password, ends every session issued before it,
// and does not sign the caller in.
func TestARecoveryLinkSetsThePasswordAndEndsEverySessionBeforeIt(t *testing.T) {
	t.Parallel()

	instance := newRig(t).withMail(true)

	openSomewhere := instance.as(testsupport.AccountAEmail)
	require.Equal(t, http.StatusOK, openSomewhere.get(meURL).Status)

	token := resetToken(t, instance, testsupport.AccountAEmail)

	answer := instance.anonymous().post(confirmResetURL, body(
		"token", quoted(token),
		"password", quoted(recoveredPassword),
		"password_confirm", quoted(recoveredPassword),
	))

	require.Equal(t, http.StatusNoContent, answer.Status, answer.Body)
	assert.Empty(t, answer.Body)
	assert.Nil(t, answer.sessionCookie(t),
		"a recovery signed the caller in; the password is proven again at the sign-in page instead")

	assert.Equal(t, http.StatusUnauthorized, openSomewhere.get(meURL).Status,
		"a session issued before the recovery survived it")

	assert.Equal(t, http.StatusUnauthorized, instance.anonymous().post(loginURL, body(
		"email", quoted(testsupport.AccountAEmail), "password", quoted(testsupport.Password))).Status,
		"the old password still signs in")

	assert.Equal(t, http.StatusOK, instance.anonymous().post(loginURL, body(
		"email", quoted(testsupport.AccountAEmail), "password", quoted(recoveredPassword))).Status,
		"the new password does not sign in")
}

// FR-074's one refusal. Four different broken links, one answer.
func TestEveryUnusableRecoveryLinkIsAnsweredTheSameWay(t *testing.T) {
	t.Parallel()

	instance := newRig(t).withMail(true)

	record, err := instance.instance.App.FindAuthRecordByEmail(usersCollection, testsupport.AccountAEmail)
	require.NoError(t, err)

	// Spent: used once already, which rotates the token key underneath it.
	spent := resetToken(t, instance, testsupport.AccountAEmail)
	require.Equal(t, http.StatusNoContent, instance.anonymous().post(confirmResetURL, body(
		"token", quoted(spent),
		"password", quoted(recoveredPassword),
		"password_confirm", quoted(recoveredPassword),
	)).Status)

	// Expired: signed with PocketBase's own key, because NewPasswordResetToken
	// takes the collection's thirty minutes and nothing shortens it.
	expired := signedJWT(t, record.TokenKey()+record.Collection().PasswordResetToken.Secret, map[string]any{
		core.TokenClaimType:         core.TokenTypePasswordReset,
		core.TokenClaimId:           record.Id,
		core.TokenClaimCollectionId: record.Collection().Id,
		core.TokenClaimEmail:        record.Email(),
		"exp":                       time.Now().Add(-time.Minute).Unix(),
	})

	// Tampered: a real token with its signature altered.
	usable := resetToken(t, instance, testsupport.AccountBEmail)
	tampered := usable[:len(usable)-4] + "AAAA"

	// The wrong purpose entirely: an ordinary session token presented as a
	// recovery link. A caller holding one is exactly the caller who would try.
	wrongPurpose := testsupport.UserToken(t, instance.instance.App, testsupport.AccountBEmail)

	broken := []struct {
		name  string
		token string
	}{
		{name: "already used", token: spent},
		{name: "expired", token: expired},
		{name: "tampered with", token: tampered},
		{name: "a session token", token: wrongPurpose},
		{name: "not a token at all", token: "expired-token-for-smoke"},
		{name: "empty", token: ""},
	}

	answers := make([]response, 0, len(broken))

	for _, link := range broken {
		answer := instance.anonymous().post(confirmResetURL, body(
			"token", quoted(link.token),
			"password", quoted(recoveredPassword),
			"password_confirm", quoted(recoveredPassword),
		))

		require.Equalf(t, http.StatusBadRequest, answer.Status, "%s: %s", link.name, answer.Body)
		assert.Equal(t, web.CodeInvalidToken, answer.envelope(t).Error.Code)

		answers = append(answers, answer)
	}

	first := withoutCorrelationID(answers[0].Body)

	for index, answer := range answers[1:] {
		assert.Equalf(t, first, withoutCorrelationID(answer.Body),
			"a link that is %s is answered differently from one that is %s — which says which tokens once existed",
			broken[index+1].name, broken[0].name)
	}

	// Account B's password was never set by the tampered token that named it.
	assert.Equal(t, http.StatusOK, instance.anonymous().post(loginURL, body(
		"email", quoted(testsupport.AccountBEmail), "password", quoted(testsupport.Password))).Status)
}

// A password the published rules refuse must leave the link USABLE. One typo
// otherwise costs another round trip through a mailbox.
func TestARefusedPasswordDoesNotSpendTheLink(t *testing.T) {
	t.Parallel()

	instance := newRig(t).withMail(true)
	token := resetToken(t, instance, testsupport.AccountAEmail)

	refused := instance.anonymous().post(confirmResetURL, body(
		"token", quoted(token),
		"password", quoted("short"),
		"password_confirm", quoted("short"),
	))

	require.Equal(t, http.StatusUnprocessableEntity, refused.Status, refused.Body)
	assert.Equal(t, [][2]string{{domainidentity.FieldPassword, domain.CodeTooShort}}, refused.envelope(t).fieldCodes())

	accepted := instance.anonymous().post(confirmResetURL, body(
		"token", quoted(token),
		"password", quoted(recoveredPassword),
		"password_confirm", quoted(recoveredPassword),
	))

	assert.Equal(t, http.StatusNoContent, accepted.Status,
		"a rejected password spent the link: %s", accepted.Body)
}

// The two typed passwords have to agree, and the refusal says so on the member
// that is wrong rather than on the one that is not.
func TestTheTwoTypedPasswordsHaveToAgree(t *testing.T) {
	t.Parallel()

	instance := newRig(t).withMail(true)

	answer := instance.anonymous().post(confirmResetURL, body(
		"token", quoted(resetToken(t, instance, testsupport.AccountAEmail)),
		"password", quoted(recoveredPassword),
		"password_confirm", quoted(recoveredPassword+"-not"),
	))

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	assert.Equal(t, [][2]string{{"password_confirm", "mismatch"}}, answer.envelope(t).fieldCodes())
}

// A password chosen through recovery is held to the SAME published rules as one
// chosen at registration (FR-004, FR-074). One rule set, two doors.
func TestARecoveredPasswordIsHeldToThePublishedRules(t *testing.T) {
	t.Parallel()

	instance := newRig(t).withMail(true)

	var published authConfigDTO
	instance.anonymous().get(authConfigURL).decode(t, &published)

	shortest := make([]byte, published.PasswordRules.MinLength)
	for index := range shortest {
		shortest[index] = 'a'
	}

	tooShort := string(shortest[:len(shortest)-1])

	refused := instance.anonymous().post(confirmResetURL, body(
		"token", quoted(resetToken(t, instance, testsupport.AccountAEmail)),
		"password", quoted(tooShort),
		"password_confirm", quoted(tooShort),
	))
	assert.Equal(t, http.StatusUnprocessableEntity, refused.Status,
		"a password below the published minimum was accepted through a recovery link")

	accepted := instance.anonymous().post(confirmResetURL, body(
		"token", quoted(resetToken(t, instance, testsupport.AccountAEmail)),
		"password", quoted(string(shortest)),
		"password_confirm", quoted(string(shortest)),
	))
	assert.Equal(t, http.StatusNoContent, accepted.Status,
		"a password exactly at the published minimum was refused: %s", accepted.Body)
}

// T223o. The two lifetimes contracts/auth.md documents are the ones this
// instance inherits, asserted against the collection rather than against the
// document. They are PocketBase v0.40.1's own defaults and MediKube writes
// neither, so nothing but this would notice an upgrade moving one.
func TestTheDocumentedTokenLifetimesAreTheOnesThisInstanceUses(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	collection, err := instance.instance.App.FindCollectionByNameOrId(usersCollection)
	require.NoError(t, err)

	assert.Equal(t, int64((30 * time.Minute).Seconds()), collection.PasswordResetToken.Duration,
		"contracts/auth.md documents a thirty-minute recovery link")
	assert.Equal(t, int64((24 * time.Hour).Seconds()), collection.VerificationToken.Duration,
		"contracts/auth.md documents a twenty-four-hour confirmation link")
}

// FR-073's last hole, and the one that is easy to leave open: an instance whose
// relay refuses the message must still answer 202.
//
// Only a REGISTERED address can fail at sending, because an unregistered one
// has nothing to send. A handler that surfaced the failure would therefore be
// answering 500 for exactly the addresses that have accounts and 202 for the
// rest — the same oracle, wearing the status code of an honest mistake.
func TestAMailFailureIsNotAnnouncedToTheCaller(t *testing.T) {
	t.Parallel()

	instance := newRig(t).withMail(true)

	instance.instance.App.OnMailerSend().BindFunc(func(*core.MailerEvent) error {
		return errors.New("the relay refused the message")
	})

	known := instance.anonymous().post(passwordResetURL, body("email", quoted(testsupport.AccountAEmail)))
	unknown := instance.anonymous().post(passwordResetURL, body("email", quoted("nobody@example.test")))

	require.Equal(t, http.StatusAccepted, known.Status,
		"a send that failed was announced to the caller, which says the address has an account: %s", known.Body)
	require.Equal(t, http.StatusAccepted, unknown.Status, unknown.Body)

	assert.Equal(t, withoutCorrelationID(unknown.Body), withoutCorrelationID(known.Body))
}
