package stream_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/web/views/ids"
)

// T156 and SC-007: "appears in another open view within five seconds".
//
// This is the only test that exercises the whole path as one thing — the API
// handler, the transaction, the post-commit hook, the hub, the per-subscriber
// re-authorisation, the re-fetch, the render and the frame — and every one of
// those links is separately covered and separately capable of being correct
// while the chain is broken. A pre-commit hook publishes and the row is wrong;
// a hoisted authorization check passes every single-account test; a publisher
// bound to a collection nobody registered publishes nothing at all and looks
// exactly like an idle instance.
//
// Five seconds is a TIMEOUT and not a threshold. The whole path is in-process,
// so the real figure is sub-millisecond and the bound has four orders of
// magnitude of headroom: it can only fail if the pipeline is actually broken.
// Asserting on the measured elapsed time instead would be the flaky gate
// Constitution VIII forbids — the tolerance loose enough never to flake is
// loose enough to pass an oracle. The figure is reported, not asserted.

const sc007 = 5 * time.Second

func TestAWriteReachesASecondOpenViewWithinFiveSeconds(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	amara := medikube.token(t, testsupport.AccountAEmail)

	// Two open views on one account: the browser tab that makes the change and
	// the one that has to learn about it.
	acting := medikube.open(t, amara, "?patient="+testsupport.AccountAPatientSelfID)
	watching := medikube.open(t, amara, "?patient="+testsupport.AccountAPatientSelfID)

	require.Equal(t, http.StatusOK, acting.Response.StatusCode)
	require.Equal(t, http.StatusOK, watching.Response.StatusCode)

	const name = "Amoxicillin"

	started := time.Now()

	_, _ = medikube.create(t, amara, testsupport.AccountAPatientSelfID, name)

	seen := watching.nextPatch(sc007)
	elapsed := time.Since(started)

	assert.Equal(t, "#"+ids.RecordRows(kind.Medication), seen.selector())
	assert.Equal(t, "prepend", seen.mode(), "a new row is prepended into the list, not patched in place")
	assert.Containsf(t, seen.elements(), name,
		"the frame reached the second view but carries no rendered row: %q", seen.elements())

	// The positive control. realtime.Hub.Publish is a no-op with no
	// subscribers, so a test whose subscriber was never registered looks
	// identical to a passing one.
	acted := acting.nextPatch(sc007)
	assert.Equal(t, "#"+ids.RecordRows(kind.Medication), acted.selector(),
		"the view that made the change was not told about it, so the assertion above may be measuring one lucky subscriber")

	t.Logf("SC-007: the write reached the second open view in %s (the bound is %s)", elapsed, sc007)
}

func TestAChangeAndADeletionBothReachASecondOpenView(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	amara := medikube.token(t, testsupport.AccountAEmail)

	created, etag := medikube.create(t, amara, testsupport.AccountAPatientSelfID, "Amoxicillin")

	watching := medikube.open(t, amara, "?patient="+testsupport.AccountAPatientSelfID)
	require.Equal(t, http.StatusOK, watching.Response.StatusCode)

	const renamed = "Ibuprofen"

	changed := medikube.rename(t, amara, created, etag, renamed)

	update := watching.nextPatch(sc007)
	require.Equal(t, "#"+ids.RecordRow(kind.Medication, created), update.selector())
	assert.Contains(t, update.elements(), renamed,
		"the second view was patched with the row as it was before the change")
	assert.Empty(t, update.mode(), "a change replaces the row outright, which is the default mode")

	medikube.remove(t, amara, created, changed)

	removal := watching.nextPatch(sc007)
	assert.Equal(t, "#"+ids.RecordRow(kind.Medication, created), removal.selector())
	assert.Equal(t, "remove", removal.mode(),
		"a deleted record was patched rather than removed, so the row stays on the page forever")
	assert.Empty(t, removal.elements())
}

// The subscription is made before the header block is committed, so a change
// committed in the gap between the two is buffered rather than lost. Without
// that ordering there is a window on every page load in which a write
// disappears and the only symptom is a row that is missing until a reload.
func TestAChangeCommittedWhileTheStreamIsOpeningIsNotLost(t *testing.T) {
	t.Parallel()

	medikube := serve(t, fastHeartbeat())

	amara := medikube.token(t, testsupport.AccountAEmail)

	watching := medikube.open(t, amara, "?patient="+testsupport.AccountAPatientSelfID)
	require.Equal(t, http.StatusOK, watching.Response.StatusCode)

	// Several writes in a row, faster than the subscriber drains them. The hub
	// buffers realtime.SubscriberBuffer of them and every one must arrive.
	const writes = 5

	wanted := make([]string, 0, writes)

	for range writes {
		_, _ = medikube.create(t, amara, testsupport.AccountAPatientSelfID, "Amoxicillin")
		wanted = append(wanted, "#"+ids.RecordRows(kind.Medication))
	}

	arrived := make([]string, 0, len(wanted))

	for range wanted {
		arrived = append(arrived, watching.nextPatch(sc007).selector())
	}

	assert.Equal(t, wanted, arrived, "the stream dropped or reordered a committed change")
}

// rename patches one medication through the API and returns the new version.
func (i *instance) rename(t *testing.T, token, id, etag, name string) string {
	t.Helper()

	body := strings.NewReader(`{"name":"` + name + `"}`)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPatch,
		i.base+"/api/v1/records/"+kind.Medication.Segment()+"/"+id, body)
	require.NoError(t, err)

	request.Header.Set("Authorization", token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", etag)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	defer func() { _ = response.Body.Close() }()

	raw, _ := io.ReadAll(response.Body)
	require.Equalf(t, http.StatusOK, response.StatusCode, "renaming %s: %s", id, raw)

	return response.Header.Get("ETag")
}
