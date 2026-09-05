//go:build phileak

// exercise_test.go is the half of the PHI-leak suite that drives the
// application. capture.go holds the four sinks and the one assertion; this file
// boots a real MediKube, plants distinctive values in it through its own public
// interface, and walks every route the inventory declares.
//
// Phases 002-006 extend THIS file — a sentinel, a step, a route — and never the
// assertion. The assertion is one line and adding to it is how a suite acquires
// a second opinion about what counts as a leak.
//
// WHY IT IS A _test.go FILE, and why moving it back would break the build.
//
// tasks.md calls it exercise.go. It cannot be one. sole_test.go enforces
// cross-artifact finding M6 — there is ONE PHI-leak harness in this repository
// — which is why internal/obs/db_test.go asserts over the span sink by
// importing this package rather than by capturing spans itself. That test is
// `package obs`, so it is part of internal/obs's own build, and it pulls in
// every NON-TEST file of this package.
//
// This file reaches internal/obs twice over: directly, for the request logger,
// the metrics registry, the reporter and the instrumented database connection,
// and again through internal/web/apitest, which assembles the instance. As a
// non-test file it therefore closed the loop — internal/obs's test imports
// phileak, phileak imports internal/obs — and `go vet -tags=phileak ./...` and
// `golangci-lint run ./...` both failed with "import cycle not allowed in
// test". A _test.go file is not part of the importable package, so the loop
// does not exist.
//
// The constraint this expresses is worth stating on its own, because nothing
// else in the repository does: THE IMPORTABLE SURFACE OF THIS PACKAGE MUST NOT
// REACH internal/obs. capture.go keeps to it — its heaviest import is
// internal/logging — and anything added beside it has to.

package phileak

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/obs"
	"medikube/internal/platform/pb"
	"medikube/internal/testsupport"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/apitest"
	"medikube/internal/web/views/shell"
)

// The sentinels: values that exist nowhere in MediKube, are written into the
// instance through its own interface, and are then looked for in everything the
// process emits.
//
// They are deliberately unlike each other and deliberately not words. A
// sentinel that could plausibly be part of a route, a status, a header name or
// a Go identifier turns a leak into an argument about whether the match was
// real, and a suite whose failures have to be argued about is one that gets
// muted.
//
// What they must NOT be is anything the application legitimately echoes. Two in
// particular are absent by decision rather than by oversight, and the reason is
// recorded at unsentinelled below.
const (
	MedicationName  = "Zyxenvalotide"
	AlternativeName = "Qorbaxil-Retard"
	Dosage          = "437.25mg"
	Frequency       = "thrice-weekly-at-dusk"
	Indication      = "hereditary-angioedema-prophylaxis"
	SideEffects     = "vertigo-on-waking"
	Notes           = "she-asked-that-her-brother-not-be-told"
	SearchTerm      = "narrowing-by-a-typed-in-word"
	AccountEmail    = "marisol.vandeleur@example.invalid"
	AccountName     = "Marisol Vandeleur"
	//nolint:gosec // a fabricated credential is the whole point of this constant
	AccountPassword = "trombiculid-wallaby-8823-Q"
	//nolint:gosec // as above
	NewAccountPasswrd = "sternocleidomastoid-6471-Z"
	CookieCrumb       = "a-crumb-a-browser-carried"
	HeaderCrumb       = "a-value-a-proxy-added"
	BodyCrumb         = "a-member-no-operation-declares"
	PatientFirstName  = "Oluwaseun-Ekaterina"
	PatientLastName   = "Mbeki-Thorvaldsen"
	PatientAddress    = "14-asylum-lane-flat-9"
	PhotoFilename     = "portrait-in-a-hospital-gown.png"
)

// Sentinel is one planted value and what it stands for. The meaning is printed
// beside a failure, because "Qorbaxil-Retard reached the log stream" is a
// puzzle and "the alternative name of a drug reached the log stream" is a bug
// report.
type Sentinel struct {
	Value   string
	Meaning string
}

// Sentinels is the whole planted set. A later phase adds to it here and to the
// step that plants it below; nothing else changes.
func Sentinels() []Sentinel {
	return []Sentinel{
		{MedicationName, "the name of a drug, which discloses a condition"},
		{AlternativeName, "the brand a person recorded instead of the generic"},
		{Dosage, "how much of it they take"},
		{Frequency, "how often they take it"},
		{Indication, "the reason, which names a condition outright"},
		{SideEffects, "what it does to them"},
		{Notes, "whatever they chose to write down"},
		{SearchTerm, "a word typed into a query string"},
		{AccountEmail, "an address, which identifies the person"},
		{AccountName, "the name they are called by"},
		{AccountPassword, "a password"},
		{NewAccountPasswrd, "the password they changed it to"},
		{CookieCrumb, "a value carried in a cookie"},
		{HeaderCrumb, "a value added by something in front of the process"},
		{BodyCrumb, "a member of a request body no operation declares"},
		{PatientFirstName, "the first name of a person whose health is recorded"},
		{PatientLastName, "their family name"},
		{PatientAddress, "where they live"},
		{PhotoFilename, "the name of the file their photograph came in as"},
	}
}

// SentinelValues is what AssertNoSentinels is handed.
func SentinelValues() []string {
	values := make([]string, 0, len(Sentinels()))
	for _, s := range Sentinels() {
		values = append(values, s.Value)
	}

	return values
}

// Unsentinelled records what is deliberately NOT a sentinel, so that a later
// author who wonders why finds the answer rather than the omission.
//
// Both are values MediKube puts in a URL PATH, and internal/obs's request
// logger records the path of every request by design (it records no query
// string, no header, no cookie and no body). Making either a sentinel would
// turn a published design decision into a red suite that the owner of this file
// cannot fix, which is a worse outcome than writing down that the decision
// exists. It is reported rather than asserted.
//
// Unsentinelled is keyed by the value and carries the reason. It is read by
// phileak_test.go, so an entry that stops being true fails rather than sitting
// here as a paragraph nobody re-reads.
func Unsentinelled() map[string]string {
	return map[string]string{
		staleRecoveryToken: "a recovery token, carried in the PATH of /reset-password/{token}, which internal/obs's request logger " +
			"records for every request by design (it records no query string, no header, no cookie and no body). " +
			"Making it a sentinel would turn a published design decision into a red suite the owner of this file " +
			"cannot fix. Reported instead: FR-054 and FR-038 disagree here and somebody owns that (T235).",
		staleConfirmationToken: "a confirmation token in /verify-email/{token} — the same path, the same decision (T235).",
	}
}

