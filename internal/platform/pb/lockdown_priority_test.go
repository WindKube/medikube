package pb_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/platform/pb"
)

// findMiddleware looks the handler up by id rather than by position.
// RouterGroup.Bind only appends (tools/router/group.go:59); the priority sort
// happens per route inside Router.loadMux. Asserting a slice index would be
// asserting registration order, which is a different fact.
func findMiddleware(middlewares []*hook.Handler[*core.RequestEvent], id string) *hook.Handler[*core.RequestEvent] {
	for _, m := range middlewares {
		if m.Id == id {
			return m
		}
	}

	return nil
}

// -1009 is not a magic number, it is a position in a chain, and the neighbours
// are what give it meaning:
//
//	-1020 pbLoadAuthToken        — bound earlier, the lockdown cannot see who is asking
//	-1015 superuserIPsWhitelist
//	-1010 securityHeaders        — bound earlier, the lockdown short-circuits past it
//	                               and its 404 loses four headers a genuine 404 keeps
//	-1009 medikubeLockdown
//	-1000 rateLimit
//	 ...  the record-CRUD handler — bound later, it has already answered
//
// The lockdown short-circuits, so everything bound after it is skipped. That is
// what makes the lower bound a correctness question and not a preference.
func TestLockdownIsBoundAfterTheSecurityHeaders(t *testing.T) {
	t.Parallel()

	h := newHarness(t, bindMediKubeServe)

	lockdown := findMiddleware(h.se.Router.Middlewares, pb.LockdownMiddlewareID)
	require.NotNil(t, lockdown,
		"the lockdown must be bound on the ROOT router: RouterGroup.children is unexported, so a sub-group binding is unreachable and applies to nothing")

	t.Run("it sits exactly one step after loadAuthToken", func(t *testing.T) {
		assert.Equal(t, apis.DefaultSecurityHeadersMiddlewarePriority+1, lockdown.Priority)
		assert.Equal(t, -1009, lockdown.Priority, "the numeric value, so an upstream renumber is loud rather than silent")
		assert.Equal(t, -1009, pb.LockdownPriority)
	})

	t.Run("the priorities it is ordered against have not moved", func(t *testing.T) {
		assert.Equal(t, -1020, apis.DefaultLoadAuthTokenMiddlewarePriority)
		assert.Equal(t, -1000, apis.DefaultRateLimitMiddlewarePriority)
		assert.Equal(t, -1015, apis.DefaultSuperuserIPsWhitelistMiddlewarePriority)
	})

	t.Run("it runs after the actor is known and before anything answers", func(t *testing.T) {
		loadAuthToken := findMiddleware(h.se.Router.Middlewares, apis.DefaultLoadAuthTokenMiddlewareId)
		require.NotNil(t, loadAuthToken)

		assert.Greater(t, lockdown.Priority, loadAuthToken.Priority,
			"bound earlier, e.Auth is nil and the superuser carve-out cannot exist")
		assert.Less(t, lockdown.Priority, apis.DefaultRateLimitMiddlewarePriority,
			"bound later, the rate limiter would already have counted a request the lockdown denies exists")
		assert.Greater(t, lockdown.Priority, apis.DefaultSecurityHeadersMiddlewarePriority,
			"bound earlier, the lockdown short-circuits before the security headers are set and its 404 is distinguishable from a genuine one")
		assert.Greater(t, lockdown.Priority, apis.DefaultSuperuserIPsWhitelistMiddlewarePriority,
			"the superuser IP allowlist still gets its say; the lockdown must not be able to invert that order")
	})
}
