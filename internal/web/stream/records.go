package stream

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/starfederation/datastar-go/datastar"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/realtime"
	"medikube/internal/records"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/ids"
)

// OpStreamRecords is the operation id internal/httproute registers this
// handler under. It is a constant here and matched against the route table by
// records_test.go rather than spelled twice.
const OpStreamRecords = "streamRecords"

// ParamKind is the stream's one query parameter: a comma list of registered
// path segments, absent meaning every registered kind.
const ParamKind = "kind"

// ErrNoHub is a build whose stream was wired without the fan-out. It is an
// internal failure and never the caller's: a stream that subscribed to nothing
// would hold a connection open forever and send only heartbeats, which is
// indistinguishable from a working stream on an account nobody is writing to.
var ErrNoHub = errors.New("stream: the record stream was wired without the realtime hub, so no committed change could ever reach it")

// Subscriber is the half of realtime.Hub this package uses, declared here by
// the consumer so a test can hand in its own.
type Subscriber interface {
	Subscribe(ctx context.Context) <-chan realtime.Event
}

// Deps is what the composition root supplies.
type Deps struct {
	// Resolve hands over the kind registry. It is a function for the same
	// reason internal/web/api's is: a kind's repository needs the cursor
	// codec, which is keyed from a secret the migrations create, and the
	// migrations run inside OnServe long after the handler table is built.
	Resolve api.Resolve

	Hub Subscriber

	// Heartbeat overrides HeartbeatInterval. Zero means the constant, which is
	// what production passes; a test that waited 25 seconds for its first
	// assertion would be a test nobody runs.
	Heartbeat time.Duration

	// Now is the clock the heartbeat reads. Zero means time.Now.
	Now func() time.Time
}

// Handlers is the stream's contribution to the route table: one operation, every
// registered kind, no route of its own.
func Handlers(deps Deps) (httproute.Handlers, error) {
	if deps.Resolve == nil {
		return nil, api.ErrNoRecords
	}

	if deps.Hub == nil {
		return nil, ErrNoHub
	}

	if deps.Heartbeat <= 0 {
		deps.Heartbeat = HeartbeatInterval
	}

	if deps.Now == nil {
		deps.Now = time.Now
	}

	return httproute.Handlers{
		OpStreamRecords: web.WithActor((&streams{deps: deps}).records),
	}, nil
}

type streams struct {
	deps Deps
}

// records is the per-subscriber handler.
//
// Everything expensive happens before the stream opens: an unregistered kind is
// refused with a status a client can read, because a refusal delivered as an
// SSE frame is indistinguishable from a working stream that never sends
// anything. Once newStream has committed the header block the response is
// 200 and the only remaining outcomes are frames and a close.
func (s *streams) records(e *core.RequestEvent, actor access.Actor) error {
	handler, err := s.deps.Resolve()
	if err != nil {
		return err
	}

	entries, err := entriesByKind(handler)
	if err != nil {
		return err
	}

	selected, err := selection(e, entries)
	if err != nil {
		return err
	}

	if !actor.Authenticated() {
		// Unreachable through the router — the route table declares AuthUser
		// and internal/httproute binds apis.RequireAuth — and here anyway,
		// because a stream is the one response shape where the failure of that
		// binding would look exactly like success.
		return fmt.Errorf("stream: the record stream needs a session: %w",
			&web.Coded{Status: http.StatusUnauthorized, Code: web.CodeUnauthenticated})
	}

	ctx := e.Request.Context()

	// Subscribed before the stream opens, so a change committed between the
	// header block and the first read is buffered rather than lost.
	events := s.deps.Hub.Subscribe(ctx)

	sse, err := newStream(e)
	if err != nil {
		return err
	}

	return s.pump(ctx, sse, actor, entries, selected, events)
}

