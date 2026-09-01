package identity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// One body for four vocabularies. `valid` arrives as a method expression
// (Role.Valid), so the test names the method it is asserting rather than a
// closure that could quietly call something else.
func assertVocabulary[T ~string](t *testing.T, want []T, all func() []T, valid func(T) bool, rejected []string) {
	t.Helper()

	require.Equal(t, want, all(),
		"the accessor publishes the vocabulary the form offers, in the order it offers it")

	for _, value := range want {
		t.Run("accepts "+string(value), func(t *testing.T) {
			t.Parallel()

			assert.True(t, valid(value), "%q is published and must therefore be accepted", value)
		})
	}

	for _, value := range rejected {
		t.Run("refuses "+value, func(t *testing.T) {
			t.Parallel()

			assert.False(t, valid(T(value)), "%q is not in the vocabulary and must be refused", value)
		})
	}

	t.Run("the accessor clones", func(t *testing.T) {
		t.Parallel()

		got := all()
		require.NotEmpty(t, got)
		got[0] = T("mutated")

		assert.Equal(t, want, all(), "a caller reordered or overwrote the vocabulary for everybody")
	})
}

// data-model §1's four enums. The rejected column is deliberately made of the
// near misses — the empty string, another case, and a value borrowed from a
// sibling vocabulary — because those are the ones a hand-written check lets
// through.
func TestEachEnumAcceptsItsVocabularyAndNothingElse(t *testing.T) {
	t.Parallel()

	t.Run("Role", func(t *testing.T) {
		t.Parallel()

		assertVocabulary(t, []Role{RoleUser, RoleAdmin}, Roles, Role.Valid,
			[]string{"", " ", "User", "ADMIN", "superuser", "owner", "metric"})
	})

	t.Run("UnitSystem", func(t *testing.T) {
		t.Parallel()

		assertVocabulary(t, []UnitSystem{UnitSystemMetric, UnitSystemImperial}, UnitSystems, UnitSystem.Valid,
			[]string{"", "Metric", "METRIC", "us", "si", "user"})
	})

	t.Run("DateFormat", func(t *testing.T) {
		t.Parallel()

		assertVocabulary(t, []DateFormat{DateFormatISO, DateFormatDMY, DateFormatMDY}, DateFormats, DateFormat.Valid,
			[]string{"", "ISO", "iso8601", "ymd", "dd/mm/yyyy", "metric"})
	})

	t.Run("Theme", func(t *testing.T) {
		t.Parallel()

		assertVocabulary(t, []Theme{ThemeSystem, ThemeLight, ThemeDark}, Themes, Theme.Valid,
			[]string{"", "System", "DARK", "auto", "high-contrast", "iso"})
	})
}

// The defaults data-model §1 gives each column. They are values in the
// vocabulary by construction here, so a default that stopped being publishable
// fails before a migration can write it into a select field.
func TestTheDefaultsAreThemselvesInTheVocabulary(t *testing.T) {
	t.Parallel()

	assert.Equal(t, RoleUser, DefaultRole)
	assert.Equal(t, UnitSystemMetric, DefaultUnitSystem)
	assert.Equal(t, DateFormatISO, DefaultDateFormat)
	assert.Equal(t, ThemeSystem, DefaultTheme)
	assert.Equal(t, "en", DefaultLocale)

	assert.True(t, DefaultRole.Valid())
	assert.True(t, DefaultUnitSystem.Valid())
	assert.True(t, DefaultDateFormat.Valid())
	assert.True(t, DefaultTheme.Valid())
}
