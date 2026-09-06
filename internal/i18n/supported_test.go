package i18n

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSupportedIsDerivedFromTheEmbeddedDirectory is FR-010/US3-1: adding one
// file makes a language appear everywhere Supported() is read, with no Go
// slice to edit. loadBundle is the fs.FS injection point Supported() itself
// uses against the real embedded locales/.
func TestSupportedIsDerivedFromTheEmbeddedDirectory(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"locales/active.en.toml": &fstest.MapFile{Data: []byte(`
[language.name]
other = "English"
`)},
		"locales/active.de.toml": &fstest.MapFile{Data: []byte(`
[language.name]
other = "Deutsch"
`)},
	}

	_, langs, err := loadBundle(fsys, "locales")
	require.NoError(t, err)

	tags := make([]string, 0, len(langs))
	for _, l := range langs {
		tags = append(tags, l.Tag.String())
	}

	assert.Contains(t, tags, "de")
	assert.Contains(t, tags, "en")
	assert.Len(t, langs, 2, "no file besides the two fixtures should appear")
}

func TestSupportedIsSortedByTag(t *testing.T) {
	t.Parallel()

	for _, l := range Supported() {
		require.NotEmpty(t, l.Name)
	}

	tags := make([]string, 0, len(Supported()))
	for _, l := range Supported() {
		tags = append(tags, l.Tag.String())
	}

	assert.IsIncreasing(t, tags)
}
