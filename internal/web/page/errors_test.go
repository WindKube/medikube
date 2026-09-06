package page_test

import (
	"bytes"
	"context"
	stderrors "errors"
	"html"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xhtml "golang.org/x/net/html"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/store"
	"medikube/internal/web"
	"medikube/internal/web/page"
	viewserrors "medikube/internal/web/views/errors"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/shell"
	"medikube/internal/web/views/viewstest"
)

// T230. contracts/pages.md's three error views, rendered to a buffer.
//
// They are driven by internal/httproute's ErrorViews table and never by a list
// written here. That table is what the browser gate reads, so a view added to
// it and not implemented, or renamed there and not here, is a failure of this
// test rather than a gap in the gate — the lesson served_test.go records, where
// a curated list of three walked past nine registered pages.

// The three things FR-046 forbids a page to carry, handed to the renderer for
// real.
//
// This is the whole difference between a test that bites and one that is true
// by absence. An empty web.Failure asserted to contain no query would pass
// against a view that rendered every member it was given; a failure whose code,
// message and field errors are ALL a driver's own words fails the moment a view
// renders any of them.
const (
	poisonTrace = "goroutine 41 [running]: medikube/internal/store.(*repo).find(0xc000123456)"
	poisonField = "dose"

	// A request id shaped like the real thing and unmistakable in a diff. It is
	// the one member of the failure a view may render.
	poisonRequestID = "01JQ8Z9YQ4V6ERRORVIEWTEST"
)

// The two that name a table are composed from the kind rather than spelled: the
// collection's name belongs to internal/domain/kind and the architecture gate
// refuses a second spelling of it (research D-05). Composing also keeps them
// honest — a driver names the table the failure really touched.
func poisonQuery() string {
	return "SELECT id, name, dose FROM " + kind.Medication.Collection() + " WHERE owner_id = 'x9f2q'"
}

func poisonDriver() string {
	return "sqlite: no such column: " + kind.Medication.Collection() + ".dosage"
}

// poisoned is a failure carrying every forbidden thing at once.
func poisoned() web.Failure {
	return web.Failure{
		Code:      poisonTrace,
		Message:   poisonDriver() + " while running " + poisonQuery(),
		RequestID: poisonRequestID,
		Fields: []domain.FieldError{
			{Field: poisonField, Code: poisonTrace, Message: poisonQuery()},
		},
	}
}

func forbiddenInAnyErrorPage() map[string]string {
	return map[string]string{
		poisonQuery():  "the query that failed",
		poisonDriver(): "the driver's own message",
		poisonTrace:    "a stack trace",
		poisonField:    "a field error from the failure",
	}
}

func newErrorPages(t *testing.T) *page.ErrorPages {
	t.Helper()

	pages, err := page.NewErrorPages()
	require.NoError(t, err)

	return pages
}

// errorMarkup renders one error page to a buffer, which is the fast layer of
// Constitution Principle VIII's gate.
func errorMarkup(t *testing.T, pages *page.ErrorPages, status int, failure web.Failure, signedIn bool) string {
	t.Helper()

	var buffer bytes.Buffer
	require.NoError(t, pages.Document(status, failure, signedIn).Render(context.Background(), &buffer))

	return buffer.String()
}

// errorTree parses a whole rendered document. viewstest.Render parses a
// FRAGMENT in a context element and these are complete documents, so the parse
// is here; every matcher and every walk below is viewstest's.
func errorTree(t *testing.T, markup string) *xhtml.Node {
	t.Helper()

	root, err := xhtml.Parse(strings.NewReader(markup))
	require.NoErrorf(t, err, "the error page rendered markup that does not parse:\n%s", markup)

	return root
}

// only requires exactly one match, because "the landmark is missing" and "the
// landmark is there twice" are different defects and asking for the first hides
// the second.
func only(t *testing.T, root *xhtml.Node, match viewstest.Matcher, what string) *xhtml.Node {
	t.Helper()

	found := viewstest.Find(root, match)
	require.Lenf(t, found, 1, "expected exactly one %s in the rendered error page", what)

	return found[0]
}

