package config

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const quickstartPath = "specs/001-walking-skeleton/quickstart.md"

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "cannot locate this test's own source file")

	return filepath.Join(filepath.Dir(file), "..", "..")
}

func readDoc(t *testing.T, rel string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
	require.NoError(t, err)

	return string(b)
}

var envSectionHeading = regexp.MustCompile(`(?im)^#{2,4} .*\b(environment|configuration)\b.*$`)

// documentedEnvironment returns the body of README's environment section. The
// section, not the whole file: a variable named once in a prose aside is not a
// documented setting.
func documentedEnvironment(t *testing.T, doc string) string {
	t.Helper()

	loc := envSectionHeading.FindStringIndex(doc)
	require.NotNil(t, loc,
		"README.md has no documented-environment section: add a heading matching %s "+
			"listing every MEDIKUBE_ variable (FR-051)", envSectionHeading)

	body := doc[loc[1]:]
	if next := strings.Index(body, "\n## "); next != -1 {
		body = body[:next]
	}

	return body
}

// FR-051: one documented set of settings. A field that exists but appears in no
// operator-facing document is undocumented configuration, which is the failure
// mode this test exists to make impossible.
func TestEveryVariableIsDocumented(t *testing.T) {
	t.Parallel()

	keys := envKeys()
	require.NotEmpty(t, keys)

	readme := documentedEnvironment(t, readDoc(t, "README.md"))
	quickstart := readDoc(t, quickstartPath)

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			assert.Contains(t, readme, key, "%s is missing from README.md's documented environment", key)
			assert.Contains(t, quickstart, key, "%s is missing from %s", key, quickstartPath)
		})
	}
}

// The other direction. A variable that documentation still advertises after the
// field behind it was renamed or removed is worse than no documentation.
func TestDocumentationInventsNoVariables(t *testing.T) {
	t.Parallel()

	declared := map[string]bool{}
	for _, key := range envKeys() {
		declared[key] = true
	}

	mentions := regexp.MustCompile(`MEDIKUBE_[A-Z0-9_]+`)
	for _, doc := range []string{"README.md", quickstartPath} {
		body := readDoc(t, doc)
		if doc == "README.md" {
			body = documentedEnvironment(t, body)
		}

		for _, key := range mentions.FindAllString(body, -1) {
			assert.True(t, declared[key], "%s documents %s, which no config field declares", doc, key)
		}
	}
}
