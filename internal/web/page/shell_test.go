package page_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
	"medikube/internal/web/views/ids"
)

// T250. #error-banner and #toast are rendered on EVERY page, signed in or out,
// even though both start empty: Datastar patches by id and an element that
// does not exist cannot be patched, so a page missing either container is a
// live view that silently never updates.
func TestErrorBannerAndToastAreRenderedOnEveryPage(t *testing.T) {
	t.Parallel()

	rig := newBrowser(t)

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind != httproute.KindPage {
			continue
		}

		t.Run(route.OpID, func(t *testing.T) {
			t.Parallel()

			b := rig
			if route.Auth == httproute.AuthPublic {
				b = rig.anonymous()
			}

			_, _, body := b.get(route.SmokeURL)

			assert.Containsf(t, body, fmt.Sprintf("id=%q", ids.ErrorBanner), "%s renders no #error-banner", route.OpID)
			assert.Containsf(t, body, fmt.Sprintf("id=%q", ids.Toast), "%s renders no #toast", route.OpID)

			// Both containers are empty on first render: neither carries a
			// toast or a banner message on a plain GET, which the smoke run
			// only proves once these are non-empty on the routes that patch
			// them.
			assert.Regexpf(t,
				fmt.Sprintf("id=%q[^>]*></div>", ids.ErrorBanner),
				body, "%s's #error-banner is not empty on first render", route.OpID)
		})
	}
}

// The three error views carry both too — the whole reason FR-046 says the
// person "still has navigation" is that they still have the full shell.
func TestErrorBannerAndToastAreRenderedOnEveryErrorView(t *testing.T) {
	t.Parallel()

	rig := newBrowser(t).anonymous()

	for _, view := range httproute.Inventory().ErrorViews() {
		if view.SmokeURL == "" {
			continue
		}

		t.Run(string(view.Name), func(t *testing.T) {
			t.Parallel()

			_, _, body := rig.get(view.SmokeURL)

			assert.Containsf(t, body, fmt.Sprintf("id=%q", ids.ErrorBanner), "%s renders no #error-banner", view.Name)
			assert.Containsf(t, body, fmt.Sprintf("id=%q", ids.Toast), "%s renders no #toast", view.Name)
		})
	}
}

// A quick sanity check that the ids package's own two constants are what the
// shell actually uses — a rename on one side and not the other would make
// both tests above pass for the wrong reason (a substring that happens to
// still be there) rather than the right one.
func TestTheShellIDsAreNotAccidentallyGeneric(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, ids.ErrorBanner)
	require.NotEmpty(t, ids.Toast)
	require.False(t, strings.Contains(ids.ErrorBanner, " "))
	require.False(t, strings.Contains(ids.Toast, " "))
}
