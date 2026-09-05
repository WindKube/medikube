//go:build phileak

// phileak_test.go is the assertion half of the PHI-leak suite. It is one
// question — did any planted value reach any sink — plus the guards that stop
// that question being answered over nothing.
//
// Phases 002-006 extend exercise.go. NOTHING here changes: a suite that grows a
// second opinion about what counts as a leak is a suite with a hole in it.

package phileak

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
)

// TestNothingAPersonRecordedReachesADiagnosticSink is T235.
//
// One exercise, several questions about it. The exercise boots a real instance,
// clones the committed fixture and drives every route the inventory declares,
// so it is run once and the subtests share it.
func TestNothingAPersonRecordedReachesADiagnosticSink(t *testing.T) {
	result := Run(t)

	t.Run("no sentinel reaches any sink", func(t *testing.T) {
		assertNothingLeakedButWhatIsRegistered(t, result.Capture)
	})

	t.Run("every sink recorded what the exercise drove", func(t *testing.T) {
		assertSinksAreNotEmpty(t, result)
	})

	t.Run("every sentinel reached the process", func(t *testing.T) {
		assertSentinelsWereSubmitted(t, result)
	})

	t.Run("every sentinel the application may hand back, it did", func(t *testing.T) {
		assertSentinelsWereStored(t, result)
	})

	t.Run("every route the inventory declares was driven", func(t *testing.T) {
		assertEveryRouteWasDriven(t, result)
	})

	t.Run("what is deliberately not a sentinel says so", func(t *testing.T) {
		assertUnsentinelledIsStillTrue(t)
	})
}

// sinkEvidence is what each sink must contain for a clean scan of it to mean
// anything.
//
// This is rule 11 applied to a four-sink capture: "no sentinel in the Sentry
// transport" is true of a process that reported nothing, "no sentinel in the
// span recorder" is true of a build with no tracing, and neither reading is
// distinguishable from a clean one. Each marker below is written by the
// APPLICATION — a message the request logger emits, a metric the registry
// declares, an attribute otelsql sets, a member sentry-go's envelope carries —
// so a marker that stops appearing means the sink stopped being fed.
var sinkEvidence = map[string][]string{
	SinkLogs: {
		// The one line internal/obs's request logger writes per request.
		"http_request",
		// Its correlation field, which is what FR-054's join is made of.
		"request_id",
	},
	SinkMetrics: {
		// The counter internal/obs declares, which only ObserveRequest fills.
		"medikube_http_requests_total",
		"medikube_http_request_duration_seconds",
		// Both directions of the label allowlist, exercised by real traffic: a
		// pattern that is published survives as itself, and everything else —
		// including a resolved path carrying a record id — becomes `other`. A
		// sink carrying only one of the two would mean the allowlist was never
		// asked a question it could get wrong.
		`route="GET /api/v1/records/{kind}/{id}"`,
		`route="other"`,
	},
	SinkTraces: {
		// The resource internal/obs builds by hand.
		"service.name",
		// otelsql's own attribute, which is only present if the database was
		// actually opened through the instrumented connect function.
		"db.system.name",
	},
	SinkSentry: {
		// sentry-go's envelope header and the event type it carries.
		"sentry.go",
		"exception",
	},
}

// assertSinksAreNotEmpty is the guard on the guard.
func assertSinksAreNotEmpty(t *testing.T, result *Result) {
	t.Helper()

	// The exercise itself has to have happened. Every number below is a floor
	// well under what a full walk produces, so it fails when the exercise
	// breaks and not when a route is added.
	require.Greater(t, result.Requests, 40, "the exercise barely drove anything")
	require.Positive(t, result.Faults, "no request failed, so no error message was ever constructed to report")
	require.Positive(t, result.Reported, "nothing was handed to the error reporter, so the Sentry sink is empty by absence")
	require.Positive(t, result.Exports, "the OTLP exporter never sent, so the trace sink is empty by absence")
	require.Positive(t, result.Envelopes, "the Sentry client never sent, so the Sentry sink is empty by absence")

	sinks := result.Capture.Sinks()
	require.Len(t, sinks, len(sinkEvidence),
		"a sink was registered or removed without a row in sinkEvidence, so it would be scanned but never proven non-empty")

	for _, sink := range sinks {
		markers, known := sinkEvidence[sink.Name]
		require.Truef(t, known, "%s has no evidence row: add one or it is scanned without being proven fed", sink.Name)
		require.NotEmptyf(t, sink.Text, "%s is empty, so a clean scan of it says nothing", sink.Name)

		lowered := strings.ToLower(sink.Text)

		for _, marker := range markers {
			assert.Containsf(t, lowered, strings.ToLower(marker),
				"%s does not carry %q, so nothing the application emits reached it and a clean scan is true by absence",
				sink.Name, marker)
		}
	}
}

