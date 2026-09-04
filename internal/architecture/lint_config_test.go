package architecture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T298, T299. depguard_test.go and forbidigo_test.go each demonstrate their
// gate once by breaking it and watching `task lint:go` fail — that is the
// falsifiable half. This is the other half: the configuration those
// demonstrations depend on has to keep naming the right packages and the
// right patterns, or a later edit to .golangci.yml could narrow the gate
// silently and neither demonstration would ever run again to notice.
//
// It reads .golangci.yml as text rather than as YAML (research: no YAML
// library is a direct dependency here, and forbidden_deps_test.go already
// treats go.mod the same way for the same reason) and asserts the exact
// substrings the gate is built on.

func TestGolangciConfigNamesThePBBoundaryPackages(t *testing.T) {
	t.Parallel()

	config := golangciConfig(t)

	// Constitution Principle II: only these packages may import PocketBase.
	// Adding one is a plan.md amendment, not a lint tweak — so the list is
	// asserted verbatim rather than by count.
	pbPackages := []string{
		"cmd/medikube",
		"internal/logging",
		"internal/obs",
		"internal/store",
		"internal/platform/pb",
		"internal/httproute",
		"internal/web",
		"internal/cli",
		"internal/testsupport",
	}

	for _, pkg := range pbPackages {
		assert.Containsf(t, config, pkg,
			"the pocketbase-stays-in-adapters exemption list no longer names %s", pkg)
	}

	assert.Contains(t, config, "pocketbase-stays-in-adapters")
	assert.Contains(t, config, "github.com/pocketbase/pocketbase")
}

func TestGolangciConfigNamesTheForbidigoPatterns(t *testing.T) {
	t.Parallel()

	config := golangciConfig(t)

	// Principle VI's one-log-stream rule and the record-hook lockdown
	// (reconciliation C13), each as the literal pattern forbidigo_test.go's
	// demonstration is built against.
	patterns := []string{
		`\.Logger$`,
		`^(log/)?slog\.`,
		`^log\.(Print|Fatal|Panic)`,
		`OnRecords?(Create|Update|Delete|View|List)Request`,
	}

	for _, pattern := range patterns {
		assert.Containsf(t, config, pattern,
			"forbidigo no longer forbids the pattern %q", pattern)
	}

	assert.Contains(t, config, "forbidigo")
}

func golangciConfig(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".golangci.yml"))
	require.NoError(t, err)

	return string(raw)
}
