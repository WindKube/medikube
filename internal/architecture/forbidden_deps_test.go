// Package architecture holds the assertions that keep MediKube's dependency
// surface honest. It carries no production code: the constitution's Forbidden
// Dependencies list is a build gate rather than a README paragraph
// (Principle IX), and this is the file where that gate is spent.
package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A module path prefix rather than an exact path, so that a major-version
// suffix (`/v2`), a fork's subdirectory or a plugin subpackage cannot walk
// around the entry.
type forbiddenModule struct {
	name   string
	path   string
	reason string
}

var forbiddenModules = []forbiddenModule{
	{
		name:   "Gin",
		path:   "github.com/gin-gonic/gin",
		reason: "PocketBase's router is the router; a second one gets the costs of both and the benefits of neither",
	},
	{
		name:   "Huma",
		path:   "github.com/danielgtaylor/huma",
		reason: "the OpenAPI document is built from MediKube's own route registry, not served by a second framework",
	},
	{
		name:   "Viper",
		path:   "github.com/spf13/viper",
		reason: "caarlos0/env into one validated struct is the only configuration mechanism MediKube defines",
	},
	{
		name:   "samber/mo",
		path:   "github.com/samber/mo",
		reason: "mo.Result severs the errors.Is / errors.As / %w chain that error mapping, Sentry and zerolog all depend on",
	},
	{
		name:   "samber/ro",
		path:   "github.com/samber/ro",
		reason: "not a read-only helper but a pre-1.0 reactive library, and it would sit in the path of the realtime layer",
	},
	{
		name:   "samber/slog-zerolog",
		path:   "github.com/samber/slog-zerolog",
		reason: "zerolog ships NewSlogHandler, so the Principle VI bridge needs no adapter",
	},
	{
		name:   "PocketBase jsvm",
		path:   "github.com/pocketbase/pocketbase/plugins/jsvm",
		reason: "MediKube ships no scripting runtime",
	},
	{
		// The cgo ban has a dependency half and a source half. This is the
		// dependency half: mattn/go-sqlite3 is the driver modernc.org/sqlite
		// exists to replace, and pulling it in silently ends CGO_ENABLED=0.
		name:   "cgo SQLite driver",
		path:   "github.com/mattn/go-sqlite3",
		reason: "it requires cgo, which would end the single static binary and the distroless image",
	},
}

// Word boundaries, so that Datastar's own vocabulary — reactive, reactivity —
// cannot be mistaken for a vendored SPA framework.
var forbiddenFrontend = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{name: "React", pattern: regexp.MustCompile(`(?i)\breact\b`)},
	{name: "HTMX", pattern: regexp.MustCompile(`(?i)\bhtmx\b`)},
	{name: "Alpine", pattern: regexp.MustCompile(`(?i)\balpine\b`)},
}

func TestGoModDeclaresNoForbiddenModule(t *testing.T) {
	t.Parallel()

	required := goModRequirements(t, repoRoot(t))
	require.NotEmpty(t, required, "parsed no requirements out of go.mod — the parser is broken, not the module")

	for _, forbidden := range forbiddenModules {
		t.Run(forbidden.name, func(t *testing.T) {
			t.Parallel()

			var offenders []string
			for _, module := range required {
				if isUnder(module, forbidden.path) {
					offenders = append(offenders, module)
				}
			}
			assert.Emptyf(t, offenders, "go.mod requires %s — %s", forbidden.name, forbidden.reason)
		})
	}
}

func TestNoSourceFileImportsAForbiddenModule(t *testing.T) {
	t.Parallel()

	imports := goImports(t, repoRoot(t))
	require.NotEmpty(t, imports, "walked the tree and found no Go imports at all — the walk is broken")

	for _, forbidden := range forbiddenModules {
		t.Run(forbidden.name, func(t *testing.T) {
			t.Parallel()

			var offenders []string
			for path, files := range imports {
				if isUnder(path, forbidden.path) {
					offenders = append(offenders, path+" in "+strings.Join(files, ", "))
				}
			}
			sort.Strings(offenders)
			assert.Emptyf(t, offenders, "%s is imported — %s", forbidden.name, forbidden.reason)
		})
	}
}

