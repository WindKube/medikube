package identity

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

// The two readers every validation test in this package uses. They go through
// errors.As rather than a type assertion so that a Validate() which starts
// wrapping its result keeps passing for the right reason.
func codesByField(t *testing.T, err error) map[string][]string {
	t.Helper()

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid, "expected a *domain.ValidationError, got %v", err)

	byField := make(map[string][]string, len(invalid.Fields))
	for _, field := range invalid.Fields {
		byField[field.Field] = append(byField[field.Field], field.Code)
	}

	return byField
}

func messageFor(t *testing.T, err error, field string) string {
	t.Helper()

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)

	for _, refusal := range invalid.Fields {
		if refusal.Field == field {
			return refusal.Message
		}
	}

	require.FailNowf(t, "no refusal for that field", "field %q is not among %v", field, invalid.Fields)

	return ""
}

// An account that nothing objects to. Every case below breaks exactly one thing
// about it, so a failure names the rule and not the fixture.
func validUser() User {
	return User{
		ID:         "usr0000000001",
		Email:      "amara@example.test",
		Name:       "Amara Okafor",
		Role:       DefaultRole,
		UnitSystem: DefaultUnitSystem,
		Locale:     DefaultLocale,
		DateFormat: DefaultDateFormat,
		Theme:      DefaultTheme,
	}
}

func TestValidateAcceptsAnAccountThatBreaksNoRule(t *testing.T) {
	t.Parallel()

	assert.NoError(t, validUser().Validate())
}

// data-model §1's validation table, rule by rule, at both sides of every
// boundary it draws.
func TestValidateRefusesEachFieldWithTheCodeTheDataModelGivesIt(t *testing.T) {
	t.Parallel()

	longestLocal := strings.Repeat("a", 255-len("@example.test"))

	tests := []struct {
		name   string
		breaks func(*User)
		field  string
		code   string
	}{
		{name: "email absent", breaks: func(u *User) { u.Email = "" }, field: "email", code: domain.CodeRequired},
		{name: "email blank", breaks: func(u *User) { u.Email = "   " }, field: "email", code: domain.CodeRequired},
		{name: "email is not an address", breaks: func(u *User) { u.Email = "amara" }, field: "email", code: CodeInvalidEmail},
		{name: "email has no domain", breaks: func(u *User) { u.Email = "amara@" }, field: "email", code: CodeInvalidEmail},
		{
			name:   "email carries a display name",
			breaks: func(u *User) { u.Email = "Amara Okafor <amara@example.test>" },
			field:  "email",
			code:   CodeInvalidEmail,
		},
		{
			name:   "email is padded",
			breaks: func(u *User) { u.Email = " amara@example.test " },
			field:  "email",
			code:   CodeInvalidEmail,
		},
		{
			name:   "email one character over the limit",
			breaks: func(u *User) { u.Email = longestLocal + "a@example.test" },
			field:  "email",
			code:   domain.CodeTooLong,
		},
		{name: "name absent", breaks: func(u *User) { u.Name = "" }, field: "name", code: domain.CodeRequired},
		{
			name:   "name is only whitespace",
			breaks: func(u *User) { u.Name = " \t " },
			field:  "name",
			code:   domain.CodeRequired,
		},
		{
			name:   "name one character over the limit",
			breaks: func(u *User) { u.Name = strings.Repeat("a", 121) },
			field:  "name",
			code:   domain.CodeTooLong,
		},
		{
			name:   "name over the limit once trimmed",
			breaks: func(u *User) { u.Name = " " + strings.Repeat("a", 121) + " " },
			field:  "name",
			code:   domain.CodeTooLong,
		},
		{name: "role absent", breaks: func(u *User) { u.Role = "" }, field: "role", code: domain.CodeInvalidValue},
		{name: "role unknown", breaks: func(u *User) { u.Role = "superuser" }, field: "role", code: domain.CodeInvalidValue},
		{
			name:   "unit system unknown",
			breaks: func(u *User) { u.UnitSystem = "us" },
			field:  "unit_system",
			code:   domain.CodeInvalidValue,
		},
		{
			name:   "date format unknown",
			breaks: func(u *User) { u.DateFormat = "ymd" },
			field:  "date_format",
			code:   domain.CodeInvalidValue,
		},
		{name: "theme unknown", breaks: func(u *User) { u.Theme = "auto" }, field: "theme", code: domain.CodeInvalidValue},
		{name: "locale absent", breaks: func(u *User) { u.Locale = "" }, field: "locale", code: domain.CodeRequired},
		{
			name:   "locale is a three letter code",
			breaks: func(u *User) { u.Locale = "eng" },
			field:  "locale",
			code:   domain.CodeInvalidValue,
		},
		{name: "locale is upper case", breaks: func(u *User) { u.Locale = "EN" }, field: "locale", code: domain.CodeInvalidValue},
		{
			name:   "locale region is not two letters",
			breaks: func(u *User) { u.Locale = "en-USA" },
			field:  "locale",
			code:   domain.CodeInvalidValue,
		},
		{
			name:   "locale is a full tag with a script",
			breaks: func(u *User) { u.Locale = "zh-Hant-TW" },
			field:  "locale",
			code:   domain.CodeInvalidValue,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			user := validUser()
			test.breaks(&user)

			assert.Equal(t, map[string][]string{test.field: {test.code}}, codesByField(t, user.Validate()))
		})
	}
}

