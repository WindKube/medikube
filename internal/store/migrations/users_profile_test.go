package migrations

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/identity"
)

// data-model §1's seven columns, asserted by shape rather than by name alone: a
// select field carrying the wrong vocabulary and one carrying none look the
// same from a distance, and the second refuses every write.
func TestUsersCarriesTheSevenMediKubeColumns(t *testing.T) {
	t.Parallel()

	users := usersSchema(t, newTestApp(t))

	t.Run("text columns", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			column   string
			required bool
			min      int
			max      int
		}{
			{column: usersFieldName, required: true, min: usersNameMin, max: usersNameMax},
			{column: usersFieldLocale, required: true, max: usersLocaleMax},
		}

		for _, testCase := range cases {
			field := users.Fields.GetByName(testCase.column)
			require.NotNilf(t, field, "%s is missing", testCase.column)

			text, isText := field.(*core.TextField)
			require.Truef(t, isText, "%s is a %s field, not text", testCase.column, field.Type())

			assert.Equal(t, testCase.required, text.Required, testCase.column)
			assert.Equal(t, testCase.min, text.Min, testCase.column)
			assert.Equal(t, testCase.max, text.Max, testCase.column)
		}
	})

	t.Run("enum columns", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			column string
			values []string
		}{
			{column: usersFieldRole, values: enumValues(identity.Roles())},
			{column: usersFieldUnitSystem, values: enumValues(identity.UnitSystems())},
			{column: usersFieldDateFormat, values: enumValues(identity.DateFormats())},
			{column: usersFieldTheme, values: enumValues(identity.Themes())},
		}

		for _, testCase := range cases {
			field := users.Fields.GetByName(testCase.column)
			require.NotNilf(t, field, "%s is missing", testCase.column)

			selectField, isSelect := field.(*core.SelectField)
			require.Truef(t, isSelect, "%s is a %s field, not a select", testCase.column, field.Type())

			assert.ElementsMatch(t, testCase.values, selectField.Values, testCase.column)
			assert.Equal(t, 1, selectField.MaxSelect, testCase.column)
			assert.True(t, selectField.Required, testCase.column)
		}
	})

	t.Run("disabled_at", func(t *testing.T) {
		t.Parallel()

		field := users.Fields.GetByName(usersFieldDisabledAt)
		require.NotNil(t, field)

		date, isDate := field.(*core.DateField)
		require.Truef(t, isDate, "disabled_at is a %s field, not a date", field.Type())
		assert.False(t, date.Required, "an account that is not disabled has no date to record")
	})
}

// data-model §0: this migration itself adds no file field. PocketBase ships
// `users` with an unprotected `avatar`, and the boot assertion refuses to
// start on one — so the migration drops it rather than leaving a gate that
// has to be waived. Phase 002's patients.photo is Protected and is asserted
// separately (internal/store/migrations/assertions_test.go); the blanket
// "zero file fields in the schema" this test used to make stopped being true
// the day that column shipped.
func TestTheMigrationLeavesNoFileFieldOnUsers(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	users := usersSchema(t, app)

	for _, field := range users.Fields {
		_, isFile := field.(*core.FileField)
		assert.Falsef(t, isFile, "users.%s is a file field", field.GetName())
	}

	assert.Nil(t, users.Fields.GetByName(stockUsersAvatarField))
	assert.Empty(t, users.OAuth2.MappedFields.AvatarURL,
		"the OAuth2 avatar mapping still points at a column that no longer exists")
}

// AuthRule is not one of the five, and getting it wrong in the direction the
// lockdown pushes everything else is catastrophic in a way nothing reports: nil
// disables authentication altogether — password, OAuth2, OTP — and FR-005
// depends on it staying an empty string.
func TestUsersAuthConfigurationIsWhatFR005AndFR008Require(t *testing.T) {
	t.Parallel()

	users := usersSchema(t, newTestApp(t))

	require.NotNil(t, users.AuthRule, "nil here disallows authentication altogether")
	assert.Empty(t, *users.AuthRule, "any record of the collection may authenticate")
	assert.Nil(t, users.ManageRule, "nobody manages another account's auth record through the API")

	assert.True(t, users.PasswordAuth.Enabled)
	assert.Equal(t, []string{core.FieldNameEmail}, users.PasswordAuth.IdentityFields)

	password, isPassword := users.Fields.GetByName(core.FieldNamePassword).(*core.PasswordField)
	require.True(t, isPassword)
	assert.Equal(t, usersPasswordMin, password.Min, "FR-004's published floor, enforced at the column")

	assert.EqualValues(t, 7*24*60*60, users.AuthToken.Duration, "FR-008: seven days")
	assert.EqualValues(t, 1800, users.PasswordResetToken.Duration, "FR-074: PocketBase's thirty minutes")
	assert.EqualValues(t, 86400, users.VerificationToken.Duration, "FR-075: PocketBase's twenty-four hours")

	assert.False(t, users.OAuth2.Enabled, "phase 006 owns external sign-in")
	assert.False(t, users.MFA.Enabled, "ordinary accounts do not use MFA in this phase")
}

// FR-003: two addresses differing only in letter case are the same address.
// PocketBase's own unique index is on the raw column and would let both in.
func TestTwoAccountsCannotDifferOnlyByTheCaseOfTheirAddress(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	users := usersSchema(t, app)

	var found bool
	for _, index := range users.Indexes {
		if strings.Contains(index, usersEmailLowerIndex) {
			found = true
			assert.Contains(t, strings.ToUpper(index), "UNIQUE")
			assert.Contains(t, index, "LOWER(email)")
		}
	}
	require.True(t, found, "%s is not declared", usersEmailLowerIndex)

	newUser(t, app, "amara@example.test")

	collection, err := app.FindCollectionByNameOrId(usersCollection)
	require.NoError(t, err)

	duplicate := core.NewRecord(collection)
	duplicate.SetEmail("Amara@Example.Test")
	duplicate.SetPassword("a-long-enough-password")
	duplicate.Set(usersFieldName, "Amara Again")
	duplicate.Set(usersFieldRole, string(identity.DefaultRole))
	duplicate.Set(usersFieldUnitSystem, string(identity.DefaultUnitSystem))
	duplicate.Set(usersFieldLocale, identity.DefaultLocale)
	duplicate.Set(usersFieldDateFormat, string(identity.DefaultDateFormat))
	duplicate.Set(usersFieldTheme, string(identity.DefaultTheme))

	assert.Error(t, app.Save(duplicate),
		"the second registration must lose, so exactly one account exists (spec Edge Cases)")
}

func usersSchema(t *testing.T, app core.App) *core.Collection {
	t.Helper()

	users, err := app.FindCollectionByNameOrId(usersCollection)
	require.NoError(t, err)

	return users
}
