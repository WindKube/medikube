package httproute_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
	"medikube/internal/records"
)

// T183a, contracts/pages.md §3.5. records.StatusViews is the single source
// both the per-kind filters and the SmokeVariants read from: a status view
// added to the catalogue with no variant carried by a registered page route
// fails here, and a page's own SmokeVariants entry that names no catalogue
// kind is a status view that could outlive what it claims to cover.
func TestEveryStatusViewHasASmokeVariantOnARegisteredPage(t *testing.T) {
	t.Parallel()

	variants := make([]string, 0)
	for _, route := range httproute.Inventory().Routes() {
		if route.Kind != httproute.KindPage {
			require.Empty(t, route.SmokeVariants, "%s is not a page but carries SmokeVariants", route.OpID)

			continue
		}

		variants = append(variants, route.SmokeVariants...)
	}

	for _, view := range records.StatusViews {
		t.Run(view.Name, func(t *testing.T) {
			prefix := "/" + view.Kind.Segment() + "?"

			found := false
			for _, variant := range variants {
				if strings.HasPrefix(variant, prefix) && strings.Contains(variant, view.Query) {
					found = true

					break
				}
			}

			assert.True(t, found, "no registered page carries a SmokeVariant for %s (%s)", view.Name, prefix)
		})
	}
}

// The mirror: every SmokeVariant this page table carries is concrete, with no
// unbound parameter — describePage already refuses one at boot, so this is a
// second, cheaper witness that the inventory a test builds agrees with it.
func TestEverySmokeVariantIsConcrete(t *testing.T) {
	t.Parallel()

	for _, route := range httproute.Inventory().Routes() {
		for _, variant := range route.SmokeVariants {
			assert.True(t, strings.HasPrefix(variant, "/"), "%s's variant %q is not absolute", route.OpID, variant)
			assert.NotContains(t, variant, "{", "%s's variant %q still has an unbound parameter", route.OpID, variant)
		}
	}
}