// The values that stand in for a token in the two token-bearing page routes.
// They are what contracts/pages.md's smoke URLs use and are expired by
// construction, so the pages answer their ask-again state.
const (
	staleRecoveryToken     = "no-longer-usable-recovery-token"
	staleConfirmationToken = "no-longer-usable-confirmation-token"
)

// The two places a value reaches the process that no operation reads: a header
// something in front of the process added, and a cookie a browser carried.
// Neither is declared anywhere in MediKube, which is what makes them a clean
// probe — anything that records either records everything.
const (
	crumbHeader = "X-Phileak-Crumb"
	crumbCookie = "phileak_crumb"
)

// missingRecordID is an identifier of the right shape that no fixture uses. It
// is what a genuine miss is produced with.
const missingRecordID = "mkmednobody0001"

// notFoundPath produces the 404 error view. It is not a route, which is the
// point of it.
const notFoundPath = "/no-route-answers-this"

// release and environment are what the two exporters stamp on everything they
// send. Neither is derived from the machine: a hostname or a working directory
// reaching a third party is the same disclosure by a duller route.
const (
	release     = "phileak"
	environment = "phileak"
)

// Result is one completed exercise: the capture to assert over, and the
// measurements that stop the assertion passing over nothing.
type Result struct {
	Capture *Capture

	// Patterns is every ServeMux pattern the application actually matched,
	// counted. It is read from the request inside the process and never
	// declared by the caller — a hand-written list of what was driven is the
	// list that says three while nine are registered.
	Patterns map[string]int

	// Paths is every concrete method-and-path that was requested. It is what
	// answers for the PocketBase-native routes, whose registered pattern is
	// parameterised where the inventory records a literal.
	Paths map[string]int

	// Requests is how many requests reached the router.
	Requests int

	// Faults is how many of them the error middleware recorded an occurrence
	// for, and Reported how many of those the Sentry client accepted.
	Faults   int
	Reported int

	// Exports is how many times the OTLP exporter sent, and Envelopes how many
	// times the Sentry client did.
	Exports   int
	Envelopes int

	// Submitted is every request the exercise sent — line, headers, cookies and
	// body — concatenated. It is what proves a sentinel REACHED the process:
	// a sentinel no request ever carried is one whose absence from every sink
	// is a fact about the exercise rather than about MediKube.
	Submitted string

	// Echoed is every response body the exercise received, concatenated.
	//
	// It is what proves a sentinel was PLANTED. "No sink carries the drug
	// name" is true of a fixture that never recorded one, and that reading is
	// indistinguishable from a clean run unless something asserts the value
	// really is in the instance. A value the application hands back is a value
	// the application holds.
	Echoed string
}

// Run boots one instance, drives it, flushes both exporters and hands back
// everything the assertions need.
//
// It is one function rather than a builder because there is exactly one
// arrangement worth running: every sink wired, every route walked. A knob here
// would be a way to run the suite with a sink switched off.
func Run(t testing.TB) *Result {
	t.Helper()

	capture := New(t)

	// The trace sink. See traceTrap for why it is the exporter's wire payload
	// rather than the capture's own span recorder.
	traces := newTraceTrap(t)
	capture.Watch(SinkTraces, traces.render)

	tracing, err := obs.StartTracing(t.Context(), config.OTelConfig{
		Enabled:     true,
		Endpoint:    traces.address(),
		Insecure:    true,
		SampleRatio: 1,
		Environment: environment,
	}, release, capture.Logger())
	require.NoError(t, err, "starting tracing")
	require.True(t, tracing.Active(),
		"tracing is inactive, so no query would be instrumented and the trace sink would be empty")

	// The Sentry sink, for the same reason: startSentry's transport seam is
	// unexported, so the only way to observe the real client — real options,
	// real scrubber, real transport — is to give it a destination.
	events := newSentryTrap(t)
	capture.Watch(SinkSentry, events.render)

	reporter, err := obs.StartSentry(config.SentryConfig{
		DSN:         events.dsn(),
		Environment: environment,
		SampleRate:  1,
	}, release, capture.Logger())
	require.NoError(t, err, "starting the error reporter")
	require.True(t, reporter.Active(),
		"the reporter is inactive, so nothing would be assembled and the Sentry sink would be empty")

	// The measurements. The published set is the inventory's own patterns, so
	// the allowlist and the routes cannot drift apart.
	metrics := obs.NewMetrics(publishedPatterns()...)
	capture.WatchMetrics(metrics.Registry())

	app := newInstrumentedApp(t, tracing)

	instance, err := apitest.Wire(app,
		// FR-002's operator switch, opened so that the sign-up operation is a
		// route this suite walks rather than a route it skips.
		apitest.WithRegistrationOpen(true),
		// The production heartbeat is 25 seconds and this suite holds a stream
		// open for a fraction of that.
		apitest.WithStreamHeartbeat(40*time.Millisecond),
	)
	require.NoError(t, err, "wiring a MediKube instance")

	watcher := &watcher{patterns: map[string]int{}, paths: map[string]int{}}
	bindCapture(app, capture, metrics, reporter, watcher)

	server := httptest.NewServer(testsupport.NewEdgeHandler(t, app))
	t.Cleanup(server.Close)

	client := &client{t: t, base: server.URL, instance: instance}

	drive(t, client)

	// Both exporters batch. Without the flush the suite would scan whatever
	// happened to have been sent already, which on a fast machine is nothing.
	flushCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 15*time.Second)
	defer cancel()

	require.NoError(t, tracing.Shutdown(flushCtx), "flushing the tracer provider")
	require.NoError(t, reporter.Shutdown(flushCtx), "flushing the error reporter")

	return &Result{
		Capture:   capture,
		Patterns:  watcher.snapshot(watcher.patterns),
		Paths:     watcher.snapshot(watcher.paths),
		Requests:  watcher.requests(),
		Faults:    watcher.faultCount(),
		Reported:  watcher.reportedCount(),
		Exports:   traces.count(),
		Envelopes: events.count(),
		Submitted: client.submitted(),
		Echoed:    client.echoed(),
	}
}

