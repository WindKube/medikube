package phileak

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

// The capture is the instrument the whole PHI-leak suite reads. An instrument
// that reports clean because it was pointed at nothing is worse than none, so
// every sink is proved to catch a sentinel here, in the package that owns it —
// not in the suite that uses it, where a miss would look like a pass.
const sentinel = "Amoxicillin-9f2c"

func TestEverySinkCatchesASentinel(t *testing.T) {
	t.Parallel()

	leaks := []struct {
		sink string
		leak func(t *testing.T, capture *Capture)
	}{
		{
			sink: SinkLogs,
			leak: func(t *testing.T, capture *Capture) {
				logger := capture.Logger()
				logger.Info().Str("name", sentinel).Msg("saved a record")
			},
		},
		{
			sink: SinkTraces,
			leak: func(t *testing.T, capture *Capture) {
				_, span := capture.Tracer("test").Start(context.Background(), "record.save")
				span.SetAttributes(attribute.String("record.name", sentinel))
				span.End()
			},
		},
		{
			sink: SinkSentry,
			leak: func(t *testing.T, capture *Capture) {
				capture.SentryTransport().SendEvent(&sentry.Event{
					Message: "could not save " + sentinel,
				})
			},
		},
		{
			sink: SinkMetrics,
			leak: func(t *testing.T, capture *Capture) {
				// A label value on a real registry, which is where a metrics
				// endpoint leaks: a middleware that took a path segment for a
				// dimension.
				registry := prometheus.NewRegistry()
				records := prometheus.NewCounterVec(
					prometheus.CounterOpts{Name: "medikube_records_total", Help: "records written"},
					[]string{"name"},
				)
				registry.MustRegister(records)
				records.WithLabelValues(sentinel).Inc()

				capture.WatchMetrics(registry)
			},
		},
	}

	for _, one := range leaks {
		t.Run(one.sink, func(t *testing.T) {
			t.Parallel()

			capture := complete(t)
			one.leak(t, capture)

			failures := recordFailures(func(report Reporter) {
				capture.AssertNoSentinels(report, sentinel)
			})

			require.Len(t, failures, 1, "exactly one sink should have reported")
			assert.Contains(t, failures[0], one.sink)
			assert.Contains(t, failures[0], sentinel)
		})
	}
}

func TestAnEmittedValueThatIsNotASentinelIsNotAFinding(t *testing.T) {
	t.Parallel()

	capture := complete(t)

	logger := capture.Logger()
	logger.Info().Str("record_id", "mkmedamara00001").Msg("saved a record")

	failures := recordFailures(func(report Reporter) {
		capture.AssertNoSentinels(report, sentinel)
	})

	assert.Empty(t, failures,
		"an id is not a leak; a capture that flagged one would be turned off within a week")
}

func TestTheScanIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	capture := complete(t)

	logger := capture.Logger()
	logger.Info().Str("name", strings.ToUpper(sentinel)).Msg("saved a record")

	failures := recordFailures(func(report Reporter) {
		capture.AssertNoSentinels(report, sentinel)
	})

	assert.Len(t, failures, 1,
		"a value that reached the stream upper-cased is the same disclosure")
}

func TestACaptureMissingASinkRefusesToReportClean(t *testing.T) {
	t.Parallel()

	// New wires three of the four; the Prometheus gatherer has to be handed
	// over. A suite that forgot would otherwise pass with a quarter of the
	// question unasked.
	capture := New(t)

	failures := recordFailures(func(report Reporter) {
		capture.AssertNoSentinels(report, sentinel)
	})

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0], SinkMetrics)
	assert.Contains(t, failures[0], "cannot report that nothing leaked")
}

func TestAnEmptySentinelListIsRefused(t *testing.T) {
	t.Parallel()

	capture := complete(t)

	failures := recordFailures(func(report Reporter) {
		capture.AssertNoSentinels(report)
	})

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0], "would pass over any output at all")
}

