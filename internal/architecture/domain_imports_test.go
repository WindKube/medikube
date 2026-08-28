package architecture

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T043. depguard already denies PocketBase outside the [PB] packages, and
// denies net/http and templ under internal/domain — but a denylist only refuses
// what somebody thought to name. Principle II's rule has the other shape: the
// domain imports the standard library and zerolog, and nothing else. That is an
// allowlist, and this is where it is spent.
//
// The failure this prevents is not an exotic dependency added on a whim. It is
// the plausible one — a validator reaching for go-playground/validator, an
// entity reaching for samber/lo, a mapper reaching for a SQL driver — each
// arriving with a defensible reason, each leaving the domain untestable without
// the thing it now needs and unmovable across a PocketBase upgrade.
const domainTree = "internal/domain"

// The one third-party import the domain has. Principle VII puts the redacting
// marshaller on the entity itself, so the type that holds patient data is the
// type that decides what the log stream may see; that needs zerolog's Event in
// the domain and there is no version of it that does not.
var domainMayImport = map[string]bool{
	"github.com/rs/zerolog": true,
}

// Test files may reach for the assertion library and nothing else. Named
// rather than waved through with a blanket `_test.go` exemption, because a
// forbidden dependency arrives in a test first and most reasonably.
var domainTestsMayAlsoImport = map[string]bool{
	"github.com/stretchr/testify/assert":  true,
	"github.com/stretchr/testify/require": true,
}

// Standard-library packages that are inside the standard library and still
// outside the domain: being stdlib is not the test. net/http is stdlib, and an
// entity that knows a status code has taken on the edge's job.
//
// database/sql is denied and database/sql/driver is not, which is the whole
// distinction: driver is the marshalling contract domain.Date implements so the
// store can hand it to a column, and it names no database, opens no connection
// and pulls in no engine.
var stdlibDeniedInDomain = map[string]string{
	"net/http":          "Principle II: services decide, handlers speak HTTP",
	"net/http/httptest": "Principle II: an entity has no transport to test against",
	"net/url":           "Principle II: a URL is the edge's vocabulary",
	"database/sql":      "Principle II: the domain names no storage — database/sql/driver, the marshalling contract, is allowed and this is not",
	"html/template":     "Principle II: rendering belongs to internal/web",
	"text/template":     "Principle II: rendering belongs to internal/web",
	"log":               "Principle VI: one stream, and it is zerolog's",
	"log/slog":          "Principle VI: log/slog belongs to the PocketBase bridge in internal/logging",
}

func TestTheDomainImportsOnlyTheStandardLibraryAndZerolog(t *testing.T) {
	t.Parallel()

	var offences []string
	scanned := map[string]bool{}

	for imported, files := range goImports(t, repoRoot(t)) {
		for _, file := range files {
			if !isUnder(file, domainTree) {
				continue
			}

			scanned[file] = true

			if reason, denied := stdlibDeniedInDomain[imported]; denied {
				offences = append(offences, file+" imports "+imported+" — "+reason)
				continue
			}

			if domainImportIsPermitted(imported, strings.HasSuffix(file, "_test.go")) {
				continue
			}

			offences = append(offences, file+" imports "+imported+
				" — Principle II: internal/domain imports the standard library and zerolog, nothing else")
		}
	}

	require.Greater(t, len(scanned), 10,
		"the walk found almost no files under %s; it is not looking where it thinks it is", domainTree)

	sort.Strings(offences)
	assert.Empty(t, offences)
}

func domainImportIsPermitted(imported string, isTest bool) bool {
	switch {
	case domainMayImport[imported]:
		return true
	case isTest && domainTestsMayAlsoImport[imported]:
		return true
	case strings.HasPrefix(imported, "medikube/"+domainTree+"/"), imported == "medikube/"+domainTree:
		// The domain is one layer, not one package. A kind naming a role is
		// still the domain talking to itself.
		return true
	default:
		return isStandardLibrary(imported)
	}
}

// A module path always carries a dot in its first element — `github.com/...`,
// `gopkg.in/...`. A standard-library path never does. The same rule go.mod
// parsing uses above, for the same reason: it costs no dependency.
func isStandardLibrary(imported string) bool {
	first, _, _ := strings.Cut(imported, "/")

	return !strings.Contains(first, ".")
}
