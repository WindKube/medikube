package web

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// Component is templ.Component's method set, declared here rather than
// imported.
//
// A generated templ component satisfies it with no adapter and no conversion,
// because the method set is identical — the same reason internal/records
// declares Renderer. Declaring it keeps the rendering library out of the import
// graph of everything that only needs to hand a component on.
type Component interface {
	Render(ctx context.Context, w io.Writer) error
}

// The wire contract of Datastar's non-SSE element patch, read out of the
// vendored browser runtime (internal/web/static/datastar.js, v1.0.2) rather
// than from the Go SDK — which has no non-SSE API at all: every entry point
// there is on ServerSentEventGenerator. This path imports nothing.
const (
	// DatastarRequestHeader is set on every request the runtime makes, and is
	// the discriminator a handler keys on. Accept is not: any client can send
	// one.
	DatastarRequestHeader = "Datastar-Request"

	DatastarSelectorHeader       = "Datastar-Selector"
	DatastarModeHeader           = "Datastar-Mode"
	DatastarViewTransitionHeader = "Datastar-Use-View-Transition"
)

// PatchMode is where the fragment goes relative to its target.
type PatchMode string

// The modes the runtime understands (datastar/consts.go:35-49). Outer is the
// default and is what a full-region re-render uses.
const (
	PatchOuter   PatchMode = "outer"
	PatchInner   PatchMode = "inner"
	PatchRemove  PatchMode = "remove"
	PatchReplace PatchMode = "replace"
	PatchPrepend PatchMode = "prepend"
	PatchAppend  PatchMode = "append"
	PatchBefore  PatchMode = "before"
	PatchAfter   PatchMode = "after"
)

// PatchModes publishes the vocabulary, so a mode declared above and reachable
// nowhere is a value the runtime would silently ignore.
func PatchModes() []PatchMode {
	return []PatchMode{PatchOuter, PatchInner, PatchRemove, PatchReplace, PatchPrepend, PatchAppend, PatchBefore, PatchAfter}
}

// ErrNoPatchTarget is a patch that says nothing about where it goes.
//
// With no selector the runtime matches top-level elements by their own id and,
// finding none, logs PatchElementsNoTargetsFound — a console warning, which
// fails contracts/pages.md assertion 5 while the request itself looks like a
// success. Requiring the caller to choose between a selector and the fragment's
// own id is what stops that being reached by forgetting.
var ErrNoPatchTarget = errors.New("web: a patch needs a selector or an explicit ByElementID; with neither it targets nothing and only warns in the console")

// ErrUnknownPatchMode is a mode the runtime does not implement. It would be
// sent, ignored and the element would not move.
var ErrUnknownPatchMode = errors.New("web: the patch mode is not one the Datastar runtime implements")

// Render writes a component as an HTML response.
//
// It renders into a buffer first, and that is the whole point. templ's own
// pooled writer is a 4 KB bufio.Writer flushed as it fills rather than held to
// the end (runtime/buffer.go:15-51), so a component that fails on its second
// kilobyte has already committed a 200 and half a page — and the error
// middleware, which owns every response in this application, has nothing left
// to answer with. templ's own http.Handler buffers for the same reason.
func Render(e *core.RequestEvent, status int, component Component) error {
	Localize(e)

	body, err := renderToString(e.Request.Context(), component)
	if err != nil {
		return err
	}

	if e.Written() {
		return errors.New("web: the component cannot be written: the response has already gone")
	}

	return e.HTML(status, body)
}

// patchOptions is what a patch says about where it goes.
type patchOptions struct {
	selector  string
	byID      bool
	mode      PatchMode
	viewTrans bool
}

// PatchOption configures one element patch.
type PatchOption func(*patchOptions)

// WithSelector targets the elements a CSS selector matches.
func WithSelector(selector string) PatchOption {
	return func(o *patchOptions) { o.selector = selector }
}

// WithSelectorID targets one element by id. It takes the bare id rather than a
// selector so that internal/web/views/ids' constants can be handed straight in
// and a "#" cannot go missing.
func WithSelectorID(id string) PatchOption {
	return func(o *patchOptions) { o.selector = "#" + id }
}

// ByElementID says the fragment carries its own target: the runtime matches
// each top-level element against the element with that id. It is explicit
// rather than the default because the failure mode is a console warning and a
// page that silently did not change.
func ByElementID() PatchOption {
	return func(o *patchOptions) { o.byID = true }
}

// WithMode places the fragment relative to its target. Absent, the runtime
// replaces the target outright.
func WithMode(mode PatchMode) PatchOption {
	return func(o *patchOptions) { o.mode = mode }
}

// WithViewTransition asks the browser for a view transition around the patch.
func WithViewTransition() PatchOption {
	return func(o *patchOptions) { o.viewTrans = true }
}

// Patch writes a component as a Datastar element patch over a plain text/html
// response — the non-SSE path, which is what the browser runtime honours for
// an ordinary fetch.
//
// The status is 200 and cannot be anything else. The runtime's response handler
// treats every other status as a failure and patches nothing: 201, 204 and 303
// included, silently and with no console message on 204. That is worth stating
// because contracts/streams.md says create, edit and delete "all use the
// non-SSE fast path" while contracts/records.md documents create as 201 and
// delete as 204 — a handler cannot have both, and this function is the half
// that is not negotiable.
func Patch(e *core.RequestEvent, component Component, options ...PatchOption) error {
	var opts patchOptions
	for _, option := range options {
		option(&opts)
	}

	if opts.selector == "" && !opts.byID {
		return ErrNoPatchTarget
	}

	if opts.mode != "" && !validPatchMode(opts.mode) {
		return ErrUnknownPatchMode
	}

	Localize(e)

	body, err := renderToString(e.Request.Context(), component)
	if err != nil {
		return err
	}

	if e.Written() {
		return errors.New("web: the patch cannot be written: the response has already gone")
	}

	header := e.Response.Header()

	if opts.selector != "" {
		header.Set(DatastarSelectorHeader, opts.selector)
	}

	if opts.mode != "" {
		header.Set(DatastarModeHeader, string(opts.mode))
	}

	if opts.viewTrans {
		header.Set(DatastarViewTransitionHeader, "true")
	}

	return e.HTML(http.StatusOK, body)
}

// IsDatastarRequest reports whether the Datastar runtime sent the request. It
// reads the runtime's own header and not Accept, because Accept is a preference
// any client can send and this is a statement about which client is on the
// other end.
func IsDatastarRequest(e *core.RequestEvent) bool {
	return strings.EqualFold(e.Request.Header.Get(DatastarRequestHeader), "true")
}

func validPatchMode(mode PatchMode) bool {
	for _, declared := range PatchModes() {
		if mode == declared {
			return true
		}
	}

	return false
}

// buffers is the render pool. It is this package's own rather than templ's so
// that nothing here has to import the rendering library to hold a []byte.
var buffers = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func renderToString(ctx context.Context, component Component) (string, error) {
	if component == nil {
		return "", errors.New("web: there is no component to render")
	}

	buffer, ok := buffers.Get().(*bytes.Buffer)
	if !ok {
		buffer = new(bytes.Buffer)
	}

	defer func() {
		buffer.Reset()
		buffers.Put(buffer)
	}()

	if err := component.Render(ctx, buffer); err != nil {
		return "", err
	}

	return buffer.String(), nil
}