func TestASinkThatCannotBeReadIsAFindingRatherThanAnEmptyStream(t *testing.T) {
	t.Parallel()

	// A registry that fails to gather, not a stub renderer: a collector that
	// reports an error is the way a real metrics endpoint goes dark, and it
	// produces an empty scrape body if nothing looks at the status.
	registry := prometheus.NewRegistry()
	registry.MustRegister(brokenCollector{
		desc: prometheus.NewDesc("medikube_broken", "a collector that cannot collect", nil, nil),
	})

	capture := New(t)
	capture.WatchMetrics(registry)

	var metrics Sink

	for _, sink := range capture.Sinks() {
		if sink.Name == SinkMetrics {
			metrics = sink
		}
	}

	assert.Contains(t, metrics.Text, "could not be read",
		"a gatherer that failed must not read as a stream with nothing in it")
}

// The metrics sink is the one that reaches into somebody else's library, so the
// three fields a scrape publishes are each proved to be scanned. A leak through
// any of them is the same disclosure, and the exposition text is the only place
// they appear together.
func TestTheMetricsSinkScansNamesLabelsAndHelp(t *testing.T) {
	t.Parallel()

	cases := []struct {
		field    string
		sentinel string
		register func(registry *prometheus.Registry, sentinel string)
	}{
		{
			field: "the metric name",
			// A name is restricted to [a-zA-Z0-9_:], so the sentinel takes the
			// shape a name-derived leak would actually take.
			sentinel: "amoxicillin_9f2c",
			register: func(registry *prometheus.Registry, sentinel string) {
				registry.MustRegister(prometheus.NewCounter(
					prometheus.CounterOpts{Name: "medikube_" + sentinel + "_doses_total", Help: "doses"},
				))
			},
		},
		{
			field:    "a label value",
			sentinel: sentinel,
			register: func(registry *prometheus.Registry, sentinel string) {
				doses := prometheus.NewCounterVec(
					prometheus.CounterOpts{Name: "medikube_doses_total", Help: "doses"},
					[]string{"medication"},
				)
				registry.MustRegister(doses)
				doses.WithLabelValues(sentinel).Inc()
			},
		},
		{
			field:    "the HELP text",
			sentinel: sentinel,
			register: func(registry *prometheus.Registry, sentinel string) {
				registry.MustRegister(prometheus.NewGauge(
					prometheus.GaugeOpts{Name: "medikube_doses", Help: "doses of " + sentinel},
				))
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.field, func(t *testing.T) {
			t.Parallel()

			registry := prometheus.NewRegistry()
			testCase.register(registry, testCase.sentinel)

			capture := New(t)
			capture.WatchMetrics(registry)

			failures := recordFailures(func(report Reporter) {
				capture.AssertNoSentinels(report, testCase.sentinel)
			})

			require.Len(t, failures, 1)
			assert.Contains(t, failures[0], SinkMetrics)
			assert.Contains(t, failures[0], testCase.sentinel)
		})
	}
}

// brokenCollector is a collector whose Collect reports an error, which is what
// makes Gather fail.
type brokenCollector struct {
	desc *prometheus.Desc
}

func (c brokenCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c brokenCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.NewInvalidMetric(c.desc, assert.AnError)
}

func TestTheCaptureHoldsExactlyTheFourSinks(t *testing.T) {
	t.Parallel()

	// The list is asserted rather than counted, so adding a fifth sink without
	// naming it here is a decision somebody has to make on purpose.
	assert.Equal(t,
		[]string{SinkTraces, SinkMetrics, SinkSentry, SinkLogs},
		complete(t).Names())
}

// complete is a capture with all four sinks wired and nothing emitted into any
// of them.
func complete(t *testing.T) *Capture {
	t.Helper()

	capture := New(t)
	capture.WatchMetrics(prometheus.NewRegistry())

	return capture
}

// recordFailures runs fn against a Reporter that records instead of failing,
// and honours Fatal by leaving the goroutine exactly as testing does — so an
// assertion that gives up early is measured as giving up early.
func recordFailures(fn func(report Reporter)) []string {
	report := new(recorder)

	var done sync.WaitGroup

	done.Add(1)

	go func() {
		defer done.Done()

		fn(report)
	}()

	done.Wait()

	return report.failures
}

type recorder struct {
	failures []string
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func (r *recorder) Fatal(args ...any) {
	r.failures = append(r.failures, fmt.Sprint(args...))
	runtime.Goexit()
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
	runtime.Goexit()
}
