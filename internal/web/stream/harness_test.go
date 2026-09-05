package stream_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/web/apitest"
)

// The external suite drives the real thing: real migrations, the seeded
// fixture, the real repository, the real authorization checkpoint, the real
// post-commit hooks and the real route table, over a real socket.
//
// A real socket and not tests.ApiScenario, and that is not a preference.
// ApiScenario drives the mux with a httptest.ResponseRecorder, which cannot be
// read while a handler is still writing to it — so a long-lived stream is
// exactly the response shape it cannot express — and which answers
// ErrNotSupported to a write deadline, so the one thing this package is most
// afraid of getting wrong would be invisible to it.

const streamPath = "/api/v1/streams/records"

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
func (f frame) signals() string  { return f.field("signals") }

func (f frame) isElementPatch() bool { return f.Event == "datastar-patch-elements" }
func (f frame) isHeartbeat() bool    { return f.Event == "datastar-patch-signals" }

// instance is one wired MediKube served over a real listener.
type instance struct {
	*apitest.Instance

	base string
}

func serve(t *testing.T, options ...apitest.Option) *instance {
	t.Helper()

	wired := apitest.New(t, options...)

	server := httptest.NewServer(testsupport.NewEdgeHandler(t, wired.App))
	// Registered before any stream's cancel, so it runs after them:
	// httptest.Server.Close waits for outstanding requests and an open stream
	// is outstanding until somebody cancels it.
	t.Cleanup(server.Close)

	return &instance{Instance: wired, base: server.URL}
}

func (i *instance) token(t *testing.T, email string) string {
	t.Helper()

	return testsupport.UserToken(t, i.App, email)
}

// session is one open stream.
type session struct {
	t *testing.T

	Response *http.Response
	frames   chan frame
}

// open subscribes. It returns after the response head has arrived, so a caller
// can assert on the status of a refusal as well as on the frames of a stream.
func (i *instance) open(t *testing.T, token, query string) *session {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, i.base+streamPath+query, nil)
	require.NoError(t, err)

	request.Header.Set("Accept", "text/event-stream")

	if token != "" {
		request.Header.Set("Authorization", token)
	}

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	t.Cleanup(func() { _ = response.Body.Close() })

	s := &session{t: t, Response: response, frames: make(chan frame, 256)}

	if response.StatusCode == http.StatusOK {
		go readFrames(response.Body, s.frames)
	}

	return s
}

func readFrames(body io.Reader, out chan<- frame) {
	defer close(out)

	scanner := bufio.NewScanner(body)

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

// next waits for the next frame.
//
// The bound is generous and is a bound, never a threshold. The whole path is
// in-process — a channel, a map, a re-fetch against an embedded SQLite — so a
// frame that has not arrived in this long is a broken pipeline and not a slow
// one, and asserting on the elapsed time instead would be the flaky gate
// Constitution VIII forbids.
func (s *session) next(within time.Duration) frame {
	s.t.Helper()

	select {
	case f, open := <-s.frames:
		require.True(s.t, open, "the stream closed instead of sending a frame")

		return f
	case <-time.After(within):
		s.t.Fatalf("no frame arrived within %s", within)

		return frame{}
	}
}

// nextPatch skips heartbeats, which arrive on their own schedule and are not
// what an element assertion is about.
func (s *session) nextPatch(within time.Duration) frame {
	s.t.Helper()

	deadline := time.Now().Add(within)

	for {
		remaining := time.Until(deadline)
		require.Positive(s.t, remaining, "no element patch arrived within %s", within)

		if f := s.next(remaining); f.isElementPatch() {
			return f
		}
	}
}

// drained returns every frame delivered so far without waiting for another.
func (s *session) drained() []frame {
	s.t.Helper()

	var collected []frame

	for {
		select {
		case f, open := <-s.frames:
			if !open {
				return collected
			}

			collected = append(collected, f)
		default:
			return collected
		}
	}
}

// elementPatches is drained, minus the heartbeats.
func (s *session) elementPatches() []frame {
	s.t.Helper()

	var patches []frame

	for _, f := range s.drained() {
		if f.isElementPatch() {
			patches = append(patches, f)
		}
	}

	return patches
}

// create writes one medication through the API, which is the path a person
// takes and therefore the one that has to reach a stream.
func (i *instance) create(t *testing.T, token, patientID, name string) (string, string) {
	t.Helper()

	body := strings.NewReader(`{"patient":"` + patientID + `","name":"` + name + `","status":"active"}`)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		i.base+"/api/v1/records/"+kind.Medication.Segment(), body)
	require.NoError(t, err)

	request.Header.Set("Authorization", token)
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusCreated, response.StatusCode, "creating %q: %s", name, raw)

	var created struct {
		ID string `json:"id"`
	}

	require.NoError(t, json.Unmarshal(raw, &created))
	require.NotEmpty(t, created.ID)

	return created.ID, response.Header.Get("ETag")
}

// remove deletes one medication through the API, If-Match and all.
func (i *instance) remove(t *testing.T, token, id, etag string) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodDelete,
		i.base+"/api/v1/records/"+kind.Medication.Segment()+"/"+id, nil)
	require.NoError(t, err)

	request.Header.Set("Authorization", token)
	request.Header.Set("If-Match", etag)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	defer func() { _ = response.Body.Close() }()

	raw, _ := io.ReadAll(response.Body)
	require.Equalf(t, http.StatusNoContent, response.StatusCode, "deleting %s: %s", id, raw)
}

// fastHeartbeat is what every test that is not about the heartbeat's interval
// uses: production's 25 seconds is longer than a test suite should take.
func fastHeartbeat() apitest.Option {
	return apitestHeartbeat(50 * time.Millisecond)
}

func apitestHeartbeat(interval time.Duration) apitest.Option {
	return apitest.WithStreamHeartbeat(interval)
}
