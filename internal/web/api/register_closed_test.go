package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
	"medikube/internal/testsupport"
	"medikube/internal/web"
)

// T206, FR-002 and defect D15. Registration is CLOSED by default, and a closed
// instance answers 403 `registration_closed`.
//
// 403 and not 404, deliberately. A 404 is what this codebase answers for
// owner-scoped data, where the existence of the thing is itself the secret;
// whether an operator opened self-registration is instance-wide configuration,
// identical for every caller, so there is no oracle here to close. Saying so
// plainly is what lets the sign-up page render an explanation rather than a
// dead end.

func TestAClosedInstanceRefusesEveryRegistration(t *testing.T) {
	t.Parallel()

	documents := map[string]string{
		"a submission that would otherwise succeed": body(
			"email", quoted(newAccountEmail),
			"name", quoted(newAccountName),
			"password", quoted(newAccountPassword),
		),
		"a submission that would otherwise be 422": body(
			"email", quoted("not-an-address"),
			"name", quoted(""),
			"password", quoted("short"),
		),
		"a submission that would otherwise be 409": body(
			"email", quoted(testsupport.AccountAEmail),
			"name", quoted(newAccountName),
			"password", quoted(newAccountPassword),
		),
		"a submission naming a role": body(
			"email", quoted(newAccountEmail),
			"name", quoted(newAccountName),
			"password", quoted(newAccountPassword),
			"role", quoted("admin"),
		),
		"an empty document": "{}",
	}

	for name, document := range documents {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			instance := newRig(t)

			answer := instance.anonymous().post(registerURL, document)

			require.Equalf(t, http.StatusForbidden, answer.Status, "%s: %s", name, answer.Body)
			assert.Equal(t, web.CodeRegistrationClosed, answer.envelope(t).Error.Code)
			assert.Nil(t, answer.sessionCookie(t))

			assert.False(t, instance.accountExists(t, newAccountEmail),
				"a closed instance created an account anyway")
		})
	}
}

// The refusal comes FIRST, before the submission is so much as looked at. A
// closed instance that answered 422 for one body and 403 for another would be
// running its validator for anonymous callers and confirming which addresses
// parse — and its 409 branch would confirm which addresses are registered.
func TestAClosedInstanceAnswersEveryRegistrationIdentically(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	answers := []response{
		instance.anonymous().post(registerURL, body(
			"email", quoted(newAccountEmail),
			"name", quoted(newAccountName),
			"password", quoted(newAccountPassword),
		)),
		instance.anonymous().post(registerURL, body(
			"email", quoted(testsupport.AccountAEmail),
			"name", quoted(testsupport.AccountAName),
			"password", quoted(testsupport.Password),
		)),
		instance.anonymous().post(registerURL, "{}"),
	}

	first := withoutCorrelationID(answers[0].Body)

	for index, answer := range answers[1:] {
		assert.Equalf(t, first, withoutCorrelationID(answer.Body),
			"submission %d is answered differently, which says something about the address in it", index+2)
	}

	assert.NotContains(t, answers[1].Body, testsupport.AccountAEmail)
}

// An open instance is the control. Without it every assertion above would pass
// on an instance that refused registration for some other reason entirely.
func TestAnOpenInstanceIsWhatMakesTheClosedOneAnAssertion(t *testing.T) {
	t.Parallel()

	answer := openRig(t).anonymous().post(registerURL, body(
		"email", quoted(newAccountEmail),
		"name", quoted(newAccountName),
		"password", quoted(newAccountPassword),
	))

	require.Equal(t, http.StatusCreated, answer.Status, answer.Body)
}

// T220's other half, asserted from here because it is the same requirement: the
// route is registered UNCONDITIONALLY. A route that disappears under some
// configurations is a route the inventory gate cannot check, and a smoke gate
// that visited it would be asserting against whichever configuration CI
// happened to have.
func TestBothRegistrationRoutesArePresentWhateverTheOperatorChose(t *testing.T) {
	t.Parallel()

	present := map[string]httproute.Route{}
	for _, route := range httproute.Inventory().Routes() {
		present[route.OpID] = route
	}

	operation, registered := present["register"]
	require.True(t, registered, "the API operation is missing from the inventory")
	assert.Equal(t, httproute.AuthPublic, operation.Auth,
		"a person with no session is the only person who can register")

	page, registered := present["registerPage"]
	require.True(t, registered, "the sign-up page is missing from the inventory")
	assert.NotEmpty(t, page.Landmark, "the page cannot be smoked without a landmark")
	assert.NotEmpty(t, page.SmokeURL)

	// And it is there on an instance whose operator closed registration, which
	// is the configuration the default gives every deployment.
	closed := newRig(t)
	assert.False(t, closed.instance.Accounts.Service.RegistrationOpen())
	assert.NotEqual(t, http.StatusNotFound, closed.anonymous().post(registerURL, "{}").Status,
		"the closed instance answers 404, which is the answer for a route that is not there")
}
