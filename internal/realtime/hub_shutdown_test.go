package realtime_test

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/realtime"
)

// T129 asks for a goroutine count. It is written with testing/synctest
// instead, and the substitution is an upgrade rather than a shortcut:
// synctest.Test fails the test if any goroutine started inside the bubble is
// still parked when the bubble's root returns, and it names the line that
// started it. runtime.NumGoroutine() cannot do that — the runtime's own
// workers move under it and a goroutine that has just been unblocked may not
// have been descheduled yet, so the assertion needs a polling loop and an
// arbitrary deadline, which is the flaky gate Constitution VIII forbids.
//
// Two things must both hold and only one of them is about goroutines: the
// watcher has to exit, AND the map entry has to be gone. A watcher that
// returned without deleting its subscription would leave the bubble clean and
// the hub leaking memory for the life of the process, so every case below
// asserts the count as well.
//
// Nothing in this file may call t.Parallel, t.Run or t.Deadline: synctest
// refuses all three inside a bubble. That is why these are separate top-level
// functions rather than one table.
func TestCancellingASubscribersContextUnsubscribesIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub := realtime.New()

		ctx, cancel := context.WithCancel(context.Background())
		events := hub.Subscribe(ctx)
		require.Equal(t, 1, hub.Subscribers())

		cancel()
		synctest.Wait()

		assert.Equal(t, 0, hub.Subscribers(), "the cancelled subscription is out of the map, not merely unread")

		_, open := <-events
		assert.False(t, open, "the channel is closed, so the handler's range ends instead of blocking forever")
	})
}

// The publisher must not deliver to a subscription whose context is done, and
// a delivery attempt on a closed channel would panic the request that
// triggered it.
func TestPublishingAfterASubscriberIsCancelledDeliversNothingAndDoesNotPanic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub := realtime.New()

		ctx, cancel := context.WithCancel(context.Background())
		events := hub.Subscribe(ctx)

		cancel()
		synctest.Wait()

		assert.NotPanics(t, func() {
			hub.Publish(realtime.Event{RecordID: "rec_one", PatientID: "acct_one"})
		})

		_, open := <-events
		assert.False(t, open)
	})
}

// Shutdown has to release watchers whose context is still live, and this is
// the case that proves it: no context here is ever cancelled. A hub whose
// watcher waited on ctx.Done() alone would park three goroutines forever and
// this bubble would fail with a deadlock naming the line that started them.
func TestShutdownReleasesEverySubscriberWhoseContextIsStillLive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub := realtime.New()

		subscriptions := make([]<-chan realtime.Event, 0, 3)
		for range 3 {
			subscriptions = append(subscriptions, hub.Subscribe(context.Background()))
		}
		require.Equal(t, 3, hub.Subscribers())

		hub.Shutdown()
		synctest.Wait()

		assert.Equal(t, 0, hub.Subscribers())
		for i, events := range subscriptions {
			_, open := <-events
			assert.Falsef(t, open, "subscription %d is still open after shutdown", i)
		}
	})
}

func TestShutdownIsIdempotentAndSilencesLaterPublishes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub := realtime.New()

		hub.Shutdown()
		assert.NotPanics(t, hub.Shutdown, "the container may shut down after a signal handler already did")
		assert.NotPanics(t, func() {
			hub.Publish(realtime.Event{RecordID: "rec_one", PatientID: "acct_one"})
		}, "a post-commit hook that outlives the hub must not take the process down with it")
	})
}

// Subscribe cannot return a channel nobody will ever close: a handler that
// arrived one instant after shutdown would range over it until the process
// ended.
func TestSubscribingAfterShutdownYieldsAnAlreadyClosedChannel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub := realtime.New()
		hub.Shutdown()

		events := hub.Subscribe(context.Background())
		synctest.Wait()

		_, open := <-events
		assert.False(t, open)
		assert.Equal(t, 0, hub.Subscribers())
	})
}

// The subscription a handler abandons because it fell behind must be cleaned
// up as thoroughly as one that was cancelled: the map entry gone and the
// watcher exited.
func TestAnOverrunSubscriptionLeavesNothingBehind(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub := realtime.New()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		events := hub.Subscribe(ctx)
		for range realtime.SubscriberBuffer + 1 {
			hub.Publish(realtime.Event{RecordID: "rec_one", PatientID: "acct_one"})
		}

		require.Equal(t, 0, hub.Subscribers())
		for range realtime.SubscriberBuffer {
			<-events
		}
		_, open := <-events
		require.False(t, open)

		// The handler noticed and returned, which cancels its context. The
		// watcher must then exit rather than find a subscription that is
		// already gone and block on something.
		cancel()
		synctest.Wait()

		assert.Equal(t, 0, hub.Subscribers())
	})
}
