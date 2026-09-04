package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/cli"
	"medikube/internal/httproute"
)

// T294. e2e/routes.ts derives the browser gate's entire page inventory from
// `medikube routes --json`'s own output, so this asserts the thing that
// output actually is, wired against the real registry rather than a fixture:
// every KindPage route carries a non-empty Landmark and SmokeURL in the JSON
// e2e/routes.ts parses, under the exact field names it reads (landmark,
// smoke_url). internal/httproute/registry.go already panics at boot on a page
// missing either, so this is what proves that guarantee actually reaches the
// wire format the Playwright side depends on, and not just the Go struct.
func TestRoutesJSONCarriesEveryPagesLandmarkAndSmokeURL(t *testing.T) {
	t.Parallel()

	registry := httproute.Inventory()

	var stdout, stderr bytes.Buffer
	deps := cli.Deps{Stdout: &stdout, Stderr: &stderr, Routes: registry.Routes}

	handled, err := cli.Dispatch([]string{"routes", "--json"}, deps)
	require.True(t, handled)
	require.NoError(t, err)

	var rows []cli.RouteRow
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &rows))

	byOpID := make(map[string]cli.RouteRow, len(rows))
	for _, row := range rows {
		byOpID[row.OpID] = row
	}

	pages := 0
	for _, route := range registry.Routes() {
		if route.Kind != httproute.KindPage {
			continue
		}
		pages++

		row, wired := byOpID[route.OpID]
		require.Truef(t, wired, "e2e/routes.ts derives its inventory from this output, and %s is missing from it", route.OpID)

		assert.Equal(t, "page", row.Kind, "%s is a page in the registry but not in the JSON", route.OpID)
		assert.NotEmpty(t, row.Landmark, "%s carries no landmark for the browser gate to assert", route.OpID)
		assert.NotEmpty(t, row.SmokeURL, "%s carries no smoke URL for the browser gate to open", route.OpID)
		assert.NotContains(t, row.SmokeURL, "{", "%s's smoke URL still has an unbound parameter", route.OpID)
	}

	require.Positive(t, pages, "the registry declares no pages at all")
}