// newInstrumentedApp is testsupport.NewApp with one addition it has no seam
// for: the database is opened through internal/obs's instrumented connect
// function, so every query the exercise causes produces a real span from the
// real instrumentation.
//
// Assembling the app here rather than asking internal/testsupport for it is a
// deliberate, narrow duplication: NewApp takes no core.BaseAppConfig and the
// whole value of the trace sink is that the spans come from production code
// rather than from a span this fixture opened by hand.
func newInstrumentedApp(t testing.TB, tracing *obs.Tracing) *tests.TestApp {
	t.Helper()

	connect := obs.InstrumentedDBConnect(tracing)
	require.NotNil(t, connect,
		"InstrumentedDBConnect answered nil, so PocketBase would open the database itself and nothing would be traced")

	app, err := tests.NewTestAppWithConfig(core.BaseAppConfig{
		DataDir: testsupport.FixtureDir(),
		// tests.NewTestApp's own value. It names the environment variable the
		// settings encryption key is read from; a different one here would
		// make the cloned fixture unreadable.
		EncryptionEnv: "pb_test_env",
		DBConnect:     connect,
	})
	require.NoError(t, err, "cloning the fixture at %s", testsupport.FixtureDir())

	t.Cleanup(app.Cleanup)

	return app
}

// captureServeHookID names this suite's OnServe handler.
const captureServeHookID = "phileakCapture"

// observerID names the middleware that counts what the application served.
const observerID = "phileakObserver"

// bindCapture points the application's own diagnostics at the capture.
//
// The request logger is REPLACED rather than added to: internal/web/apitest
// binds it with zerolog.Nop(), and a second logger beside it would mean one
// request wrote two lines under two correlation ids.
//
// The replacement is a plain Bind and MUST NOT be an Unbind followed by a Bind,
// which is what this was first written as. RouterGroup.Unbind removes the
// middleware from the group, recurses into every child route removing it there
// too, and then adds the id to an EXCLUDE LIST on each of them
// (tools/router/group.go:78-108). Router.BuildMux consults that list per route
// (tools/router/router.go:115-121), and RouterGroup.Bind clears the exclusion
// only on the group it is called on — so re-binding on the root leaves every
// registered route still excluding it. The measured symptom was 64 requests and
// 2 log lines, both from the mux catch-all BuildMux registers after the exclude
// lists were written. A plain Bind works because BuildMux replays the group's
// middlewares through hook.Bind, which replaces by id (tools/hook/hook.go:82-90)
// and therefore keeps the last one bound.
//
// The other two are wired here because nothing in the shipped composition root
// calls them yet: Metrics.ObserveRequest has no call site and Reporter.Report
// has no call site (both are noted as unwired in the composition root's own
// report). This suite therefore stands in for that wiring, at the same
// position and with the same inputs the middleware will use. When those call
// sites land, this handler shrinks and the assertion does not move.
func bindCapture(app core.App, capture *Capture, metrics *obs.Metrics, reporter *obs.Reporter, w *watcher) {
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Id: captureServeHookID,
		// After MediKube's own binding, which is what puts the Nop logger and
		// every middleware on the router in the first place.
		Priority: pb.ServeHookPriority + 100,
		Func: func(se *core.ServeEvent) error {
			se.Router.Bind(obs.RequestLogger(capture.Logger()))
			se.Router.Bind(observer(metrics, reporter, w))

			return se.Next()
		},
	})
}

// observer is the measurement and error-reporting middleware the edge will
// eventually own.
//
// It sits one step outside the request logger, so that what it sees — the
// status, the recorded occurrence — is exactly what the one log line for the
// request reports.
func observer(metrics *obs.Metrics, reporter *obs.Reporter, w *watcher) *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id:       observerID,
		Priority: apis.DefaultActivityLoggerMiddlewarePriority - 11,
		Func: func(e *core.RequestEvent) error {
			start := time.Now()

			err := e.Next()

			// The error middleware answers the client and returns nil, so the
			// occurrence it recorded is what there is to report.
			fault := err
			if fault == nil {
				fault = obs.Fault(e)
			}

			status := e.Status()
			if status == 0 {
				status = http.StatusInternalServerError
			}

			// The registered pattern and never the resolved path — that
			// distinction is the whole of FR-055's label clause.
			pattern := e.Request.Pattern
			if pattern == "" {
				pattern = e.Request.Method + " " + e.Request.URL.RequestURI()
			}

			took := time.Since(start)

			metrics.ObserveRequest(pattern, e.Request.Method, status, took)

			// And again with the RESOLVED ADDRESS, query string and all.
			//
			// This second observation is a probe and not a mistake. Passing
			// the pattern alone would make a clean metrics sink true by
			// construction: a registered pattern has a record id nowhere in it
			// and a search term nowhere in it, so no scan of the exposition
			// could ever find one, and the sink would be reported clean having
			// been offered nothing it could get wrong.
			//
			// What is handed over here is exactly what a middleware that took
			// the request rather than the route would pass — the mistake
			// FR-055's allowlist exists to survive — so the clean result
			// becomes a statement about internal/obs's allowlist rather than
			// about this file's good manners. Both label values are then in
			// the exposition and sinkEvidence requires both.
			metrics.ObserveRequest(e.Request.Method+" "+e.Request.URL.RequestURI(), e.Request.Method, status, took)

			// Every occurrence, not only the five hundreds the shipped call
			// site will report. The question this sink answers is whether any
			// error message the application CONSTRUCTS carries a person's
			// data, and which status that message was answered with is a
			// routing decision that will change — so the reporter is handed
			// the raw fault and the withholding is its own to get right.
			reported := false
			if fault != nil {
				reported = reporter.Report(e, fault)
			}

			w.record(e.Request.Pattern, e.Request.Method+" "+e.Request.URL.Path, fault != nil, reported)

			return err
		},
	}
}

