package page_test

import (
	"html"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/testsupport"
	"medikube/internal/web/apitest"
	"medikube/internal/web/page"
	"medikube/internal/web/views/auth"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/settings"
	"medikube/internal/web/views/shell"
)

// T220 and T223n. The six pages of the account surface, driven through the
// whole edge: the middleware chain, the route table and the renderer, exactly
// as a browser reaches them.

func newBrowserWith(t *testing.T, options ...apitest.Option) *browser {
	t.Helper()

	instance := apitest.New(t, options...)

	return &browser{
		t:       t,
		app:     instance.App,
		handler: testsupport.NewEdgeHandler(t, instance.App),
		token:   testsupport.UserToken(t, instance.App, testsupport.AccountAEmail),
	}
}

// accountRoutes is the six pages as the route table declares them, so a test
// addresses what the router serves rather than a path it composed.
func accountRoutes(t *testing.T) map[string]httproute.Route {
	t.Helper()

	wanted := map[string]httproute.Route{}

	for _, route := range httproute.Inventory().Routes() {
		for _, opID := range page.AccountPageOperations() {
			if route.OpID == opID {
				wanted[opID] = route
			}
		}
	}

	require.Len(t, wanted, len(page.AccountPageOperations()),
		"the route table no longer declares every account page")

	return wanted
}

// The registration contract of contracts/pages.md, run against what the handler
// actually renders. A route may declare region[name="Settings"] and render a
// section labelled something else, and nothing outside Playwright would notice.
func TestEachAccountPageRendersTheLandmarkItsRouteDeclares(t *testing.T) {
	t.Parallel()

	for opID, route := range accountRoutes(t) {
		t.Run(opID, func(t *testing.T) {
			t.Parallel()

			require.NotEmpty(t, route.Landmark, "%s declares no landmark", opID)
			require.NotEmpty(t, route.SmokeURL, "%s declares no smoke URL", opID)

			status, headers, body := newBrowser(t).get(route.SmokeURL)

			require.Equal(t, http.StatusOK, status,
				"the smoke URL the browser gate opens does not answer: %s", body)
			assert.Contains(t, headers.Get("Content-Type"), "text/html")

			// Two of these carry a form a credential is about to be typed
			// into and the third carries somebody's own address.
			assert.Equal(t, "private, no-store", headers.Get("Cache-Control"))

			assertLandmark(t, body, route.Landmark)

			assert.Contains(t, body, attribute("id", ids.Main))
			assert.Contains(t, body, attribute("id", ids.ErrorBanner))
			assert.Contains(t, body, attribute("id", ids.Toast))
			assert.Contains(t, body, shell.SuffixSeparator+shell.ProductName+"</title>")
		})
	}
}

// contracts/pages.md's title column, for the six rows this group serves.
func TestEachAccountPageIsTitledAsTheContractSpellsIt(t *testing.T) {
	t.Parallel()

	// P9's title is not its landmark: the gate resolves region[name="Email
	// confirmation"] and the tab says "Confirm your address", and
	// contracts/pages.md fixes both.
	titles := map[string]string{
		page.OpLoginPage:          auth.SignInLandmark,
		page.OpRegisterPage:       auth.CreateAccountLandmark,
		page.OpSettingsPage:       settings.SettingsLandmark,
		page.OpForgotPasswordPage: auth.ResetPasswordLandmark,
		page.OpResetPasswordPage:  auth.NewPasswordLandmark,
		page.OpVerifyEmailPage:    "Confirm your address",
	}

	routes := accountRoutes(t)

	for opID, title := range titles {
		t.Run(opID, func(t *testing.T) {
			t.Parallel()

			_, _, body := newBrowser(t).get(routes[opID].SmokeURL)

			assert.Contains(t, body, shell.TitleElement(title))
		})
	}
}

