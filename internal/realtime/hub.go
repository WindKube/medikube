package realtime

import (
	"context"
	"sync"

	"medikube/internal/domain/kind"
)

// Event is what one committed change tells a subscriber: which kind, which
// record, and whose it is. Nothing else.
//
// **IDs, never bodies** (research D-33, contracts/streams.md). The rule is not
// an optimisation. A hub carrying record content would have to decide at
// publish time who may see it, which is the authorizer's decision made in the
// wrong place by the one participant that does not know who is listening. With
// only identifiers on the wire, every subscriber's handler re-runs
// access.Authorizer.Patient for its own viewer and re-fetches, so a person who
// lost access mid-stream simply stops receiving patches.
//
// It carries no action beyond Created, and that is deliberate rather than an
// omission: the handler re-fetches every id it is told about, and a re-fetch
// that comes back empty — deleted, or no longer visible to this subscriber —
// is exactly a row removal. A delete/update field would be a second source of
// truth for something the fetch already answers, and the fetch is the one
// that cannot lie. Create is different: a fetch that succeeds cannot tell the
// handler whether the row already existed in the subscriber's view, so
// Created is the one bit the fetch genuinely cannot answer on its own.
type Event struct {
	Kind      kind.Kind
	RecordID  string
	PatientID string

	// Created marks an event published from the record's creation, not its
	// update. A subscriber's handler uses it to insert a new row into a list
	// rather than patch one that is not there yet.
	Created bool
}

// SubscriberBuffer is how far behind a subscriber may fall before it is
// dropped. Sixty-four events is far more than a person's own writes plus a
// burst of their own imports, and small enough that a stalled connection is
// noticed rather than accumulating.
const SubscriberBuffer = 64

// Hub fans committed-change identifiers out to the open streams.
//
// A channel and a map, with no broker interface in front of it: MediKube is
// single-instance by construction (SQLite is single-writer, the hub is
// in-process) and a seam for the second instance that does not exist is the
// speculative abstraction the constitution forbids by name. Putting a broker
// behind this API later changes this file and the one hook that publishes.
type Hub struct {
	mu     sync.Mutex
	subs   map[uint64]chan Event
	next   uint64
	lagged uint64
	closed bool
	// done releases the watcher goroutines whose subscriber context is still
	// live when the process shuts down. Without it, a stream that nobody
	// cancelled would park a goroutine until exit.
	done chan struct{}
}

// New returns a hub with nothing subscribed. It starts no goroutine: a hub
// nobody subscribes to costs one mutex and one empty map.
func New() *Hub {
	return &Hub{
		subs: make(map[uint64]chan Event),
		done: make(chan struct{}),
	}
}

// Subscribe returns the channel this subscriber reads until ctx is cancelled.
//
// The channel is closed — never left dangling — when the context is cancelled,
// when the hub shuts down, or when this subscriber falls further behind than
// SubscriberBuffer. A handler therefore ranges over it and returns when it
// ends, with no second signal to check.
func (h *Hub) Subscribe(ctx context.Context) <-chan Event {
	events := make(chan Event, SubscriberBuffer)

	h.mu.Lock()

	if h.closed {
		h.mu.Unlock()
		// A closed channel rather than a live one nothing will ever close: a
		// handler that arrived an instant after shutdown must end, not wait.
		close(events)

		return events
	}

	id := h.next
	h.next++
	h.subs[id] = events

	h.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
		case <-h.done:
		}

		h.drop(id)
	}()

	return events
}

// Publish hands an event to every current subscriber and returns. It never
// blocks.
//
// It runs inside a post-commit record hook, on the request path, so a
// subscriber that has stopped reading — a suspended browser tab, a stream
// blocked on a slow socket — must not hold that request open.
//
// A subscriber whose buffer is full is **dropped**, not skipped. Skipping is
// the worse failure: the stream stays open having silently lost one record's
// update, and the page is then wrong with nothing to notice it by. A closed
// stream is one the browser reconnects, and a reconnect renders from the store
// (FR-031).
func (h *Hub) Publish(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for id, events := range h.subs {
		select {
		case events <- event:
		default:
			delete(h.subs, id)
			close(events)
			h.lagged++
		}
	}
}

// Shutdown closes every subscription and releases every watcher. Later
// publishes are no-ops and later subscribers get an already-closed channel.
//
// It takes no arguments and returns nothing so that samber/do's Shutdowner
// calls it as part of the container's own shutdown, and it is idempotent
// because a signal handler and the container may both reach it.
func (h *Hub) Shutdown() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}

	h.closed = true
	close(h.done)

	for id, events := range h.subs {
		delete(h.subs, id)
		close(events)
	}
}

// Subscribers is how many streams are currently attached. It is what the leak
// assertion reads, and what a gauge will read in US3.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	return len(h.subs)
}

// Lagged counts the subscriptions dropped for falling behind since the process
// started. A number that climbs is a real operational signal: somebody's live
// view is being cut and reconnected rather than kept.
func (h *Hub) Lagged() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.lagged
}

// drop removes one subscription if it is still present. It is idempotent
// because a subscriber can leave twice: once by falling behind, and once when
// the handler that noticed returns and cancels its context.
func (h *Hub) drop(id uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	events, subscribed := h.subs[id]
	if !subscribed {
		return
	}

	delete(h.subs, id)
	close(events)
}
