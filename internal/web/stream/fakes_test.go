package stream

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/realtime"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	"medikube/internal/web"
	"medikube/internal/web/api"
)

// The in-package harness. It drives the handler over a real socket and never
// through a httptest.ResponseRecorder, because a recorder cannot be read while
// the handler is still writing and cannot take a write deadline at all — the
// two properties every assertion in this package turns on.
//
// The registry is built from internal/records/recordstest rather than from a
// database: what these tests are about is the loop — the kind filter, the
// per-event re-authorisation, the render and the frame — and a real repository
// would only make the same assertions slower and less able to say what was
// wrong. internal/web/stream's external suite drives the real thing.

// frame is one parsed server-sent event.
type frame struct {
	Event string
	Data  []string
}

func (f frame) field(prefix string) string {
	for _, line := range f.Data {
		if after, found := strings.CutPrefix(line, prefix+" "); found {
			return after
		}
	}

	return ""
}

func (f frame) selector() string { return f.field("selector") }
func (f frame) elements() string { return f.field("elements") }
func (f frame) mode() string     { return f.field("mode") }

// rig is one running handler: a server, a hub it subscribes to and the frames
// it produced.
type rig struct {
	t *testing.T

	events chan realtime.Event
	frames chan frame

	authorizer *countingAuthorizer
	sessions   *fakeSessions
	services   map[kind.Kind]*recordstest.FakeKindService

	response *http.Response
	failure  func() error
}

type rigOptions struct {
	actor     access.Actor
	query     string
	deny      bool
	kinds     []kind.Kind
	heartbeat time.Duration
	now       func() time.Time
	openErr   error
}

type rigOption func(*rigOptions)

func withQuery(query string) rigOption        { return func(o *rigOptions) { o.query = query } }
func withStreamFilterDenying() rigOption      { return func(o *rigOptions) { o.deny = true } }
func withKinds(kinds ...kind.Kind) rigOption  { return func(o *rigOptions) { o.kinds = kinds } }
func withHeartbeat(d time.Duration) rigOption { return func(o *rigOptions) { o.heartbeat = d } }

// withNoSession is a stream whose identity cannot be re-checked at all.
func withNoSession() rigOption { return func(o *rigOptions) { o.openErr = ErrNoSession } }

// actorOf is the seeded fake's owner as an authenticated actor.
func actorOf(userID string) access.Actor {
	return access.Actor{UserID: userID, RequestID: "test-request"}
}

func newRig(t *testing.T, options ...rigOption) *rig {
	t.Helper()

	chosen := rigOptions{
		actor:     actorOf(recordstest.OwnerID),
		kinds:     []kind.Kind{kind.Medication},
		heartbeat: time.Hour,
	}

	for _, option := range options {
		option(&chosen)
	}

	r := &rig{
		t:          t,
		events:     make(chan realtime.Event, realtime.SubscriberBuffer),
		frames:     make(chan frame, 64),
		authorizer: &countingAuthorizer{inner: recordstest.Authorizer{Owner: chosen.actor.UserID}},
		sessions:   &fakeSessions{openErr: chosen.openErr},
		services:   make(map[kind.Kind]*recordstest.FakeKindService),
	}

	registry := records.NewRegistry()

	for _, k := range chosen.kinds {
		service := recordstest.NewFakeKindService().ForKind(k)
		r.services[k] = service

		// A declared kind's audit target is its own; the synthetic kind has no
		// row in either vocabulary and borrows one, exactly as recordstest
		// does — the registry checks both and a fake waved past the check
		// would only prove the check can be waved past.
		registration := recordstest.Registration(k, audit.TargetKind(k.Enum()))
		if !k.Valid() {
			registration = recordstest.SyntheticRegistration()
		}

		registration.Service = service
		registration.Authorizer = r.authorizer
		registration.Stream = recordstest.StreamFilter{Deny: chosen.deny}

		if k.Valid() {
			require.NoError(t, registry.Register(registration))

			continue
		}

		require.NoError(t, registry.RegisterSynthetic(registration, recordstest.Segment, recordstest.Collection))
	}

	handler := records.NewHandler(registry)

	streams := &streams{deps: Deps{
		Resolve:   api.Resolve(func() (*records.Handler, error) { return handler, nil }),
		Hub:       fakeHub{events: r.events},
		Heartbeat: chosen.heartbeat,
		Now:       chosen.now,
		Sessions:  r.sessions,
	}}

	if streams.deps.Now == nil {
		streams.deps.Now = time.Now
	}

	var (
		mu      sync.Mutex
		failure error
	)

	r.failure = func() error {
		mu.Lock()
		defer mu.Unlock()

		return failure
	}

	// The error mapping is internal/web's own, so a refusal raised before the
	// stream opens reaches the client as the status the real edge would give
	// it — which is the whole assertion for an unregistered kind.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tracked := &trackingWriter{ResponseWriter: w}

		e := new(core.RequestEvent)
		e.Response = tracked
		e.Request = req

		err := streams.records(e, chosen.actor)
		if err == nil {
			return
		}

		mu.Lock()
		failure = err
		mu.Unlock()

		if tracked.wrote {
			return
		}

		status, _ := web.Classify(err)
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(t.Context())
	// Before the server's own cleanup, which is registered above and therefore
	// runs after this one: httptest.Server.Close waits for outstanding requests
	// and an open stream is outstanding forever.
	t.Cleanup(cancel)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+chosen.query, nil)
	require.NoError(t, err)

	response, err := server.Client().Do(request)
	require.NoError(t, err)

	t.Cleanup(func() { _ = response.Body.Close() })

	r.response = response

	if response.StatusCode == http.StatusOK {
		go readFrames(response, r.frames)
	}

	return r
}