func TestNothingInTheTreeRequiresCgo(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	t.Run("no Go file imports C", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, goImports(t, root)["C"],
			`import "C" ends CGO_ENABLED=0, and with it the single static binary and the distroless image`)
	})

	t.Run("no C or C++ translation unit is committed", func(t *testing.T) {
		t.Parallel()

		native := map[string]bool{".c": true, ".cc": true, ".cpp": true, ".cxx": true, ".h": true, ".hpp": true, ".m": true, ".mm": true}

		var offenders []string
		walkRepo(t, root, func(rel string, _ fs.DirEntry) {
			if native[strings.ToLower(filepath.Ext(rel))] {
				offenders = append(offenders, rel)
			}
		})
		sort.Strings(offenders)
		assert.Empty(t, offenders, "a native translation unit in the tree means something intends to link against C")
	})
}

func TestNoFrontendFrameworkIsVendored(t *testing.T) {
	t.Parallel()

	// Only the directories that can hold a served asset, and only file types
	// that can carry one. Widening this to the whole repository would fail on
	// specs/ and CLAUDE.md, which name these frameworks in order to forbid them.
	assets := scanAssets(t, repoRoot(t))

	for _, framework := range forbiddenFrontend {
		t.Run(framework.name, func(t *testing.T) {
			t.Parallel()

			var offenders []string
			for rel, content := range assets {
				if framework.pattern.MatchString(rel) || framework.pattern.Match(content) {
					offenders = append(offenders, rel)
				}
			}
			sort.Strings(offenders)
			assert.Emptyf(t, offenders, "%s is vendored into the served assets; MediKube's interactivity is templ + Datastar", framework.name)
		})
	}
}

// Segment-wise, so that samber/mo is not satisfied by samber/lo while
// danielgtaylor/huma still catches danielgtaylor/huma/v2.
func isUnder(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "walked to the filesystem root without finding go.mod")
		dir = parent
	}
}

// Deliberately textual. Parsing go.mod properly would mean depending on
// golang.org/x/mod, and a test that guards the dependency list should not
// widen it.
func goModRequirements(t *testing.T, root string) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	require.NoError(t, err)

	var modules []string
	for _, line := range strings.Split(string(raw), "\n") {
		if comment := strings.Index(line, "//"); comment >= 0 {
			line = line[:comment]
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && (fields[0] == "require" || fields[0] == "exclude" || fields[0] == "replace" || fields[0] == "tool") {
			fields = fields[1:]
		}
		// A module path always contains a dot in its first element; `go 1.27`,
		// `module medikube` and a bare `)` never do.
		if len(fields) > 0 && strings.Contains(strings.SplitN(fields[0], "/", 2)[0], ".") {
			modules = append(modules, fields[0])
		}
	}
	return modules
}

// Import path to the files that import it, so a failure names the offender
// instead of only the offence.
func goImports(t *testing.T, root string) map[string][]string {
	t.Helper()

	imports := map[string][]string{}
	fset := token.NewFileSet()

	walkRepo(t, root, func(rel string, _ fs.DirEntry) {
		if filepath.Ext(rel) != ".go" {
			return
		}
		file, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, parser.ImportsOnly)
		require.NoErrorf(t, err, "parsing %s", rel)

		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			require.NoErrorf(t, err, "import path in %s", rel)
			imports[path] = append(imports[path], rel)
		}
	})
	return imports
}

func scanAssets(t *testing.T, root string) map[string][]byte {
	t.Helper()

	scannable := map[string]bool{".css": true, ".html": true, ".js": true, ".json": true, ".mjs": true, ".templ": true, ".ts": true}

	assets := map[string][]byte{}
	for _, dir := range []string{"assets", "internal/web", "e2e"} {
		if _, err := os.Stat(filepath.Join(root, dir)); os.IsNotExist(err) {
			continue
		}
		walkRepo(t, filepath.Join(root, dir), func(rel string, _ fs.DirEntry) {
			if !scannable[strings.ToLower(filepath.Ext(rel))] {
				return
			}
			content, err := os.ReadFile(filepath.Join(root, dir, rel))
			require.NoErrorf(t, err, "reading %s", rel)
			assets[filepath.Join(dir, rel)] = content
		})
	}
	return assets
}

// Skips what is not MediKube's to answer for: git plumbing, tool caches,
// installed node modules and the PocketBase data directory.
func walkRepo(t *testing.T, root string, visit func(rel string, entry fs.DirEntry)) {
	t.Helper()

	skip := map[string]bool{"node_modules": true, "pb_data": true}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (skip[entry.Name()] || strings.HasPrefix(entry.Name(), ".")) {
				return fs.SkipDir
			}
			return nil
		}
		visit(filepath.ToSlash(rel), entry)
		return nil
	})
	require.NoError(t, err)
}
