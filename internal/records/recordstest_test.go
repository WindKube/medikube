package records_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The synthetic kind is a second implementation for Principle I and a second
// oneOf branch for the discriminator gate (plan.md CT-2). It is not a kind
// MediKube serves, and the way that stays true is that nothing outside a test
// can reach the package that declares it — a registration is one line, and a
// composition root that grew one would ship a fake clinical kind with real
// routes, real pages and a real audit target.
//
// The other half of the assertion lives in internal/di, which checks the
// registry it builds reports no synthetic kinds. This half is the cheaper one
// and it fails at the import rather than at boot.
const recordstestPackage = "medikube/internal/records/recordstest"

func TestOnlyTestFilesImportRecordstest(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	var (
		production []string
		tests      []string
		scanned    int
	)

	fileSet := token.NewFileSet()

	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			if skippedDir(entry.Name()) && path != root {
				return fs.SkipDir
			}

			return nil
		}

		if filepath.Ext(path) != ".go" {
			return nil
		}

		scanned++

		parsed, parseErr := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly|parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}

		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		for _, imported := range parsed.Imports {
			unquoted, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil || unquoted != recordstestPackage {
				continue
			}

			if strings.HasSuffix(relative, "_test.go") {
				tests = append(tests, relative)
			} else {
				production = append(production, relative)
			}
		}

		return nil
	}))

	require.Greater(t, scanned, 20, "the walk found almost no Go files; it is not looking where it thinks it is")
	require.NotEmpty(t, tests,
		"nothing imports %s at all, so this guard is asserting the absence of something that does not exist", recordstestPackage)

	sort.Strings(production)
	assert.Emptyf(t, production,
		"a file that is not a test imports %s — the synthetic kind is not a kind MediKube serves", recordstestPackage)
}

func skippedDir(name string) bool {
	return name == ".git" || name == ".bin" || name == "node_modules" || name == "vendor" ||
		name == "pb_data" || strings.HasPrefix(name, ".venv")
}
