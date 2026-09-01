package identity

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"medikube/internal/domain"
)

// CodeInvalidEmail is this entity's own code; the shared ones live in the
// domain package. A code is what a client switches on and what a translation is
// keyed by, so it is a constant rather than a string literal at a call site.
const CodeInvalidEmail = "invalid_email"

const (
	maxEmailLength = 255
	maxNameLength  = 120
)

// data-model §1's locale rule: a two-letter language, optionally a two-letter
// region. Narrower than BCP 47 on purpose — the value drives date and number
// presentation, and a script or a private-use subtag would be a vocabulary
// MediKube publishes without being able to honour.
var localePattern = regexp.MustCompile(`^[a-z]{2}(-[A-Za-z]{2})?$`)

// Validate reports every offending field at once, in the order data-model §1
// lists them, which is the order the form renders. FR-027: a person fixing one
// problem at a time because the server only mentioned one is a form that wastes
// four round trips to say what it knew at the first.
//
// No message repeats a submitted value. These messages are rendered, wrapped
// and carried past a log line, and on this entity the values are an address and
// a person's name (constitution VII).
func (u User) Validate() error {
	var invalid domain.ValidationError

	switch {
	case strings.TrimSpace(u.Email) == "":
		invalid.Add("email", domain.CodeRequired, "an email address is required")
	case utf8.RuneCountInString(u.Email) > maxEmailLength:
		invalid.Addf("email", domain.CodeTooLong, "an email address may be at most %d characters", maxEmailLength)
	case !isBareAddress(u.Email):
		invalid.Add("email", CodeInvalidEmail, "that is not an email address")
	}

	// Trimmed, because a name of spaces is a name nobody can read and a limit
	// measured before trimming is a limit padding can walk around. Runes, not
	// bytes: the storage layer counts the same way, so a name accepted here
	// cannot be refused there.
	switch name := strings.TrimSpace(u.Name); {
	case name == "":
		invalid.Add("name", domain.CodeRequired, "a display name is required")
	case utf8.RuneCountInString(name) > maxNameLength:
		invalid.Addf("name", domain.CodeTooLong, "a display name may be at most %d characters", maxNameLength)
	}

	if !u.Role.Valid() {
		invalid.Addf("role", domain.CodeInvalidValue, "a role is one of: %s", vocabulary(Roles()))
	}

	if !u.UnitSystem.Valid() {
		invalid.Addf("unit_system", domain.CodeInvalidValue, "a unit system is one of: %s", vocabulary(UnitSystems()))
	}

	switch {
	case u.Locale == "":
		invalid.Add("locale", domain.CodeRequired, "a language is required")
	case !localePattern.MatchString(u.Locale):
		invalid.Add("locale", domain.CodeInvalidValue,
			"a language is a two-letter code, optionally with a two-letter region, such as en or en-GB")
	}

	if !u.DateFormat.Valid() {
		invalid.Addf("date_format", domain.CodeInvalidValue, "a date format is one of: %s", vocabulary(DateFormats()))
	}

	if !u.Theme.Valid() {
		invalid.Addf("theme", domain.CodeInvalidValue, "a theme is one of: %s", vocabulary(Themes()))
	}

	return invalid.OrNil()
}

// isBareAddress refuses everything a mailbox may legally be wrapped in.
// "Amara <amara@example.test>" parses, and storing it would make the display
// name part of the sign-in identity and put an angle bracket in the address the
// unique index is built on.
func isBareAddress(value string) bool {
	parsed, err := mail.ParseAddress(value)

	return err == nil && parsed.Address == value
}

// The vocabulary in the message, so a refusal tells the person what is on offer
// instead of only that their value was not. It is published data — never the
// value that was submitted.
func vocabulary[T ~string](values []T) string {
	spellings := make([]string, 0, len(values))
	for _, value := range values {
		spellings = append(spellings, string(value))
	}

	return strings.Join(spellings, ", ")
}