// watcher counts what the application served, from inside the application.
type watcher struct {
	mu       sync.Mutex
	patterns map[string]int
	paths    map[string]int
	total    int
	faults   int
	reported int
}

func (w *watcher) record(pattern, path string, faulted, reported bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.total++

	if pattern != "" {
		w.patterns[pattern]++
	}

	w.paths[path]++

	if faulted {
		w.faults++
	}

	if reported {
		w.reported++
	}
}

func (w *watcher) snapshot(from map[string]int) map[string]int {
	w.mu.Lock()
	defer w.mu.Unlock()

	out := make(map[string]int, len(from))
	for key, count := range from {
		out[key] = count
	}

	return out
}

func (w *watcher) requests() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.total
}

func (w *watcher) faultCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.faults
}

func (w *watcher) reportedCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.reported
}

// publishedPatterns is every pattern MediKube serves, which is what may become
// a metric label. The external rows are left out because PocketBase serves them
// under its own parameterised patterns and a label for a route MediKube does
// not register would be a published set nobody published.
func publishedPatterns() []string {
	routes := httproute.Inventory().Routes()
	patterns := make([]string, 0, len(routes))

	for _, route := range routes {
		if route.Kind == httproute.KindExternal {
			continue
		}

		patterns = append(patterns, route.Pattern())
	}

	return patterns
}

// traceTrap is the OTLP destination, and it is what SinkTraces is registered
// with instead of the capture's own span recorder.
//
// That substitution is deliberate and is the one place this file departs from
// capture.go's default arrangement, so here is the reason in full. The only
// production code in this build that writes a span is otelsql, wired by
// obs.InstrumentedDBConnect, which takes an *obs.Tracing — a struct whose
// provider is unexported and which only obs.StartTracing constructs, and
// StartTracing builds a provider around an OTLP exporter. There is therefore no
// exported seam through which production code can be pointed at
// Capture.TracerProvider(). Handing the recorder to a span this fixture opened
// by hand would capture attributes this fixture wrote, which is a test of the
// fixture.
//
// What is captured instead is strictly stronger: the bytes the exporter puts on
// the wire. A span attribute that never leaves the process is not a
// disclosure; these are the ones that do. The payload is protobuf, and protobuf
// carries strings as literal UTF-8, so a substring scan over it finds exactly
// what a person reading the trace at the far end would see.
//
// RECOMMENDED FOLLOW-UP, not mine to write: an exported
// obs.TracingWith(trace.TracerProvider) would let this be the span recorder as
// capture.go intended, and would cost internal/obs three lines.
type traceTrap struct {
	server *httptest.Server

	mu       sync.Mutex
	payloads [][]byte
}

func newTraceTrap(t testing.TB) *traceTrap {
	t.Helper()

	trap := new(traceTrap)

	trap.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		trap.mu.Lock()
		trap.payloads = append(trap.payloads, body)
		trap.mu.Unlock()

		// An empty 200 is an empty ExportTraceServiceResponse, which is what a
		// collector that accepted everything answers. Anything else would make
		// the exporter retry and fill the log sink with its own complaints.
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))

	t.Cleanup(trap.server.Close)

	return trap
}

// address is the host:port form otlptracehttp.WithEndpoint wants.
func (o *traceTrap) address() string { return strings.TrimPrefix(o.server.URL, "http://") }

func (o *traceTrap) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()

	return len(o.payloads)
}

func (o *traceTrap) render() (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	var out strings.Builder

	fmt.Fprintf(&out, "OTLP/HTTP trace exports: %d\n", len(o.payloads))

	for _, payload := range o.payloads {
		out.WriteString(printable(payload))
		out.WriteByte('\n')
	}

	return out.String(), nil
}

// sentryTrap is the error-reporting destination. sentry-go's HTTP transport
// sends the envelope uncompressed, so what arrives here is the JSON a person at
// the far end would read.
type sentryTrap struct {
	server *httptest.Server

	mu        sync.Mutex
	envelopes []string
}

func newSentryTrap(t testing.TB) *sentryTrap {
	t.Helper()

	trap := new(sentryTrap)

	trap.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		trap.mu.Lock()
		trap.envelopes = append(trap.envelopes, printable(body))
		trap.mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))

	t.Cleanup(trap.server.Close)

	return trap
}

// dsn is a well-formed Sentry DSN pointing at this listener. The public key is
// arbitrary and the project id is 1.
func (s *sentryTrap) dsn() string {
	return "http://0123456789abcdef0123456789abcdef@" +
		strings.TrimPrefix(s.server.URL, "http://") + "/1"
}

func (s *sentryTrap) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.envelopes)
}

func (s *sentryTrap) render() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out strings.Builder

	fmt.Fprintf(&out, "Sentry envelopes: %d\n", len(s.envelopes))

	for _, envelope := range s.envelopes {
		out.WriteString(envelope)
		out.WriteByte('\n')
	}

	return out.String(), nil
}

// printable makes a binary payload readable without making it lie: every byte
// that is not printable ASCII becomes a dot, and every byte that is survives
// unchanged. A sentinel is printable ASCII, so the scan is unaffected and the
// excerpt beside a failure is something a person can read.
func printable(raw []byte) string {
	out := make([]byte, len(raw))

	for i, b := range raw {
		if b == '\n' || (b >= 0x20 && b < 0x7f) {
			out[i] = b

			continue
		}

		out[i] = '.'
	}

	return string(out)
}

