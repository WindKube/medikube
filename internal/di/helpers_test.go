package di_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// syncBuffer is a log sink safe to write from several goroutines at once, and
// it has to be.
//
// samber/do shuts services down in parallel
// (do/v2@v2.1.0 scope.go:495 shutdownServicesWithoutDependenciesInParallel)
// and calls the injector's Logf from each of those goroutines. zerolog does
// not serialise writes — it requires the destination to — so a plain
// bytes.Buffer here is a data race the -race detector finds on every shutdown
// with more than one service in it. In production the destination is os.Stdout,
// where each write is one write(2) and the question does not arise.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// packagesImporting walks the repository and returns, sorted and deduplicated,
// the directories of every non-test Go file that imports importPath.
//
// It parses rather than greps, so a path in a comment or a string is not a
// hit, and it reads the whole tree rather than a list somebody maintains,
// which is the only version of this assertion that cannot be walked past by
// adding a file.
func packagesImporting(tb testing.TB, importPath string) []string {
	tb.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(tb, err)

	var packages []string

	scanned := 0
	fileSet := token.NewFileSet()

	require.NoError(tb, filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			if skippedDir(entry.Name()) && path != root {
				return fs.SkipDir
			}

			return nil
		}

		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
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
			if unquoteErr != nil || unquoted != importPath {
				continue
			}

			packages = append(packages, filepath.ToSlash(filepath.Dir(relative)))
		}

		return nil
	}))

	require.Greater(tb, scanned, 20, "the walk found almost no Go files; it is not looking where it thinks it is")

	slices.Sort(packages)

	return slices.Compact(packages)
}

func skippedDir(name string) bool {
	return name == ".git" || name == ".bin" || name == "node_modules" || name == "vendor" ||
		name == "pb_data" || strings.HasPrefix(name, ".venv")
}
