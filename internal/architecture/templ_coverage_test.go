package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T259, Principle VIII, SC-014. Every templ component under
// internal/web/views/** must be reachable from something a *_test.go in its
// own directory renders — directly, or through another component in the same
// package that wraps it. A component nobody's test ever renders is one the
// render gate only catches once it is already on a page.
var templDeclaration = regexp.MustCompile(`(?m)^templ\s+(\w+)\(`)

func TestEveryTemplComponentIsRenderedByATest(t *testing.T) {
	t.Parallel()

	webRoot := filepath.Join(repoRoot(t), "internal/web")

	dirs := map[string]bool{}
	err := filepath.WalkDir(filepath.Join(webRoot, "views"), func(path string, d os.DirEntry, err error) error {
		require.NoError(t, err)
		if !d.IsDir() && strings.HasSuffix(path, ".templ") {
			dirs[filepath.Dir(path)] = true
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, dirs)

	// A component's test does not have to live beside it — errors.templ's
	// views are exercised from internal/web/page/errors_test.go, for one — so
	// the root seed is every *_test.go under internal/web, not just this dir.
	testSource := concatTree(t, webRoot, "_test.go")

	var checked int

	for dir := range dirs {
		names, templSource := declaredComponents(t, dir)

		reachable := map[string]bool{}
		var mark func(string)
		mark = func(name string) {
			if reachable[name] {
				return
			}
			reachable[name] = true
			for _, other := range names {
				if other != name && strings.Contains(templSource, other+"(") {
					mark(other)
				}
			}
		}

		for _, name := range names {
			if strings.Contains(testSource, name+"(") {
				mark(name)
			}
		}

		for _, name := range names {
			checked++
			assert.Truef(t, reachable[name], "%s declares templ %s(...), which no test in %s reaches", dir, name, dir)
		}
	}

	require.Greater(t, checked, 20, "the walk found almost no templ components; it is not looking where it thinks it is")
}

func declaredComponents(t *testing.T, dir string) ([]string, string) {
	t.Helper()

	source := concatFiles(t, dir, ".templ")

	matches := templDeclaration.FindAllStringSubmatch(source, -1)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match[1])
	}

	return names, source
}

func concatTree(t *testing.T, root, suffix string) string {
	t.Helper()

	var all strings.Builder
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() || !strings.HasSuffix(path, suffix) {
			return nil
		}
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		all.Write(content)
		return nil
	})
	require.NoError(t, err)

	return all.String()
}

func concatFiles(t *testing.T, dir, suffix string) string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	var all strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, err)
		all.Write(content)
	}

	return all.String()
}
