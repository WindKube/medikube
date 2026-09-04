package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fragment is a Component that writes what it was given.
type fragment string

func (f fragment) Render(_ context.Context, w io.Writer) error {
	_, err := io.WriteString(w, string(f))

	return err
}

// halfWritten is the shape that matters: a component that writes more than
// templ's 4 KB buffer holds and then fails. Rendered straight into the
// response it would have committed a 200 and a partial body before anybody
// could know it failed.
type halfWritten struct{ prefix int }

func (h halfWritten) Render(_ context.Context, w io.Writer) error {
	if _, err := io.WriteString(w, strings.Repeat("x", h.prefix)); err != nil {
		return err
	}

	return errors.New("the record was deleted while the page was rendering")
}

// contextual is what templ generates: it reads ctx.Err() before writing a byte.
type contextual struct{}

func (contextual) Render(ctx context.Context, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	_, err := io.WriteString(w, "<p>never</p>")

	return err
}

func TestRenderWritesTheComponentAsHTML(t *testing.T) {
	t.Parallel()

	e, recorder := event(t, http.MethodGet, "/x")

	require.NoError(t, Render(e, http.StatusOK, fragment(`<main id="main">hello</main>`)))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, `<main id="main">hello</main>`, recorder.Body.String())
	assert.Equal(t, "text/html; charset=utf-8", recorder.Header().Get(headerContentType))
}

// The trap. templ's own pooled writer is a 4 KB bufio.Writer flushed as it
// fills rather than held to the end, so a component that fails at byte 8192 has
// already committed a 200 and half a page. Rendering into a buffer first is
// what keeps the error middleware in charge of the response.
func TestAComponentThatFailsAfterFourKilobytesWritesNothingAtAll(t *testing.T) {
	t.Parallel()

	e, recorder := event(t, http.MethodGet, "/x")

	err := Render(e, http.StatusOK, halfWritten{prefix: 8192})
	require.Error(t, err)

	assert.False(t, e.Written(), "the status was committed, so the error middleware can no longer answer")
	assert.Empty(t, recorder.Body.String(), "a partial page reached the client and looked like a whole one")
	assert.Equal(t, 200, recorder.Code, "httptest's recorder default, i.e. nothing was written")
	assert.Empty(t, recorder.Header().Get(headerContentType))
}

func TestRenderRefusesToWriteOverAResponseThatHasGone(t *testing.T) {
	t.Parallel()

	e, _ := event(t, http.MethodGet, "/x")
	require.NoError(t, Render(e, http.StatusOK, fragment("<p>first</p>")))
	require.Error(t, Render(e, http.StatusOK, fragment("<p>second</p>")))
}

func TestRenderKeepsAContentTypeTheHandlerAlreadyChose(t *testing.T) {
	t.Parallel()

	e, recorder := event(t, http.MethodGet, "/x")
	e.Response.Header().Set(headerContentType, "application/xhtml+xml")

	require.NoError(t, Render(e, http.StatusOK, fragment("<p/>")))
	assert.Equal(t, "application/xhtml+xml", recorder.Header().Get(headerContentType))
}

// A cancelled request must not produce a response. templ's generated code
// returns ctx.Err() before writing, and buffering is what makes that reach the
// caller rather than a truncated 200.
func TestACancelledRequestRendersNothing(t *testing.T) {
	t.Parallel()

	e, recorder := event(t, http.MethodGet, "/x")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	e.Request = e.Request.WithContext(ctx)

	err := Render(e, http.StatusOK, contextual{})
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, recorder.Body.String())

	status, code := Classify(err)
	assert.Equal(t, StatusClientClosed, status)
	assert.Equal(t, CodeClientClosed, code)
}

// The non-SSE element patch, read out of the vendored browser runtime rather
// than guessed: status exactly 200, a Content-Type containing text/html, the
// fragment as the body, and the patch options as datastar-* headers. The
// runtime treats every other status as a failure and patches nothing.
func TestAPatchIsAlwaysTwoHundredWhateverTheHandlerMeant(t *testing.T) {
	t.Parallel()

	e, recorder := event(t, http.MethodPatch, "/x")

	require.NoError(t, Patch(e, fragment(`<li id="r1">a</li>`), ByElementID()))

	assert.Equal(t, http.StatusOK, recorder.Code,
		"the Datastar runtime patches on 200 and on nothing else, so any other status silently patches nothing")
	assert.Contains(t, recorder.Header().Get(headerContentType), "text/html")
	assert.Equal(t, `<li id="r1">a</li>`, recorder.Body.String())
}

