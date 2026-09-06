package i18n

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/BurntSushi/toml"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var catalogueUnmarshalFuncs = map[string]goi18n.UnmarshalFunc{"toml": toml.Unmarshal}

// pluralFormsByLang is contracts/catalogue.md §4's table, transcribed: the
// exact CLDR forms each shipped language's own plural rule declares
// (verified there against go-i18n's internal/plural/rule_gen.go). A language
// this build does not ship is simply not checked for form completeness.
var pluralFormsByLang = map[string][]string{
	"en": {"one", "other"},
	"pl": {"one", "few", "many", "other"},
}

// catalogueFile is one parsed active.<lang>.toml, keyed by message id.
type catalogueFile struct {
	lang     string
	messages map[string]*goi18n.Message
}

// parseCatalogue reads every active.<lang>.toml under dir in fsys, keyed by
// the tag its filename carries.
func parseCatalogue(fsys fs.FS, dir string) (map[string]catalogueFile, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}

	files := make(map[string]catalogueFile, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "active.") || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}

		buf, err := fs.ReadFile(fsys, dir+"/"+entry.Name())
		if err != nil {
			return nil, err
		}

		parsed, err := goi18n.ParseMessageFileBytes(buf, entry.Name(), catalogueUnmarshalFuncs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}

		lang := parsed.Tag.String()
		messages := make(map[string]*goi18n.Message, len(parsed.Messages))
		for _, m := range parsed.Messages {
			messages[m.ID] = m
		}

		files[lang] = catalogueFile{lang: lang, messages: messages}
	}

	return files, nil
}

// formsPresent is which of the six CLDR form fields a parsed message
// actually set.
func formsPresent(m *goi18n.Message) []string {
	var forms []string

	if m.Zero != "" {
		forms = append(forms, "zero")
	}
	if m.One != "" {
		forms = append(forms, "one")
	}
	if m.Two != "" {
		forms = append(forms, "two")
	}
	if m.Few != "" {
		forms = append(forms, "few")
	}
	if m.Many != "" {
		forms = append(forms, "many")
	}
	if m.Other != "" {
		forms = append(forms, "other")
	}

	return forms
}

// catalogueIssues is invariants (a), (b) and the plural-form completeness
// invariant of data-model.md §4, checked against every file under dir in
// fsys, English being the reference for (a) and (b).
func catalogueIssues(fsys fs.FS, dir string) ([]string, error) {
	files, err := parseCatalogue(fsys, dir)
	if err != nil {
		return nil, err
	}

	ref, ok := files["en"]
	if !ok {
		return nil, fmt.Errorf("no active.en.toml under %s", dir)
	}

	var issues []string

	for lang, file := range files {
		if lang == "en" {
			continue
		}

		for id := range ref.messages {
			if _, present := file.messages[id]; !present {
				issues = append(issues, fmt.Sprintf("%s: missing %s", lang, id))
			}
		}

		for id := range file.messages {
			if _, present := ref.messages[id]; !present {
				issues = append(issues, fmt.Sprintf("%s: surplus %s", lang, id))
			}
		}
	}

	for lang, file := range files {
		required, known := pluralFormsByLang[lang]
		if !known {
			continue
		}

		wanted := append([]string(nil), required...)
		sort.Strings(wanted)

		for id, m := range file.messages {
			present := formsPresent(m)
			if len(present) < 2 {
				// A fixed-form message (no count) sets only "other".
				continue
			}

			sort.Strings(present)
			if strings.Join(present, ",") != strings.Join(wanted, ",") {
				issues = append(issues, fmt.Sprintf("%s: %s has forms %v, want %v", lang, id, present, wanted))
			}
		}
	}

	sort.Strings(issues)

	return issues, nil
}

func TestCatalogueTheShippedFilesMatch(t *testing.T) {
	t.Parallel()

	issues, err := catalogueIssues(localeFS, localesDir)
	require.NoError(t, err)
	assert.Empty(t, issues, "the shipped catalogues disagree: %v", issues)
}

func TestCatalogueDetectsAMissingID(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"locales/active.en.toml": &fstest.MapFile{Data: []byte(`
[language.name]
other = "English"
[nav.timeline]
other = "Timeline"
`)},
		"locales/active.pl.toml": &fstest.MapFile{Data: []byte(`
[language.name]
other = "Polski"
`)},
	}

	issues, err := catalogueIssues(fsys, "locales")
	require.NoError(t, err)
	assert.Contains(t, issues, "pl: missing nav.timeline")
}

func TestCatalogueDetectsASurplusID(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"locales/active.en.toml": &fstest.MapFile{Data: []byte(`
[language.name]
other = "English"
`)},
		"locales/active.pl.toml": &fstest.MapFile{Data: []byte(`
[language.name]
other = "Polski"
[nav.timeline]
other = "Oś czasu"
`)},
	}

	issues, err := catalogueIssues(fsys, "locales")
	require.NoError(t, err)
	assert.Contains(t, issues, "pl: surplus nav.timeline")
}

func TestCatalogueDetectsIncompletePluralForms(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"locales/active.en.toml": &fstest.MapFile{Data: []byte(`
[language.name]
other = "English"
[kind.allergy]
one = "{{.PluralCount}} allergy"
other = "{{.PluralCount}} allergies"
`)},
		"locales/active.pl.toml": &fstest.MapFile{Data: []byte(`
[language.name]
other = "Polski"
[kind.allergy]
one = "{{.PluralCount}} alergia"
other = "{{.PluralCount}} alergii"
`)},
	}

	issues, err := catalogueIssues(fsys, "locales")
	require.NoError(t, err)
	require.NotEmpty(t, issues)
	assert.Contains(t, issues[0], "kind.allergy")
	assert.Contains(t, issues[0], "pl:")
}
