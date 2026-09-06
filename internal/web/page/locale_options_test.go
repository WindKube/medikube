package page

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/i18n"
	"medikube/internal/web/views/settings"
)

// T037, FR-010/US3-1: localeOptions takes the shipped-language list as a
// parameter rather than reading i18n.Supported() itself, so this asserts it
// offers whatever internal/i18n derives (here, the real embedded catalogue's
// own list) without this package's own code changing to add a language.
func TestLocaleOptionsOffersEveryShippedLanguage(t *testing.T) {
	t.Parallel()

	langs := i18n.Supported()
	options := localeOptions(langs, "pl")

	assert.Len(t, options, len(langs))

	var selected *settings.Option
	for i := range options {
		assert.Equal(t, langs[i].Tag.String(), options[i].Value)
		assert.Equal(t, langs[i].Name, options[i].Label)

		if options[i].Selected {
			selected = &options[i]
		}
	}

	if assert.NotNil(t, selected, "pl is shipped but nothing was selected") {
		assert.Equal(t, "pl", selected.Value)
	}
}