// readFrames parses the SSE wire format: lines until a blank one.
func readFrames(response *http.Response, out chan<- frame) {
	defer close(out)

	scanner := bufio.NewScanner(response.Body)

	current := frame{}

	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case line == "":
			if current.Event != "" {
				out <- current
			}

			current = frame{}
		case strings.HasPrefix(line, "event: "):
			current.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			current.Data = append(current.Data, strings.TrimPrefix(line, "data: "))
		}
	}
}

// next waits for one frame. The timeout is a bound and never a threshold: the
// whole path is in-process, so a frame that has not arrived in a second is a
// broken pipeline rather than a slow one.
func (r *rig) next() frame {
	r.t.Helper()

	select {
	case f, open := <-r.frames:
		require.True(r.t, open, "the stream closed instead of sending a frame (handler error: %v)", r.failure())

		return f
	case <-time.After(5 * time.Second):
		r.t.Fatalf("no frame arrived (handler error: %v)", r.failure())

		return frame{}
	}
}

// silence asserts that nothing arrives before the barrier does.
//
// It takes a barrier rather than a duration because "wait and see" is the
// flaky shape: a sleep long enough never to miss a frame is long enough to be
// worth skipping, and one short enough to be quick misses the frame it was
// looking for. The barrier is an event the stream MUST deliver, so once it has
// arrived every event published before it has provably been through the loop.
func (r *rig) silenceUntil(barrier realtime.Event, want string) {
	r.t.Helper()

	r.publish(barrier)

	f := r.next()
	require.Equalf(r.t, want, f.selector(),
		"the barrier frame is not the one expected — an event that should have produced nothing produced this: %+v", f)
}

func (r *rig) publish(event realtime.Event) {
	r.t.Helper()

	select {
	case r.events <- event:
	case <-time.After(time.Second):
		r.t.Fatal("the handler is not reading its subscription")
	}
}

// trackingWriter reports whether the response has already gone, so the harness
// can tell a refusal raised before the stream opened from a failure raised
// after it — the same distinction internal/web's error middleware makes with
// e.Written().
type trackingWriter struct {
	http.ResponseWriter

	wrote bool
}

func (w *trackingWriter) WriteHeader(status int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *trackingWriter) Write(b []byte) (int, error) {
	w.wrote = true

	return w.ResponseWriter.Write(b)
}

// Unwrap is what http.ResponseController follows to reach the flusher and the
// deadline setter, exactly as PocketBase's router.ResponseWriter does.
func (w *trackingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// drainHeartbeat consumes the beat every stream opens with.
func (r *rig) drainHeartbeat() {
	r.t.Helper()

	first := r.next()
	require.Equal(r.t, "datastar-patch-signals", first.Event,
		"a stream must open with a heartbeat, or a page opened at the wrong moment compares against a signal that has never been set")
}

// fakeHub hands every subscriber the same channel. One handler per rig, so
// there is nothing to fan out.
type fakeHub struct {
	events chan realtime.Event
}

func (h fakeHub) Subscribe(context.Context) <-chan realtime.Event { return h.events }

// authCall is one consultation of the checkpoint.
type authCall struct {
	Actor  string
	Kind   kind.Kind
	Record string
	Need   access.Permission
}

// countingAuthorizer records every consultation, which is how "the handler
// re-runs the checkpoint for every event" is asserted rather than assumed.
type countingAuthorizer struct {
	inner records.Authorizer

	mu     sync.Mutex
	calls  []authCall
	fail   error
	denied map[string]bool
}

func (a *countingAuthorizer) Record(
	ctx context.Context,
	actor access.Actor,
	k kind.Kind,
	recordID string,
	need access.Permission,
) (access.Grant, error) {
	a.mu.Lock()
	a.calls = append(a.calls, authCall{Actor: actor.UserID, Kind: k, Record: recordID, Need: need})
	failure := a.fail
	refused := a.denied[recordID]
	a.mu.Unlock()

	if failure != nil {
		return access.Grant{}, failure
	}

	if refused {
		return access.Grant{}, nil
	}

	return a.inner.Record(ctx, actor, k, recordID, need)
}

// deny and allow move the checkpoint's answer while the stream is open, which
// is the only way to exercise "a subscriber who loses access mid-stream stops
// receiving patches without the stream erroring".
func (a *countingAuthorizer) deny(recordID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.denied == nil {
		a.denied = make(map[string]bool)
	}

	a.denied[recordID] = true
}

func (a *countingAuthorizer) failWith(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.fail = err
}

func (a *countingAuthorizer) consultations() []authCall {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]authCall(nil), a.calls...)
}

// fakeSessions is a revocable identity, so the in-package rig can end a session
// while a stream is open without a database, a token or a password change.
// internal/web/stream's external suite does it the real way.
type fakeSessions struct {
	openErr error

	mu    sync.Mutex
	err   error
	calls int
}

func (s *fakeSessions) Open(*core.RequestEvent) (Session, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}

	return s, nil
}

func (s *fakeSessions) Live(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls++

	return s.err
}

// end revokes the session the stream is running on. The error is what the
// production implementation returns for a token that no longer authenticates.
func (s *fakeSessions) end() {
	s.failWith(fmt.Errorf("%w: the token no longer authenticates", ErrSessionEnded))
}

// failWith is the other half: a re-check that could not be MADE, which must
// also end the stream and, unlike an ended session, must be reported.
func (s *fakeSessions) failWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.err = err
}

func (s *fakeSessions) checks() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.calls
}
