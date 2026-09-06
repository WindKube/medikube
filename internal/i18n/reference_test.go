package i18n

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// referenceCall is one i18n.T/i18n.N call site a source file names literally.
type referenceCall struct {
	file string
	line int
	id   string
}

func (r referenceCall) String() string { return fmt.Sprintf("%s:%d: %s", r.file, r.line, r.id) }

// callPattern matches i18n.T(ctx, "id") and i18n.N(ctx, "id", ...) — D-08. It
// is deliberately anchored on the literal `ctx` first argument, which is what
// every call site in this codebase passes, and the closing quote must be
// followed immediately by `)` or `,`: a call that builds its id by
// concatenation (web.Message's i18n.T(ctx, "error."+code)) is a KnownDynamicIDs
// producer, not a literal this scan can check.
var callPattern = regexp.MustCompile(`i18n\.(?:T|N)\(ctx, "([^"]+)"\s*[,)]`)

// scanReferences walks every .templ and .go file under root (excluding
// *_templ.go, which templ generates) for i18n.T/i18n.N literal id calls.
func scanReferences(root string) ([]referenceCall, error) {
	var calls []referenceCall

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if strings.HasSuffix(name, "_templ.go") {
			return nil
		}
		if !strings.HasSuffix(name, ".templ") && !strings.HasSuffix(name, ".go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}

		for i, line := range strings.Split(string(content), "\n") {
			for _, match := range callPattern.FindAllStringSubmatch(line, -1) {
				calls = append(calls, referenceCall{file: rel, line: i + 1, id: match[1]})
			}
		}

		return nil
	})

	return calls, err
}

// TestEveryReferencedIDExistsInEnglish is invariant (c) of data-model.md §4:
// every i18n.T/i18n.N literal under internal/web, plus every id
// KnownDynamicIDs() names, must be defined in active.en.toml.
func TestEveryReferencedIDExistsInEnglish(t *testing.T) {
	t.Parallel()

	calls, err := scanReferences(filepath.Join("..", "web"))
	require.NoError(t, err)

	catalogue, err := parseCatalogue(localeFS, localesDir)
	require.NoError(t, err)

	defined := catalogue["en"].messages

	var missing []string
	for _, call := range calls {
		if _, ok := defined[call.id]; !ok {
			missing = append(missing, call.String())
		}
	}

	for _, id := range KnownDynamicIDs() {
		if _, ok := defined[id]; !ok {
			missing = append(missing, "KnownDynamicIDs: "+id)
		}
	}

	assert.Empty(t, missing, "referenced but not in active.en.toml: %v", missing)
}

// TestScanReferencesFindsAnUndefinedID proves the scan itself catches what
// it is meant to: a call site naming an id nothing defines.
func TestScanReferencesFindsAnUndefinedID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "example.templ"),
		[]byte(`package example

templ Example() {
	<span>{ i18n.T(ctx, "nav.nonexistent") }</span>
}
`),
		0o600,
	))

	calls, err := scanReferences(dir)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, "nav.nonexistent", calls[0].id)

	catalogue, err := parseCatalogue(localeFS, localesDir)
	require.NoError(t, err)

	_, defined := catalogue["en"].messages[calls[0].id]
	assert.False(t, defined, "the fixture's id must not be a real catalogue entry")
}