// assertSentinelsWereSubmitted proves every planted value actually reached the
// process. There are no exemptions: a sentinel the exercise never sends is a
// sentinel whose absence from the sinks is a fact about this file.
func assertSentinelsWereSubmitted(t *testing.T, result *Result) {
	t.Helper()

	require.NotEmpty(t, result.Submitted, "no request was recorded, so this guard looked at nothing")

	submitted := strings.ToLower(result.Submitted)

	for _, sentinel := range Sentinels() {
		assert.Containsf(t, submitted, strings.ToLower(sentinel.Value),
			"%s (%s) is never sent to the application, so its absence from every sink proves nothing",
			sentinel.Value, sentinel.Meaning)
	}
}

// neverEchoed is the sentinels MediKube must never hand back, keyed by the
// value and carrying the reason.
//
// It is an exemption table and it bites in the useful direction: an entry here
// asserts the value is NOT in any response, so the day a refusal starts quoting
// the password it was given, or a validation message starts quoting the value
// it rejected, this table is what fails.
var neverEchoed = map[string]string{
	AccountPassword:   "a password is written to the instance and never rendered back out of it (T235)",
	NewAccountPasswrd: "the replacement password, for the same reason (T235)",
	SearchTerm:        "a rejected filter value and a mistyped confirmation phrase; contracts/README.md's refusals carry a field and a code and never the submitted value (T235)",
	CookieCrumb:       "a cookie the exercise attaches to every request; no operation reads it and nothing may reflect it (T235)",
	HeaderCrumb:       "a header the exercise attaches to every request; same rule (T235)",
	BodyCrumb:         "the value of a body member no operation declares; the decoder's refusal names the member and must not quote what was in it (T235)",
	PhotoFilename:     "the uploaded file's name is discarded at the edge; the photograph is stored and served under a name the instance minted (contracts/patient-photo.md)",
}

// assertSentinelsWereStored is the other half of planted-ness: for everything
// that is a person's own record, the application handed it back, so the
// instance really is holding it.
func assertSentinelsWereStored(t *testing.T, result *Result) {
	t.Helper()

	require.NotEmpty(t, result.Echoed, "no response body was recorded, so this guard looked at nothing")

	echoed := strings.ToLower(result.Echoed)

	var stored int

	for _, sentinel := range Sentinels() {
		reason, exempt := neverEchoed[sentinel.Value]

		if exempt {
			assert.NotContainsf(t, echoed, strings.ToLower(sentinel.Value),
				"%s is in neverEchoed (%s) and the application handed it back anyway", sentinel.Value, reason)

			continue
		}

		stored++

		assert.Containsf(t, echoed, strings.ToLower(sentinel.Value),
			"%s (%s) is not in any response, so the instance may never have held it — plant it or add it to neverEchoed with a reason",
			sentinel.Value, sentinel.Meaning)
	}

	require.Greater(t, stored, 5,
		"almost every sentinel is exempt from being stored, so this guard is checking nothing")

	for value, reason := range neverEchoed {
		require.NotEmptyf(t, reason, "the exemption for %s has no reason", value)
		require.Containsf(t, strings.Join(SentinelValues(), "\n"), value,
			"%s is exempt from being echoed and is not a sentinel any more: strike it out of neverEchoed", value)
	}
}

// undriven is the routes the exercise deliberately does not walk, keyed by
// operation id and carrying the reason and the task that owns it.
//
// It is a map and not a list because a bare list is a set of decisions nobody
// wrote down, and it is keyed by the operation id rather than by a path prefix
// because a prefix exemption covers whatever is registered under it next
// (internal/store/filter_test.go:697 documents that as a real bug).
var undriven = map[string]string{
	"nativeAdminUI": "PocketBase registers GET /_/{path...} inside apis.Serve (apis/serve.go:84) and not inside " +
		"apis.NewRouter, which is what internal/testsupport.NewEdgeHandler calls — so no harness in this repository " +
		"serves the admin UI and a request for it is answered by the mux catch-all. The exercise asks for /_/ anyway, " +
		"and the assertion below fails if it ever starts being served, at which point this entry comes out (T235).",
}

