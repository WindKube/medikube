package page

import (
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/httproute"
	"medikube/internal/web"
	viewserrors "medikube/internal/web/views/errors"
	"medikube/internal/web/views/shell"
)

// ErrorPages renders contracts/pages.md's three error views: the 404 that every
// refusal on somebody else's data also produces, the 403 that asks for a
// session, and the 500.
//
// All three render inside the full shell, so a person who hits an error still
// has navigation, and all three carry the request id and nothing else out of
// the failure (FR-046).
type ErrorPages struct {
	links accountLinks

	// envelopeSurface is the set of leading path segments that answer with the
	// JSON envelope rather than with a page, derived from the route table.
	envelopeSurface map[string]struct{}
}

// NewErrorPages builds the renderer. It resolves the addresses its navigation
// and its sign-in prompt need from the route table, so a page it links to is a
// page the router serves.
func NewErrorPages() (*ErrorPages, error) {
	links, err := newAccountLinks()
	if err != nil {
		return nil, err
	}

	return &ErrorPages{links: links, envelopeSurface: envelopeSurface()}, nil
}

// Render is internal/web's ErrorView seam: the whole of what the composition
// root has to wire.
//
// It reports false for the API surface, which leaves web.Errors to write the
// JSON envelope — a program that asked for JSON must not be handed a document.
func (p *ErrorPages) Render(e *core.RequestEvent, status int, failure web.Failure) (bool, error) {
	if e == nil || e.Request == nil || !p.RendersPage(e.Request.URL.Path) {
		return false, nil
	}

	actor, carried := web.ActorFrom(e.Request.Context())

	// A request with no actor renders the signed-out navigation. It fails to
	// the smaller surface deliberately: the alternative is offering an account's
	// own links to somebody the request could not identify.
	signedIn := carried && actor.Authenticated()

	view, name := p.view(status, failure.RequestID)

	nav := p.links.signedOutNav("")
	if signedIn {
		nav = p.links.signedInNav("")
	}

	// No nav entry is current: an error view belongs to no page in the table,
	// and marking one would announce the reader as being somewhere they are
	// not. RenderPage is what applies the no-store cache header — an error
	// page carries somebody's navigation and a correlation id, and a 404 that
	// a shared cache kept would be answered to the next caller from the cache
	// rather than from the authorization checkpoint — and the theme class.
	return true, RenderPage(e, status, name, NavState{SignedIn: signedIn, Nav: nav}, view)
}

// RendersPage reports whether a failure on this path is answered with a page.
func (p *ErrorPages) RendersPage(path string) bool {
	_, envelope := p.envelopeSurface[leadingSegment(path)]

	return !envelope
}

// Document is the whole page for one failure.
//
// It takes the entire web.Failure and passes ONE member of it on, and the
// discarding is here rather than at three call sites on purpose: the code, the
// message and the field errors are each a place a driver's own words can end
// up, and the views have no member that could receive them (FR-046).
func (p *ErrorPages) Document(status int, failure web.Failure, signedIn bool) web.Component {
	view, name := p.view(status, failure.RequestID)

	nav := p.links.signedOutNav("")
	if signedIn {
		nav = p.links.signedInNav("")
	}

	// No nav entry is current: an error view belongs to no page in the table,
	// and marking one would announce the reader as being somewhere they are not.
	return shell.Document(shell.DocumentProps{
		Title:    name,
		SignedIn: signedIn,
		Nav:      nav,
		Main:     view,
	})
}

// view chooses one of the three by status family, and returns its landmark's
// name as the page's title — contracts/pages.md gives the error views no title
// column, and one string is better than two spellings of one idea.
//
// 401 joins 403 because both are the same thing to a person: a page that wants
// a session they have not got. Everything the table does not name is an
// unexpected failure, which is what makes this total — a status nobody
// anticipated renders a page rather than an empty one.
func (p *ErrorPages) view(status int, requestID string) (web.Component, string) {
	switch status {
	case http.StatusNotFound:
		return viewserrors.NotFound(viewserrors.NotFoundProps{
			RequestID: requestID,
		}), viewserrors.NotFoundLandmark

	case http.StatusUnauthorized, http.StatusForbidden:
		return viewserrors.SignInRequired(viewserrors.SignInRequiredProps{
			RequestID:  requestID,
			SignInHref: p.links.loginPage,
		}), viewserrors.SignInRequiredLandmark

	default:
		return viewserrors.ServerError(viewserrors.ServerErrorProps{
			RequestID: requestID,
		}), viewserrors.ServerErrorLandmark
	}
}

// envelopeSurface is the leading path segment of every route that is not a
// page, less every segment a page uses.
//
// It is derived from the route table and never spelled, because the one thing
// this decision must not become is a prefix somebody wrote down: a literal
// "/api/" would keep answering JSON for a surface that had moved, and would
// answer a whole HTML document to a client that asked for JSON on a surface
// that had been added. Today it resolves to the API base and PocketBase's own
// dashboard root; the day a phase adds a third, this follows it.
func envelopeSurface() map[string]struct{} {
	surface := map[string]struct{}{}
	pages := map[string]struct{}{}

	for _, route := range httproute.Inventory().Routes() {
		segment := leadingSegment(route.Path)

		if route.Kind == httproute.KindPage {
			pages[segment] = struct{}{}

			continue
		}

		surface[segment] = struct{}{}
	}

	// A segment a page shares with anything else belongs to the page: a
	// browser reaching it must get a page, and the JSON caller on the same
	// segment is on a route that answered its own error.
	for segment := range pages {
		delete(surface, segment)
	}

	return surface
}

// leadingSegment is the first path segment, without its slashes. The
// application root has none, which is why the empty string is a legitimate
// answer rather than a failure.
func leadingSegment(path string) string {
	trimmed := strings.TrimPrefix(path, "/")

	if cut := strings.IndexByte(trimmed, '/'); cut >= 0 {
		return trimmed[:cut]
	}

	return trimmed
}
