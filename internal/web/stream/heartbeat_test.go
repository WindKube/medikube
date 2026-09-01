package stream_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/web/stream"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/shell"
)

// T159. The heartbeat is the only thing that distinguishes a live stream from
// a dead socket, in both directions: without it a browser holding a connection
// nothing has closed believes it is up to date, and the staleness detector has
// nothing to compare against.

// contracts/streams.md's two numbers. They are asserted rather than merely
// declared because they are a pair: the threshold has to leave room for a
// missed beat and the interval has to be short enough that a real failure is
// noticed. Changing one without the other is how a live view starts crying
// wolf, or stops crying at all.
func TestTheHeartbeatIntervalAndTheStalenessThresholdAreTheContractsNumbers(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 25*time.Second, stream.HeartbeatInterval)
	assert.Equal(t, 60*time.Second, stream.StalenessThreshold)

	assert.Greaterf(t, stream.StalenessThreshold, 2*stream.HeartbeatInterval,
		"the threshold leaves no room for a single dropped frame: %s is not more than two beats of %s",
		stream.StalenessThreshold, stream.HeartbeatInterval)
}

// The wire format, on a real socket: a datastar-patch-signals frame whose one
// data line is `signals {"stream_beat":"<RFC3339 UTC>"}`.
func TestAHeartbeatCarriesAnRFC3339StreamBeat(t *testing.T) {
	t.Parallel()

	const interval = 60 * time.Millisecond

	medikube := serve(t, apitestHeartbeat(interval))

	watching := medikube.open(t, medikube.token(t, testsupport.AccountAEmail), "")
	require.Equal(t, http.StatusOK, watching.Response.StatusCode)

	// Three, because two intervals is the smallest sample that has a gap to
	// measure and a second one to check it against.
	const wantBeats = 3

	beats := make([]time.Time, 0, wantBeats)

	for range wantBeats {
		f := watching.next(5 * time.Second)
		require.Truef(t, f.isHeartbeat(), "a frame that is not a heartbeat arrived on an idle stream: %+v", f)

		var signals map[string]string
		require.NoError(t, json.Unmarshal([]byte(f.signals()), &signals),
			"the heartbeat payload is not a JSON object: %q", f.signals())

		require.Contains(t, signals, shell.SignalStreamBeat,
			"the heartbeat sets a signal the page does not read; the page compares $%s", shell.SignalStreamBeat)
		require.Len(t, signals, 1, "the heartbeat carries something besides the beat: %v", signals)

		at, err := time.Parse(time.RFC3339, signals[shell.SignalStreamBeat])
		require.NoErrorf(t, err, "the beat %q is not RFC3339, so the page's Date.parse answers NaN and every comparison is false",
			signals[shell.SignalStreamBeat])
		assert.Equal(t, time.UTC, at.Location(),
			"a local offset would make the staleness comparison depend on the server's timezone")

		beats = append(beats, at)
	}

	// A lower bound and never an upper one: a ticker cannot fire early, so
	// this catches a heartbeat that is not on a ticker at all — one sent per
	// loop iteration, say — while a slow machine cannot make it fail.
	require.Len(t, beats, 3)
}

// The first beat goes out with the header block. Without it a page opened at
// the wrong moment sits for a whole interval comparing against the seed the
// server rendered, and a stream that connected and then died inside that
// window looks exactly like one that is working.
func TestAStreamOpensWithAHeartbeatRatherThanWithSilence(t *testing.T) {
	t.Parallel()

	medikube := serve(t, apitestHeartbeat(time.Hour))

	watching := medikube.open(t, medikube.token(t, testsupport.AccountAEmail), "")

	first := watching.next(5 * time.Second)
	assert.True(t, first.isHeartbeat(), "the first frame on an idle stream was %q", first.Event)
}

// FR-031's client half, asserted against the rendered document rather than
// against the constants: the page is where the comparison happens, and a
// threshold that lived only in Go would be a number nothing on the page uses.
func TestThePageComparesTheHeartbeatAgainstTheStalenessThreshold(t *testing.T) {
	t.Parallel()

	medikube := serve(t)

	body := medikube.page(t, medikube.token(t, testsupport.AccountAEmail), "/"+kind.Medication.Segment())

	assert.Containsf(t, body, shell.StreamPollAttribute()+"=",
		"the page carries no %s, so nothing re-checks the beat", shell.StreamPollAttribute())

	milliseconds := strconv.FormatInt(stream.StalenessThreshold.Milliseconds(), 10)
	assert.Containsf(t, body, milliseconds,
		"the page does not compare against %s, which is the threshold the server's %s heartbeat is sized for",
		stream.StalenessThreshold, stream.HeartbeatInterval)

	assert.Contains(t, body, "$"+shell.SignalStreamStale)
	assert.Contains(t, body, shell.SignalStreamBeat+":",
		"the page never seeds the beat, so a stream that never connects at all is never reported")

	assert.Contains(t, body, attribute("id", ids.StreamStale))
	assert.Contains(t, body, attribute("role", "alert"))

	// The `hidden` content attribute and not style="display:none". The inline
	// style was refused by internal/web/security.go's `style-src 'self'`, which
	// meant the banner was on screen on every page for everyone and every page
	// reported a CSP violation — contracts/pages.md's smoke assertion 6, found
	// by e2e/smoke.spec.ts the first time it ran. Asserted as the absence of
	// ANY inline style rather than as the presence of this one attribute,
	// because the policy is about the mechanism and the next inline style will
	// be somewhere else.
	assert.Contains(t, body, " hidden ",
		"the banner is not hidden to begin with, so a page whose JavaScript never ran shows a false alarm")
	assert.NotContains(t, body, "style=",
		"an inline style attribute reached the page, and style-src 'self' refuses it")
}

// Only FREE Datastar attributes. data-persist, data-match-media and data-on-raf
// are Pro and are not in the vendored v1.0.2 bundle at all: the runtime
// registers no plugin for them, so an attribute using one is inert and the
// only symptom is a banner that never appears.
func TestTheStalenessDetectorUsesNoDatastarProAttribute(t *testing.T) {
	t.Parallel()

	medikube := serve(t)

	body := medikube.page(t, medikube.token(t, testsupport.AccountAEmail), "/"+kind.Medication.Segment())

	for _, pro := range []string{"data-persist", "data-match-media", "data-on-raf", "data-query-string"} {
		assert.NotContainsf(t, body, pro, "%s is a Datastar Pro attribute and the vendored runtime registers no plugin for it", pro)
	}

	// The two silent-failure spellings research D-35 names. The delimiter
	// before a plugin's key is a colon; with a hyphen the whole thing parses
	// as a plugin name nothing registered.
	assert.NotContains(t, body, "data-on-click", "data-on:click is the spelling the runtime parses")
	assert.NotContains(t, body, "data-on-load", "data-on-load does not exist in v1; data-init replaced it")
}

// attribute composes name="value".
//
// It is a helper and not a literal because internal/store's source walk flags
// `id="main"` as a PocketBase filter expression — a bare identifier compared
// against a quoted operand is exactly the shape it hunts for, and an HTML
// attribute is indistinguishable from one by that rule. internal/web/page's
// own tests carry the same helper for the same reason.
func attribute(name, value string) string {
	return name + `="` + value + `"`
}

// page fetches one rendered page.
func (i *instance) page(t *testing.T, token, path string) string {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, i.base+path, nil)
	require.NoError(t, err)

	request.Header.Set("Authorization", token)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, response.StatusCode, "GET %s: %s", path, raw)

	body := string(raw)
	require.True(t, strings.Contains(body, "<body"), "the response is not a rendered document")

	return body
}