// assertEveryRouteWasDriven is the anti-curation guard.
//
// The population is the inventory, and what counts as driven is measured INSIDE
// the process — the ServeMux pattern the request actually matched, or the
// concrete method and path it actually asked for. A list of what this file
// intended to drive would be the list that says three while nine are
// registered.
func assertEveryRouteWasDriven(t *testing.T, result *Result) {
	t.Helper()

	routes := httproute.Inventory().Routes()
	require.Greater(t, len(routes), 30, "the inventory is nearly empty, so this guard walks nothing")

	var (
		missed  []string
		checked int
	)

	for _, route := range routes {
		if reason, exempt := undriven[route.OpID]; exempt {
			require.NotEmptyf(t, reason, "the exemption for %s has no reason", route.OpID)

			// The exemption expires by itself. Every entry here says the route
			// cannot be reached from this harness; the day it can, the claim
			// is false and the entry has to go, rather than quietly excusing a
			// route nothing walks.
			assert.Zerof(t, result.Patterns[route.Pattern()]+result.Paths[route.Pattern()],
				"%s is exempt from being driven (%s) and was driven anyway: strike it out of undriven",
				route.OpID, reason)

			continue
		}

		checked++

		if result.Patterns[route.Pattern()] > 0 || result.Paths[route.Pattern()] > 0 {
			continue
		}

		missed = append(missed, route.OpID+" ("+route.Pattern()+")")
	}

	require.Greater(t, checked, 30, "almost every route is exempt, so this guard is checking nothing")

	assert.Empty(t, missed,
		"the exercise never reached these routes, so nothing this suite says covers them — drive them in exercise.go or exempt them in undriven with a reason")

	for opID := range undriven {
		found := false

		for _, route := range routes {
			if route.OpID == opID {
				found = true

				break
			}
		}

		assert.Truef(t, found, "%s is exempt from being driven and is not a route any more: strike it out of undriven", opID)
	}
}

// taggedFiles is the two files this suite is, and the guard below is defect
// D18 made permanent.
//
// D18: `task test:phileak` runs `go test -tags=phileak` and, before this task,
// NO FILE IN THE REPOSITORY CARRIED THAT TAG. The command re-ran the untagged
// tests in a tenth of a second and exited 0, while the Taskfile's own comment
// and CLAUDE.md both stated as fact that the suite was build-tagged and
// invisible to `task test`. The gate reported success while asserting nothing.
//
// The tag is therefore load-bearing in both directions and has to be asserted
// rather than assumed. Note what happens if somebody deletes it: an untagged Go
// file is compiled into EVERY build, so this test runs either way and fails
// either way — which is the only arrangement in which a missing tag is loud.
var taggedFiles = []string{"exercise_test.go", "phileak_test.go"}

func TestTheBuildTagIsWhatKeepsThisSuiteOutOfTaskTest(t *testing.T) {
	t.Parallel()

	const directive = "//go:build phileak"

	root := repoRoot(t)
	mine := filepath.Join(root, "internal", "testsupport", "phileak")

	require.NotEmpty(t, taggedFiles, "no file is checked, so this guard proves nothing")

	for _, name := range taggedFiles {
		source, err := os.ReadFile(filepath.Join(mine, name)) //nolint:gosec // this package reading its own two files
		require.NoErrorf(t, err, "%s is one of the two files this suite is", name)

		assert.Truef(t, strings.HasPrefix(string(source), directive+"\n"),
			"%s does not open with %q, so `go test` runs it and `task test:phileak` is once again a command that asserts nothing (defect D18)",
			name, directive)
	}

	// The linter has to be told about the tag too, or it never type-checks
	// either file and the strictest gate in the repository silently skips the
	// most valuable test in it — which is exactly what run.build-tags already
	// says about `sselive`.
	config, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	require.NoError(t, err)

	assert.Contains(t, string(config), "- phileak",
		".golangci.yml's run.build-tags does not name phileak, so golangci-lint never reads these two files")

	// And the Taskfile has to be the thing that passes it.
	taskfile, err := os.ReadFile(filepath.Join(root, "Taskfile.yaml"))
	require.NoError(t, err)

	assert.Contains(t, string(taskfile), "-tags=phileak",
		"Taskfile.yaml no longer runs this suite with its tag")
}

// knownLeak is one disclosure this suite has FOUND, reproduces on every run,
// and cannot fix from inside its own two files.
//
// Read this before adding to it. A register like this is one edit away from
// being the hole the suite exists to close, so it is built to be hostile to its
// own growth:
//
//   - an entry asserts the leak IS still there. The day it is fixed, the entry
//     fails and has to be struck out. A register cannot rot into a list of
//     things that used to be true.
//   - an entry matches on the exact text the value appears in, not on the sink
//     and the value alone. The SAME value reaching the SAME sink through a
//     different mechanism is an unregistered finding and fails.
//   - there is a hard cap below. The register is a triage note with an owner
//     and a fix, not a policy about what MediKube may disclose.
//
// Nothing here is a decision that the disclosure is acceptable. It is a
// decision about which file the fix belongs in.
type knownLeak struct {
	// Sink is one of capture.go's four names.
	Sink string
	// Sentinel is the value disclosed.
	Sentinel string
	// Fragment is the surrounding text, and it is what makes the entry narrow:
	// it must appear in the excerpt beside the value.
	Fragment string
	// Where the disclosure is produced and who owns the fix.
	Producer string
	Owner    string
	Fix      string
}

