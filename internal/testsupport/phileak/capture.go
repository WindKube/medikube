package phileak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"medikube/internal/config"
	"medikube/internal/logging"
)

// The four sinks a person's data can leave the process through, named once so
// that a failure says which one and so that a suite that forgot to wire one is
// refused rather than reported as clean.
//
// They are four because each is a different mechanism with a different author.
// A log line is written deliberately; a metric label is written by a middleware
// that took a path segment for a dimension; a span attribute is written by an
// instrumentation library that thought a query string was useful context; a
// Sentry event is assembled by a framework out of whatever the request happened
// to hold. Only the first is caught by reading the code.
const (
	SinkLogs    = "the zerolog stream"
	SinkMetrics = "the Prometheus gatherer"
	SinkTraces  = "the OTel span recorder"
	SinkSentry  = "the Sentry transport"
)

// requiredSinks is what a complete capture holds. AssertNoSentinels refuses to
// report a clean result while one of them is missing, because "no sentinel was
// found" and "nothing was looked at" produce the same green tick otherwise.
var requiredSinks = []string{SinkLogs, SinkMetrics, SinkTraces, SinkSentry}

// Capture records everything the application emits while it is exercised, and
// answers one question about it: did any of these strings get out.
//
// It is the only such package in the repository, by design. A second log
// capture in internal/obs, or a phi_leak_test.go beside the code under test,
// would each assert over one sink and each look like the assertion — which is
// how a suite ends up with three partial gates and no complete one
// (cross-artifact finding M6). Phases 002-006 extend the exercise that drives
// the application; they do not add a sink of their own without adding it here.
type Capture struct {
	logs *syncBuffer

	spans    *tracetest.SpanRecorder
	provider *sdktrace.TracerProvider

	sentry *SentryTransport

	mu      sync.Mutex
	sources map[string]func() (string, error)
	order   []string
}

// New wires the three sinks this build can construct and registers cleanup for
// the tracer provider.
//
// The Prometheus gatherer is deliberately not among them and has to be handed
// over with WatchMetrics — see that method for why.
func New(t testing.TB) *Capture {
	t.Helper()

	capture := &Capture{
		logs:    new(syncBuffer),
		spans:   tracetest.NewSpanRecorder(),
		sentry:  new(SentryTransport),
		sources: make(map[string]func() (string, error), len(requiredSinks)),
	}

	capture.provider = sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(capture.spans))
	t.Cleanup(func() {
		_ = capture.provider.Shutdown(context.Background())
	})

	capture.watch(SinkLogs, func() (string, error) { return capture.logs.String(), nil })
	capture.watch(SinkTraces, func() (string, error) { return renderSpans(capture.spans.Ended()), nil })
	capture.watch(SinkSentry, capture.sentry.render)

	return capture
}

// Logger is MediKube's own logger writing into the capture rather than to
// stdout. It is built through internal/logging so the redaction, the field
// names and the zerolog object marshallers on the domain types are all on the
// path — a capture that used a bare zerolog.New would be testing a logger the
// application does not have.
//
// The level is debug: a value disclosed on a line nobody reads in production is
// still a value written to disk.
func (c *Capture) Logger() zerolog.Logger {
	return logging.NewTo(c.logs, config.LogConfig{Level: zerolog.LevelDebugValue}, "phileak")
}

// TracerProvider records every span ended while it is installed.
func (c *Capture) TracerProvider() *sdktrace.TracerProvider { return c.provider }

// Tracer is the convenience the exercise uses to open a span.
func (c *Capture) Tracer(name string) trace.Tracer { return c.provider.Tracer(name) }

// SentryTransport is a sentry.Transport that sends nothing anywhere. It is
// handed to sentry.NewClient as ClientOptions.Transport, which is the only way
// to observe an event without a DSN and a network.
func (c *Capture) SentryTransport() *SentryTransport { return c.sentry }

// WatchMetrics registers the Prometheus exposition text.
//
// It takes a rendering function rather than a prometheus.Gatherer because
// github.com/prometheus/client_golang cannot currently be imported: go.mod pins
// it as an indirect requirement but go.sum carries no entry for its own
// dependencies — client_model, common, procfs and beorn7/perks — so a build
// that imports it fails with "missing go.sum entry". Resolving that is a go.mod
// change and therefore a deliberate act, not a side effect of writing this
// file.
//
// Scanning the exposition text rather than the *dto.MetricFamily structures is
// not a workaround, though: metric names and label values are the two places a
// person's data reaches a metrics endpoint, and the exposition format is
// exactly those two things rendered. It is also what a scrape actually
// receives, which is what the requirement is about.
func (c *Capture) WatchMetrics(render func() (string, error)) {
	c.watch(SinkMetrics, render)
}

// Watch registers any further sink a later phase adds. The name appears in the
// failure, so make it the operator's word for the thing.
func (c *Capture) Watch(name string, render func() (string, error)) {
	c.watch(name, render)
}

func (c *Capture) watch(name string, render func() (string, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, seen := c.sources[name]; !seen {
		c.order = append(c.order, name)
	}

	c.sources[name] = render
}

// Sink is one recorded stream, rendered as the text a scan runs over.
type Sink struct {
	Name string
	Text string
}

// Sinks renders every registered source. A source that fails to render is
// returned as a sink whose text is the error, so a broken renderer is a finding
// rather than a silently empty stream.
func (c *Capture) Sinks() []Sink {
	c.mu.Lock()
	defer c.mu.Unlock()

	sinks := make([]Sink, 0, len(c.order))

	for _, name := range c.order {
		text, err := c.sources[name]()
		if err != nil {
			text = fmt.Sprintf("phileak: %s could not be read: %v", name, err)
		}

		sinks = append(sinks, Sink{Name: name, Text: text})
	}

	return sinks
}

// Reporter is what a failure is reported to. It is this rather than testing.TB
// because testing.TB carries an unexported method and cannot be implemented
// outside the testing package — which would leave the one assertion the whole
// suite rests on with no way to prove that it fails. capture_test.go drives it
// with a recorder and asserts each sink is caught.
//
// *testing.T and *testing.B both satisfy it.
type Reporter interface {
	Helper()
	Errorf(format string, args ...any)
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}

// Names is the registered sinks, sorted, for a test that asserts the capture is
// complete before an exercise is written against it.
func (c *Capture) Names() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	names := append([]string(nil), c.order...)
	sort.Strings(names)

	return names
}