// landmarkName reads the accessible name out of a role[name="X"] selector, so
// the expected title and the expected section come from the same declaration
// the browser gate opens the page to find.
func landmarkName(t *testing.T, selector string) string {
	t.Helper()

	parts := landmarkSelector.FindStringSubmatch(selector)
	require.Lenf(t, parts, 3, "%q is not a role[name=\"...\"] selector", selector)

	return parts[2]
}

// The shell of contracts/pages.md, asserted on an error page: smoke assertions
// 2, 3 and 4 of that document, run in Go rather than in a browser.
//
// An error view that rendered its own bare markup instead of the shell would
// answer the right status with no navigation, and the person who hit it would
// have nowhere to go.
func TestEveryDeclaredErrorViewRendersInsideTheFullShellCarryingItsOwnLandmark(t *testing.T) {
	t.Parallel()

	pages := newErrorPages(t)
	views := httproute.Inventory().ErrorViews()

	// The guard on the guard. contracts/pages.md declares three error views and
	// this test is worth nothing the day the table it walks stops declaring
	// them.
	require.Len(t, views, 3,
		"the route inventory no longer declares contracts/pages.md's three error views; this test is walking something else")

	for _, view := range views {
		t.Run(string(view.Name), func(t *testing.T) {
			t.Parallel()

			require.NotEmpty(t, view.Landmark, "%s declares no landmark", view.Name)
			require.NotZero(t, view.Status, "%s declares no status", view.Name)

			name := landmarkName(t, view.Landmark)

			for _, signedIn := range []bool{false, true} {
				t.Run(session(signedIn), func(t *testing.T) {
					t.Parallel()

					markup := errorMarkup(t, pages, view.Status, poisoned(), signedIn)
					root := errorTree(t, markup)

					// The four landmarks, signed in or out. The navigation
					// landmark is on EVERY page in the application and what
					// changes signed out is its contents.
					banner := only(t, root, viewstest.And(viewstest.Tag("header"), viewstest.WithAttr("role", "banner")), "banner")
					assert.NotEmpty(t, viewstest.Text(banner))

					navigation := only(t, root, viewstest.And(
						viewstest.Tag("nav"), viewstest.WithAttr("aria-label", "Primary")), "primary navigation")
					assert.NotEmpty(t, viewstest.Find(navigation, viewstest.Tag("a")),
						"the navigation landmark is empty, so a person who hit this error has nowhere to go")

					main := only(t, root, viewstest.And(viewstest.Tag("main"), viewstest.WithID(ids.Main)), "main landmark")
					only(t, root, viewstest.And(viewstest.Tag("footer"), viewstest.WithAttr("role", "contentinfo")), "contentinfo")

					// The two patch containers Datastar addresses by id. They
					// are on every page including this one, because a patch
					// aimed at an element that does not exist is a console
					// warning and therefore a failed smoke run.
					only(t, root, viewstest.WithID(ids.ErrorBanner), "error banner container")
					only(t, root, viewstest.WithID(ids.Toast), "toast container")

					// Smoke assertion 3: the view's own landmark is present,
					// INSIDE main, and non-empty.
					landmark := only(t, root, viewstest.Region(name), view.Landmark)
					assert.Truef(t, viewstest.Descends(main, landmark),
						"%s renders outside <main>, so the browser gate resolves it against the wrong container", view.Landmark)
					assert.NotEmpty(t, viewstest.Text(landmark), "%s is present and empty", view.Landmark)

					// Smoke assertion 4. The error views carry no title column
					// in contracts/pages.md, so the tab says what the landmark
					// says — one string, not two spellings of one idea.
					assert.Contains(t, markup, shell.TitleElement(name))

					// The one thing an error page carries out of the failure.
					assert.Contains(t, viewstest.Text(landmark), poisonRequestID,
						"the page carries no request id, so nobody can quote a reference for it (FR-054)")
				})
			}
		})
	}
}

