package realtime_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/realtime"
)

// The hub is what stands between a committed write and somebody's open page,
// and the one rule it exists to keep is that it carries identifiers (plus the
// one bit a re-fetch genuinely cannot answer, Created) and not record content
// (research D-33, contracts/streams.md). A body on this type would move the
// "may this person see it" decision from the per-subscriber authorizer to the
// publisher, which does not know who is listening.
//
// This is a shape assertion rather than a value assertion on purpose: a value
// assertion passes the day somebody adds a `Body any` field and leaves it
// empty in the one test that looks at it.
func TestTheEventCarriesIdentifiersAndNothingElse(t *testing.T) {
	t.Parallel()

	event := reflect.TypeFor[realtime.Event]()
	require.Equal(t, reflect.Struct, event.Kind())

	names := make([]string, 0, event.NumField())
	for i := range event.NumField() {
		field := event.Field(i)
		names = append(names, field.Name)

		if field.Name == "Created" {
			assert.Equalf(t, reflect.Bool, field.Type.Kind(),
				"realtime.Event.Created is the one bit that is not an identifier — a create/update flag — and must stay a bool")
		} else {
			assert.Equalf(t, reflect.String, field.Type.Kind(),
				"realtime.Event.%s is %s; every member but Created is an identifier, and anything else is a record body arriving by another name",
				field.Name, field.Type.Kind())
		}

		assert.Truef(t, field.IsExported(), "realtime.Event.%s is unexported, so a subscriber cannot read it", field.Name)
	}

	assert.ElementsMatch(t, []string{"Kind", "RecordID", "PatientID", "Created"}, names,
		"the hub's event is Kind, RecordID, PatientID and Created — contracts/streams.md's three identifiers plus the create/update bit a re-fetch cannot answer")
}

func medicationEvent(recordID, patientID string) realtime.Event {
	return realtime.Event{Kind: kind.Medication, RecordID: recordID, PatientID: patientID}
}

func TestAnEventReachesEverySubscriber(t *testing.T) {
	t.Parallel()

	hub := realtime.New()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	first := hub.Subscribe(ctx)
	second := hub.Subscribe(ctx)
	require.Equal(t, 2, hub.Subscribers())

	published := medicationEvent("rec_one", "acct_one")
	hub.Publish(published)

	assert.Equal(t, published, <-first)
	assert.Equal(t, published, <-second, "a second subscriber is not a competing consumer; both see the event")
}

func TestEveryEventArrivesInThePublishedOrder(t *testing.T) {
	t.Parallel()

	hub := realtime.New()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	events := hub.Subscribe(ctx)

	want := []realtime.Event{
		medicationEvent("rec_one", "acct_one"),
		medicationEvent("rec_two", "acct_one"),
		medicationEvent("rec_three", "acct_two"),
	}
	for _, event := range want {
		hub.Publish(event)
	}

	got := make([]realtime.Event, 0, len(want))
	for range want {
		got = append(got, <-events)
	}

	assert.Equal(t, want, got)
}

// Nothing is subscribed and nothing is listening: a publish from a post-commit
// hook must not block the write it follows.
func TestPublishingWithNoSubscribersIsANoOp(t *testing.T) {
	t.Parallel()

	hub := realtime.New()

	hub.Publish(medicationEvent("rec_one", "acct_one"))

	assert.Equal(t, 0, hub.Subscribers())
	assert.Equal(t, uint64(0), hub.Lagged())
}

// The publisher runs inside a post-commit hook on the request path. A
// subscriber that has stopped reading — a browser tab the operating system
// suspended, a stream blocked on a slow socket — must not hold that request
// open.
//
// This asserts by completing rather than by timing: a hub that blocked on a
// full subscriber would hang here and `go test` would kill the package. There
// is no duration in the assertion, so there is no threshold to tune and nothing
// to go flaky under load (Constitution VIII).
func TestASlowSubscriberDoesNotBlockThePublisher(t *testing.T) {
	t.Parallel()

	hub := realtime.New()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Subscribed and never read from. The channel is deliberately discarded:
	// this is the tab nobody is looking at.
	_ = hub.Subscribe(ctx)
	healthy := hub.Subscribe(ctx)

	// Far past the buffer, so the slow subscriber is overrun several times
	// over and the healthy one still sees everything.
	total := realtime.SubscriberBuffer * 4
	for i := range total {
		hub.Publish(medicationEvent("rec_"+string(rune('a'+i%26)), "acct_one"))
		// Reading as the publisher goes is what a live stream does, and it is
		// what keeps this subscriber healthy while the other one is not.
		<-healthy
	}

	assert.Equal(t, 1, hub.Subscribers(), "the subscriber that stopped reading is gone; the one that kept up is not")
	assert.Positive(t, hub.Lagged(), "falling behind is counted, so an operator can see it happening")
}

// A subscriber that fell behind is dropped rather than silently skipped.
//
// The distinction is the whole of FR-031: a stream that ends is one the browser
// reconnects, and a reconnect re-renders from the store. A stream that stays
// open having quietly dropped one record's event shows a page that is wrong
// with no way for anybody to notice.
func TestASubscriberThatFallsBehindIsClosedRatherThanSkipped(t *testing.T) {
	t.Parallel()

	hub := realtime.New()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	events := hub.Subscribe(ctx)

	for i := range realtime.SubscriberBuffer + 1 {
		hub.Publish(medicationEvent("rec_"+string(rune('a'+i%26)), "acct_one"))
	}

	// Drain what the buffer did hold, then the channel must be closed.
	for range realtime.SubscriberBuffer {
		<-events
	}

	// A non-blocking receive, so a hub that silently skipped the event instead
	// of dropping the subscriber fails here and says so, rather than parking
	// this goroutine until the package-wide test timeout.
	select {
	case _, open := <-events:
		assert.False(t, open, "the overrun subscription is closed, so its handler ends the stream and the browser reconnects")
	default:
		t.Fatal("the overrun subscription is still open and empty: the event was skipped, and nothing on the page will ever say so")
	}

	assert.Equal(t, 0, hub.Subscribers())
	assert.Equal(t, uint64(1), hub.Lagged())
}

func TestPublishingIsSafeFromManyGoroutinesAtOnce(t *testing.T) {
	t.Parallel()

	hub := realtime.New()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	events := hub.Subscribe(ctx)

	const publishers = 8

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range publishers {
			<-events
		}
	}()

	for i := range publishers {
		go hub.Publish(medicationEvent("rec_"+string(rune('a'+i)), "acct_one"))
	}

	<-done
	assert.Equal(t, 1, hub.Subscribers())
}
