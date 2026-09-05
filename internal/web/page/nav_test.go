package page_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
)

// medicationsListPath is the route table's own address for P4, read back
// rather than spelled, so a rename there cannot leave this test asserting
// against a path nothing serves.
func medicationsListPath(t *testing.T) string {
	t.Helper()

	for _, route := range httproute.Inventory().Routes() {
		if route.OpID == "medicationListPage" {
			return route.Path
		}
	}

	require.Fail(t, "medicationListPage is not in the route table")

	return ""
}

func settingsPath(t *testing.T) string {
	t.Helper()

	for _, route := range httproute.Inventory().Routes() {
		if route.OpID == "settingsPage" {
			return route.Path
		}
	}

	require.Fail(t, "settingsPage is not in the route table")

	return ""
}

// T253, FR-050. Every signed-in page's primary navigation offers a route back
// to the medication list and to settings, and the page a request is ON marks
// its own entry aria-current — so a person is never more than one link away
// from the record list, whichever page put them somewhere else.
func TestEverySignedInPageOffersTheMedicationListAndSettings(t *testing.T) {
	t.Parallel()

	rig := newBrowser(t)
	medications := medicationsListPath(t)
	settings := settingsPath(t)

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind != httproute.KindPage || route.Auth != httproute.AuthUser {
			continue
		}

		t.Run(route.OpID, func(t *testing.T) {
			t.Parallel()

			_, _, body := rig.get(route.SmokeURL)

			nav := navMarkup(t, body)

			assert.Containsf(t, nav, fmt.Sprintf("href=%q", medications), "%s's nav has no link back to the medication list", route.OpID)
			assert.Containsf(t, nav, fmt.Sprintf("href=%q", settings), "%s's nav has no link to settings", route.OpID)
		})
	}
}

// The page a request is on marks its own entry aria-current="page" — the
// medication list on itself, and nowhere else on the medication list's own
// entry.
func TestTheCurrentPageIsMarkedAriaCurrent(t *testing.T) {
	t.Parallel()

	rig := newBrowser(t)
	medications := medicationsListPath(t)

	_, _, body := rig.get(medications)
	nav := navMarkup(t, body)

	assert.Containsf(t, nav, fmt.Sprintf("href=%q aria-current=\"page\"", medications),
		"the medication list's own nav entry is not marked current while on that page")
}

// T253. Signed out, the navigation holds Sign in and Create account, which is
// FR-050's other half: a session-required page reached without a session
// still renders the shell, and the two links a signed-out visitor needs are
// what fill it.
func TestSignedOutNavigationOffersSignInAndCreateAccount(t *testing.T) {
	t.Parallel()

	rig := newBrowser(t).anonymous()

	_, _, body := rig.get("/login")
	nav := navMarkup(t, body)

	assert.Contains(t, nav, "Sign in")
	assert.Contains(t, nav, "Create account")
}

// navMarkup isolates the <nav>...</nav> element, so an href elsewhere in the
// page (a link inside the page's own landmark, say) cannot pass a check meant
// for the shell's navigation landmark.
func navMarkup(t *testing.T, body string) string {
	t.Helper()

	start := strings.Index(body, "<nav")
	require.GreaterOrEqualf(t, start, 0, "no <nav> element in:\n%s", body)

	end := strings.Index(body[start:], "</nav>")
	require.GreaterOrEqualf(t, end, 0, "unterminated <nav> element in:\n%s", body)

	return body[start : start+end+len("</nav>")]
}
