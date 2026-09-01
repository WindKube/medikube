package httproute_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
)

// T096. e2e/routes.ts shells out to `medikube routes --json` at Playwright's
// collection phase, so this list IS the browser gate's target list. A target
// that quietly stops being produced is a page that quietly stops being checked,
// and nothing else in the repository would notice.

func TestSmokeTargetsAreExactlyThePagesAndTheErrorViews(t *testing.T) {
	t.Parallel()

	registry := httproute.Inventory()

	wantNames := make([]string, 0)
	for _, route := range registry.Routes() {
		if route.Kind == httproute.KindPage {
			wantNames = append(wantNames, route.OpID)
		}
	}
	require.NotEmpty(t, wantNames, "the table declares no pages at all")

	for _, view := range registry.ErrorViews() {
		if view.SmokeURL != "" {
			wantNames = append(wantNames, string(view.Name))
		}
	}

	gotNames := make([]string, 0, len(registry.SmokeTargets()))
	for _, target := range registry.SmokeTargets() {
		gotNames = append(gotNames, target.Name)
	}

	assert.ElementsMatch(t, wantNames, gotNames)
}

func TestSmokeTargetsExcludeEveryKindThatIsNotAPage(t *testing.T) {
	t.Parallel()

	registry := httproute.Inventory()

	pages := make(map[string]httproute.Route)
	for _, route := range registry.Routes() {
		if route.Kind == httproute.KindPage {
			pages[route.OpID] = route
		}
	}

	views := make(map[string]httproute.ErrorView)
	for _, view := range registry.ErrorViews() {
		views[string(view.Name)] = view
	}

	for _, target := range registry.SmokeTargets() {
		if page, isPage := pages[target.Name]; isPage {
			assert.Equal(t, page.SmokeURL, target.URL)
			assert.Equal(t, page.Landmark, target.Landmark)
			assert.Equal(t, page.Auth, target.Auth)
			// contracts/pages.md: "A 4xx on a page route would fail the gate."
			assert.Equal(t, http.StatusOK, target.Status)

			continue
		}

		view, isView := views[target.Name]
		require.Truef(t, isView, "%q is neither a registered page nor a declared error view", target.Name)
		assert.Equal(t, view.SmokeURL, target.URL)
		assert.Equal(t, view.Landmark, target.Landmark)
		assert.Equal(t, view.Status, target.Status)
		assert.Equal(t, view.Auth, target.Auth)
	}
}

// Assertion 3 of contracts/pages.md is "the page's own landmark is present and
// non-empty", so a target with no landmark is a target that asserts nothing.
// Assertion 1 needs a URL a browser can actually open, which is why P8 and P9
// carry a deliberately invalid token rather than a {token} placeholder.
func TestEverySmokeTargetIsOpenable(t *testing.T) {
	t.Parallel()

	targets := httproute.Inventory().SmokeTargets()
	require.NotEmpty(t, targets)

	seen := make(map[string]bool, len(targets))

	for _, target := range targets {
		t.Run(target.Name, func(t *testing.T) {
			t.Parallel()

			assert.NotEmpty(t, target.Landmark)
			assert.True(t, strings.HasPrefix(target.URL, "/"), "%q is not a path a browser can open", target.URL)
			assert.NotContains(t, target.URL, "{", "an unbound parameter is not a URL")
			assert.GreaterOrEqual(t, target.Status, http.StatusOK)
		})

		assert.False(t, seen[target.Name], "%q appears twice in the smoke list", target.Name)
		seen[target.Name] = true
	}
}

