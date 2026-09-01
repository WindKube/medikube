package pb_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PocketBase's file routes are a second, undocumented way to reach a stored
// file: /api/files/{collection}/{recordId}/{filename} serves it, and
// /api/files/token mints the short-lived credential that unlocks a protected
// one. Neither applies MediKube's authorization. Files leave this application
// through MediKube's own /api/v1 routes or they do not leave it.
func TestPocketBaseFileRoutesAre404ForEveryOrdinaryCaller(t *testing.T) {
	t.Parallel()

	h := newHarness(t, bindMediKubeServe)

	genuine := h.genuine404(t)

	actors := []struct {
		name  string
		token string
	}{
		{name: "anonymous", token: ""},
		{name: "an ordinary signed-in person", token: h.userToken(t)},
	}

	for _, actor := range actors {
		t.Run(actor.name+"/file token", func(t *testing.T) {
			// Without the lockdown this is 401 for a guest and 200 for a
			// signed-in person — a working token factory for anybody with an
			// account.
			res := h.do(t, http.MethodPost, "/api/files/token", actor.token, "{}")

			assert.Equal(t, http.StatusNotFound, res.Status)
			assert.Equal(t, genuine.Body, res.Body)
		})

		for _, collection := range h.collectionNames(t) {
			t.Run(actor.name+"/download from "+collection, func(t *testing.T) {
				target := "/api/files/" + collection + "/" + probeRecordID + "/probe.png"
				res := h.do(t, http.MethodGet, target, actor.token, "")

				assert.Equal(t, http.StatusNotFound, res.Status)
				assert.Equal(t, genuine.Body, res.Body)
			})
		}
	}
}

// "No MediKube route issues a PocketBase file token" is a claim about code that
// does not exist, and the only way to make that a gate rather than a promise is
// to look. The two mint sites in v0.40.1 are apis/file.go:61-82 (which calls
// Record.NewFileToken) and core/record_tokens.go:145-168 itself.
func TestNoMediKubeCodeIssuesAPocketBaseFileToken(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"NewFileToken",
		"OnFileTokenRequest",
		"TokenTypeFile",
	}

	root := repoRoot(t)
	fset := token.NewFileSet()

	var findings []string

	for _, dir := range []string{"internal", "cmd"} {
		walkGoFiles(t, filepath.Join(root, dir), func(path string) {
			rel, relErr := filepath.Rel(root, path)
			require.NoError(t, relErr)

			// This file names them in its own table; exempting exactly it, by
			// path, keeps the walk honest about everything else.
			if filepath.ToSlash(rel) == "internal/platform/pb/no_file_routes_test.go" {
				return
			}

			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			require.NoError(t, err)

			ast.Inspect(file, func(n ast.Node) bool {
				ident, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				for _, name := range forbidden {
					if ident.Sel.Name == name {
						findings = append(findings, filepath.ToSlash(rel)+": "+name)
					}
				}

				return true
			})
		})
	}

	assert.Empty(t, findings,
		"a PocketBase file token bypasses MediKube's authorization entirely; files are served only from /api/v1 routes")
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)

	// internal/platform/pb/<this file> -> repository root
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func walkGoFiles(t *testing.T, dir string, fn func(path string)) {
	t.Helper()

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			switch {
			case strings.HasPrefix(entry.Name(), "."),
				entry.Name() == "node_modules",
				// `go` ignores testdata, so whatever is in there is not
				// necessarily parseable Go.
				entry.Name() == "testdata",
				entry.Name() == "pb_data":
				return filepath.SkipDir
			}

			return nil
		}

		if strings.HasSuffix(path, ".go") {
			fn(path)
		}

		return nil
	})
	require.NoError(t, err)
}