func TestThePatchOptionsTravelAsHeaders(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		options []PatchOption
		headers map[string]string
	}{
		{
			"by the fragment's own id",
			[]PatchOption{ByElementID()},
			map[string]string{},
		},
		{
			"a selector",
			[]PatchOption{WithSelector("#error-banner")},
			map[string]string{DatastarSelectorHeader: "#error-banner"},
		},
		{
			"a selector built from an id",
			[]PatchOption{WithSelectorID("error-banner")},
			map[string]string{DatastarSelectorHeader: "#error-banner"},
		},
		{
			"a mode",
			[]PatchOption{ByElementID(), WithMode(PatchInner)},
			map[string]string{DatastarModeHeader: "inner"},
		},
		{
			"a removal",
			[]PatchOption{WithSelectorID("r1"), WithMode(PatchRemove)},
			map[string]string{DatastarSelectorHeader: "#r1", DatastarModeHeader: "remove"},
		},
		{
			"a view transition",
			[]PatchOption{ByElementID(), WithViewTransition()},
			map[string]string{DatastarViewTransitionHeader: "true"},
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			e, recorder := event(t, http.MethodPatch, "/x")
			require.NoError(t, Patch(e, fragment(`<li id="r1">a</li>`), one.options...))

			for _, name := range []string{DatastarSelectorHeader, DatastarModeHeader, DatastarViewTransitionHeader} {
				assert.Equal(t, one.headers[name], recorder.Header().Get(name), name)
			}
		})
	}
}

// With no selector the runtime matches top-level elements by their id, and a
// fragment whose id matches nothing logs PatchElementsNoTargetsFound — a
// console warning, which fails contracts/pages.md assertion 5. Requiring the
// caller to say which of the two they meant is what stops that being reached
// by forgetting.
func TestAPatchWithNoTargetingAtAllIsRefused(t *testing.T) {
	t.Parallel()

	e, recorder := event(t, http.MethodPatch, "/x")

	err := Patch(e, fragment(`<li>a</li>`))
	require.Error(t, err, "a patch with neither a selector nor an explicit by-id targets nothing and only warns in the console")
	assert.Empty(t, recorder.Body.String())

	status, _ := Classify(err)
	assert.Equal(t, http.StatusInternalServerError, status, "a wiring mistake was reported as the caller's fault")
}

func TestAPatchModeNobodyDeclaredIsRefused(t *testing.T) {
	t.Parallel()

	e, _ := event(t, http.MethodPatch, "/x")

	require.Error(t, Patch(e, fragment(`<li id="r1">a</li>`), ByElementID(), WithMode("replace-all")))
}

func TestEveryDeclaredPatchModeIsAccepted(t *testing.T) {
	t.Parallel()

	for _, mode := range PatchModes() {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()

			e, recorder := event(t, http.MethodPatch, "/x")
			require.NoError(t, Patch(e, fragment(`<li id="r1">a</li>`), ByElementID(), WithMode(mode)))
			assert.Equal(t, string(mode), recorder.Header().Get(DatastarModeHeader))
		})
	}
}

// A patch that failed to render has written nothing, so the error middleware
// still owns the response — the same property Render has and for the same
// reason.
func TestAFailedPatchWritesNothing(t *testing.T) {
	t.Parallel()

	e, recorder := event(t, http.MethodPatch, "/x")

	require.Error(t, Patch(e, halfWritten{prefix: 8192}, ByElementID()))
	assert.False(t, e.Written())
	assert.Empty(t, recorder.Body.String())
}

// The runtime always sends Datastar-Request: true, which is the discriminator a
// handler keys on to decide between a patch and a documented API response.
func TestADatastarRequestIsRecognisedByItsOwnHeaderAndNothingElse(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"true":  true,
		"TRUE":  true,
		"false": false,
		"":      false,
		"1":     false,
	}

	for value, expected := range cases {
		t.Run("header="+value, func(t *testing.T) {
			t.Parallel()

			e, _ := event(t, http.MethodGet, "/x")
			if value != "" {
				e.Request.Header.Set(DatastarRequestHeader, value)
			}

			assert.Equal(t, expected, IsDatastarRequest(e))
		})
	}

	t.Run("an Accept header is not the discriminator", func(t *testing.T) {
		t.Parallel()

		e, _ := event(t, http.MethodGet, "/x")
		e.Request.Header.Set("Accept", "text/event-stream, text/html, application/json")

		assert.False(t, IsDatastarRequest(e),
			"Accept is a preference any client can send; Datastar-Request is what the runtime sets")
	})
}
