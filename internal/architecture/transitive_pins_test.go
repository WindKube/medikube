package architecture

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A module MediKube depends on without choosing its version.
//
// plan.md's dependency table pins spf13/cobra as "Transitive, via PocketBase's
// RootCmd. Never a direct require": the version is pocketbase v0.40.1's own
// go.mod requirement, read from the module cache and verified, and it moves
// when PocketBase moves. A direct require makes it MediKube's choice — which
// is how a `go get -u` ends up pinning a cobra PocketBase was not built
// against, in the one package that constructs the whole command surface.
//
// The rule is enforced from both ends because either alone can be satisfied
// while the other is broken: a file can import the module while go.mod still
// says `// indirect` (go.mod is then simply stale), and go.mod can require it
// directly with nothing importing it.
type transitiveModule struct {
	path   string
	reason string
}

var transitiveModules = []transitiveModule{
	{
		path: "github.com/spf13/cobra",
		reason: "plan.md: transitive via PocketBase's RootCmd, never a direct require — " +
			"the version moves when PocketBase moves, and naming *cobra.Command in a " +
			"signature is what promotes it",
	},
}

func TestNoSourceFileImportsAModuleMediKubeDoesNotPin(t *testing.T) {
	t.Parallel()

	imports := goImports(t, repoRoot(t))
	require.NotEmpty(t, imports, "walked the tree and found no Go imports at all — the walk is broken")

	for _, transitive := range transitiveModules {
		t.Run(transitive.path, func(t *testing.T) {
			t.Parallel()

			var offenders []string

			for path, files := range imports {
				if isUnder(path, transitive.path) {
					offenders = append(offenders, path+" in "+strings.Join(files, ", "))
				}
			}

			sort.Strings(offenders)
			assert.Emptyf(t, offenders, "%s is imported directly — %s", transitive.path, transitive.reason)
		})
	}
}

func TestGoModKeepsAnUnpinnedModuleIndirect(t *testing.T) {
	t.Parallel()

	direct := directRequirements(t, repoRoot(t))
	require.NotEmpty(t, direct, "parsed no direct requirements out of go.mod — the parser is broken, not the module")

	// The parser is proved on a module that must be direct, so a parser that
	// found nothing directly required cannot pass this file by default.
	assert.Contains(t, direct, "github.com/pocketbase/pocketbase",
		"the direct-requirement parser no longer recognises a requirement MediKube certainly has")

	for _, transitive := range transitiveModules {
		t.Run(transitive.path, func(t *testing.T) {
			t.Parallel()

			assert.NotContainsf(t, direct, transitive.path,
				"go.mod requires %s directly — %s", transitive.path, transitive.reason)
		})
	}
}

// directRequirements returns the module paths go.mod requires WITHOUT an
// `// indirect` marker.
//
// Textual for the same reason goModRequirements is: parsing go.mod properly
// would mean depending on golang.org/x/mod, and a test that guards the
// dependency list should not widen it.
func directRequirements(t *testing.T, root string) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	require.NoError(t, err)

	var direct []string

	for _, line := range strings.Split(string(raw), "\n") {
		code := line
		if comment := strings.Index(line, "//"); comment >= 0 {
			if strings.Contains(line[comment:], "indirect") {
				continue
			}

			code = line[:comment]
		}

		fields := strings.Fields(code)
		if len(fields) > 0 && (fields[0] == "require" || fields[0] == "tool") {
			fields = fields[1:]
		}

		// A module path always contains a dot in its first element; `go 1.27`,
		// `module medikube` and a bare `)` never do. Two fields, because a
		// requirement is a path and a version and the `tool` directive is a
		// path alone.
		if len(fields) == 2 && strings.Contains(strings.SplitN(fields[0], "/", 2)[0], ".") {
			direct = append(direct, fields[0])
		}
	}

	return direct
}
