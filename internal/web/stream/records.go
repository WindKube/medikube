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

// ParamKind is a comma list of registered path segments, absent meaning every
// registered kind.
const ParamKind = "kind"

// ParamPatient is `?patient=`, required (contracts/medications-rescope.md,
// FR-015): a stream over patient-scoped data names its patient the same way a
// list does, and there is no fallback to the caller's own active patient.
const ParamPatient = "patient"

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

	// Sessions re-checks the identity a stream was opened with. Nil means the
	// production one, which re-validates the request's own token against the
	// instance; the in-package harness supplies a revocable fake.
	Sessions Sessions
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

	if deps.Sessions == nil {
		deps.Sessions = tokenSessions{}
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

	patientID := e.Request.URL.Query().Get(ParamPatient)
	if patientID == "" {
		return web.ErrPatientRequired
	}

	// Captured before anything is subscribed to, because a stream that cannot
	// re-check its identity must not open at all.
	session, err := s.deps.Sessions.Open(e)
	if err != nil {
		return err
	}

	ctx := e.Request.Context()

	// Subscribed before the stream opens, so a change committed between the
	// header block and the first read is buffered rather than lost.
	events := s.deps.Hub.Subscribe(ctx)

	sse, err := newStream(e)
	if err != nil {
		return err
	}

	return s.pump(ctx, sse, subscriber{actor: actor, session: session}, entries, selected, patientID, events)
}

// subscriber is who a stream is running as: the actor every record is
// authorized against, and the session that authority expires with.
//
// The two travel together because they are two halves of one question that the
// loop has to keep asking. Re-authorising the record while never re-checking
// the identity is what let a revoked session keep receiving rows.
type subscriber struct {
	actor   access.Actor
	session Session
}

// pump is the subscriber loop: heartbeat, event, shutdown.
func (s *streams) pump(
	ctx context.Context,
	sse *datastar.ServerSentEventGenerator,
	who subscriber,
	entries map[kind.Kind]records.Entry,
	selected map[kind.Kind]struct{},
	patientID string,
	events <-chan realtime.Event,
) error {
	// The first beat goes out immediately. Without it a page opened at the
	// wrong moment sits for 25 seconds with no $_stream_beat at all, and the
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
			// Before the beat, not after it: a beat is what tells the page its
			// live view is healthy, and one sent on a session that has ended
			// is that page being told so wrongly.
			if err := who.session.Live(ctx); err != nil {
				return s.revoked(ctx, err)
			}

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

			// FR-032: authorized against the signed-in person at the moment
			// of access. The checkpoint below answers for the record; this
			// answers for the person, and without it the record check runs
			// against an identity that stopped existing an hour ago.
			if err := who.session.Live(ctx); err != nil {
				return s.revoked(ctx, err)
			}

			if err := s.patch(ctx, sse, who.actor, entries, selected, patientID, event); err != nil {
				return s.ended(ctx, err)
			}
		}
	}
}

// revoked ends the stream and decides whether the ending is worth reporting.
//
// Every failed identity re-check ends it. A session that ended is not a failure
// though — it is a person signing out, changing their password, or a token
// reaching its expiry — so it closes the connection and says nothing, and the
// browser's reconnect is refused by the route's own authentication. A check
// that could not be MADE is reported: a stream carrying somebody's records must
// stop when nothing can say whose they still are, and that one is not routine.
func (s *streams) revoked(ctx context.Context, err error) error {
	if errors.Is(err, ErrSessionEnded) {
		return nil
	}

	return s.ended(ctx, err)
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
	patientID string,
	event realtime.Event,
) error {
	// 0. the subscriber's own patient: a stream is opened for one patient
	// (contracts/medications-rescope.md), and an event naming another is not
	// this subscriber's to re-authorize at all.
	if event.PatientID != patientID {
		return nil
	}

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
	if !entry.Stream.Streams(event.RecordID, event.PatientID) {
		return nil
	}

	// 2. re-authorise, for THIS subscriber, for THIS event's patient.
	grant, err := entry.Authorizer.Patient(ctx, actor, event.PatientID, access.PermView)
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
		return s.patchRow(ctx, sse, entry, record, event.Created)

	case errors.Is(err, domain.ErrNotFound):
		return s.removeRow(sse, entry, event)

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

// patchRow renders the row and patches it in.
//
// An update patches the row element in place by its own id, which is the
// default outer-patch behaviour. A create has no such element to patch: the
// row does not exist in any open list yet, so it is prepended into the list's
// tbody instead, keyed by the same row id it will carry from then on.
func (s *streams) patchRow(
	ctx context.Context,
	sse *datastar.ServerSentEventGenerator,
	entry records.Entry,
	record records.Record,
	created bool,
) error {
	html, err := render(ctx, entry.Views.Row(record))
	if err != nil {
		return err
	}

	if created {
		return sse.PatchElements(html,
			datastar.WithSelectorID(ids.RecordRows(entry.Kind)),
			datastar.WithModePrepend())
	}

	return sse.PatchElements(html, datastar.WithSelectorID(ids.RecordRow(entry.Kind, record.ID)))
}

// removeRow answers a record the subscriber cannot fetch any more.
//
// Step 2 already authorized this subscriber against the event's patient, and
// that patient (unlike the deleted record) still exists — so unlike phase
// 001's owner-scoped checkpoint, which granted for an id that was not there by
// design (research D-20), reaching here means the grant was genuine and not a
// side effect of a miss. There is nothing left to compare here.
func (s *streams) removeRow(sse *datastar.ServerSentEventGenerator, entry records.Entry, event realtime.Event) error {
	return sse.RemoveElementByID(ids.RecordRow(entry.Kind, event.RecordID))
}

// heartbeat is FR-031's half of the staleness detection that the server can
// do. The member name is SignalStreamBeat's value and the two are pinned
// together by heartbeat_test.go: a tag that drifted would leave the page
// comparing a signal the server never sets.
type heartbeat struct {
	StreamBeat string `json:"_stream_beat"`
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
