package i18n

import (
	"context"
	"testing"

	"github.com/BurntSushi/toml"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestResolveOrder(t *testing.T) {
	t.Parallel()

	t.Run("account locale wins over Accept-Language", func(t *testing.T) {
		t.Parallel()

		l := Resolve("pl", "en;q=0.9")
		assert.Equal(t, "pl", l.Tag.String())
	})

	t.Run("Accept-Language with a region matches the base language", func(t *testing.T) {
		t.Parallel()

		l := Resolve("", "pl-PL,en;q=0.8")
		assert.Equal(t, "pl", l.Tag.String())
	})

	t.Run("an unknown account locale falls back to Accept-Language", func(t *testing.T) {
		t.Parallel()

		l := Resolve("xx", "pl")
		assert.Equal(t, "pl", l.Tag.String())
	})

	t.Run("nothing recognisable resolves to English", func(t *testing.T) {
		t.Parallel()

		l := Resolve("", "de,fr;q=0.8")
		assert.Equal(t, "en", l.Tag.String())
	})

	t.Run("no Accept-Language at all resolves to English", func(t *testing.T) {
		t.Parallel()

		l := Resolve("", "")
		assert.Equal(t, "en", l.Tag.String())
	})
}

// FR-009: a phrase present in English but missing in the chosen language
// renders the English text — go-i18n's own behaviour for a Localizer built
// with the language and English as fallbacks (contracts/catalogue.md §5).
// The shipped catalogues are deliberately complete, so this builds a bundle
// with a genuine gap to exercise the fallback itself.
func TestTFallsBackToEnglishWhenTheChosenLanguageLacksAnID(t *testing.T) {
	t.Parallel()

	b := goi18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	_, err := b.ParseMessageFileBytes([]byte(`
[only.in.english]
other = "English only"
`), "active.en.toml")
	require.NoError(t, err)

	_, err = b.ParseMessageFileBytes([]byte(`
[language.name]
other = "Polski"
`), "active.pl.toml")
	require.NoError(t, err)

	l := &Localizer{inner: goi18n.NewLocalizer(b, "pl", "en"), Tag: language.Make("pl")}
	ctx := With(context.Background(), l)

	assert.Equal(t, "English only", T(ctx, "only.in.english"))
}

func TestTWithNoLocalizerOnContextIsEnglish(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Timeline", T(context.Background(), "nav.timeline"))
}

func TestTOnAnUndefinedIDReturnsTheIDRatherThanPanicking(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "nav.does_not_exist", T(context.Background(), "nav.does_not_exist"))
}

func TestNReturnsEveryPolishPluralForm(t *testing.T) {
	t.Parallel()

	ctx := With(context.Background(), Resolve("pl", ""))

	cases := []struct {
		count int
		want  string
	}{
		{1, "1 alergia"},
		{2, "2 alergie"},
		{5, "5 alergii"},
		{22, "22 alergie"},
		{0, "0 alergii"},
	}

	for _, c := range cases {
		got := N(ctx, "kind.allergy", c.count)
		assert.Equalf(t, c.want, got, "count %d", c.count)
	}
}

func TestNInEnglishUsesOnlyOneAndOther(t *testing.T) {
	t.Parallel()

	ctx := With(context.Background(), Resolve("en", ""))

	assert.Equal(t, "1 allergy", N(ctx, "kind.allergy", 1))
	assert.Equal(t, "2 allergies", N(ctx, "kind.allergy", 2))
}

func TestIsSupported(t *testing.T) {
	t.Parallel()

	assert.True(t, IsSupported("pl"))
	assert.True(t, IsSupported("pl-PL"))
	assert.True(t, IsSupported("en"))
	assert.False(t, IsSupported("xx"))
	assert.False(t, IsSupported(""))
}

func TestSupportedListsEnglishAndPolish(t *testing.T) {
	t.Parallel()

	tags := make([]string, 0, len(Supported()))
	for _, l := range Supported() {
		tags = append(tags, l.Tag.String())
	}

	require.Contains(t, tags, "en")
	require.Contains(t, tags, "pl")
}
