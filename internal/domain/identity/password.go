package identity

import (
	"strings"
	"unicode/utf8"

	"medikube/internal/domain"
)

// The field names the contracts give a password: `password` at registration and
// at a reset, `new_password` at a change (contracts/auth.md, contracts/account.md).
// Two labels for one rule set, named here so neither is spelled at a call site.
const (
	FieldPassword    = "password"
	FieldNewPassword = "new_password"
)

// The codes only these rules raise. The shared ones — too_short, too_long —
// stay in the domain package with every other entity's copy of them.
const (
	CodeSameAsEmail = "same_as_email"
	CodeSameAsName  = "same_as_name"
)

// PasswordRules is FR-004's rules as data rather than as a boolean check.
// contracts/auth.md publishes them verbatim on GET /api/v1/auth/config so the
// sign-up form can state them before the person chooses: a rule discovered by
// violating it is not published, which is what FR-004 asks for.
//
// The booleans are not documentation. ValidatePasswordField reads them, so a
// rule switched off here stops being enforced and one switched on cannot be
// enforced without being published — the two cannot drift.
type PasswordRules struct {
	MinLength    int
	MaxLength    int
	RejectsEmail bool
	RejectsName  bool
}

// The one place the numbers live. The storage layer states the same floor again
// through the users collection's password field, which is defence in depth and
// not a second source: a mismatch there is a failing migration assertion.
//
// The ceiling is a denial-of-service floor rather than a security rule. bcrypt
// hashes whatever it is handed, and an unbounded password is an unbounded
// amount of work for an anonymous caller to ask for.
var passwordRules = PasswordRules{
	MinLength:    8,
	MaxLength:    200,
	RejectsEmail: true,
	RejectsName:  true,
}

func PublishedPasswordRules() PasswordRules { return passwordRules }

// ValidatePassword is data-model §1's check, reported on the `password` field.
func ValidatePassword(password, email, name string) error {
	return ValidatePasswordField(FieldPassword, password, email, name)
}

// ValidatePasswordField is the same rules under the field name the endpoint
// uses. A change reports them on `new_password`, and a rule restated to change
// its label is how one endpoint ends up enforcing something the form did not
// publish.
//
// email and name are the account's, and either may be empty: a recovery sets a
// password with nothing but a token in hand (FR-074), and there is then no
// address to be equal to. The password itself never reaches a message — it is a
// credential, and these messages are rendered, wrapped and logged.
func ValidatePasswordField(field, password, email, name string) error {
	rules := PublishedPasswordRules()

	var invalid domain.ValidationError

	// Characters, not bytes, as FR-004 words it and as PocketBase's password
	// field counts it. A byte count would publish one rule and enforce another
	// to anybody writing outside ASCII.
	switch length := utf8.RuneCountInString(password); {
	case length < rules.MinLength:
		invalid.Addf(field, domain.CodeTooShort, "a password must be at least %d characters", rules.MinLength)
	case length > rules.MaxLength:
		invalid.Addf(field, domain.CodeTooLong, "a password may be at most %d characters", rules.MaxLength)
	}

	if rules.RejectsEmail && email != "" && strings.EqualFold(password, email) {
		invalid.Add(field, CodeSameAsEmail, "a password must not be the account's email address")
	}

	if rules.RejectsName && name != "" && strings.EqualFold(password, name) {
		invalid.Add(field, CodeSameAsName, "a password must not be the account's display name")
	}

	return invalid.OrNil()
}
