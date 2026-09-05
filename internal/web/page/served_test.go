package page_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
)

// The gate that catches a page nobody wired, driven by THE ROUTE TABLE.
//
// Its predecessor iterated page.AccountPageOperations(), a curated list, and
// that is precisely how three finished-looking pages shipped answering 501:
// /forgot-password, /reset-password/{token} and /verify-email/{token} were
// declared in the route table with their landmarks and their smoke URLs, were
// asserted by name in the browser gate's target list — and were simply absent
// from the list this test walked, so nothing ever asked them for a response.
// A page can only escape this version by being written down below, with a
// reason.

// pageStubExempt is the one door out, and its value is the reason and the task
// that will close it. A map from opID to prose rather than a list of opIDs,
// following internal/store/filter_test.go's filterDSLExempt and
// internal/architecture/kind_literals_test.go's kindLiteralExempt: an exemption
// nobody had to justify is an exemption nobody revisits.
var pageStubExempt = map[string]string{}

func TestEveryPageInTheRouteTableIsServedRatherThanStubbed(t *testing.T) {
	t.Parallel()

	// Signed in, because a session page reached without one answers 403 and
	// this test would then be asserting the sign-in prompt is not a 501.
	rig := newBrowser(t)

	var pages, exercised int

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind != httproute.KindPage {
			continue
		}

		pages++

		reason, exempt := pageStubExempt[route.OpID]

		t.Run(route.OpID, func(t *testing.T) {
			require.NotEmptyf(t, route.SmokeURL, "%s declares no smoke URL, so nothing can open it", route.OpID)

			status, _, body := rig.get(route.SmokeURL)

			if exempt {
				// The exemption is checked in the other direction too. A page
				// that has since been implemented and left listed here would
				// otherwise keep its licence to answer 501 forever.
				assert.Equalf(t, http.StatusNotImplemented, status,
					"%s is exempt (%s) and is now served: strike it out of pageStubExempt", route.OpID, reason)

				return
			}

			assert.NotEqualf(t, http.StatusNotImplemented, status,
				"%s (%s) is published as a page and answers the stub: %s", route.OpID, route.Pattern(), body)
		})

		if !exempt {
			exercised++
		}
	}

	// The guard on the guard. Both counts are literal on purpose: a walk that
	// stopped finding pages, or an exemption map that grew until it covered
	// them, would otherwise pass this test by asserting nothing at all.
	require.Greater(t, pages, 8,
		"the route table has stopped declaring pages; this test is walking something else")
	require.Greater(t, exercised, 7,
		"almost every page is exempt: pageStubExempt has become an off switch for the gate")
}
