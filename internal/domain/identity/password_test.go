package identity

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

// FR-004 and data-model §1. These four numbers are the published contract —
// contracts/auth.md serves them verbatim as `password_rules` — so they are
// pinned here once, and every other assertion in this file derives its
// boundaries from PublishedPasswordRules() rather than repeating them. That is
// what "the rules the API publishes are the same values the validator
// enforces, asserted from one source" means: change the rule and this test is
// the only place that has to agree.
func TestThePublishedRulesAreTheRulesTheSpecificationStates(t *testing.T) {
	t.Parallel()

	rules := PublishedPasswordRules()

	assert.Equal(t, 8, rules.MinLength, "FR-004 sets the floor at eight characters")
	assert.Equal(t, 200, rules.MaxLength, "data-model §1 caps the length as a bcrypt denial-of-service floor")
	assert.True(t, rules.RejectsEmail, "FR-004 refuses a password equal to the account's address")
	assert.True(t, rules.RejectsName, "FR-004 refuses a password equal to the account's display name")
}

// The published rules are data a caller reads, not a copy of a boolean check.
// A rule switched off in the published struct must stop being enforced, and one
// switched on must be enforced — otherwise the sign-up form states something
// the server does not do.
func TestEveryPublishedRuleIsEnforcedAtItsBoundary(t *testing.T) {
	t.Parallel()

	rules := PublishedPasswordRules()

	const email = "amara@example.test"
	const name = "Amara Okafor"

	tests := []struct {
		name     string
		password string
		want     []string
	}{
		{name: "empty", password: "", want: []string{domain.CodeTooShort}},
		{
			name:     "one short of the minimum",
			password: strings.Repeat("a", rules.MinLength-1),
			want:     []string{domain.CodeTooShort},
		},
		{name: "exactly the minimum", password: strings.Repeat("a", rules.MinLength)},
		{name: "exactly the maximum", password: strings.Repeat("a", rules.MaxLength)},
		{
			name:     "one over the maximum",
			password: strings.Repeat("a", rules.MaxLength+1),
			want:     []string{domain.CodeTooLong},
		},
		{name: "the account's address", password: email, want: []string{CodeSameAsEmail}},
		{name: "the account's address in another case", password: "AMARA@EXAMPLE.TEST", want: []string{CodeSameAsEmail}},
		{name: "the account's display name", password: name, want: []string{CodeSameAsName}},
		{name: "the account's display name in another case", password: "amara okafor", want: []string{CodeSameAsName}},
		{name: "a password nothing objects to", password: "correct horse battery staple"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePassword(test.password, email, name)

			if len(test.want) == 0 {
				assert.NoError(t, err)

				return
			}

			assert.Equal(t, test.want, codesByField(t, err)[FieldPassword])
		})
	}
}

// A password is measured in characters, as FR-004 words it and as PocketBase's
// password field counts it. Seven multi-byte characters are fourteen bytes and
// must still be refused; eight are sixteen bytes and must be accepted. A byte
// count here would silently publish a different rule to anybody outside ASCII.
func TestTheLengthIsCountedInCharactersAndNotInBytes(t *testing.T) {
	t.Parallel()

	rules := PublishedPasswordRules()

	short := strings.Repeat("é", rules.MinLength-1)
	long := strings.Repeat("é", rules.MinLength)

	require.Greater(t, len(short), rules.MinLength, "the fixture is not multi-byte and proves nothing")

	assert.Equal(t, []string{domain.CodeTooShort}, codesByField(t, ValidatePassword(short, "", ""))[FieldPassword])
	assert.NoError(t, ValidatePassword(long, "", ""))
}

// FR-004 requires the rules to be published rather than discovered by breaking
// them, and a refusal that does not state the rule sends the person back to
// guess. The password itself never appears — it is a credential, and this
// message is rendered, wrapped and carried past a log line (constitution VII).
func TestTheRefusalStatesTheRuleAndNeverThePassword(t *testing.T) {
	t.Parallel()

	rules := PublishedPasswordRules()

	t.Run("too short", func(t *testing.T) {
		t.Parallel()

		const password = "hunter2"
		message := messageFor(t, ValidatePassword(password, "", ""), FieldPassword)

		assert.Contains(t, message, strconv.Itoa(rules.MinLength))
		assert.NotContains(t, message, password)
	})

	t.Run("too long", func(t *testing.T) {
		t.Parallel()

		password := strings.Repeat("z", rules.MaxLength+1)
		message := messageFor(t, ValidatePassword(password, "", ""), FieldPassword)

		assert.Contains(t, message, strconv.Itoa(rules.MaxLength))
		assert.NotContains(t, message, password)
	})

	t.Run("the same as the address or the name", func(t *testing.T) {
		t.Parallel()

		const email = "amara@example.test"
		const name = "Amara Okafor"

		err := ValidatePassword(email, email, name)
		message := messageFor(t, err, FieldPassword)

		assert.NotContains(t, message, email, "the message repeats the credential the person just typed")
		assert.NotContains(t, message, name)
	})
}

// FR-027 applies to the password rules too: a password that is both too short
// and the display name is refused for both reasons at once, not for whichever
// the implementation happened to check first.
func TestEveryBrokenRuleIsReportedAtOnce(t *testing.T) {
	t.Parallel()

	err := ValidatePassword("ada", "ada@example.test", "ada")

	assert.Equal(t, []string{domain.CodeTooShort, CodeSameAsName}, codesByField(t, err)[FieldPassword])
}

// There is no address and no display name to be equal to during a recovery,
// where the rules are checked with only the token in hand (FR-074). An empty
// comparison value must not turn into a rule the person cannot satisfy.
func TestAnAbsentAddressOrNameIsNotSomethingToBeEqualTo(t *testing.T) {
	t.Parallel()

	assert.NoError(t, ValidatePassword("correct horse battery staple", "", ""))
	assert.Equal(t, []string{domain.CodeTooShort}, codesByField(t, ValidatePassword("", "", ""))[FieldPassword])
}

// The same rules under the two field names the contracts use: `password` at
// registration and at a reset (contracts/auth.md), `new_password` at a change
// (contracts/account.md). One rule set, two labels — a second implementation
// for the second label is how the two drift apart.
func TestTheSameRulesAreReportedUnderTheFieldTheEndpointNames(t *testing.T) {
	t.Parallel()

	const password = "short"

	assert.Equal(t, map[string][]string{FieldPassword: {domain.CodeTooShort}},
		codesByField(t, ValidatePassword(password, "", "")))

	assert.Equal(t, map[string][]string{FieldNewPassword: {domain.CodeTooShort}},
		codesByField(t, ValidatePasswordField(FieldNewPassword, password, "", "")))
}

// OrNil's contract, restated where it matters most: a valid password must
// compare equal to nil, not merely be an error interface holding a nil pointer.
func TestAValidPasswordReturnsAnUntypedNil(t *testing.T) {
	t.Parallel()

	err := ValidatePassword("correct horse battery staple", "amara@example.test", "Amara Okafor")

	assert.Nil(t, err)
	assert.NoError(t, err)
}