// client drives one instance over a real listener.
//
// A real socket rather than the handler in-process, and not for realism's sake:
// http.Request.Pattern is filled in by net/http's ServeMux when it dispatches,
// and the pattern is what the metric label and the coverage measurement are
// both read from.
type client struct {
	t        testing.TB
	base     string
	instance *apitest.Instance

	// bearer is the API caller's credential; cookie is the browser's. The two
	// reach the actor through different middleware, so a page driven with a
	// bearer token would be exercising a path no browser takes.
	bearer string
	cookie string

	mu       sync.Mutex
	bodies   strings.Builder
	requests strings.Builder
}

// submitted is every request this client sent, whole.
func (c *client) submitted() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.requests.String()
}

// echoed is every response body this client received.
func (c *client) echoed() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.bodies.String()
}

// answer is one response, read whole.
type answer struct {
	Status int
	Header http.Header
	Body   string
}

func (c *client) do(method, path, body string) *answer {
	c.t.Helper()

	return c.doWith(method, path, body, nil)
}

// doWith is do plus headers this one request needs — If-Match, an Accept, a
// second correlation id.
func (c *client) doWith(method, path, body string, headers map[string]string) *answer {
	c.t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	request, err := http.NewRequestWithContext(c.t.Context(), method, c.base+path, reader)
	require.NoError(c.t, err, "%s %s", method, path)

	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}

	// On every request, always. A header and a cookie are two of the four
	// places a value reaches the process, and the sinks that would record
	// either do it for every request or for none.
	request.Header.Set(crumbHeader, HeaderCrumb)
	// The attributes gosec asks for are a SERVER's to set on a Set-Cookie. This
	// is a client putting a cookie on a request it is sending, where they have
	// no meaning: net/http serialises Name and Value and drops the rest.
	request.AddCookie(&http.Cookie{Name: crumbCookie, Value: CookieCrumb}) //nolint:gosec // a client-side request cookie

	if c.bearer != "" {
		request.Header.Set("Authorization", c.bearer)
	}

	if c.cookie != "" {
		request.AddCookie(&http.Cookie{Name: web.SessionCookieName, Value: c.cookie}) //nolint:gosec // as above
	}

	for name, value := range headers {
		request.Header.Set(name, value)
	}

	c.remember(request, body)

	response, err := http.DefaultClient.Do(request)
	require.NoError(c.t, err, "%s %s", method, path)

	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(response.Body)
	require.NoError(c.t, err, "reading the body of %s %s", method, path)

	c.mu.Lock()
	c.bodies.Write(raw)
	c.bodies.WriteByte('\n')
	c.mu.Unlock()

	return &answer{Status: response.StatusCode, Header: response.Header, Body: string(raw)}
}

// remember records what went out, so that the planted-ness of every sentinel is
// a measurement rather than a belief about what this file does.
func (c *client) remember(request *http.Request, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Fprintf(&c.requests, "%s %s\n", request.Method, request.URL.RequestURI())

	for name, values := range request.Header {
		fmt.Fprintf(&c.requests, "  %s: %s\n", name, strings.Join(values, ", "))
	}

	if body != "" {
		fmt.Fprintf(&c.requests, "  %s\n", body)
	}
}

// as signs the client in through the operation a browser and a client both use,
// and keeps both credentials so that pages and API operations are each driven
// the way they are actually reached.
func (c *client) as(email, password string) {
	c.t.Helper()

	answered := c.do(http.MethodPost, "/api/v1/auth/login",
		fmt.Sprintf(`{"email":%q,"password":%q}`, email, password))
	require.Equal(c.t, http.StatusOK, answered.Status, "signing in as %s: %s", email, answered.Body)

	for _, cookie := range (&http.Response{Header: answered.Header}).Cookies() {
		if cookie.Name == web.SessionCookieName {
			c.cookie = cookie.Value
			c.bearer = cookie.Value

			return
		}
	}

	require.Fail(c.t, "the sign-in answered no session cookie", "%s", answered.Body)
}

// token mints a credential for a seeded account without going through sign-in,
// for the accounts whose journey is not what is being driven.
func (c *client) token(email string) {
	c.t.Helper()

	minted := testsupport.UserToken(c.t, c.instance.App, email)
	c.bearer = minted
	c.cookie = minted
}

func (c *client) anonymous() {
	c.bearer = ""
	c.cookie = ""
}

// recordsOfKind is the collection address, composed from the kind table. The
// plural is declared once, in internal/domain/kind.
func recordsOfKind() string { return "/api/v1/records/" + kind.Medication.Segment() }

func recordAddress(id string) string { return recordsOfKind() + "/" + id }

// sentinelPatientFor plants one minimal patient for the given account,
// directly against the database: the registration-time hook that provisions a
// self-record automatically (FR-005) is a different story's work, so an
// account minted in this exercise has none, and a medication sentinel needs a
// patient to be filed against.
func sentinelPatientFor(c *client, email string) string {
	c.t.Helper()

	owner, err := c.instance.App.FindAuthRecordByEmail("users", email)
	require.NoError(c.t, err, "finding the sentinel account %s", email)

	collection, err := c.instance.App.FindCollectionByNameOrId("patients")
	require.NoError(c.t, err)

	record := core.NewRecord(collection)
	record.Set("owner", owner.Id)
	record.Set("first_name", "Sentinel")
	record.Set("last_name", "Patient")
	require.NoError(c.t, c.instance.App.Save(record), "planting the sentinel patient for %s", email)

	return record.Id
}

func pageOfKind() string { return "/" + kind.Medication.Segment() }

// drive walks the whole inventory.
//
// The order is a person's, not a route table's: an account is created before it
// is signed into, a record is written before it is read, and the account is
// deleted last — which is also the only order in which the account deletion
// exercises PocketBase's reference cascade over a trail that has something in
// it (research D-22).
func drive(t testing.TB, c *client) {
	t.Helper()

	driveHealth(c)
	drivePublicAuth(c)
	driveRecords(c)
	drivePatients(c)
	drivePages(c)
	driveNativePaths(c)
	driveAccountLifecycle(c)
}

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