// The accepted side of the same boundaries. A rule that refuses a legitimate
// account is as much a defect as one that admits a bad one.
func TestValidateAcceptsTheValuesAtTheEdgeOfEveryLimit(t *testing.T) {
	t.Parallel()

	longestLocal := strings.Repeat("a", 255-len("@example.test"))

	tests := []struct {
		name string
		with func(*User)
	}{
		{name: "a one character name", with: func(u *User) { u.Name = "A" }},
		{name: "a name of exactly the limit", with: func(u *User) { u.Name = strings.Repeat("a", 120) }},
		{name: "a name of multi-byte characters", with: func(u *User) { u.Name = strings.Repeat("é", 120) }},
		{name: "an address of exactly the limit", with: func(u *User) { u.Email = longestLocal + "@example.test" }},
		{name: "an address with a plus tag", with: func(u *User) { u.Email = "amara+medikube@example.test" }},
		{name: "an address in mixed case", with: func(u *User) { u.Email = "Amara@Example.Test" }},
		{name: "the admin role", with: func(u *User) { u.Role = RoleAdmin }},
		{name: "imperial units", with: func(u *User) { u.UnitSystem = UnitSystemImperial }},
		{name: "the dmy date format", with: func(u *User) { u.DateFormat = DateFormatDMY }},
		{name: "the dark theme", with: func(u *User) { u.Theme = ThemeDark }},
		{name: "a language with a region", with: func(u *User) { u.Locale = "en-GB" }},
		{name: "a language with a lower case region", with: func(u *User) { u.Locale = "pt-br" }},
		{name: "a disabled account", with: func(u *User) { u.DisabledAt = sentinelInstant }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			user := validUser()
			test.with(&user)

			assert.NoError(t, user.Validate())
		})
	}
}

// FR-027. Six broken fields are six entries, in the order the form renders
// them — not the first one the implementation tripped over.
func TestValidateReportsEveryOffendingFieldAtOnce(t *testing.T) {
	t.Parallel()

	user := User{Email: "amara", Name: "", Role: "superuser", UnitSystem: "us", Locale: "EN", DateFormat: "ymd", Theme: "auto"}

	var invalid *domain.ValidationError
	require.ErrorAs(t, user.Validate(), &invalid)

	got := make([]string, 0, len(invalid.Fields))
	for _, refusal := range invalid.Fields {
		got = append(got, refusal.Field)
	}

	assert.Equal(t, []string{"email", "name", "role", "unit_system", "locale", "date_format", "theme"}, got)
}

// Constitution VII. Every one of these messages is rendered to the person and
// carried in an error that other layers wrap; a rule that echoed the submitted
// value would put an address, and one day a diagnosis, into a log line.
func TestNoRefusalRepeatsTheValueThatWasSubmitted(t *testing.T) {
	t.Parallel()

	const address = "not-an-address-at-all"
	const displayName = "Amara Okafor Was Here"

	user := validUser()
	user.Email = address
	user.Name = strings.Repeat(displayName, 10)
	user.Locale = "klingon"

	var invalid *domain.ValidationError
	require.ErrorAs(t, user.Validate(), &invalid)

	for _, refusal := range invalid.Fields {
		assert.NotContains(t, refusal.Message, address)
		assert.NotContains(t, refusal.Message, displayName)
		assert.NotContains(t, refusal.Message, "klingon")
	}

	assert.NotContains(t, invalid.Error(), address)
	assert.NotContains(t, invalid.Error(), displayName)
}

// FR-012, as far as this package can enforce it: the permission tier and the
// account status live on the entity the server owns and on nothing else. The
// DTOs that FR-012 also constrains are asserted where they live, in
// internal/web/api — but a `Registration` or `ProfileUpdate` struct added here
// carrying a Role is the same defect one layer earlier, and this is the walk
// that refuses it.
func TestOnlyTheUserEntityCarriesTheRoleAndTheAccountStatus(t *testing.T) {
	t.Parallel()

	const serverOwned = "the permission tier and the account status are set by the server, " +
		"never by a structure a request can fill (FR-012)"

	sources, err := filepath.Glob("*.go")
	require.NoError(t, err)
	require.NotEmpty(t, sources)

	fileSet := token.NewFileSet()

	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}

		content, err := os.ReadFile(source)
		require.NoError(t, err)

		parsed, err := parser.ParseFile(fileSet, source, content, parser.SkipObjectResolution)
		require.NoError(t, err)

		ast.Inspect(parsed, func(node ast.Node) bool {
			declared, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}

			structure, ok := declared.Type.(*ast.StructType)
			if !ok || declared.Name.Name == "User" {
				return true
			}

			for _, field := range structure.Fields.List {
				for _, name := range field.Names {
					assert.NotEqual(t, "Role", name.Name, "%s.%s: %s", declared.Name.Name, name.Name, serverOwned)
					assert.NotEqual(t, "DisabledAt", name.Name, "%s.%s: %s", declared.Name.Name, name.Name, serverOwned)
				}
			}

			return true
		})
	}
}