// pump is the subscriber loop: heartbeat, event, shutdown.
func (s *streams) pump(
	ctx context.Context,
	sse *datastar.ServerSentEventGenerator,
	actor access.Actor,
	entries map[kind.Kind]records.Entry,
	selected map[kind.Kind]struct{},
	events <-chan realtime.Event,
) error {
	// The first beat goes out immediately. Without it a page opened at the
	// wrong moment sits for 25 seconds with no $stream_beat at all, and the
	// staleness detector compares against a signal that has never been set.
	if err := s.beat(sse); err != nil {
		return s.ended(ctx, err)
	}

	ticker := time.NewTicker(s.deps.Heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// cancelBaseCtx cancels every request context at the start of
			// PocketBase's terminate sequence, so this is the shutdown path as
			// well as the disconnect one. Returning nil rather than the
			// context error keeps a clean shutdown out of the error stream.
			return nil

		case <-ticker.C:
			if err := s.beat(sse); err != nil {
				return s.ended(ctx, err)
			}

		case event, open := <-events:
			if !open {
				// The hub closed this subscription: shutdown, or this
				// subscriber fell further behind than realtime.SubscriberBuffer.
				// Either way the stream ends and the browser reconnects, which
				// re-renders from the store.
				return nil
			}

			if err := s.patch(ctx, sse, actor, entries, selected, event); err != nil {
				return s.ended(ctx, err)
			}
		}
	}
}

// patch is contracts/streams.md's four steps, in order, for one event.
//
// Step 2 is the re-authorization and it is inside this function on purpose. It
// must not be hoisted to where the subscription is made: fanning out ids rather
// than bodies is what makes per-subscriber authorization possible at all, and a
// check made once at subscribe time is a check that cannot notice access being
// lost while the stream is open.
func (s *streams) patch(
	ctx context.Context,
	sse *datastar.ServerSentEventGenerator,
	actor access.Actor,
	entries map[kind.Kind]records.Entry,
	selected map[kind.Kind]struct{},
	event realtime.Event,
) error {
	// 1. the subscriber's own kind selection.
	if len(selected) > 0 {
		if _, wanted := selected[event.Kind]; !wanted {
			return nil
		}
	}

	entry, registered := entries[event.Kind]
	if !registered {
		return nil
	}

	// The kind's own filter, which is not authorization and never stands in
	// for it: it runs once per event for every subscriber and knows nothing
	// about who is listening.
	if !entry.Stream.Streams(event.RecordID, event.OwnerID) {
		return nil
	}

	// 2. re-authorise, for THIS subscriber, for THIS event.
	grant, err := entry.Authorizer.Record(ctx, actor, event.Kind, event.RecordID, access.PermView)
	if err != nil {
		return err
	}

	if !grant.Allows(access.PermView) {
		// A subscriber who may not see this record is told nothing at all —
		// not an empty frame, not a removal. Zero frames is the assertion
		// contracts/streams.md makes and the one FR-032 turns on.
		return nil
	}

	// 3. re-fetch, through the kind's own service, so the stream cannot read
	// anything a request could not.
	record, err := entry.Service.Get(ctx, actor, event.RecordID)

	switch {
	case err == nil:
		// 4. render and patch by the deterministic id the row carries.
		return s.patchRow(ctx, sse, entry, record)

	case errors.Is(err, domain.ErrNotFound):
		return s.removeRow(sse, actor, event)

	case errors.Is(err, domain.ErrForbidden):
		// Access lost mid-stream. contracts/streams.md's specified behaviour is
		// that the patches stop and the stream does NOT error — the view stops
		// updating, the staleness detector says so, and the next action lands
		// on the sign-in page. Returning the refusal here would close the
		// stream and the browser would reconnect into the same refusal, in a
		// loop.
		return nil //nolint:nilerr // swallowing the refusal is the specified behaviour

	default:
		return err
	}
}

func (s *streams) patchRow(
	ctx context.Context,
	sse *datastar.ServerSentEventGenerator,
	entry records.Entry,
	record records.Record,
) error {
	html, err := render(ctx, entry.Views.Row(record))
	if err != nil {
		return err
	}

	return sse.PatchElements(html, datastar.WithSelectorID(ids.RecordRow(entry.Kind, record.ID)))
}

// removeRow answers a record the subscriber cannot fetch any more.
//
// The owner comparison is the one place this package knows what ownership is,
// and it is here because a deleted record has no owner left for the checkpoint
// to resolve: internal/service/access grants for an id that is not there, by
// design (research D-20), so "granted, then not found" is reached both by a row
// that was just deleted and — if that guard were removed — by a row of somebody
// else's that never existed for this subscriber. Sending a removal in the
// second case would put another account's record id on this account's wire,
// which is the disclosure FR-033 closes and the zero-frames assertion
// contracts/streams.md requires.
//
// Phase 005 replaces it: a share makes "who could see this record before it was
// deleted" a question only the checkpoint can answer, and the checkpoint will
// need to be asked about a record that no longer exists.
func (s *streams) removeRow(sse *datastar.ServerSentEventGenerator, actor access.Actor, event realtime.Event) error {
	if event.OwnerID != actor.UserID {
		return nil
	}

	return sse.RemoveElementByID(ids.RecordRow(event.Kind, event.RecordID))
}

