package architecture

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T003. Amendment 1.4.0 confines go-i18n and golang.org/x/text to
// internal/i18n — plan.md's Structure Decision. .golangci.yml's
// i18n-stays-in-its-package depguard rule is the build gate; this is the
// mechanical proof it fires, the same shape forbidden_deps_test.go already
// uses for the constitution's forbidden-module list.
var i18nConfinedModules = []string{
	"github.com/nicksnyder/go-i18n",
	"golang.org/x/text",
}

func TestOnlyI18nImportsGoI18nOrXText(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	imports := goImports(t, root)
	require.NotEmpty(t, imports, "walked the tree and found no Go imports at all — the walk is broken")

	for _, module := range i18nConfinedModules {
		t.Run(module, func(t *testing.T) {
			t.Parallel()

			var offenders []string
			for path, files := range imports {
				if !isUnder(path, module) {
					continue
				}

				for _, file := range files {
					if strings.HasPrefix(file, "internal/i18n/") {
						continue
					}
					offenders = append(offenders, path+" in "+file)
				}
			}

			sort.Strings(offenders)
			assert.Emptyf(t, offenders, "%s is imported outside internal/i18n — amendment 1.4.0 confines it there", module)
		})
	}
}
