package identity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every field of User, and whether the log stream is allowed to see it. This
// map is the decision, written down: a field added to the struct without an
// entry here fails the first assertion below, which forces whoever adds it to
// answer the question rather than inherit an answer.
//
// Only the opaque record id is true. plan.md's FR-038 row says the marshaller
// emits ids and nothing else, and everything else on this struct either names a
// person (the address, the display name) or describes them (their preferences,
// when their account was disabled).
var userFieldMayBeLogged = map[string]bool{
	"ID":             true,
	"Email":          false,
	"EmailConfirmed": false,
	"Name":           false,
	"Role":           false,
	"UnitSystem":     false,
	"Locale":         false,
	"DateFormat":     false,
	"Theme":          false,
	"DisabledAt":     false,
	"CreatedAt":      false,
	"UpdatedAt":      false,
}

// A recognisable instant. Shared with validate_test.go, which needs a disabled
// account and no other property of the value.
var sentinelInstant = time.Date(2032, time.August, 20, 4, 5, 6, 0, time.UTC)

type sentinelField struct {
	name string
	// The strings that would appear in the rendered line if this field reached
	// it. Empty for a field whose type carries no distinguishable value — a
	// bool — which the exact-line assertion covers instead.
	tokens []string
}

// fillWithSentinels writes into every field of a User a value that could only
// have come from that field, and reports what to search the rendered line for.
// Reflection rather than a list somebody maintains: a new field is filled the
// moment it exists, and a field of a type this test cannot fill stops the build
// instead of being quietly skipped.
func fillWithSentinels(t *testing.T, u *User) []sentinelField {
	t.Helper()

	value := reflect.ValueOf(u).Elem()
	structure := value.Type()
	require.NotZero(t, structure.NumField(), "User has no fields at all")

	fields := make([]sentinelField, 0, structure.NumField())
	for i := range structure.NumField() {
		field := structure.Field(i)
		target := value.Field(i)

		mayBeLogged, decided := userFieldMayBeLogged[field.Name]
		require.Truef(t, decided,
			"User.%s is new and nobody has said whether the log stream may see it — add it to "+
				"userFieldMayBeLogged, and if the answer is true, say why in the same commit (FR-038)",
			field.Name)

		found := sentinelField{name: field.Name}

		switch {
		// The trailing Z keeps one sentinel from being a substring of another.
		case target.Kind() == reflect.String:
			sentinel := fmt.Sprintf("SENTINEL%dZ", i)
			target.SetString(sentinel)
			found.tokens = []string{sentinel}
		case target.Kind() == reflect.Bool:
			target.SetBool(true)
		case field.Type == reflect.TypeOf(time.Time{}):
			instant := sentinelInstant.AddDate(i, 0, 0)
			target.Set(reflect.ValueOf(instant))
			found.tokens = []string{
				instant.Format(time.RFC3339),
				instant.Format(time.DateOnly),
				fmt.Sprint(instant.Unix()),
			}
		default:
			t.Fatalf("User.%s is a %s, which this test does not know how to fill — teach it, then "+
				"decide deliberately whether MarshalZerologObject may emit it (FR-038)",
				field.Name, field.Type)
		}

		require.Falsef(t, mayBeLogged && len(found.tokens) == 0,
			"User.%s is allowed into the log stream but carries no value this test can look for", field.Name)

		fields = append(fields, found)
	}

	return fields
}

func renderLogLine(t *testing.T, u User) string {
	t.Helper()

	var buf bytes.Buffer
	// Log() and Send() so the line carries nothing but the marshaller's own
	// output — no level and no message to read the assertions past.
	logger := zerolog.New(&buf)
	logger.Log().EmbedObject(u).Send()

	return buf.String()
}

// FR-038 and constitution VII. Not "the fields we remembered to check": every
// field of the struct, filled with a value that could only have come from it,
// then looked for in the rendered line.
func TestNoUserFieldReachesTheLogStreamExceptItsIdentifier(t *testing.T) {
	t.Parallel()

	var user User
	fields := fillWithSentinels(t, &user)
	line := renderLogLine(t, user)

	emitted := 0
	for _, field := range fields {
		if userFieldMayBeLogged[field.name] {
			emitted++

			assert.Contains(t, line, field.tokens[0],
				"User.%s is the identifier the log line needs to be addressable at all", field.name)

			continue
		}

		for _, token := range field.tokens {
			assert.NotContains(t, line, token,
				"User.%s reached the log stream; an account names a person and "+
					"MarshalZerologObject emits the record id only (FR-038)", field.name)
		}
	}

	require.Positive(t, emitted, "nothing was emitted, so every assertion above passed vacuously")
}

// The named half of the same guarantee: the rendered line is exactly one key.
// A marshaller that started emitting a second — the role, the locale, however
// harmless it looked to whoever added it — fails here.
func TestTheUserLogLineIsExactlyItsIdentifier(t *testing.T) {
	t.Parallel()

	line := renderLogLine(t, User{
		ID:         "usr0000000001",
		Email:      "amara@example.test",
		Name:       "Amara Okafor",
		Role:       RoleAdmin,
		Locale:     "en-GB",
		DisabledAt: sentinelInstant,
	})

	assert.JSONEq(t, `{"user_id":"usr0000000001"}`, line)

	var keys map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &keys))
	assert.Len(t, keys, 1)
}

// The three FR-038 names in so many words, asserted by value and not only by
// the shape of the line above. The token is here because an auth record's
// tokenKey has no business on this struct in the first place: if one is ever
// added, this fails before it can be logged.
func TestTheAddressTheNameAndAnyTokenNeverReachTheLogStream(t *testing.T) {
	t.Parallel()

	user := validUser()
	user.Email = "amara@example.test"
	user.Name = "Amara Okafor"

	line := renderLogLine(t, user)

	for _, secret := range []string{"amara@example.test", "Amara Okafor", "example.test", "amara"} {
		assert.NotContains(t, line, secret)
	}

	assert.NotContains(t, line, "token")
	assert.NotContains(t, line, "password")
}

// data-model §1: a non-null disabled_at is what refuses a sign-in, and the zero
// value is an account in good standing. A method rather than a stored flag,
// because a second truth is a second thing to keep in step.
func TestIsDisabledIsTrueOnlyOnceTheOperatorHasSetTheInstant(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		when time.Time
		want bool
	}{
		{name: "an active account", when: time.Time{}, want: false},
		{name: "an account the operator disabled", when: sentinelInstant, want: true},
		{name: "an instant in the future", when: sentinelInstant.AddDate(100, 0, 0), want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, User{DisabledAt: test.when}.IsDisabled())
		})
	}
}