// drivePatients: the seven patient operations and both patient pages, with a
// person's names, address and photograph filename as the sentinels.
func drivePatients(c *client) {
	c.token(testsupport.AccountAEmail)

	created := c.do(http.MethodPost, "/api/v1/patients", jsonBody(c.t, api.PatientCreate{
		FirstName: PatientFirstName, LastName: PatientLastName, BirthDate: "1990-01-01", Address: PatientAddress,
	}))
	require.Equal(c.t, http.StatusCreated, created.Status, "the sentinel patient was not created: %s", created.Body)

	var patient struct {
		ID string `json:"id"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(created.Body), &patient))

	address := "/api/v1/patients/" + patient.ID

	c.do(http.MethodGet, "/api/v1/patients", "")
	c.do(http.MethodGet, address, "")
	c.do(http.MethodPatch, address, jsonBody(c.t, api.PatientPatch{Address: ptr(PatientAddress)}))
	c.do(http.MethodGet, "/api/v1/patients/"+missingRecordID, "")

	body, contentType := photoUpload(c.t)
	c.doWith(http.MethodPut, address+"/photo", body, map[string]string{"Content-Type": contentType})
	c.do(http.MethodGet, address+"/photo", "")
	c.do(http.MethodGet, address+"/photo?size=original", "")
	c.do(http.MethodDelete, address+"/photo", "")

	c.bearer = ""
	c.do(http.MethodGet, "/patients", "")
	c.do(http.MethodGet, "/patients/"+patient.ID, "")
	c.do(http.MethodGet, "/patients/"+missingRecordID, "")
}

func photoUpload(t testing.TB) (string, string) {
	t.Helper()

	raw, err := base64.StdEncoding.DecodeString(onePixelPNG)
	require.NoError(t, err)

	var buffer bytes.Buffer
	form := multipart.NewWriter(&buffer)
	part, err := form.CreateFormFile("photo", PhotoFilename)
	require.NoError(t, err)
	_, err = part.Write(raw)
	require.NoError(t, err)
	require.NoError(t, form.Close())

	return buffer.String(), form.FormDataContentType()
}

// driveHealth: both operations an operator has, both deliberately incurious.
func driveHealth(c *client) {
	c.anonymous()
	c.do(http.MethodGet, "/api/v1/healthz", "")
	c.do(http.MethodGet, "/api/v1/readyz", "")
}

// drivePublicAuth: everything reachable without a session, including the two
// token operations, which are driven with a token that is expired by
// construction because that is the most likely real visit and because a minted
// one would be test-only code in a security path.
func drivePublicAuth(c *client) {
	c.anonymous()

	c.do(http.MethodGet, "/api/v1/auth/config", "")

	// The recovery request, with an address that has an account and one that
	// does not. Both must answer identically (FR-073), and the pair is what
	// gives the anti-enumeration path a sentinel to disclose.
	c.do(http.MethodPost, "/api/v1/auth/password-reset",
		jsonBody(c.t, api.PasswordResetRequest{Email: AccountEmail}))
	c.do(http.MethodPost, "/api/v1/auth/password-reset",
		jsonBody(c.t, api.PasswordResetRequest{Email: testsupport.AccountAEmail}))

	c.do(http.MethodPost, "/api/v1/auth/password-reset/confirm",
		jsonBody(c.t, api.PasswordResetConfirm{
			Token:           staleRecoveryToken,
			Password:        AccountPassword,
			PasswordConfirm: AccountPassword,
		}))

	c.do(http.MethodPost, "/api/v1/auth/verify-email/confirm",
		jsonBody(c.t, api.EmailVerificationConfirm{Token: staleConfirmationToken}))

	// A sign-in that fails, with a password that is a sentinel. A failed
	// authentication is the single most likely place for a credential to be
	// written down beside the address it was tried against.
	c.do(http.MethodPost, "/api/v1/auth/login",
		jsonBody(c.t, api.LoginRequest{Email: testsupport.AccountAEmail, Password: AccountPassword}))

	// A body carrying a member no operation declares. It is refused by the
	// decoder — and the refusal names the member, which is how an unknown
	// member's VALUE ends up somewhere it was never decoded into.
	c.do(http.MethodPost, "/api/v1/auth/login",
		fmt.Sprintf(`{"email":%q,"password":%q,"crumb":%q}`,
			testsupport.AccountAEmail, testsupport.Password, BodyCrumb))
}

// jsonBody renders a request body from the DTO the operation actually decodes.
//
// The DTOs and not hand-written JSON: a member renamed on the wire is then a
// compile failure here rather than a step that quietly stops planting the
// sentinel it was written to plant.
func jsonBody(t testing.TB, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	require.NoError(t, err, "rendering a request body")

	return string(raw)
}

// sentinelMedication is the record every sentinel that belongs to a person is
// planted in. Every optional member is filled: an unfilled one is a column no
// sink could have leaked.
func sentinelMedication(patientID string) api.MedicationCreate {
	started := "2024-03-15"
	ended := "2024-09-30"

	return api.MedicationCreate{
		Patient:         patientID,
		Name:            MedicationName,
		AlternativeName: AlternativeName,
		Dosage:          Dosage,
		Frequency:       Frequency,
		Indication:      Indication,
		StartedOn:       &started,
		EndedOn:         &ended,
		Status:          string(clinical.TherapyStatusActive),
		SideEffects:     SideEffects,
		Notes:           Notes,
	}
}

// driveRecords walks contracts/records.md's seven operations against a record
// that holds every sentinel, including the refusals: a stranger's read, a
// genuine miss, a stale precondition, an unpublished filter value and a forged
// cursor. The refusals matter more than the successes here — a refusal is
// where an error message is constructed, and an error message is what reaches
// the Sentry sink.
func driveRecords(c *client) {
	c.token(testsupport.AccountAEmail)

	// Opened before the write, because the whole question about a live stream
	// is what it publishes when something changes underneath it.
	patientID := testsupport.AccountAPatientSelfID

	streamed := c.openStream(1200*time.Millisecond, patientID)

	created := c.do(http.MethodPost, recordsOfKind(), jsonBody(c.t, sentinelMedication(patientID)))
	require.Equal(c.t, http.StatusCreated, created.Status,
		"the sentinel record was not created, so every read below reads nothing: %s", created.Body)

	id := decodedID(c.t, created.Body)
	address := recordAddress(id)

	read := c.do(http.MethodGet, address, "")
	require.Equal(c.t, http.StatusOK, read.Status, "%s", read.Body)

	etag := read.Header.Get("ETag")
	require.NotEmpty(c.t, etag, "no ETag, so the precondition steps below would not be preconditions")

	// The lists, narrowed and unnarrowed, and the two refusals a query string
	// can produce.
	c.do(http.MethodGet, recordsOfKind()+"?patient="+patientID, "")
	c.do(http.MethodGet, recordsOfKind()+"?patient="+patientID+"&status="+string(clinical.TherapyStatusActive)+"&limit=5", "")
	c.do(http.MethodGet, recordsOfKind()+"?patient="+patientID+"&status="+SearchTerm, "")
	c.do(http.MethodGet, recordsOfKind()+"?patient="+patientID+"&cursor="+SearchTerm, "")
	c.do(http.MethodGet, "/api/v1/records?patient="+patientID+"&kind="+kind.Medication.Segment()+"&limit=3", "")
	c.do(http.MethodGet, "/api/v1/records?patient="+patientID+"&from="+SearchTerm, "")

	// A stranger asking for it, and an id nobody has ever held. FR-033 says
	// the two answers are the same bytes; this suite asks the narrower
	// question of whether either of them says anything to a sink.
	c.token(testsupport.AccountBEmail)
	c.do(http.MethodGet, address, "")
	c.do(http.MethodPatch, address, jsonBody(c.t, api.MedicationPatch{Notes: ptr(Notes)}))

	c.token(testsupport.AccountAEmail)
	c.do(http.MethodGet, recordAddress(missingRecordID), "")

	// The updates: one that succeeds, one whose precondition is stale, one
	// with no precondition at all.
	patch := jsonBody(c.t, api.MedicationPatch{
		Name:  ptr(MedicationName),
		Notes: ptr(Notes),
	})

	updated := c.doWith(http.MethodPatch, address, patch, map[string]string{"If-Match": etag})
	require.Equal(c.t, http.StatusOK, updated.Status, "%s", updated.Body)

	c.doWith(http.MethodPatch, address, patch, map[string]string{"If-Match": `"not-the-current-version"`})
	c.do(http.MethodPatch, address, patch)

	current := updated.Header.Get("ETag")
	if current == "" {
		current = etag
	}

	c.doWith(http.MethodDelete, address, "", map[string]string{"If-Match": current})

	<-streamed
}

// decodedID reads the id out of a created record's body without asserting on
// the rest of the shape, which is another suite's business.
func decodedID(t testing.TB, body string) string {
	t.Helper()

	var decoded struct {
		ID string `json:"id"`
	}

	require.NoError(t, json.Unmarshal([]byte(body), &decoded), "reading the created record's id from %s", body)
	require.NotEmpty(t, decoded.ID, "the created record has no id: %s", body)

	return decoded.ID
}

// openStream subscribes to the element stream and reads it for a bounded
// window, in the background, so that the writes below happen while it is open.
//
// The window is a bound and never a threshold: nothing is asserted about what
// arrives, only that the route was served and that whatever it wrote went
// through the same sinks as everything else.
func (c *client) openStream(window time.Duration, patientID string) <-chan struct{} {
	done := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.t.Context()), window)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+"/api/v1/streams/records?patient="+patientID, nil)
	require.NoError(c.t, err)

	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set(crumbHeader, HeaderCrumb)
	request.AddCookie(&http.Cookie{Name: crumbCookie, Value: CookieCrumb}) //nolint:gosec // a client-side request cookie

	if c.bearer != "" {
		request.Header.Set("Authorization", c.bearer)
	}

	c.remember(request, "")

	// The goroutine below is the only thing that may close this body: closing
	// it here would end the stream before anything had been published into it.
	response, err := http.DefaultClient.Do(request) //nolint:bodyclose // closed by the reader goroutine
	require.NoError(c.t, err, "opening the element stream")

	go func() {
		defer close(done)
		defer cancel()
		defer func() { _ = response.Body.Close() }()

		// Read until the window closes. The frames themselves are
		// internal/web/stream's business; what matters here is that the
		// handler ran with records changing underneath it.
		_, _ = io.Copy(io.Discard, response.Body)
	}()

	return done
}

// ptr is the address of a literal, for the patch DTOs whose optional members
// are pointers.
func ptr[T any](value T) *T { return &value }

// drivePages walks contracts/pages.md's nine pages and the two error views a
// URL can produce, each with the credential a browser would actually carry.
//
// A page is driven with the session COOKIE and never with a bearer token: the
// cookie is turned into one by a middleware bound at a priority chosen for
// exactly this, and a page driven with the header would skip it.
func drivePages(c *client) {
	c.anonymous()

	// The public pages, signed out, which is how a person reaches them.
	for _, path := range []string{
		"/login",
		"/register",
		"/forgot-password",
		"/reset-password/" + staleRecoveryToken,
		"/verify-email/" + staleConfirmationToken,
	} {
		c.do(http.MethodGet, path, "")
	}

	// The sign-in-required view: a session page reached with no session.
	c.do(http.MethodGet, "/settings", "")

	// The not-found view: a path that matches no route at all.
	c.do(http.MethodGet, notFoundPath, "")

	c.do(http.MethodGet, shell.AppCSSHref, "")
	c.do(http.MethodGet, shell.DatastarJSHref, "")

	c.token(testsupport.AccountAEmail)
	c.bearer = ""

	for _, path := range []string{
		"/",
		pageOfKind(),
		pageOfKind() + "/" + testsupport.NameOnlyMedicationID,
		pageOfKind() + "/" + testsupport.ScriptedMedicationID,
		"/settings",
	} {
		c.do(http.MethodGet, path, "")
	}

	// The detail page of a record this account does not own, and of one that
	// has never existed. Both render the not-found view.
	c.do(http.MethodGet, pageOfKind()+"/"+missingRecordID, "")
}

// driveNativePaths walks contracts/README.md's documented PocketBase-native
// routes.
//
// They are in this exercise because PocketBase serves them and PocketBase
// assembles its own error messages out of the submitted record — which is the
// class of disclosure MediKube's own handlers are written to avoid and which
// nothing in MediKube reviews. Every one is driven with a sentinel where the
// route takes an address or a password.
func driveNativePaths(c *client) {
	c.anonymous()

	const users = "/api/collections/users"

	c.do(http.MethodGet, users+"/auth-methods", "")
	c.do(http.MethodPost, users+"/auth-with-password",
		fmt.Sprintf(`{"identity":%q,"password":%q}`, AccountEmail, AccountPassword))
	c.do(http.MethodPost, users+"/request-password-reset",
		fmt.Sprintf(`{"email":%q}`, AccountEmail))
	c.do(http.MethodPost, users+"/confirm-password-reset",
		fmt.Sprintf(`{"token":%q,"password":%q,"passwordConfirm":%q}`,
			staleRecoveryToken, AccountPassword, AccountPassword))
	c.do(http.MethodPost, users+"/confirm-verification",
		fmt.Sprintf(`{"token":%q}`, staleConfirmationToken))

	c.token(testsupport.AccountAEmail)
	c.do(http.MethodPost, users+"/auth-refresh", "")
	c.do(http.MethodPost, users+"/request-verification",
		fmt.Sprintf(`{"email":%q}`, testsupport.AccountAEmail))

	const superusers = "/api/collections/_superusers"

	c.anonymous()
	c.do(http.MethodGet, superusers+"/auth-methods", "")
	c.do(http.MethodPost, superusers+"/auth-with-password",
		fmt.Sprintf(`{"identity":%q,"password":%q}`, AccountEmail, AccountPassword))
	c.do(http.MethodPost, superusers+"/auth-refresh", "")

	// The admin UI itself. It ships in production and is a route an operator
	// reaches; what it must not do is name anybody in a diagnostic.
	c.do(http.MethodGet, "/_/", "")
}

// driveAccountLifecycle is one person's whole account, start to finish: created
// with a sentinel address and a sentinel name, signed into, read, renamed,
// its password changed to a second sentinel, its address confirmation resent,
// signed out of and finally deleted.
//
// Deletion is last and is not decoration. PocketBase unsets an audit row's
// actor reference when the account it names is removed (research D-22), which
// is the one legitimate write to an append-only trail — and it happens against
// a trail this exercise has just filled with the account's own operations.
func driveAccountLifecycle(c *client) {
	c.anonymous()

	registered := c.do(http.MethodPost, "/api/v1/auth/register", jsonBody(c.t, api.RegisterRequest{
		Email:    AccountEmail,
		Name:     AccountName,
		Password: AccountPassword,
	}))
	require.Equal(c.t, http.StatusCreated, registered.Status,
		"the sentinel account was not created, so the rest of this journey drives nothing: %s", registered.Body)

	// A second sign-up on the same address. It must be refused, and the
	// refusal is the enumeration-shaped one: whatever it says, it says it
	// about an address that exists.
	c.do(http.MethodPost, "/api/v1/auth/register", jsonBody(c.t, api.RegisterRequest{
		Email:    AccountEmail,
		Name:     AccountName,
		Password: AccountPassword,
	}))

	c.as(AccountEmail, AccountPassword)

	c.do(http.MethodGet, "/api/v1/me", "")
	c.do(http.MethodPatch, "/api/v1/me", jsonBody(c.t, api.MePatch{Name: ptr(AccountName)}))
	c.do(http.MethodPost, "/api/v1/auth/verify-email", "")
	c.do(http.MethodPost, "/api/v1/auth/refresh", "")

	// A record of this account's own, so that the deletion below has a cascade
	// to perform rather than an empty account to remove.
	//
	// The registration hook that provisions a self-record automatically
	// (FR-005) is a different story's work; a sentinel account minted here has
	// none, so one is planted directly to give the medication below a patient
	// to be filed against.
	patientID := sentinelPatientFor(c, AccountEmail)
	c.do(http.MethodPost, recordsOfKind(), jsonBody(c.t, sentinelMedication(patientID)))

	// The wrong current password first: a refusal here is where a password
	// would be written down if anything wrote one down.
	c.do(http.MethodPut, "/api/v1/me/password", jsonBody(c.t, api.ChangePasswordRequest{
		CurrentPassword: NewAccountPasswrd,
		NewPassword:     NewAccountPasswrd,
	}))

	changed := c.do(http.MethodPut, "/api/v1/me/password", jsonBody(c.t, api.ChangePasswordRequest{
		CurrentPassword: AccountPassword,
		NewPassword:     NewAccountPasswrd,
	}))
	require.Equal(c.t, http.StatusNoContent, changed.Status, "%s", changed.Body)

	// Changing the password rotates the token key, which ends every session
	// including this one.
	c.as(AccountEmail, NewAccountPasswrd)

	// The deletion refused for the two reasons it can be, then performed.
	c.do(http.MethodDelete, "/api/v1/me", jsonBody(c.t, api.DeleteAccountRequest{
		Password:     NewAccountPasswrd,
		Confirmation: SearchTerm,
	}))
	c.do(http.MethodDelete, "/api/v1/me", jsonBody(c.t, api.DeleteAccountRequest{
		Password:     AccountPassword,
		Confirmation: identity.DeleteConfirmationPhrase,
	}))

	deleted := c.do(http.MethodDelete, "/api/v1/me", jsonBody(c.t, api.DeleteAccountRequest{
		Password:     NewAccountPasswrd,
		Confirmation: identity.DeleteConfirmationPhrase,
	}))
	require.Equal(c.t, http.StatusNoContent, deleted.Status,
		"the sentinel account was not deleted, so the reference cascade over the trail never ran: %s", deleted.Body)

	// Signing out is driven on a seeded account, because the account whose
	// journey this was no longer exists.
	c.token(testsupport.AccountCEmail)
	c.do(http.MethodPost, "/api/v1/auth/logout", "")
}
