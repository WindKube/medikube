package api_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"medikube/internal/testsupport"
	"medikube/internal/web/apitest"
)

// T202a. The latency half of research D-17, reported and NEVER asserted.
//
// It is a Benchmark function, so `go test ./...` does not run it and it cannot
// fail a merge. That is deliberate and it is Constitution VIII: a latency
// comparison has no threshold anybody can defend, and a gate that fails on a
// busy runner teaches everybody to re-run the build rather than to read it.
// The MECHANISM is what blocks merge — auth_timing_test.go's counting seam, and
// internal/web/api's not-found parity — and this reports the number a human
// investigates when the mechanism report and the wall clock disagree.
//
// It reports a RATIO OF MEDIANS. A mean is dominated by the occasional 40ms
// scheduling stall on a shared runner; a median over a few hundred samples is
// stable enough for a trend line. A ratio near 1 is the design working. A ratio
// drifting away from 1 is a human's problem, investigated against T202's
// assertion — never auto-failed.
//
// Run it with:
//
//	go test ./internal/web/api/ -run '^$' -bench 'Timing' -benchtime 200x

// probe is one instance and one address, timed.
type probe struct {
	name    string
	request func() *http.Request
}

// BenchmarkSignInRefusalTiming compares the two ways a sign-in is refused.
//
// The naive shape answers an unknown address in microseconds and a wrong
// password in tens of milliseconds, because only the second pays for a bcrypt
// comparison. The fixed dummy hash is what makes them cost the same; this is
// what says by how much.
func BenchmarkSignInRefusalTiming(b *testing.B) {
	instance := apitest.New(b)
	handler := testsupport.NewEdgeHandler(b, instance.App)

	refusal := func(email, password string) func() *http.Request {
		document := `{"email":"` + email + `","password":"` + password + `"}`

		return func() *http.Request {
			request := httptest.NewRequest(http.MethodPost, loginURL, strings.NewReader(document))
			request.Header.Set("Content-Type", "application/json")

			return request
		}
	}

	compare(b, handler, http.StatusUnauthorized,
		probe{name: "no_such_account", request: refusal("nobody@example.test", testsupport.Password)},
		probe{name: "wrong_password", request: refusal(testsupport.AccountAEmail, "not-the-password")},
	)
}

// BenchmarkNotFoundTiming compares the two ways a record read answers 404: a
// stranger's real record, and an id that never existed.
//
// FR-033 makes those two byte-identical. Making them take comparable time is
// the same requirement applied to the clock, and it has the same shape — the
// refusal path does the work the miss path does — so it is reported here beside
// the sign-in pair rather than in a file of its own (T226).
func BenchmarkNotFoundTiming(b *testing.B) {
	instance := apitest.New(b)
	handler := testsupport.NewEdgeHandler(b, instance.App)
	token := testsupport.UserToken(b, instance.App, testsupport.AccountBEmail)

	read := func(id string) func() *http.Request {
		return func() *http.Request {
			request := httptest.NewRequest(http.MethodGet, recordURL(id), nil)
			request.Header.Set("Authorization", token)

			return request
		}
	}

	compare(b, handler, http.StatusNotFound,
		// Somebody else's real record, refused.
		probe{name: "another_accounts_record", request: read(testsupport.NameOnlyMedicationID)},
		// An identifier of the right shape that nothing has ever used.
		probe{name: "never_existed", request: read(missingID)},
	)
}

// compare times both probes over b.N samples each and reports the medians and
// their ratio.
//
// The two are interleaved rather than run one after the other, so a machine that
// gets busy halfway through the run slows both halves rather than one — which
// is exactly the artefact that would otherwise look like a drift.
func compare(b *testing.B, handler http.Handler, want int, first, second probe) {
	b.Helper()

	firstSamples := make([]time.Duration, 0, b.N)
	secondSamples := make([]time.Duration, 0, b.N)

	b.ResetTimer()

	for range b.N {
		firstSamples = append(firstSamples, once(b, handler, first, want))
		secondSamples = append(secondSamples, once(b, handler, second, want))
	}

	b.StopTimer()

	firstMedian := median(firstSamples)
	secondMedian := median(secondSamples)

	b.ReportMetric(float64(firstMedian.Nanoseconds())/1e6, first.name+"_median_ms")
	b.ReportMetric(float64(secondMedian.Nanoseconds())/1e6, second.name+"_median_ms")

	if secondMedian > 0 {
		b.ReportMetric(float64(firstMedian)/float64(secondMedian), "ratio")
	}
}

func once(b *testing.B, handler http.Handler, p probe, want int) time.Duration {
	b.Helper()

	recorder := httptest.NewRecorder()

	started := time.Now()
	handler.ServeHTTP(recorder, p.request())
	elapsed := time.Since(started)

	// Asserted, because a probe that started answering 401 instead of 404 —
	// or 500 instead of either — would produce a beautifully stable ratio for
	// a path nobody is measuring.
	if recorder.Code != want {
		b.Fatalf("%s answered %d rather than %d: %s", p.name, recorder.Code, want, recorder.Body.String())
	}

	return elapsed
}

func median(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}

	sorted := slices.Clone(samples)
	slices.Sort(sorted)

	return sorted[len(sorted)/2]
}
