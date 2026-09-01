package identity_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/identity"
)

// TestARecoveryRequestIsAnsweredByAValueNothingCanVary is T223a's FR-073 half,
// and it is deliberately a check of the SHAPE rather than of a returned value.
//
// A test that called the constructor twice and compared the results would pass
// against a constructor that branched on an account it was handed. What cannot
// pass is a constructor with somewhere to put the account: AcknowledgeRecovery
// takes no arguments, so "the address is registered" is not a fact it is in a
// position to know, and a later edit that wanted to answer differently would
// have to change this signature — which is a review, not an oversight.
func TestARecoveryRequestIsAnsweredByAValueNothingCanVary(t *testing.T) {
	t.Parallel()

	constructor := reflect.TypeOf(identity.AcknowledgeRecovery)

	assert.Zero(t, constructor.NumIn(),
		"the acknowledgement is built from something, and whatever that is can differ between an address with an account and one without")
	assert.False(t, constructor.IsVariadic(), "a variadic constructor is a parameter list with the count hidden")
	require.Equal(t, 1, constructor.NumOut())

	first, second := identity.AcknowledgeRecovery(), identity.AcknowledgeRecovery()
	assert.Equal(t, first, second)
	assert.Equal(t, identity.RecoveryStatus, first.Status)
}

// TestTheAcknowledgementSaysNothingAboutAnAccount. The one string a caller sees
// is about the request, not about the register: it is as true for an address
// nobody has as for one somebody does.
func TestTheAcknowledgementSaysNothingAboutAnAccount(t *testing.T) {
	t.Parallel()

	status := identity.AcknowledgeRecovery().Status

	require.NotEmpty(t, status)

	for _, disclosure := range []string{"exists", "registered_", "unknown", "no_account", "not_found", "sent_to"} {
		assert.NotContains(t, status, disclosure,
			"the one value every recovery request answers with reads as a statement about the account")
	}
}

// TestAPasswordChosenThroughRecoveryIsHeldToTheRulesRegistrationPublishes is
// T223a's FR-004/FR-074 half.
//
// FR-074 says the link lets a person "set a new password meeting the published
// rules", and the published rules are one set. The rules are reached through a
// field label — `password` at registration and at a reset, `new_password` at a
// change — and the label is a parameter, so a rule that read it would be a
// second rule set nothing else in the suite could see. This is what notices.
func TestAPasswordChosenThroughRecoveryIsHeldToTheRulesRegistrationPublishes(t *testing.T) {
	t.Parallel()

	rules := identity.PublishedPasswordRules()

	cases := map[string]string{
		"one character short of the minimum": strings.Repeat("a", rules.MinLength-1),
		"empty":                              "",
		"one character past the maximum":     strings.Repeat("a", rules.MaxLength+1),
		"exactly the account's address":      "amara@example.test",
		"exactly the account's name":         "Amara Okonkwo",
		"acceptable":                         "a-perfectly-ordinary-passphrase",
	}

	for name, password := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			const (
				email = "amara@example.test"
				who   = "Amara Okonkwo"
			)

			registration := identity.ValidatePassword(password, email, who)
			recovery := identity.ValidatePasswordField(identity.FieldPassword, password, email, who)
			change := identity.ValidatePasswordField(identity.FieldNewPassword, password, email, who)

			assert.Equal(t, registration, recovery,
				"a password set through a recovery link is held to different rules than one chosen at registration")

			assert.Equal(t, codes(t, registration), codes(t, change),
				"the rules differ by the name of the field they are reported on, so one endpoint enforces what another does not publish")
		})
	}
}

// codes is the refusal without its field label, which is the only thing the two
// endpoints are entitled to differ by.
func codes(t *testing.T, err error) []string {
	t.Helper()

	if err == nil {
		return nil
	}

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid, "a password refusal that is not a validation error cannot be rendered beside the field")

	found := make([]string, 0, len(invalid.Fields))
	for _, field := range invalid.Fields {
		found = append(found, field.Code+": "+field.Message)
	}

	return found
}

// TestTheDeletionPhraseIsTheExactWordsTheFormAsksFor. FR-013 asks for an
// explicitly typed confirmation, and the words are a contract between the form,
// the handler and the person: a phrase that drifted by one character on either
// side is an account nobody can delete.
func TestTheDeletionPhraseIsTheExactWordsTheFormAsksFor(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "DELETE MY ACCOUNT", identity.DeleteConfirmationPhrase)
	assert.NotEqual(t, strings.ToLower(identity.DeleteConfirmationPhrase), identity.DeleteConfirmationPhrase,
		"a phrase with no capitals is one a person types by accident")
}