// registerCap stops the register becoming the place findings go to die.
const registerCap = 3

// knownLeaks is empty. It held D20's two entries, both closed by
// obs.Recordable; the guard below fails on any entry that stops being true.
var knownLeaks = []knownLeak{}

type leakReport struct {
	messages []string
	fatal    []string
}

func (r *leakReport) Helper() {}

func (r *leakReport) Errorf(format string, args ...any) {
	r.messages = append(r.messages, fmt.Sprintf(format, args...))
}

func (r *leakReport) Fatal(args ...any) {
	r.fatal = append(r.fatal, fmt.Sprint(args...))
}

func (r *leakReport) Fatalf(format string, args ...any) {
	r.fatal = append(r.fatal, fmt.Sprintf(format, args...))
}

// assertNothingLeakedButWhatIsRegistered is the suite's one assertion.
func assertNothingLeakedButWhatIsRegistered(t *testing.T, capture *Capture) {
	t.Helper()

	sentinels := SentinelValues()
	require.NotEmpty(t, sentinels, "an empty sentinel list makes the scan below pass over any output at all")
	require.LessOrEqual(t, len(knownLeaks), registerCap,
		"the known-leak register has outgrown a triage note; fix them rather than record them")

	// The one assertion, run against a recorder rather than against *testing.T.
	// That is the seam capture.go's Reporter interface exists for, and it is
	// what lets the findings be triaged against the register below without the
	// assertion itself acquiring a second opinion about what a leak is.
	report := new(leakReport)
	capture.AssertNoSentinels(report, sentinels...)

	require.Empty(t, report.fatal, "the capture refused to scan at all")

	matched := make([]bool, len(knownLeaks))

	for _, finding := range report.messages {
		if known := matchKnown(finding, matched); known {
			continue
		}

		// Reported verbatim, so the sink is named exactly as capture.go names
		// it and the excerpt is the one a reader needs.
		t.Error(finding)
	}

	for index, leak := range knownLeaks {
		require.NotEmpty(t, leak.Owner, "a register entry with no owner is a note nobody acts on")
		require.NotEmpty(t, leak.Fix, "a register entry with no fix is a complaint")

		assert.Truef(t, matched[index],
			"%s no longer discloses %s through %q — the defect is fixed, so strike it out of knownLeaks (owner: %s)",
			leak.Sink, leak.Sentinel, leak.Fragment, leak.Owner)

		t.Logf("KNOWN, UNFIXED, NOT MINE: %s carries %s via %q — %s. Fix: %s",
			leak.Sink, leak.Sentinel, leak.Fragment, leak.Owner, leak.Fix)
	}
}

// matchKnown reports whether one finding is one of the registered ones, marking
// it seen. All three of sink, value and surrounding text must agree: two of
// three would let a second disclosure of the same value into the same sink ride
// in on the first one's entry.
func matchKnown(finding string, matched []bool) bool {
	for index, leak := range knownLeaks {
		if !strings.HasPrefix(finding, leak.Sink) {
			continue
		}

		if !strings.Contains(finding, leak.Sentinel) || !strings.Contains(finding, leak.Fragment) {
			continue
		}

		matched[index] = true

		return true
	}

	return false
}

// assertUnsentinelledIsStillTrue keeps exercise.go's Unsentinelled() honest.
//
// Every entry there is a value this suite deliberately does NOT look for, with
// the reason. Two things can rot: the value can quietly become a sentinel after
// all, which would make the paragraph a lie, and the value can stop being sent,
// which would make it a note about nothing. Both fail here.
func assertUnsentinelledIsStillTrue(t *testing.T) {
	t.Helper()

	excluded := Unsentinelled()
	require.NotEmpty(t, excluded, "nothing is recorded as deliberately unsentinelled, so this guard looks at nothing")

	sentinels := strings.Join(SentinelValues(), "\n")

	for value, reason := range excluded {
		require.NotEmptyf(t, reason, "%s is excluded with no reason", value)
		assert.NotContainsf(t, sentinels, value,
			"%s is recorded as deliberately unsentinelled and is a sentinel as well: one of the two is wrong", value)
	}
}
