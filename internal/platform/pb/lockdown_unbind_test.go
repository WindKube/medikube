package pb_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/platform/pb"
)

// unbindingBinder is a RouteBinder that removes the lockdown, which any
// RouteBinder can do: RouterGroup.Unbind strips a handler by id and reports
// nothing, so the instance boots with PocketBase's record API wide open.
type unbindingBinder struct{}

func (unbindingBinder) Bind(se *core.ServeEvent) error {
	se.Router.Unbind(pb.LockdownMiddlewareID)

	return nil
}

// The route table binds after the lockdown, so it can undo it. The assertion
// that catches this has to run after the route table, and previously ran before
// it -- which made it structurally incapable of catching the one thing it is
// for.
func TestServeRefusesToStartIfTheRouteTableUnbindsTheLockdown(t *testing.T) {
	t.Parallel()

	t.Run("an unbinding route table aborts the serve", func(t *testing.T) {
		t.Parallel()

		app := serveApp(t)
		pb.BindServe(app, pb.ServeOptions{Routes: unbindingBinder{}})

		_, err := triggerServe(t, app, pocketBaseServer())

		require.Error(t, err, "the lockdown was removed and the instance started anyway")
		assert.ErrorIs(t, err, pb.ErrLockdownUnbound)
	})

	t.Run("an ordinary route table still boots", func(t *testing.T) {
		t.Parallel()

		app := serveApp(t)
		pb.BindServe(app, pb.ServeOptions{Routes: &recordingBinder{}})

		_, err := triggerServe(t, app, pocketBaseServer())
		assert.NoError(t, err)
	})

	t.Run("a lockdown rebound at the wrong priority is refused too", func(t *testing.T) {
		t.Parallel()

		app := serveApp(t)
		pb.BindServe(app, pb.ServeOptions{Routes: reprioritisingBinder{}})

		_, err := triggerServe(t, app, pocketBaseServer())

		require.Error(t, err)
		assert.ErrorIs(t, err, pb.ErrLockdownUnbound)
	})
}

// reprioritisingBinder keeps the id but moves it below the security headers,
// which is the subtler half of the same failure: the middleware is present, so
// a presence check passes, and its 404 is distinguishable again.
type reprioritisingBinder struct{}

func (reprioritisingBinder) Bind(se *core.ServeEvent) error {
	se.Router.Unbind(pb.LockdownMiddlewareID)

	lockdown := pb.Lockdown()
	lockdown.Priority = pb.LockdownPriority - 100
	se.Router.Bind(lockdown)

	return nil
}
