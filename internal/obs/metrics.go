package obs

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"medikube/internal/config"
)

// MetricsPath is the only path the measurement listener answers.
const MetricsPath = "/metrics"

// labelOther is where every value that is not on a published list ends up.
//
// FR-055 requires metric labels to be drawn from bounded, published sets. The
// mechanism is an allowlist rather than a sanitiser: a sanitiser has to
// recognise the value it must not emit, and a search term, a record id and a
// medication name are unbounded in a way no pattern catches. An allowlist has
// to recognise the value it may, and there are twenty of those.
const labelOther = "other"

const metricNamespace = "medikube"

// methods is the published method set. http.Request.Method is a token the
// client chose — Go's server accepts any valid token, not only these — so it
// is as unbounded as a path is until it is checked against a list.
var methods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodConnect,
	http.MethodOptions,
	http.MethodTrace,
}

// recordKinds is medikube_records_total{kind}'s allowlist: every clinical
// record kind this build knows (internal/domain/kind, medication today) plus
// the three phase-002 entities that are not a kind.Kind (research D-05) but
// are still "a record" for this counter's purpose.
var recordKinds = []string{"medication", "patient", "practitioner", "facility"}

// thumbSizes is medikube_files_thumb_duration_seconds{size}'s allowlist:
// config.FilesConfig's default MEDIKUBE_FILES_PHOTO_THUMBS plus the original,
// which is timed as size zero — a read, not a generation, but the same
// histogram either way. An operator who reconfigures the thumbnail sizes
// gets labelOther on the new one rather than an unbounded label value.
var thumbSizes = []string{"original", "100x100t", "400x400f"}

// switchOutcomes is medikube_patients_switch_total{outcome}'s allowlist,
// mirroring internal/service/patient.SetActivePatient's own return paths.
var switchOutcomes = []string{"ok", "cleared", "not_found", "unauthenticated", "error"}

// Metrics is the registry, the collectors and the label allowlist.
//
// It is not process state and there is no package-level default registry:
// prometheus.DefaultRegisterer is reachable from every package in the binary,
// which is how a collector nobody reviewed ends up on the operator's endpoint.
type Metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	latency  *prometheus.HistogramVec

	recordsTotal  *prometheus.CounterVec
	photoBytes    prometheus.Histogram
	thumbDuration *prometheus.HistogramVec
	patientSwitch *prometheus.CounterVec

	// The published route set, as registered patterns. Empty means every
	// observation lands in labelOther, which is a useless endpoint rather than
	// a leaking one — the right way round for a default.
	routes map[string]struct{}
}

// NewMetrics builds the registry. PublishRoutes adds the route patterns that
// may become label values; it is separate from construction because this
// phase's own counters (RecordCreated, PatientSwitch, ...) are wired into the
// service and store layers before the route table — which patterns are
// derived from — exists, and a Metrics that could not be built until its
// consumers already had one would make every one of them wait on the route
// table too.
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()

	metrics := &Metrics{
		registry: registry,
		routes:   make(map[string]struct{}),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricNamespace,
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Requests handled, by registered route pattern, method and status.",
		}, []string{"route", "method", "status"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "Time to handle a request, by registered route pattern and method.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"route", "method"}),
		recordsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "records_total",
			Help:      "Records created, by kind.",
		}, []string{"kind"}),
		photoBytes: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Subsystem: "files",
			Name:      "photo_bytes",
			Help:      "Size of an uploaded patient photograph, in bytes.",
			Buckets:   prometheus.ExponentialBuckets(1<<10, 4, 8), // 1KiB .. 4MiB
		}),
		thumbDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Subsystem: "files",
			Name:      "thumb_duration_seconds",
			Help:      "Time to produce or read one photo size, by size.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"size"}),
		patientSwitch: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricNamespace,
			Subsystem: "patients",
			Name:      "switch_total",
			Help:      "Active-patient switch attempts, by outcome.",
		}, []string{"outcome"}),
	}

	registry.MustRegister(
		metrics.requests,
		metrics.latency,
		metrics.recordsTotal,
		metrics.photoBytes,
		metrics.thumbDuration,
		metrics.patientSwitch,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return metrics
}

// RecordCreated increments medikube_records_total{kind}. kind is reduced to
// the published set first, the same way ObserveRequest reduces its own
// labels — an unlisted kind is a wiring mistake, not a new, unbounded label
// value.
func (m *Metrics) RecordCreated(kind string) {
	if m == nil {
		return
	}

	m.recordsTotal.WithLabelValues(allowed(kind, recordKinds)).Inc()
}

// ObservePhotoBytes records the size of one uploaded photograph
// (medikube_files_photo_bytes). Nil-safe: internal/store/patient's PhotoStore
// carries a possibly-unset *Metrics, and a build with no destination
// configured must not need its own nil check at every call site.
func (m *Metrics) ObservePhotoBytes(n int) {
	if m == nil {
		return
	}

	m.photoBytes.Observe(float64(n))
}

// ObserveThumbDuration records how long one size took to produce or read
// (medikube_files_thumb_duration_seconds{size}). Nil-safe, for the same
// reason as ObservePhotoBytes.
func (m *Metrics) ObserveThumbDuration(size string, took time.Duration) {
	if m == nil {
		return
	}

	m.thumbDuration.WithLabelValues(allowed(size, thumbSizes)).Observe(took.Seconds())
}