// FR-002 and defect D15, from the page's side. A closed instance renders the
// explanation INSIDE the ordinary frame and answers 200 — not 404 and not a
// missing route — and an open one renders the form. Without the open control
// every assertion below would pass on a page that rendered the explanation
// whatever the operator chose.
func TestTheSignUpPageExplainsItselfWhenRegistrationIsClosedAndOffersTheFormWhenItIsOpen(t *testing.T) {
	t.Parallel()

	path := accountRoutes(t)[page.OpRegisterPage].SmokeURL

	t.Run("closed, which is the default every deployment gets", func(t *testing.T) {
		t.Parallel()

		status, _, body := newBrowser(t).anonymous().get(path)

		require.Equal(t, http.StatusOK, status, body)
		assert.Contains(t, body, attribute("id", auth.RegistrationClosedID))
		assert.NotContains(t, body, attribute("id", ids.Field(ids.CreateAccountForm, auth.FieldPassword)),
			"a closed instance offered a password control it was always going to refuse")

		// The landmark is there either way: a landmark that appeared only under
		// one configuration is a page the browser gate cannot check on the
		// other one.
		assertLandmark(t, body, accountRoutes(t)[page.OpRegisterPage].Landmark)
	})

	t.Run("open", func(t *testing.T) {
		t.Parallel()

		status, _, body := newBrowserWith(t, apitest.WithRegistrationOpen(true)).anonymous().get(path)

		require.Equal(t, http.StatusOK, status, body)
		assert.NotContains(t, body, attribute("id", auth.RegistrationClosedID))
		assert.Contains(t, body, attribute("id", ids.Field(ids.CreateAccountForm, auth.FieldPassword)))
		assert.Contains(t, body, attribute("id", auth.PasswordRulesID),
			"the rules are published before the person chooses (FR-004)")
	})
}

// The two signed-out pages are reachable with no session at all. They are the
// only way anybody gets one, so a session requirement here would be a locked
// door with the key inside.
func TestTheSignedOutPagesAreReachableWithNoSession(t *testing.T) {
	t.Parallel()

	routes := accountRoutes(t)

	for _, opID := range []string{page.OpLoginPage, page.OpRegisterPage} {
		t.Run(opID, func(t *testing.T) {
			t.Parallel()

			status, _, body := newBrowser(t).anonymous().get(routes[opID].SmokeURL)

			require.Equal(t, http.StatusOK, status, body)
		})
	}
}

// contracts/pages.md's E2: a page that needs a session and has none is 403 with
// the sign-in prompt, not 404. The existence of /settings is not information
// about anybody, so there is no oracle to close by hiding it.
func TestTheSettingsPageRefusesACallerWithNoSession(t *testing.T) {
	t.Parallel()

	status, _, body := newBrowser(t).anonymous().get(accountRoutes(t)[page.OpSettingsPage].SmokeURL)

	assert.Equal(t, http.StatusForbidden, status, body)
	assert.NotEqual(t, http.StatusNotFound, status,
		"the settings page answers as though it did not exist, which is the answer for somebody else's data")
}

// FR-011 and FR-013 from the page's side: the page renders THIS account's
// values, and the deletion confirmation states what deleting it destroys.
func TestTheSettingsPageRendersTheSignedInAccountAndWhatItHolds(t *testing.T) {
	t.Parallel()

	path := accountRoutes(t)[page.OpSettingsPage].SmokeURL
	status, _, body := newBrowser(t).get(path)

	require.Equal(t, http.StatusOK, status, body)

	assert.Contains(t, body, testsupport.AccountAEmail)
	assert.Contains(t, body, html.EscapeString(testsupport.AccountAName))

	// The count comes from the same counter getMe answers with, and the label
	// is the kind's own published segment.
	holdings := section(t, body, attribute("id", settings.HoldingsID))
	assert.Contains(t, holdings, kind.Medication.Segment())
	assert.Contains(t, holdings, strconv.Itoa(testsupport.AccountAMedicationCount))

	// And nothing of anybody else's.
	assert.NotContains(t, body, testsupport.AccountBEmail)
}

// The same page for the account that holds nothing: the confirmation still
// states the consequence, with a zero rather than an absent line, because a
// danger zone that fell silent for an empty account would be silent for the one
// case where the person most needs to be sure.
func TestTheDangerZoneStatesAZeroForAnAccountThatHoldsNothing(t *testing.T) {
	t.Parallel()

	path := accountRoutes(t)[page.OpSettingsPage].SmokeURL
	_, _, body := newBrowser(t).as(testsupport.AccountCEmail).get(path)

	holdings := section(t, body, attribute("id", settings.HoldingsID))

	assert.Contains(t, holdings, kind.Medication.Segment())
	assert.Contains(t, holdings, strconv.Itoa(testsupport.AccountCMedicationCount))
}

// T222 and FR-075. Account C is seeded with an unconfirmed address, so this
// state is one the smoke run walks through rather than a branch somebody
// asserted once.
func TestTheConfirmationStateOnThePageIsTheOneTheAccountIsIn(t *testing.T) {
	t.Parallel()

	path := accountRoutes(t)[page.OpSettingsPage].SmokeURL
	rig := newBrowser(t)

	for name, run := range map[string]struct {
		email    string
		expected string
		absent   string
	}{
		"a confirmed address": {
			email:    testsupport.AccountAEmail,
			expected: settings.EmailConfirmedID,
			absent:   settings.EmailUnconfirmedID,
		},
		"an address nobody has confirmed": {
			email:    testsupport.AccountCEmail,
			expected: settings.EmailUnconfirmedID,
			absent:   settings.EmailConfirmedID,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, body := rig.as(run.email).get(path)

			assert.Contains(t, body, attribute("id", run.expected))
			assert.NotContains(t, body, attribute("id", run.absent))
		})
	}
}