// heartbeat is FR-031's half of the staleness detection that the server can
// do. The member name is SignalStreamBeat's value and the two are pinned
// together by heartbeat_test.go: a tag that drifted would leave the page
// comparing a signal the server never sets.
type heartbeat struct {
	StreamBeat string `json:"stream_beat"`
}

func (s *streams) beat(sse *datastar.ServerSentEventGenerator) error {
	// RFC3339 in UTC, which is what the page's Date.parse reads. A local
	// offset would make the comparison depend on the server's timezone.
	payload, err := json.Marshal(heartbeat{StreamBeat: s.deps.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		return fmt.Errorf("stream: encoding the heartbeat: %w", err)
	}

	return sse.PatchSignals(payload)
}

// ended decides whether a failed write is an error or a closed connection.
//
// A browser that navigated away and a shutdown that cancelled every request
// context both surface as a write failure or a cancelled send, and neither is
// something to report: the request is over and there is nobody to tell.
func (s *streams) ended(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return nil //nolint:nilerr // a cancelled request is not a failure to report
	}

	return err
}

// selection reads the `kind` parameter into the set of kinds this subscriber
// wants. An absent or empty parameter is every registered kind, expressed as an
// empty set so that a later kind is included without this handler changing.
//
// An unregistered value is 422 and not 404, matching the cross-kind list: a
// query parameter is reached only by a caller who already knows the path
// exists, so naming the offending parameter discloses nothing it did not
// already have.
func selection(e *core.RequestEvent, entries map[kind.Kind]records.Entry) (map[kind.Kind]struct{}, error) {
	raw := e.Request.URL.Query().Get(ParamKind)
	if raw == "" {
		return nil, nil
	}

	// The registry's own resolved segment, never kind.Kind.Segment(): a
	// synthetic kind has no row in the kind table, so its Segment() is empty
	// and every one of its spellings would silently match nothing — which is
	// how the second registered kind drops out of the selection unnoticed.
	// internal/records/handler.go's byKind map carries the same warning.
	segments := make(map[string]kind.Kind, len(entries))
	for k, entry := range entries {
		segments[entry.Segment] = k
	}

	var invalid domain.ValidationError

	selected := make(map[kind.Kind]struct{})

	// Whitespace is not trimmed, following contracts/README.md: trimming would
	// make " medications" a second spelling of a published value.
	for _, segment := range strings.Split(raw, ",") {
		k, registered := segments[segment]
		if !registered {
			invalid.Add(ParamKind, domain.CodeInvalidValue, "the kind is not one this instance serves")

			continue
		}

		selected[k] = struct{}{}
	}

	if err := invalid.OrNil(); err != nil {
		return nil, err
	}

	return selected, nil
}

// entriesByKind is the dispatch table keyed the way an event names a kind.
//
// internal/records keys its own by path segment, because that is what a URL
// carries; the hub carries a kind.Kind, so this is the one place the two meet.
func entriesByKind(handler *records.Handler) (map[kind.Kind]records.Entry, error) {
	segments := handler.Segments()
	entries := make(map[kind.Kind]records.Entry, len(segments))

	for _, segment := range segments {
		entry, err := handler.Dispatch(segment)
		if err != nil {
			return nil, fmt.Errorf("stream: the registry lists %q and cannot dispatch it: %w", segment, err)
		}

		entries[entry.Kind] = entry
	}

	return entries, nil
}

// buffers is this package's render pool, so that a component is rendered whole
// before any of it reaches the socket.
//
// Buffering is not an optimisation here: templ's own writer flushes as it
// fills, so a component that failed halfway would have already put half a row
// into a frame the browser would then patch into the DOM.
var buffers = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func render(ctx context.Context, component records.Renderer) (string, error) {
	if component == nil {
		return "", errors.New("stream: the kind rendered no row for a record it was asked about")
	}

	buffer, ok := buffers.Get().(*bytes.Buffer)
	if !ok {
		buffer = new(bytes.Buffer)
	}

	defer func() {
		buffer.Reset()
		buffers.Put(buffer)
	}()

	if err := component.Render(ctx, buffer); err != nil {
		return "", err
	}

	return buffer.String(), nil
}
