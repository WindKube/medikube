package i18n

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/BurntSushi/toml"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FR-010/US3-1: adding a language is one new file, nothing else. This test
// proves loadBundle, Supported()'s and Resolve()'s own logic derive the
// shipped-language list from the files an fs.FS holds, by handing them a
// fixture holding the real active.en.toml (read from the embed, unmodified)
// plus a complete active.de.toml built by copying every id active.en.toml
// defines. Nothing outside internal/i18n needs to change to add a language —
// the settings control's own proof of that lives in
// internal/web/page/locale_options_test.go, since asserting it here would
// require internal/i18n to import internal/web/page.
const germanLanguageName = "Deutsch"

// germanFixture builds a complete active.de.toml: every id active.en.toml
// defines, same descriptions and forms, except the overrides given.
func germanFixture(t *testing.T, overrides map[string]string) []byte {
	t.Helper()

	enBytes, err := localeFS.ReadFile(localesDir + "/active.en.toml")
	require.NoError(t, err)

	parsed, err := goi18n.ParseMessageFileBytes(enBytes, "active.en.toml", catalogueUnmarshalFuncs)
	require.NoError(t, err)

	root := map[string]any{}
	for _, m := range parsed.Messages {
		leaf := map[string]any{}
		if m.Description != "" {
			leaf["description"] = m.Description
		}

		other := m.Other
		if ov, ok := overrides[m.ID]; ok {
			other = ov
		}

		for form, value := range map[string]string{
			"zero": m.Zero, "one": m.One, "two": m.Two, "few": m.Few, "many": m.Many, "other": other,
		} {
			if value != "" {
				leaf[form] = value
			}
		}

		insertNested(root, splitID(m.ID), leaf)
	}

	buf, err := toml.Marshal(root)
	require.NoError(t, err)

	return buf
}

func splitID(id string) []string {
	var parts []string
	start := 0

	for i := 0; i < len(id); i++ {
		if id[i] == '.' {
			parts = append(parts, id[start:i])
			start = i + 1
		}
	}

	return append(parts, id[start:])
}

func insertNested(root map[string]any, parts []string, leaf map[string]any) {
	cur := root

	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = leaf
			return
		}

		next, ok := cur[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[p] = next
		}

		cur = next
	}
}

func TestAddingALanguageIsOneFileAndNothingElse(t *testing.T) {
	t.Parallel()

	enBytes, err := localeFS.ReadFile(localesDir + "/active.en.toml")
	require.NoError(t, err)

	deBytes := germanFixture(t, map[string]string{languageNameID: germanLanguageName})

	fsys := fstest.MapFS{
		"locales/active.en.toml": &fstest.MapFile{Data: enBytes},
		"locales/active.de.toml": &fstest.MapFile{Data: deBytes},
	}

	b, langs, err := loadBundle(fsys, "locales")
	require.NoError(t, err)

	tags := make([]string, 0, len(langs))
	for _, l := range langs {
		tags = append(tags, l.Tag.String())
	}
	assert.Contains(t, tags, "de", "loadBundle did not derive de from the fixture directory")

	fallback := newLocalizerFrom(b, "en")
	l := resolve(b, langs, fallback, "", "de")
	require.Equal(t, "de", l.Tag.String(), "resolve did not pick the German localizer for de")

	ctx := With(context.Background(), l)
	assert.Equal(t, germanLanguageName, T(ctx, languageNameID))
}
