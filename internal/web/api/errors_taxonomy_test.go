package api_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/web"
)

// T310. specs/001-walking-skeleton/contracts/README.md's error table is the
// published contract for every non-2xx `code` this application produces; a
// handler that invents a fifteenth code, or a table row nobody wired a
// constant for, is a contract nobody is keeping. This walks internal/web's own
// source for every `CodeXxx` constant and compares it against the table in
// both directions.
//
// web.CodeBadRequest is the one documented exception (errors.go says so in as
// many words): a 4xx PocketBase itself raises before any MediKube handler
// runs, so it is not a row the contract names. It is asserted here by name and
// by the presence of that same explanation, so a second undocumented
// constant cannot hide behind the exception silently.

func TestEveryTableCodeHasAConstant(t *testing.T) {
	t.Parallel()

	table := tableCodes(t)
	require.NotEmpty(t, table, "parsed no codes out of contracts/README.md's error table")

	constants := webCodeConstants(t)

	values := make(map[string]bool, len(constants)+1)
	for _, code := range constants {
		values[code] = true
	}

	// domain.CodeValidationFailed is the one code that lives beside the type
	// that raises it rather than in internal/web (errors.go says so by name).
	values[domain.CodeValidationFailed] = true

	for code := range table {
		assert.Containsf(t, values, code,
			"contracts/README.md's error table names %q, but internal/web declares no CodeXxx constant with that value", code)
	}
}

func TestNoConstantNamesACodeOutsideTheTable(t *testing.T) {
	t.Parallel()

	table := tableCodes(t)
	constants := webCodeConstants(t)

	for name, code := range constants {
		if name == "CodeBadRequest" {
			assert.Equal(t, web.CodeBadRequest, code, "CodeBadRequest's value drifted from web.CodeBadRequest")

			continue
		}

		assert.Containsf(t, table, code,
			"internal/web declares %s = %q, which contracts/README.md's error table does not name", name, code)
	}
}

// TestCodeBadRequestStaysTheDocumentedException guards the escape hatch
// itself: if the comment explaining why it is exempt ever goes missing, the
// exemption above is undocumented rather than deliberate.
func TestCodeBadRequestStaysTheDocumentedException(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "internal/web/errors.go"))
	require.NoError(t, err)

	assert.Contains(t, string(raw), "CodeBadRequest is the one code contracts/README.md's table does not name")
}

// backtick pulls one `...` span. The table's code column sometimes carries a
// second, unrelated backtick span — `(+ \x60fields[]\x60)`, the literal
// message quoted in the 500 row — so only the FIRST span in a cell is ever the
// top-level `code`, except the one row that lists two codes for two statuses
// side by side ("`client_closed` / `timeout`"), which is matched as its own
// special case below.
var backtick = regexp.MustCompile("`([^`]+)`")

func tableCodes(t *testing.T) map[string]bool {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "specs/001-walking-skeleton/contracts/README.md"))
	require.NoError(t, err)

	start := strings.Index(string(raw), "## The error envelope, on every non-2xx")
	require.GreaterOrEqual(t, start, 0, "contracts/README.md no longer has the error envelope section this test reads")

	section := string(raw)[start:]
	if end := strings.Index(section[1:], "\n## "); end >= 0 {
		section = section[:end+1]
	}

	codes := map[string]bool{}

	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
			continue
		}

		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) < 2 {
			continue
		}

		cell := strings.TrimSpace(cells[len(cells)-1])
		if cell == "`code`" || strings.HasPrefix(cell, "---") {
			continue // the header row or the separator row
		}

		spans := backtick.FindAllStringSubmatch(cell, -1)
		if len(spans) == 0 {
			continue
		}

		// The one row listing two codes side by side for two conditions.
		if len(spans) == 2 && cell == "`"+spans[0][1]+"` / `"+spans[1][1]+"`" {
			codes[spans[0][1]] = true
			codes[spans[1][1]] = true

			continue
		}

		codes[spans[0][1]] = true
	}

	return codes
}

// webCodeConstants walks internal/web's own *.go sources (not its
// subpackages, and not tests) for every top-level `CodeXxx = "..."` string
// constant.
func webCodeConstants(t *testing.T) map[string]string {
	t.Helper()

	dir := filepath.Join(repoRoot(t), "internal/web")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	constants := map[string]string{}
	fset := token.NewFileSet()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
		require.NoErrorf(t, err, "parsing %s", entry.Name())

		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}

			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
					continue
				}

				name := value.Names[0].Name
				if !strings.HasPrefix(name, "Code") {
					continue
				}

				lit, ok := value.Values[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}

				unquoted, err := strconv.Unquote(lit.Value)
				require.NoErrorf(t, err, "unquoting %s's value", name)

				constants[name] = unquoted
			}
		}
	}

	return constants
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "walked to the filesystem root without finding go.mod")
		dir = parent
	}
}