// AssertNoSentinels is the assertion the whole PHI-leak suite comes down to.
//
// The comparison is case-insensitive because a leak that arrived through a
// header name, a URL-escaped form or an upper-cased identifier is the same
// leak, and a case-sensitive scan is how a suite reports clean over one.
//
// The sentinel is quoted in the failure. That is safe by construction and
// deliberate: these are values the fixture invented for this purpose, and a
// failure that named only the sink would leave the reader grepping a megabyte
// of JSON for something they were not told.
func (c *Capture) AssertNoSentinels(t Reporter, sentinels ...string) {
	t.Helper()

	if len(sentinels) == 0 {
		t.Fatal("phileak: no sentinels — the assertion would pass over any output at all")
	}

	c.assertEverySinkIsWatched(t)

	for _, sink := range c.Sinks() {
		haystack := strings.ToLower(sink.Text)

		for _, sentinel := range sentinels {
			if sentinel == "" {
				t.Fatal("phileak: an empty sentinel matches everything")
			}

			needle := strings.ToLower(sentinel)
			if !strings.Contains(haystack, needle) {
				continue
			}

			t.Errorf("%s carries %q:\n%s", sink.Name, sentinel, excerpt(sink.Text, needle))
		}
	}
}

// assertEverySinkIsWatched is what stops the suite passing because a sink was
// never wired. Three green sinks and one that was never registered read exactly
// like four green sinks.
func (c *Capture) assertEverySinkIsWatched(t Reporter) {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, required := range requiredSinks {
		if _, watched := c.sources[required]; !watched {
			t.Fatalf("phileak: %s is not being captured, so this suite cannot report that nothing leaked", required)
		}
	}
}

// excerpt is the neighbourhood of the first occurrence, so the reader can see
// which line leaked without printing the whole stream.
func excerpt(text, lowerNeedle string) string {
	const window = 120

	at := strings.Index(strings.ToLower(text), lowerNeedle)
	if at < 0 {
		return ""
	}

	from := max(at-window, 0)
	to := min(at+len(lowerNeedle)+window, len(text))

	return "..." + text[from:to] + "..."
}

// SentryTransport is a sentry.Transport that keeps every event and sends none.
// Sentry assembles an event out of whatever the request held, which is why it
// is one of the four sinks rather than a detail of the error reporter.
type SentryTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (s *SentryTransport) Configure(sentry.ClientOptions) {}

func (s *SentryTransport) SendEvent(event *sentry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, event)
}

func (s *SentryTransport) Flush(time.Duration) bool { return true }

func (s *SentryTransport) FlushWithContext(context.Context) bool { return true }

func (s *SentryTransport) Close() {}

// Events is every event the client tried to send.
func (s *SentryTransport) Events() []*sentry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]*sentry.Event(nil), s.events...)
}

// render is the payload Sentry would have transmitted, plus the attachments,
// which the event's own JSON form deliberately omits and which are a file's
// worth of whatever the reporter attached.
func (s *SentryTransport) render() (string, error) {
	var out strings.Builder

	for _, event := range s.Events() {
		payload, err := json.Marshal(event)
		if err != nil {
			return "", fmt.Errorf("marshalling a Sentry event: %w", err)
		}

		out.Write(payload)
		out.WriteByte('\n')

		for _, attachment := range event.Attachments {
			fmt.Fprintf(&out, "attachment %s %s\n%s\n",
				attachment.Filename, attachment.ContentType, attachment.Payload)
		}
	}

	return out.String(), nil
}

// Value.String and not Value.Emit: Emit is deprecated in otel v1.45, and for a
// string attribute — the only kind a person's data arrives in — both return the
// value unquoted.
//
// renderSpans flattens the recorded spans into the text a scan runs over: the
// name, the status description, every attribute key and value, and every span
// event with its own attributes. A span attribute is the quietest of the four
// sinks — nothing in a code review looks like logging.
func renderSpans(spans []sdktrace.ReadOnlySpan) string {
	var out strings.Builder

	for _, span := range spans {
		fmt.Fprintf(&out, "span %s status=%s %s\n",
			span.Name(), span.Status().Code, span.Status().Description)

		for _, attribute := range span.Attributes() {
			fmt.Fprintf(&out, "  %s=%s\n", attribute.Key, attribute.Value.String())
		}

		for _, event := range span.Events() {
			fmt.Fprintf(&out, "  event %s\n", event.Name)

			for _, attribute := range event.Attributes {
				fmt.Fprintf(&out, "    %s=%s\n", attribute.Key, attribute.Value.String())
			}
		}

		// The resource is process-wide and is scanned too: a service name built
		// out of a hostname is the same disclosure by a different route.
		for _, attribute := range span.Resource().Attributes() {
			fmt.Fprintf(&out, "  resource %s=%s\n", attribute.Key, attribute.Value.String())
		}
	}

	return out.String()
}

// syncBuffer is the log destination. zerolog writes one event per Write and the
// application under exercise writes from more than one goroutine, so the buffer
// is guarded rather than assumed single-threaded.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}