// FR-046, and the assertion the whole file exists for: no stack trace, no
// driver message, no query — from a failure that really carries all three.
func TestNoErrorPageRendersAStackTraceADriverMessageOrAQuery(t *testing.T) {
	t.Parallel()

	pages := newErrorPages(t)
	views := httproute.Inventory().ErrorViews()

	require.Len(t, views, 3, "the route inventory no longer declares three error views")

	forbidden := forbiddenInAnyErrorPage()
	require.NotEmpty(t, forbidden, "nothing is being looked for, so this asserts nothing")

	for _, view := range views {
		for _, signedIn := range []bool{false, true} {
			t.Run(string(view.Name)+"/"+session(signedIn), func(t *testing.T) {
				t.Parallel()

				markup := errorMarkup(t, pages, view.Status, poisoned(), signedIn)

				for needle, what := range forbidden {
					for _, spelling := range spellings(needle) {
						assert.NotContainsf(t, markup, spelling,
							"the %s view rendered %s, which FR-046 forbids on any error page", view.Name, what)
					}
				}

				// The positive half. Without it every assertion above would
				// pass on a page that rendered nothing at all.
				assert.Contains(t, markup, poisonRequestID)
			})
		}
	}
}

// The mechanical tie between what an error page renders and what the failure
// gave it: exactly one member of web.Failure reaches the page.
//
// It is asserted member by member rather than as prose, so a member ADDED to
// the envelope later — and rendered without thinking — fails here.
func TestAnErrorPageCarriesTheRequestIdAndNothingElseFromTheFailure(t *testing.T) {
	t.Parallel()

	pages := newErrorPages(t)

	cases := map[string]web.Failure{
		"an internal failure whose every member is a driver's own words": poisoned(),
		"a refusal carrying nothing but its code": {
			Code:      web.CodeNotFound,
			Message:   web.Message(web.CodeNotFound),
			RequestID: poisonRequestID,
		},
	}

	for name, failure := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			markup := errorMarkup(t, pages, http.StatusInternalServerError, failure, false)

			assert.Contains(t, markup, failure.RequestID)
			assert.NotContains(t, markup, failure.Code,
				"the machine code reached the page; it is a client's switch, not a person's sentence")
			assert.NotContains(t, markup, failure.Message,
				"the failure's message reached the page, and a message is where a driver puts a filename")
		})
	}
}

// Totality. Every status this application can produce chooses one of the three
// declared views, so a status nobody thought about renders a page rather than a
// blank one or a panic.
func TestEveryStatusTheApplicationProducesChoosesADeclaredErrorView(t *testing.T) {
	t.Parallel()

	pages := newErrorPages(t)

	declared := map[string]struct{}{}
	for _, view := range httproute.Inventory().ErrorViews() {
		declared[landmarkName(t, view.Landmark)] = struct{}{}
	}

	require.Len(t, declared, 3, "the route inventory no longer declares three error views")

	// Every error internal/web's mapper has a row for, so the statuses come
	// from the mapper rather than from a list of numbers somebody typed.
	var invalid domain.ValidationError
	invalid.Add(poisonField, domain.CodeRequired, "a dose is required")

	classified := []error{
		domain.ErrNotFound,
		domain.ErrForbidden,
		domain.ErrUnauthenticated,
		domain.ErrVersionMismatch,
		domain.ErrConflict,
		domain.ErrRateLimited,
		context.Canceled,
		context.DeadlineExceeded,
		store.ErrInvalidCursor,
		web.ErrRegistrationClosed,
		web.ErrInvalidToken,
		web.ErrMailUnconfigured,
		invalid.OrNil(),
		stderrors.New("anything unhandled at all"),
	}

	statuses := map[int]struct{}{}
	for _, err := range classified {
		status, _ := web.Classify(err)
		statuses[status] = struct{}{}
	}

	// The guard on the guard: a mapper that stopped answering distinct statuses
	// would leave this ranging over one.
	require.Greaterf(t, len(statuses), 8,
		"web.Classify now answers only %d distinct statuses; this test has stopped covering the mapper", len(statuses))

	for status := range statuses {
		// Named by number: http.StatusText answers the empty string for
		// nginx's 499, which internal/web produces for a cancelled request.
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()

			markup := errorMarkup(t, pages, status, poisoned(), false)
			root := errorTree(t, markup)

			main := only(t, root, viewstest.And(viewstest.Tag("main"), viewstest.WithID(ids.Main)), "main landmark")

			sections := viewstest.Find(main, viewstest.And(viewstest.Tag("section"), viewstest.HasAttr("aria-label")))
			require.Lenf(t, sections, 1, "status %d renders %d labelled sections inside main, not one", status, len(sections))

			name := viewstest.Attr(sections[0], "aria-label")
			assert.Containsf(t, declared, name,
				"status %d renders a section named %q, which is not one of contracts/pages.md's three error views", status, name)
			assert.NotEmpty(t, viewstest.Text(sections[0]), "status %d renders an empty landmark", status)
		})
	}
}