// PatientSwitch increments medikube_patients_switch_total{outcome}.
func (m *Metrics) PatientSwitch(outcome string) {
	if m == nil {
		return
	}

	m.patientSwitch.WithLabelValues(allowed(outcome, switchOutcomes)).Inc()
}

// allowed reduces value to itself when it is on the published list and to
// labelOther otherwise — ObserveRequest's own mechanism, generalised so the
// three new instruments do not each grow their own copy.
func allowed(value string, published []string) string {
	if slices.Contains(published, value) {
		return value
	}

	return labelOther
}

// PublishRoutes adds patterns to ObserveRequest's route allowlist.
//
// The patterns are the registered ones — `GET /api/v1/records/medications/{id}`
// — never a resolved path. That distinction is the whole of FR-055's label
// clause: the resolved path carries the id, and the query string carries the
// search term. Called once, after the route table is built and before the
// measurements listener starts serving.
func (m *Metrics) PublishRoutes(patterns ...string) {
	for _, pattern := range patterns {
		m.routes[pattern] = struct{}{}
	}
}

// Registry is the gatherer. It is exposed rather than an http.Handler because
// the PHI-leak suite scrapes it in memory, and because a handler is a thing
// somebody mounts.
func (m *Metrics) Registry() *prometheus.Registry { return m.registry }

// ObserveRequest records one handled request. Every label is reduced to a
// published value first; nothing a caller passes reaches the endpoint verbatim.
func (m *Metrics) ObserveRequest(pattern, method string, status int, took time.Duration) {
	route := m.route(pattern)
	verb := allowedMethod(method)

	m.requests.WithLabelValues(route, verb, allowedStatus(status)).Inc()
	m.latency.WithLabelValues(route, verb).Observe(took.Seconds())
}

func (m *Metrics) route(pattern string) string {
	if _, published := m.routes[pattern]; published {
		return pattern
	}

	return labelOther
}

func allowedMethod(method string) string {
	if slices.Contains(methods, method) {
		return method
	}

	return labelOther
}

func allowedStatus(status int) string {
	if status < 100 || status > 599 {
		return labelOther
	}

	return strconv.Itoa(status)
}

// MetricsServer is the listener the measurements are published on. It is its
// own socket, never a route on the application's router: FR-055 requires a
// channel an ordinary visitor cannot reach, and a path on the public listener
// is reachable by definition.
type MetricsServer struct {
	listener net.Listener
	server   *http.Server
	stopped  chan struct{}
}

// StartMetrics binds the listener and serves the registry on it.
//
// Binding happens here rather than in the goroutine so that a port already in
// use is an error the composition root can refuse to boot on, instead of a
// line in a log nobody reads. When metrics are disabled nothing is bound at
// all — the returned server has no address and no goroutine.
//
// ctx bounds the bind and not the listener: the listener's lifetime is
// Shutdown's, because a metrics endpoint that vanished when the boot context
// was cancelled would take the operator's view of the shutdown with it.
func StartMetrics(ctx context.Context, cfg config.MetricsConfig, metrics *Metrics, log zerolog.Logger) (*MetricsServer, error) {
	if !cfg.Enabled {
		return &MetricsServer{}, nil
	}

	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(ctx, "tcp", cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("bind the metrics listener to %s: %w", cfg.Addr, err)
	}

	mux := http.NewServeMux()
	mux.Handle(MetricsPath, authorised(cfg.Token, promhttp.HandlerFor(metrics.registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
		ErrorLog:      promLogger{log: log},
	})))

	server := &MetricsServer{
		listener: listener,
		stopped:  make(chan struct{}),
		server: &http.Server{
			Handler: mux,
			// A scrape is a GET with no body; anything slower than this is not
			// a Prometheus server (gosec G112).
			ReadHeaderTimeout: 5 * time.Second,
		},
	}

	go func() {
		defer close(server.stopped)

		if err := server.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Str("addr", cfg.Addr).Msg("metrics_listener_stopped")
		}
	}()

	return server, nil
}

// Addr is the address the listener actually bound, or the empty string when
// metrics are disabled. It is the bound address and not the configured one
// because port 0 is how a test asks for a free port, and because the two
// differing is exactly the case worth catching.
func (s *MetricsServer) Addr() string {
	if s == nil || s.listener == nil {
		return ""
	}

	return s.listener.Addr().String()
}

// Shutdown stops the listener and waits for the goroutine that owns it.
func (s *MetricsServer) Shutdown(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shut the metrics listener down: %w", err)
	}

	select {
	case <-s.stopped:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for the metrics listener to stop: %w", ctx.Err())
	}
}

// authorised gates the endpoint on the bearer token when one is configured.
//
// config.validateProduction requires a token the moment the address is not
// loopback, and a required credential nothing checks is worse than no
// credential at all: it is a credential an operator believes in.
func authorised(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}

	expected := []byte("Bearer " + token)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offered := []byte(r.Header.Get("Authorization"))

		if subtle.ConstantTimeCompare(offered, expected) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

			return
		}

		next.ServeHTTP(w, r)
	})
}

// promLogger puts promhttp's own complaints on the one stream (Principle VI).
type promLogger struct {
	log zerolog.Logger
}

func (l promLogger) Println(v ...any) {
	l.log.Error().Str("component", "promhttp").Msg(fmt.Sprint(v...))
}
