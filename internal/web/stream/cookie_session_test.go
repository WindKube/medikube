package stream_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/views/ids"
)

// A browser opening this stream sends a cookie and nothing else. It cannot send
// anything else: an EventSource takes no headers, and even Datastar's fetch is
// a plain same-origin request carrying whatever the browser holds.
//
// So this is the one test that fails when the session cookie is translated into
// the Authorization header by any means other than writing that header —
// assigning e.Auth, stashing the token in the request context, anything that
// "authenticates" without leaving a token where the stream can find it again.
// internal/web/stream's session port re-reads the Authorization header on every
// event and every heartbeat, so under any of those alternatives the stream
// refuses to open at all (ErrNoSession) while every bearer-token test in the
// repository stays green. That failure is invisible to an API suite and total
// for every browser, which is why it is asserted here rather than inferred.
func TestAStreamOpenedWithNothingButACookieRunsAndEndsWithItsSession(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	amara := medikube.token(t, testsupport.AccountAEmail)

	watching := medikube.openWithCookie(t, amara)
	require.Equal(t, http.StatusOK, watching.Response.StatusCode,
		"a stream authenticated the way every browser authenticates was refused")

	_, _ = medikube.create(t, amara, testsupport.AccountAPatientSelfID, "Amoxicillin")

	require.Equal(t, "#"+ids.RecordRows(kind.Medication), watching.nextPatch(patchDeadline).selector(),
		"the cookie opened the stream but nothing was delivered on it")

	// The same revocation the ordinary-request half asserts
	// (internal/web/api/session_revocation_test.go), from the other side. The
	// cookie must not buy a longer-lived stream than the bearer token does.
	revoke(t, medikube, testsupport.AccountAEmail)

	_, ended := watching.closed(endOfStream)
	assert.Truef(t, ended, "the cookie-authenticated stream was still open %s after the session ended", endOfStream)
}

// The negative control. Without it the test above would pass against a stream
// route that authenticated nobody and admitted everybody.
func TestAStreamOpenedWithACookieThatIsNotATokenIsRefused(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	refused := medikube.openWithCookie(t, "not-a-token")

	assert.Equal(t, http.StatusUnauthorized, refused.Response.StatusCode)
}

// openWithCookie subscribes the way a browser does: the session cookie, and no
// Authorization header anywhere on the request.
func (i *instance) openWithCookie(t *testing.T, token string) *session {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		i.base+streamPath+"?patient="+testsupport.AccountAPatientSelfID, nil)
	require.NoError(t, err)

	request.Header.Set("Accept", "text/event-stream")
	request.AddCookie(&http.Cookie{Name: web.SessionCookieName, Value: token})
	require.Empty(t, request.Header.Get("Authorization"),
		"the request under test carries a bearer token, so it is not the browser's case")

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	t.Cleanup(func() { _ = response.Body.Close() })

	s := &session{t: t, Response: response, frames: make(chan frame, 256)}

	if response.StatusCode == http.StatusOK {
		go readFrames(response.Body, s.frames)
	}

	return s
}
