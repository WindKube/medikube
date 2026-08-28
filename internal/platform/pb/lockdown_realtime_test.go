package pb_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

// Realtime is a second door to the same record bodies.
//
// Subscribing to a collection delivers its create and update payloads in full,
// record included. The subscription travels on the POST, and so does the auth:
// EventSource cannot set headers, which is why the admin UI's GET stream is
// anonymous by design and why closing the POST is enough. An anonymous caller
// keeps an open, empty stream and receives nothing.
//
// Under MediKube's own schema the nil ListRule and ViewRule already stop this.
// That is the point: record CRUD had two independent controls and realtime had
// one, and the single control is the one the lockdown exists to back up.
func TestRealtimeSubscriptionIsLockedForEveryoneButASuperuser(t *testing.T) {
	t.Parallel()

	h := newHarness(t, bindMediKubeServe)

	body := `{"clientId":"irrelevant","subscriptions":["` + kind.Medication.Collection() + `"]}`

	t.Run("anonymous is refused as a route that does not exist", func(t *testing.T) {
		t.Parallel()

		res := h.do(t, http.MethodPost, "/api/realtime", "", body)
		assert.Equal(t, http.StatusNotFound, res.Status)
	})

	t.Run("the GET stream is deliberately not locked", func(t *testing.T) {
		t.Parallel()

		// Asserted structurally rather than by issuing the request: the GET is
		// an SSE stream and the handler holds it for PocketBase's five-minute
		// idle timeout, which a unit test cannot afford to wait out.
		assert.NotContains(t, lockedPatternsUnderTest(), "GET /api/realtime",
			"locking the GET would break the admin UI without closing the door, because the subscription travels on the POST")
	})

	t.Run("the locked set names it", func(t *testing.T) {
		t.Parallel()

		var found bool
		for _, locked := range lockedPatternsUnderTest() {
			if locked == "POST /api/realtime" {
				found = true
			}
		}
		require.True(t, found, "POST /api/realtime must be in LockedRoutes()")
	})
}
