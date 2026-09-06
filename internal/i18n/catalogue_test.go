package i18n

import (
	"fmt"
	"io/fs"
	"os"
	"regexp"
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

// T038: a catalogue.md invariant (a)/(b) fixture backed by a real file on
// disk under testdata/, rather than an inline fstest.MapFS, so the exact
// failure text is checked against the same shape a real omission would
// produce.
func TestCatalogueDetectsAMissingID(t *testing.T) {
	t.Parallel()

	issues, err := catalogueIssues(os.DirFS("testdata/missing-id"), "locales")
	require.NoError(t, err)
	assert.Equal(t, []string{"pl: missing nav.timeline"}, issues)
}

func TestCatalogueDetectsASurplusID(t *testing.T) {
	t.Parallel()

	issues, err := catalogueIssues(os.DirFS("testdata/surplus-id"), "locales")
	require.NoError(t, err)
	assert.Equal(t, []string{"pl: surplus nav.timeline"}, issues)
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

// idPattern is US3-5/D-03: an id is dotted, lowercase, ASCII, at least one
// dot (contracts/catalogue.md §3's "namespace, then name" shape).
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)

// idIsItsOwnText reports whether id itself, dots and underscores turned to
// spaces and lowercased, is exactly the message's own English text — an id
// that degenerates into a copy of what it says (contracts/catalogue.md's
// "search.term" would collide with "Search term") would force a rename on
// every copy edit, unlike a single meaningful word shared between an id's
// last segment and a short label (action.save / "Save", which contracts/
// catalogue.md §3 gives as its own example of a *good* id) — comparing the
// full dotted id rather than only its last segment is what tells the two
// apart. Ids whose last segment is 3 characters or fewer are skipped: short
// abbreviations coincide with short words too often to be a useful signal.
func idIsItsOwnText(id string, m *goi18n.Message) bool {
	segs := strings.Split(id, ".")
	if len(segs[len(segs)-1]) <= 3 {
		return false
	}

	asText := strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(id), ".", " "), "_", " ")

	for _, form := range []string{m.Other, m.One} {
		if form != "" && strings.ToLower(form) == asText {
			return true
		}
	}

	return false
}

// catalogueLintIssues is US3-5/D-03: every en message names a non-empty
// description, every id matches idPattern, and no id is its own English text
// (lastSegmentIsItsOwnText). Checked against active.en.toml only — a
// translation file's own text is not what an id is named for.
func catalogueLintIssues(fsys fs.FS, dir string) ([]string, error) {
	files, err := parseCatalogue(fsys, dir)
	if err != nil {
		return nil, err
	}

	en, ok := files["en"]
	if !ok {
		return nil, fmt.Errorf("no active.en.toml under %s", dir)
	}

	var issues []string

	for id, m := range en.messages {
		if m.Description == "" {
			issues = append(issues, fmt.Sprintf("%s: no description", id))
		}

		if !idPattern.MatchString(id) {
			issues = append(issues, fmt.Sprintf("%s: id does not match %s", id, idPattern.String()))
		}

		if idIsItsOwnText(id, m) {
			issues = append(issues, fmt.Sprintf("%s: id is its own English text", id))
		}
	}

	sort.Strings(issues)

	return issues, nil
}

// TestCatalogueLintTheShippedEnglishFile is expected to be red until
// active.en.toml itself is fixed: at the time this test was written it
// reports six page.*.title ids with a camelCase segment (idPattern requires
// lowercase throughout) and one id ("search.term") that is its own English
// text. This test does not fix active.en.toml — that file is owned by
// another change — it only reports the violation, per US3-5/D-03.
func TestCatalogueLintTheShippedEnglishFile(t *testing.T) {
	t.Parallel()

	issues, err := catalogueLintIssues(localeFS, localesDir)
	require.NoError(t, err)
	assert.Empty(t, issues, "active.en.toml fails the id/description lint: %v", issues)
}

func TestCatalogueLintDetectsAMissingDescription(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"locales/active.en.toml": &fstest.MapFile{Data: []byte(`
[nav.timeline]
other = "Timeline"
`)},
	}

	issues, err := catalogueLintIssues(fsys, "locales")
	require.NoError(t, err)
	assert.Contains(t, issues, "nav.timeline: no description")
}

func TestCatalogueLintDetectsABadID(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"locales/active.en.toml": &fstest.MapFile{Data: []byte(`
[NavTimeline]
description = "Bad shape: not dotted, not lowercase"
other = "Timeline"
`)},
	}

	issues, err := catalogueLintIssues(fsys, "locales")
	require.NoError(t, err)
	assert.Contains(t, issues, "NavTimeline: id does not match "+idPattern.String())
}

func TestCatalogueLintDetectsAnIDThatIsItsOwnText(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"locales/active.en.toml": &fstest.MapFile{Data: []byte(`
[search.term]
description = "Placeholder text in the record search box"
other = "Search term"
`)},
	}

	issues, err := catalogueLintIssues(fsys, "locales")
	require.NoError(t, err)
	assert.Contains(t, issues, "search.term: id is its own English text")
}

func TestCatalogueLintDoesNotFlagASingleWordThatMatchesItsLastSegment(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"locales/active.en.toml": &fstest.MapFile{Data: []byte(`
[action.save]
description = "The submit button on every form"
other = "Save"
`)},
	}

	issues, err := catalogueLintIssues(fsys, "locales")
	require.NoError(t, err)
	assert.Empty(t, issues, "action.save is contracts/catalogue.md §3's own example of a good id")
}
