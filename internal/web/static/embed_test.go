package static

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The vendored runtime is the one asset with no build step behind it, so nothing
// but this test would notice if it were truncated, replaced by a stub, or moved
// to a different version line.
func TestDatastarRuntimeIsVendoredAtV102(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, Datastar, "datastar.js embedded empty")
	assert.Greater(t, len(Datastar), 10_000, "runtime is implausibly small for a real bundle")
	assert.Contains(t, string(Datastar[:64]), "Datastar v1.0.2",
		"vendored runtime is not the v1.0.2 the Go SDK at v1.2.2 pairs with (research D-33)")
}