// contracts/pages.md declares three error views and the browser gate covers all
// three. Only two of them can be reached by opening a URL, so the third has to
// say so out loud rather than fall out of the list unremarked.
func TestEveryErrorViewIsEitherSmokeableOrSaysWhyNot(t *testing.T) {
	t.Parallel()

	views := httproute.Inventory().ErrorViews()
	require.Len(t, views, 3, "contracts/pages.md declares a not-found, a sign-in-required and a server-error view")

	for _, view := range views {
		t.Run(string(view.Name), func(t *testing.T) {
			t.Parallel()

			assert.NotEmpty(t, view.Landmark)
			assert.GreaterOrEqual(t, view.Status, http.StatusBadRequest)

			if view.SmokeURL == "" {
				assert.NotEmpty(t, view.Unreachable,
					"an error view with no smoke URL and no stated reason has silently left the browser gate")

				return
			}

			assert.Empty(t, view.Unreachable, "a view with a smoke URL is reachable; the two fields are exclusive")
		})
	}
}

// The not-found view is produced by a path the router does not know. If some
// later phase registers a route at its smoke URL, the target starts asserting
// the wrong page and the privacy view of FR-033 stops being checked.
func TestTheNotFoundSmokeURLMatchesNoRegisteredRoute(t *testing.T) {
	t.Parallel()

	registry := httproute.Inventory()

	var notFound httproute.ErrorView
	for _, view := range registry.ErrorViews() {
		if view.Status == http.StatusNotFound {
			notFound = view
		}
	}
	require.NotEmpty(t, notFound.SmokeURL)

	for _, route := range registry.Routes() {
		assert.NotEqual(t, notFound.SmokeURL, route.Path, "%s would answer the not-found smoke case", route.OpID)
	}
}

func TestDescribeErrorViewRefusesAnIncompleteDeclaration(t *testing.T) {
	t.Parallel()

	complete := httproute.ErrorView{
		Name:     "sampleView",
		Status:   http.StatusNotFound,
		Landmark: `region[name="Sample"]`,
		Auth:     httproute.AuthPublic,
		SmokeURL: "/no-such-path-for-smoke",
	}

	cases := []struct {
		name    string
		mutate  func(*httproute.ErrorView)
		message string
	}{
		{"no name", func(v *httproute.ErrorView) { v.Name = "" }, "no Name"},
		{"no landmark", func(v *httproute.ErrorView) { v.Landmark = "" }, "no Landmark"},
		{"a success status", func(v *httproute.ErrorView) { v.Status = http.StatusOK }, "status"},
		{"no auth", func(v *httproute.ErrorView) { v.Auth = "" }, "auth"},
		{
			"neither a smoke URL nor a reason",
			func(v *httproute.ErrorView) { v.SmokeURL = "" },
			"Unreachable",
		},
		{
			"both a smoke URL and a reason",
			func(v *httproute.ErrorView) { v.Unreachable = "because" },
			"exclusive",
		},
		{
			"a smoke URL with an unbound parameter",
			func(v *httproute.ErrorView) { v.SmokeURL = "/sample/{id}" },
			"unbound parameter",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			view := complete
			testCase.mutate(&view)

			assert.Contains(t, panicMessage(t, func() { httproute.Empty().DescribeErrorView(view) }), testCase.message)
		})
	}
}

func TestDescribingTheSameErrorViewTwicePanics(t *testing.T) {
	t.Parallel()

	view := httproute.ErrorView{
		Name:     "sampleView",
		Status:   http.StatusNotFound,
		Landmark: `region[name="Sample"]`,
		Auth:     httproute.AuthPublic,
		SmokeURL: "/no-such-path-for-smoke",
	}

	registry := httproute.Empty()
	registry.DescribeErrorView(view)

	assert.Contains(t, panicMessage(t, func() { registry.DescribeErrorView(view) }), "twice")
}

func TestAnErrorViewIsNotARoute(t *testing.T) {
	t.Parallel()

	registry := httproute.Empty()
	registry.DescribeErrorView(httproute.ErrorView{
		Name:     "sampleView",
		Status:   http.StatusNotFound,
		Landmark: `region[name="Sample"]`,
		Auth:     httproute.AuthPublic,
		SmokeURL: "/no-such-path-for-smoke",
	})
	registry.Handle(samplePage(), func(e *core.RequestEvent) error { return nil })

	assert.Len(t, registry.Routes(), 1, "an error view is what the application renders instead of a route, not a route of its own")
	assert.Len(t, registry.SmokeTargets(), 2)
}