// Which surface answers with a page and which with the JSON envelope, driven by
// the route table.
//
// The decision cannot be a prefix somebody wrote down: a hardcoded "/api/"
// would keep answering JSON for a surface that moved, and would answer a whole
// HTML document to a client that asked for JSON on a surface that was added.
func TestOnlyTheNonAPISurfaceAnswersAFailureWithAPage(t *testing.T) {
	t.Parallel()

	pages := newErrorPages(t)

	var asPages, asEnvelopes int

	for _, route := range httproute.Inventory().Routes() {
		t.Run(route.OpID, func(t *testing.T) {
			t.Parallel()

			if route.Kind == httproute.KindPage {
				assert.Truef(t, pages.RendersPage(route.Path),
					"%s is a page and a failure on it would answer JSON to a browser", route.Pattern())

				return
			}

			assert.Falsef(t, pages.RendersPage(route.Path),
				"%s is not a page and a failure on it would answer a whole HTML document to a client expecting JSON", route.Pattern())
		})

		if route.Kind == httproute.KindPage {
			asPages++

			continue
		}

		asEnvelopes++
	}

	// Both counts are literal, so a table that stopped declaring one of the two
	// surfaces cannot pass this by asserting only the other.
	require.Greater(t, asPages, 8, "the route table has stopped declaring pages")
	require.Greater(t, asEnvelopes, 20, "the route table has stopped declaring API routes")

	// An address no route claims is the 404 the browser gate opens, and it is a
	// page.
	unknowns := []string{"/not-found-for-smoke", "/", "/" + kind.Medication.Segment() + "/does-not-exist"}
	for _, unknown := range unknowns {
		assert.Truef(t, pages.RendersPage(unknown), "%s answers a browser with JSON", unknown)
	}
}

// The sign-in prompt of contracts/pages.md's E2 offers the sign-in page, and
// offers it WITHOUT the address that was refused: FR-046 forbids echoing the
// requested address, and a ?next= carrying it would be exactly that.
func TestTheSignInRequiredPageOffersTheSignInPageAndEchoesNoAddress(t *testing.T) {
	t.Parallel()

	pages := newErrorPages(t)

	login, found := "", false
	for _, route := range httproute.Inventory().Routes() {
		if route.OpID == page.OpLoginPage {
			login, found = route.Path, true
		}
	}

	require.True(t, found, "the route table no longer declares the sign-in page")

	markup := errorMarkup(t, pages, http.StatusForbidden, poisoned(), false)
	root := errorTree(t, markup)

	landmark := only(t, root, viewstest.Region(viewserrors.SignInRequiredLandmark), "the sign-in-required landmark")

	links := viewstest.Find(landmark, viewstest.Tag("a"))
	require.NotEmpty(t, links, "the sign-in-required view offers no way to sign in")

	offered := make([]string, 0, len(links))
	for _, link := range links {
		offered = append(offered, viewstest.Attr(link, "href"))
	}

	assert.Contains(t, offered, login)

	for _, href := range offered {
		assert.NotContains(t, href, "?", "the prompt carries a query string, which is where a refused address would ride back")
	}
}

// spellings is every form a forbidden string can reach the markup in: its own,
// and the HTML-escaped one templ writes.
//
// Without the second, the query needle could not fail. Real SQL quotes its
// string literals, templ renders ' as &#39;, and a page that printed the whole
// failing query would sail past a raw substring search — the assertion would be
// present, green and blind, which is the exact shape of guard this repository
// has been bitten by before.
func spellings(needle string) []string {
	escaped := html.EscapeString(needle)
	if escaped == needle {
		return []string{needle}
	}

	return []string{needle, escaped}
}

func session(signedIn bool) string {
	if signedIn {
		return "signed in"
	}

	return "signed out"
}
