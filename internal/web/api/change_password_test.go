package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/testsupport"
)

// T197, FR-009. A password change is possible ONLY by supplying the current
// password — and the refusal does not say which half failed.
//
// The second clause is the one with teeth. Two messages would let a caller
// learn that the current password they supplied was right, by sending a new one
// they know is unacceptable and reading which field came back. So every refusal
// on this endpoint is ONE body, and this file asserts that byte for byte rather
// than by reading the messages and agreeing they look similar.

func TestAPasswordChangeIsRefusedWithoutTheCurrentPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		document string
	}{
		{
			name:     "no current password at all",
			document: body("current_password", quoted(""), "new_password", quoted(changedPassword)),
		},
		{
			name:     "the wrong current password",
			document: body("current_password", quoted(wrongPassword), "new_password", quoted(changedPassword)),
		},
		{
			name: "somebody else's password",
			document: body("current_password", quoted("boris-is-not-amara"),
				"new_password", quoted(changedPassword)),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			instance := newRig(t)

			answer := instance.as(testsupport.AccountAEmail).put(passwordURL, test.document)

			require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
			assert.Nil(t, answer.sessionCookie(t), "a refused change re-issued a session anyway")

			// The credential is untouched: the old password still signs in.
			signedIn := instance.anonymous().post(loginURL, body(
				"email", quoted(testsupport.AccountAEmail), "password", quoted(testsupport.Password)))
			assert.Equal(t, http.StatusOK, signedIn.Status, "a refused change replaced the password anyway")

			// And the caller's own session survives, because nothing was saved
			// and therefore no token key rotated.
			assert.Equal(t, http.StatusOK, instance.as(testsupport.AccountAEmail).get(meURL).Status)
		})
	}
}

// THE assertion of this file. A wrong current password and an unacceptable new
// one are answered with the SAME BODY, so no attempt tells a caller which half
// it got right.
func TestAPasswordChangeRefusalNeverSaysWhichHalfFailed(t *testing.T) {
	t.Parallel()

	instance := newRig(t)
	signedIn := instance.as(testsupport.AccountAEmail)

	attempts := []struct {
		name     string
		document string
	}{
		{
			name:     "the current password is wrong and the new one is fine",
			document: body("current_password", quoted(wrongPassword), "new_password", quoted(changedPassword)),
		},
		{
			name:     "the current password is right and the new one is too short",
			document: body("current_password", quoted(testsupport.Password), "new_password", quoted("short")),
		},
		{
			name:     "both are wrong",
			document: body("current_password", quoted(wrongPassword), "new_password", quoted("short")),
		},
		{
			name:     "the new password is the address",
			document: body("current_password", quoted(testsupport.Password), "new_password", quoted(testsupport.AccountAEmail)),
		},
		{
			name:     "the new password is the display name",
			document: body("current_password", quoted(testsupport.Password), "new_password", quoted(testsupport.AccountAName)),
		},
		{
			name:     "neither is supplied",
			document: "{}",
		},
	}

	answers := make([]response, 0, len(attempts))

	for _, attempt := range attempts {
		answer := signedIn.put(passwordURL, attempt.document)

		require.Equalf(t, http.StatusUnprocessableEntity, answer.Status, "%s: %s", attempt.name, answer.Body)

		answers = append(answers, answer)
	}

	first := withoutCorrelationID(answers[0].Body)

	for index, answer := range answers[1:] {
		assert.Equalf(t, first, withoutCorrelationID(answer.Body),
			"%s is answered differently from %s, which tells a caller which half it got right",
			attempts[index+1].name, attempts[0].name)
	}

	// The one message must not name either half either: a body that reads "the
	// current password is incorrect" is the disclosure wearing a shared shape.
	assert.NotContains(t, answers[0].Body, "at least 8 characters",
		"the refusal quotes the rule the NEW password broke, which says the current one was right")
}

// The password rules published by GET /api/v1/auth/config are the ones a change
// is held to. A person who wants to know what a new password must look like is
// told without having to fail at one, which is why the refusal above may be
// vague (FR-004).
func TestTheRulesAChangeIsHeldToAreTheOnesTheInstancePublishes(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	var published authConfigDTO
	instance.anonymous().get(authConfigURL).decode(t, &published)

	tooShort := make([]byte, published.PasswordRules.MinLength-1)
	for index := range tooShort {
		tooShort[index] = 'a'
	}

	longEnough := string(tooShort) + "a"

	signedIn := instance.as(testsupport.AccountAEmail)

	refused := signedIn.put(passwordURL, body(
		"current_password", quoted(testsupport.Password), "new_password", quoted(string(tooShort))))
	assert.Equal(t, http.StatusUnprocessableEntity, refused.Status,
		"a password one character below the published minimum was accepted")

	accepted := signedIn.put(passwordURL, body(
		"current_password", quoted(testsupport.Password), "new_password", quoted(longEnough)))
	assert.Equal(t, http.StatusNoContent, accepted.Status,
		"a password exactly at the published minimum was refused: %s", accepted.Body)
}