// FR-008. The explanation is rendered when the person arrived from an expired
// session and never otherwise, so a first-time visitor is not told about a
// session they never had.
func TestTheSignInPageExplainsAnExpiredSessionOnlyWhenItSaysSo(t *testing.T) {
	t.Parallel()

	path := accountRoutes(t)[page.OpLoginPage].SmokeURL
	rig := newBrowser(t).anonymous()

	_, _, expired := rig.get(path + "?" + page.ParamReason + "=" + page.ReasonExpired)
	assert.Contains(t, expired, attribute("id", auth.SessionExpiredID))

	_, _, ordinary := rig.get(path)
	assert.NotContains(t, ordinary, attribute("id", auth.SessionExpiredID))

	// An unrecognised reason is an ordinary visit and not a refusal: a person
	// following a stale link has made no mistake worth a 400.
	status, _, other := rig.get(path + "?" + page.ParamReason + "=banana")
	assert.Equal(t, http.StatusOK, status)
	assert.NotContains(t, other, attribute("id", auth.SessionExpiredID))
}

// Every address the pages submit to is a route the router serves. A form
// posting to a path composed by hand is a form that 404s the day a route moves,
// and nothing else in the build would notice.
func TestEveryFormOnTheAccountPagesSubmitsToARouteTheRouterServes(t *testing.T) {
	t.Parallel()

	served := map[string]struct{}{}
	for _, route := range httproute.Inventory().Routes() {
		served[route.Path] = struct{}{}
	}

	rig := newBrowserWith(t, apitest.WithRegistrationOpen(true))
	routes := accountRoutes(t)

	for opID := range routes {
		t.Run(opID, func(t *testing.T) {
			_, _, body := rig.get(routes[opID].SmokeURL)

			addresses := datastarTargets(body)
			if len(addresses) == 0 {
				// A page with no action is allowed, and exactly one thing makes
				// it allowed: it is explaining why it cannot offer one. The
				// smoke URLs of P8 and P9 carry a deliberately dead token, and
				// the fixture has no outgoing mail, so three of these six pages
				// are in that state on this instance (FR-074, FR-076).
				assert.Truef(t, explainsItself(body),
					"%s runs no action and explains nothing", opID)

				return
			}

			for _, address := range addresses {
				_, exists := served[address]
				assert.Truef(t, exists, "%s submits to %q, which is not a route MediKube serves", opID, address)
			}
		})
	}
}

// The phrase the deletion form asks for is the phrase the service compares. A
// form asking for one spelling against a check comparing another is a deletion
// nobody can complete, and it would look like a bug in the person's typing.
func TestTheDeletionFormAsksForThePhraseTheServiceCompares(t *testing.T) {
	t.Parallel()

	_, _, body := newBrowser(t).get(accountRoutes(t)[page.OpSettingsPage].SmokeURL)

	assert.Contains(t, body, domainidentity.DeleteConfirmationPhrase)
}

// explainsItself reports whether a page is rendering one of the states that
// legitimately replaces a control: a closed instance's sign-up explanation, a
// dead recovery link, or an instance that cannot send mail. Anything else with
// no action on it is a form nobody can submit.
func explainsItself(body string) bool {
	for _, id := range []string{auth.RegistrationClosedID, auth.LinkDeadID, auth.MailUnconfiguredID} {
		if strings.Contains(body, attribute("id", id)) {
			return true
		}
	}

	return false
}

// section returns the markup from a marker to the end, which is enough to ask
// "is this inside that" for an element whose contents follow it.
func section(t *testing.T, body, marker string) string {
	t.Helper()

	at := strings.Index(body, marker)
	require.Positivef(t, at, "the document has no %s", marker)

	return body[at:]
}

// datastarTargets reads the addresses out of the @post('...') expressions the
// controls carry. It is a text scan because the alternative is to spell the
// expressions again here, which would assert this test against itself.
func datastarTargets(body string) []string {
	var found []string

	for _, verb := range []string{"@post(&#39;", "@patch(&#39;", "@put(&#39;", "@delete(&#39;"} {
		rest := body
		for {
			at := strings.Index(rest, verb)
			if at < 0 {
				break
			}

			rest = rest[at+len(verb):]

			end := strings.Index(rest, "&#39;")
			if end < 0 {
				break
			}

			found = append(found, rest[:end])
		}
	}

	return found
}
