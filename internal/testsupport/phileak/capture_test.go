package phileak

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/getsentry/sentry-go"
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
				// A label value, which is where a metrics endpoint leaks: a
				// middleware that took a path segment for a dimension.
				capture.WatchMetrics(func() (string, error) {
					return fmt.Sprintf("medikube_records_total{name=%q} 1\n", sentinel), nil
				})
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

	capture := New(t)
	capture.WatchMetrics(func() (string, error) {
		return "", assert.AnError
	})

	var metrics Sink
	for _, sink := range capture.Sinks() {
		if sink.Name == SinkMetrics {
			metrics = sink
		}
	}

	assert.Contains(t, metrics.Text, "could not be read",
		"a renderer that failed must not read as a stream with nothing in it")
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
	capture.WatchMetrics(func() (string, error) { return "", nil })

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
