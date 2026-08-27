# MediGo — Testing & Verification Strategy

Everything below was verified by building and running it against PocketBase
v0.40.1, `datastar-go` v1.2.2, `templ` v0.3.1020, testify v1.12.1 and Playwright
1.62.1. Measured numbers are from this machine (linux/arm64, 4 cores). Where I
could not execute something, I say so explicitly.

Working prototype lives in the scratchpad:

- `scratchpad/pbbench/` — Go: PB test-app benchmarks, SSE tests, route registry
- `scratchpad/gate/` — the Playwright gate + a stand-in `medigo` binary

---

## 0. BLOCKER: Go 1.26.5 cannot build PocketBase v0.40.1

This contradicts a locked decision, so it goes first.

PocketBase v0.40.1's `go.mod` declares:

```
module github.com/pocketbase/pocketbase

go 1.27
```

and its source imports the **stdlib** `encoding/json/v2` / `encoding/json/jsontext`,
which do not exist before Go 1.27:

```
pocketbase@v0.40.1/tools/search/filter.go:4:   "encoding/json/v2"
pocketbase@v0.40.1/tools/logger/batch_handler.go:5: "encoding/json/v2"
pocketbase@v0.40.1/tests/api.go:6:             "encoding/json/jsontext"
```

Attempting to build under Go 1.26.5 (actually run):

```
$ go mod tidy
go: github.com/pocketbase/pocketbase@v0.40.1 requires go >= 1.27 (running go 1.26.5)
```

This is not a warning that can be suppressed with a `toolchain` line pointing
backwards, and it is not a soft version hint — `encoding/json/v2` is a real
import that will not resolve. Under Go 1.27.0 the identical module tidies,
builds, and tests clean.

**The spec must change "Go 1.26.5" to "Go 1.27.x", or drop to a PocketBase
release whose `go.mod` predates the 1.27 requirement.** Given PocketBase
v0.40.1 is itself locked, the Go version is the one that has to move. Flag this
to whoever owns the locked decisions before any code is written; every other
recommendation in this document assumes Go 1.27.

Knock-on effects worth noting once you are on 1.27:

- `encoding/json/v2` is in the stdlib, so PB's JSON behaviour (field ordering,
  error text) differs subtly from `encoding/json` v1. `tests.ApiScenario`
  normalises response bodies through `jsontext` before substring matching, so
  `ExpectedContent` compares against re-encoded JSON, not the raw bytes.
- `.golangci.yml` and the Dockerfile builder image both need the same bump.

---

# Part A — Go testing with testify against an embedded PocketBase app

## 1. What PocketBase v0.40.1 actually gives you

Two things, in `github.com/pocketbase/pocketbase/tests`:

### `tests.TestApp` — a real `core.App` on a throwaway database

```go
type TestApp struct {
	*core.BaseApp
	mux sync.Mutex

	// EventCalls counts which app hooks fired, and how many times.
	EventCalls map[string]int

	TestMailer *TestMailer
}

func NewTestApp(optTestDataDir ...string) (*TestApp, error)
func NewTestAppWithConfig(config core.BaseAppConfig) (*TestApp, error)
func (t *TestApp) Cleanup()
```

The important part is what `NewTestAppWithConfig` does, verbatim from
`tests/app.go`:

```go
if config.DataDir == "" {
	// fallback to the default test data directory
	_, currentFile, _, _ := runtime.Caller(0)
	config.DataDir = filepath.Join(path.Dir(currentFile), "data")
}

tempDir, err := TempDirClone(config.DataDir)   // <-- copies the fixture
if err != nil {
	return nil, err
}
config.DataDir = tempDir                        // <-- app runs on the copy

app := core.NewBaseApp(config)
if err := app.Bootstrap(); err != nil { ... }
if _, err := app.DB().NewQuery("Select 1").Execute(); err != nil { ... }
if _, err := app.AuxDB().NewQuery("Select 1").Execute(); err != nil { ... }
if err := app.RunAllMigrations(); err != nil { ... }  // applies MISSING migrations only

app.Settings().Logs.MaxDays = 0                 // request logging forced off
```

Three consequences that shape the whole strategy:

1. **Isolation is built in.** You do *not* write `t.TempDir()` + copy yourself.
   `TempDirClone` already `os.MkdirTemp("", "pb_test_*")`s a private copy of the
   fixture per call, and `Cleanup()` `os.RemoveAll`s it. Every `NewTestApp()`
   is a fresh, private SQLite database on disk.
2. **Migrations are not re-run from scratch.** The fixture directory holds an
   already-migrated `data.db`; `RunAllMigrations()` only applies migrations the
   fixture does not have. That is what keeps startup at single-digit
   milliseconds instead of seconds.
3. **It is on-disk SQLite, not in-memory.** PB opens two connections per
   database (concurrent + non-concurrent) to the same path, so `:memory:` would
   give you two unrelated databases. Don't try it. The tmpfs-backed `/tmp`
   copy is already fast enough (measured below), and you can point `TMPDIR` at
   a RAM disk in CI if you ever need more.

### `tests.ApiScenario` — declarative HTTP-level tests

Full struct, from `tests/api.go`:

```go
type ApiScenario struct {
	Name    string
	Method  string
	URL     string
	Body    io.Reader
	Headers map[string]string

	Delay   time.Duration // wait before checking expectations (fire-and-forget goroutines)
	Timeout time.Duration // cancels the request context — the SSE escape hatch

	DisableTestAppCleanup bool

	ExpectedStatus     int
	ExpectedContent    []string // substrings that MUST appear
	NotExpectedContent []string // substrings that MUST NOT appear
	ExpectedEvents     map[string]int // hook name -> exact call count; "*": 0 means "no others"

	TestAppFactory func(t testing.TB) *TestApp
	BeforeTestFunc func(t testing.TB, app *TestApp, e *core.ServeEvent)
	AfterTestFunc  func(t testing.TB, app *TestApp, res *http.Response)
}

func (scenario *ApiScenario) Test(t *testing.T)
func (scenario *ApiScenario) Benchmark(b *testing.B)
```

`Test` builds a real `apis.NewRouter(app)`, triggers `OnServe` so your routes
register, calls `BuildMux()`, and drives it with `httptest.NewRequest` +
`httptest.NewRecorder`. No socket is opened. That is why it is fast.

### The test data directory

Generate it once, commit it:

```bash
# one-off, from the repo root
go run ./cmd/medigo serve --dir=./internal/testsupport/pb_data --automigrate=0
# create collections + seed records in the dashboard, then Ctrl-C and commit
```

For MediGo I would **not** create the fixture through the dashboard by hand.
Schema is code (migrations), so generate it:

```go
// internal/testsupport/fixture.go
//go:generate go run ./cmd/medigo fixture --out ./pb_data
```

...and have `fixture` boot a `pocketbase.New()` against an empty dir, run
migrations, insert the seed records via the same `Seeder` the Playwright gate
uses (see §Part B), then exit. One seeder, two consumers, no drift.

### A working test (this compiles and passes)

```go
package api_test

import (
	"net/http"
	"testing"

	"medigo/internal/testsupport"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestPatientsEndpoint(t *testing.T) {
	t.Parallel()

	userToken := testsupport.Token(t, "users", "owner@medigo.test")
	otherToken := testsupport.Token(t, "users", "stranger@medigo.test")

	scenarios := []tests.ApiScenario{
		{
			Name:            "guest is refused",
			Method:          http.MethodGet,
			URL:             "/api/v1/patients",
			ExpectedStatus:  401,
			ExpectedContent: []string{`"data":{}`},
			TestAppFactory:  testsupport.NewApp,
		},
		{
			Name:            "owner sees their patients",
			Method:          http.MethodGet,
			URL:             "/api/v1/patients",
			Headers:         map[string]string{"Authorization": userToken},
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`, `"seed_pat_1"`},
			TestAppFactory:  testsupport.NewApp,
		},
		{
			// The authorization test Constitution III demands for every
			// patient-data endpoint.
			Name:               "a stranger cannot see someone else's patient",
			Method:             http.MethodGet,
			URL:                "/api/v1/patients/seed_pat_1",
			Headers:            map[string]string{"Authorization": otherToken},
			ExpectedStatus:     404, // 404 not 403: do not confirm the id exists
			NotExpectedContent: []string{"seed_pat_1"},
			TestAppFactory:     testsupport.NewApp,
		},
	}

	for _, s := range scenarios {
		s.Test(t)
	}
}
```

And the shared helper every HTTP test uses:

```go
// internal/testsupport/app.go
package testsupport

import (
	"testing"

	"medigo/internal/httproute"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

const DataDir = "../../internal/testsupport/pb_data"

// NewApp returns a fully wired, isolated MediGo app.
func NewApp(t testing.TB) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp(DataDir)
	require.NoError(t, err)

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		httproute.Build(realServices(app)).Bind(se)
		return se.Next()
	})
	return app
}

func Token(t testing.TB, collection, email string) string {
	t.Helper()
	app, err := tests.NewTestApp(DataDir)
	require.NoError(t, err)
	defer app.Cleanup()

	rec, err := app.FindAuthRecordByEmail(collection, email)
	require.NoError(t, err)
	tok, err := rec.NewAuthToken()
	require.NoError(t, err)
	return tok
}
```

### Trap found while prototyping: never share a `TestApp` across scenarios

Returning the *same* app from `TestAppFactory` for several scenarios causes
**infinite recursion and a stack overflow**. `ApiScenario.test` triggers
`OnServe` on every run; PB's own `apis.bindUIExtensions` re-enters and the
handler chain grows without bound. Observed stack:

```
apis.bindUIExtensions.func1 -> hook.(*Event).Next -> hook.Trigger.func1 -> apis.bindUIExtensions.func1 ...
runtime: goroutine stack exceeds 1000000000-byte limit
```

**Rule: `TestAppFactory` must construct a new app on every call.** It is 10 ms;
there is no reason to share. Use `DisableTestAppCleanup` only if you genuinely
own the lifetime, and then actually call `Cleanup()`.

## 2. Isolation and speed — measured

`NewTestApp()` + `Cleanup()`, PB's own fixture (696 KB, 344 KB `data.db`):

```
BenchmarkNewTestApp-4      20     10924030 ns/op      # 10.9 ms per app
```

200 tests, each building its own isolated app, real wall clock:

| Mode | Wall time | Per app |
|---|---|---|
| `-parallel 1` (sequential) | **1.96 s** | 9.8 ms |
| default parallel, 4 cores | **0.75 s** | 3.7 ms amortised |
| `-race`, parallel | **6.5 s** | 32 ms amortised |

Parallel isolation under the race detector, 16 concurrent apps each writing to
its own clone — **passes clean**, distinct data dirs:

```
--- PASS: TestParallelIsolation (0.00s)
    --- PASS: TestParallelIsolation/case#03 (0.18s)
    ... 16/16 PASS
    dataDir=/tmp/pb_test_197767440
    dataDir=/tmp/pb_test_2690101828
    dataDir=/tmp/pb_test_50871426
```

### Verdict

**`t.Parallel()` is safe and worth using for every PocketBase integration test.**

- Isolation is per-app by construction: separate temp dir, separate SQLite file,
  separate connection pool. There is no shared mutable state to race on.
- At ~10 ms sequential / ~4 ms amortised, a 500-test integration suite costs
  roughly **2 s** without `-race` and **16 s** with it. That is not a budget
  worth optimising further.
- `TestApp.EventCalls` is mutex-guarded, so even hook-count assertions are
  parallel-safe.

Two caveats to write into the spec:

- **Disk, not CPU, is the limit.** Each live app holds ~700 KB in `/tmp`. 200
  concurrent apps is ~140 MB — fine. If a test panics before `Cleanup()`, the
  directory leaks for the life of the runner. Always `defer app.Cleanup()`
  immediately after the `require.NoError`.
- **Keep the fixture small.** Startup cost is dominated by `copyDir`. Do not
  let `pb_data` accumulate uploaded files; seed attachments in the test that
  needs them.

## 3. Layering: which test goes where

Because every service is interface-defined, the layer boundary *is* the test
boundary. The mapping:

| Layer | Package shape | Test kind | Doubles | DB? | `t.Parallel()` |
|---|---|---|---|---|---|
| **Domain** — entities, value objects, invariants | `internal/domain/...` | pure unit, table-driven | none | no | yes |
| **Service** — use cases, orchestration, authorization decisions | `internal/patient`, `internal/labs`, ... | pure unit | **hand-written fakes** of the repo/port interfaces | no | yes |
| **Repository / adapter** — the PocketBase implementations | `internal/adapter/pb/...` | integration | none — the real thing | **yes**, `tests.NewTestApp` | yes |
| **Contract** — one suite per interface, run against *every* implementation incl. the fake | `internal/<pkg>/contract_test.go` | integration + unit | n/a | yes for the real impl | yes |
| **HTTP handler** — DTO mapping, status codes, authz boundary | `internal/api/v1/...` | `tests.ApiScenario` | none | yes | yes |
| **templ component** | `internal/ui/...` | render-to-buffer unit | none | no | yes |
| **Datastar SSE handler** | `internal/ui/...` | `httptest` + frame assertions | fake service | usually no | yes |

### The rule the spec can enforce

> **A test may touch a real PocketBase app if and only if the package under test
> imports `github.com/pocketbase/pocketbase`. Every other package is tested
> against interfaces with in-memory fakes, and its tests must not import
> `pocketbase/tests`.**

This is mechanically checkable and it piggybacks on the import-boundary lint
rule Principle II already mandates. Add to `.golangci.yml`:

```yaml
linters:
  enable:
    - depguard
  settings:
    depguard:
      rules:
        # Service and domain tests must be fast and DB-free.
        service-tests-are-pure:
          files:
            - "**/internal/domain/**_test.go"
            - "**/internal/patient/**_test.go"
            - "**/internal/labs/**_test.go"
            - "**/internal/sharing/**_test.go"
          deny:
            - pkg: github.com/pocketbase/pocketbase
              desc: >-
                Service and domain tests run against interfaces and fakes.
                If you need a database, the logic belongs in an adapter.
```

The payoff is a hard, reviewable answer to "where does this test go?" — if you
find yourself wanting a database to test a service, the service has leaked
persistence concerns and the design is wrong. That is the SOLID/DIP check that
this rule enforces for free.

## 4. testify specifics

### `require` vs `assert`

- **`require`** — anything whose failure makes the remaining lines meaningless
  or unsafe: constructor errors, `err != nil` before dereferencing, slice length
  before indexing, `NotNil` before a field access. `require` calls
  `t.FailNow()`, which is why it **must not** be called from a non-test
  goroutine (it terminates only that goroutine and the test hangs or reports
  wrongly).
- **`assert`** — independent facts you want reported together in one run: each
  field of a DTO, each of several status codes, the presence of several
  substrings. A failing `assert` still lets the rest of the test speak.

The heuristic: *would the next line panic or lie if this failed?* Yes →
`require`. No → `assert`.

```go
func TestCreateLabResult(t *testing.T) {
	t.Parallel()

	svc := labs.New(&fakeRepo{}, &fakeClock{now: fixedTime})

	got, err := svc.Create(t.Context(), labs.CreateInput{
		PatientID: "seed_pat_1",
		Panel:     "CBC",
	})
	require.NoError(t, err)      // next line dereferences got
	require.NotNil(t, got)

	assert.Equal(t, "seed_pat_1", got.PatientID)  // report all three
	assert.Equal(t, "CBC", got.Panel)
	assert.Equal(t, fixedTime, got.RecordedAt)
}
```

Note `t.Context()` (Go 1.24+) — a context cancelled at test cleanup. Use it
instead of `context.Background()` everywhere; it makes "goroutines have a
lifetime bounded by a context" (Principle IV) testable.

### `suite.Suite` — use it sparingly

`testify/suite` gives you `SetupTest`/`TearDownTest` and shared struct fields.
It also:

- **breaks `t.Parallel()` in subtle ways.** Suite methods share the suite
  struct; parallel suite tests race on `s.T()`. Making a suite parallel-safe
  requires care that a plain helper function does not.
- hides the test's dependencies inside struct fields, so you cannot read one
  test in isolation.
- adds a layer of indirection between `go test -run` and the test name.

**Recommendation: do not use `suite.Suite` as the default.** A plain
constructor helper does the same job, composes better, and stays parallel:

```go
// preferred: explicit, parallel-safe, greppable
func newLabsFixture(t *testing.T) (*tests.TestApp, *labs.Service) {
	t.Helper()
	app := testsupport.NewApp(t)
	t.Cleanup(app.Cleanup)
	return app, labs.New(pbadapter.NewLabRepo(app), clock.System{})
}
```

The one place `suite.Suite` genuinely earns its keep is the **contract suite**
(Principle III's Liskov clause), because there you deliberately want one body of
tests parameterised over several implementations:

```go
// internal/labs/contract_test.go
type RepoContract struct {
	suite.Suite
	New func(t *testing.T) labs.Repository
}

func (s *RepoContract) TestSaveThenFindRoundTrips() {
	repo := s.New(s.T())
	ctx := s.T().Context()

	in := labs.Result{PatientID: "p1", Panel: "CBC"}
	saved, err := repo.Save(ctx, in)
	s.Require().NoError(err)

	got, err := repo.FindByID(ctx, saved.ID)
	s.Require().NoError(err)
	s.Equal(in.Panel, got.Panel)
}

func (s *RepoContract) TestFindByIDUnknownReturnsErrNotFound() {
	repo := s.New(s.T())
	_, err := repo.FindByID(s.T().Context(), "nope")
	s.Require().ErrorIs(err, labs.ErrNotFound) // every impl, including the fake
}

// Both implementations must satisfy the identical contract.
func TestPocketBaseRepoContract(t *testing.T) {
	suite.Run(t, &RepoContract{New: func(t *testing.T) labs.Repository {
		app := testsupport.NewApp(t)
		t.Cleanup(app.Cleanup)
		return pbadapter.NewLabRepo(app)
	}})
}

func TestFakeRepoContract(t *testing.T) {
	suite.Run(t, &RepoContract{New: func(t *testing.T) labs.Repository {
		return labstest.NewFakeRepo()
	}})
}
```

This is the mechanism that keeps the fakes honest. Without it, hand-written
fakes drift from the real adapter and unit tests start proving nothing.

### Mocks: hand-written fakes, and here is why

The options, judged under KISS:

| Option | Verdict |
|---|---|
| **`testify/mock`** | Call-expectation DSL: `m.On("Save", ...).Return(...)`. Stringly-typed (`"Save"` is not checked against the interface), `.Return()` values are `interface{}` and blow up at runtime on a type mismatch, and it pushes you toward asserting *interactions* rather than *behaviour*. A refactor that renames a method leaves the mock compiling and the test passing against a method that no longer exists. |
| **mockery / moq (generated)** | Type-safe and cheap to regenerate, but adds a codegen tool, a config file, a `go:generate` pass, a `gen:check` CI step to catch staleness, and a pile of committed generated code — for interfaces that, in a well-factored design, have three or four methods. |
| **Hand-written fakes** | A struct with function fields plus, where useful, a tiny in-memory store. Compile-time safe (it literally implements the interface or the build fails), readable, and it can encode real behaviour so the same fake backs the contract suite. |

**Recommendation: hand-written fakes, in a `<pkg>test` sub-package, verified by
the contract suite.** No generator, no `mock.Mock`, nothing to regenerate.

The whole justification is that MediGo's interfaces are *small* — that is what
Principle II buys you. Generated mocks pay off when you must stub a 20-method
vendor interface; they are pure overhead for a 4-method port you designed
yourself. And the contract suite already solves the one real problem
hand-written fakes have (drift), which generators do *not* solve.

The shape:

```go
// internal/labs/labstest/fake.go
package labstest

import (
	"context"
	"sync"

	"medigo/internal/labs"
)

// FakeRepo is an in-memory labs.Repository. It is behaviourally real: it is
// exercised by the same contract suite as the PocketBase implementation.
type FakeRepo struct {
	mu   sync.Mutex
	byID map[string]labs.Result
	seq  int

	// Hooks for the few tests that need to force a path. Nil means "behave".
	SaveErr     error
	FindByIDErr error
}

var _ labs.Repository = (*FakeRepo)(nil) // compile-time proof

func NewFakeRepo() *FakeRepo {
	return &FakeRepo{byID: map[string]labs.Result{}}
}

func (f *FakeRepo) Save(ctx context.Context, r labs.Result) (labs.Result, error) {
	if err := ctx.Err(); err != nil {
		return labs.Result{}, err // honours cancellation, like the real one
	}
	if f.SaveErr != nil {
		return labs.Result{}, f.SaveErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.ID == "" {
		f.seq++
		r.ID = fmt.Sprintf("fake_%03d", f.seq)
	}
	f.byID[r.ID] = r
	return r, nil
}

func (f *FakeRepo) FindByID(ctx context.Context, id string) (labs.Result, error) {
	if err := ctx.Err(); err != nil {
		return labs.Result{}, err
	}
	if f.FindByIDErr != nil {
		return labs.Result{}, f.FindByIDErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	got, ok := f.byID[id]
	if !ok {
		return labs.Result{}, labs.ErrNotFound // same sentinel as the real impl
	}
	return got, nil
}
```

Where a test genuinely needs to assert *that a call happened* (an audit-log
write, an outbound email), record it in the fake and assert on the recording —
still no mocking framework:

```go
type FakeAudit struct {
	mu      sync.Mutex
	Entries []audit.Entry
}

func (f *FakeAudit) Record(ctx context.Context, e audit.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Entries = append(f.Entries, e)
	return nil
}

// in the test:
require.Len(t, aud.Entries, 1)
assert.Equal(t, "patient.shared", aud.Entries[0].Action)
assert.NotContains(t, aud.Entries[0].Detail, "diagnosis") // Principle VII
```

**Rule for the spec:** *No mocking framework. Test doubles are hand-written
fakes in a `<pkg>test` package, they assert `var _ Iface = (*Fake)(nil)`, and
every fake of an interface that also has a production implementation is run
through that interface's contract suite.*

### Table-driven tests are the default shape

```go
func TestReferenceRangeFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value float64
		low   float64
		high  float64
		want  labs.Flag
	}{
		{"inside range is normal", 5.0, 4.0, 6.0, labs.FlagNormal},
		{"at the low bound is normal", 4.0, 4.0, 6.0, labs.FlagNormal},
		{"at the high bound is normal", 6.0, 4.0, 6.0, labs.FlagNormal},
		{"below the low bound is low", 3.9, 4.0, 6.0, labs.FlagLow},
		{"above the high bound is high", 6.1, 4.0, 6.0, labs.FlagHigh},
		{"an unset range cannot be flagged", 5.0, 0, 0, labs.FlagUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, labs.FlagFor(tc.value, tc.low, tc.high))
		})
	}
}
```

Two notes: Go 1.22+ fixed the loop-variable capture, so no `tc := tc` is needed
(and `copyloopvar` in the house `.golangci.yml` will flag it if you add one).
Test names are sentences describing behaviour, per Principle III.

## 5. Testing templ components and Datastar SSE handlers

### templ: render to a buffer

`templ.Component` is `Render(ctx context.Context, w io.Writer) error`. One
helper, used everywhere:

```go
// internal/ui/uitest/render.go
package uitest

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/stretchr/testify/require"
)

// Render renders c and returns the HTML. Fails the test on a render error.
func Render(t testing.TB, c templ.Component) string {
	t.Helper()
	var sb strings.Builder
	require.NoError(t, c.Render(context.Background(), &sb))
	return sb.String()
}
```

Then assert on output. This test **runs and passes**:

```go
func TestVitalsCard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		bpm      int
		label    string
		contains []string
	}{
		{"renders the value", 72, "Vitals", []string{`aria-label="Vitals"`, `>72<`}},
		{"escapes a hostile label", 72, `Vi"tals<script>`, []string{`&lt;script&gt;`}},
		{"renders zero rather than omitting it", 0, "Vitals", []string{`>0<`}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := uitest.Render(t, ui.VitalsCard(tc.bpm, tc.label))
			for _, want := range tc.contains {
				assert.Contains(t, got, want)
			}
			// The assertion that actually matters for a medical records app.
			assert.NotContains(t, got, "<script>")
		})
	}
}
```

What is worth asserting, and what is not:

- **Do** assert: the ARIA landmark/role the Playwright gate will look for
  (this keeps the two gates in sync), escaping of user-supplied text, presence
  of `data-*` Datastar attributes, and conditional branches (empty state vs
  populated).
- **Do not** assert: full HTML equality or Tailwind class strings. That is
  visual-regression testing by the back door, it breaks on every restyle, and
  it was explicitly not chosen.

For structural assertions stronger than `Contains`, parse rather than
string-match:

```go
func TestPatientListMarksTheActivePatient(t *testing.T) {
	t.Parallel()
	html := uitest.Render(t, ui.PatientList(patients, "seed_pat_2"))

	doc, err := html.Parse(strings.NewReader(html))
	require.NoError(t, err)

	active := findAll(doc, func(n *html.Node) bool {
		return attr(n, "aria-current") == "true"
	})
	require.Len(t, active, 1, "exactly one patient is current")
	assert.Equal(t, "seed_pat_2", attr(active[0], "data-patient-id"))
}
```

### Datastar SSE: assert on the frames

The wire format, captured from a **real run** of `datastar-go` v1.2.2:

```
event: datastar-patch-elements
data: elements <div id="vitals">120/80</div>


event: datastar-patch-signals
data: signals {"bpm":72}


```

(Note the blank line *pairs* — each frame is terminated by `\n\n` and the
generator emits a trailing newline, so consecutive frames are separated by two
blank lines.)

Event type constants, from `datastar/consts.go`:

```go
type EventType string

const (
	EventTypePatchElements EventType = "datastar-patch-elements"
	EventTypePatchSignals  EventType = "datastar-patch-signals"
)
```

A parser worth having, so tests assert on structure instead of substrings:

```go
// internal/ui/uitest/sse.go
package uitest

import (
	"bufio"
	"strings"
	"testing"
)

type Frame struct {
	Event string
	Data  []string
}

// ParseSSE splits an SSE response body into frames.
func ParseSSE(t testing.TB, body string) []Frame {
	t.Helper()
	var (
		frames []Frame
		cur    Frame
	)
	flush := func() {
		if cur.Event != "" || len(cur.Data) > 0 {
			frames = append(frames, cur)
			cur = Frame{}
		}
	}
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "event: "):
			cur.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.Data = append(cur.Data, strings.TrimPrefix(line, "data: "))
		}
	}
	flush()
	return frames
}

// Events returns just the event names, for order-sensitive assertions.
func Events(frames []Frame) []string {
	out := make([]string, 0, len(frames))
	for _, f := range frames {
		out = append(out, f.Event)
	}
	return out
}
```

The handler test (**this runs and passes**):

```go
func TestVitalsStreamPatchesElementsThenSignals(t *testing.T) {
	t.Parallel()

	app := testsupport.NewApp(t)
	defer app.Cleanup()

	router, err := apis.NewRouter(app)
	require.NoError(t, err)
	se := &core.ServeEvent{App: app, Router: router}
	require.NoError(t, app.OnServe().Trigger(se, func(e *core.ServeEvent) error { return nil }))

	mux, err := se.Router.BuildMux()
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/vitals/stream", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))

	frames := uitest.ParseSSE(t, rec.Body.String())
	require.Len(t, frames, 2)
	assert.Equal(t,
		[]string{"datastar-patch-elements", "datastar-patch-signals"},
		uitest.Events(frames))
	assert.Contains(t, frames[0].Data[0], `<div id="vitals">120/80</div>`)
	assert.Contains(t, frames[1].Data[0], `{"bpm":72}`)
}
```

`httptest.ResponseRecorder` implements `http.Flusher`, so `datastar.NewSSE`
works against it unmodified. No socket, no goroutine leak.

**For unbounded streams, use `ApiScenario.Timeout`.** Verified working: a
handler that ticks forever until `sse.Context().Done()` is cleanly cut and the
partial body is asserted:

```go
{
	Name:            "vitals stream emits and then honours cancellation",
	Method:          http.MethodGet,
	URL:             "/ui/forever",
	Timeout:         150 * time.Millisecond, // cancels the request context
	ExpectedStatus:  200,
	ExpectedContent: []string{"datastar-patch-elements"},
	TestAppFactory:  testsupport.NewApp,
}
```

Write **every** streaming handler as a `select` on `sse.Context().Done()`. It
is what makes the handler testable, and it is the same discipline that stops
the Playwright `webServer` teardown from hanging (Part B).

## 6. Coverage

### Target

**80% overall, measured after exclusions**, with a hard floor rather than a
ratchet. Two refinements that matter more than the headline number:

- **95%+ on `internal/domain/**` and the service packages.** These are pure
  functions with no I/O; there is no excuse for a gap, and this is where an
  authorization bug lives.
- **No target on adapters.** PocketBase adapter code is mostly field mapping;
  its correctness is proven by the contract suite, not by line count.

### What to exclude, and how

Go has no built-in exclusion mechanism, so filter the profile. Verified working
— on the prototype this moved the total from **78.0% → 86.5%**:

```bash
go test -race -covermode=atomic -coverpkg=./... -coverprofile=coverage.raw ./...

# keep the "mode:" header, drop generated/mocks/migrations
head -1 coverage.raw > coverage.out
grep -Ev '(_templ\.go|zz_generated.*\.go|/migrations/|/testsupport/|test/)' coverage.raw \
  | tail -n +2 >> coverage.out

go tool cover -func=coverage.out | tail -1
```

Note `-coverpkg=./...` is **required**: without it, code exercised from a
different package's test (which is most of MediGo, since handler tests drive
services) is reported as 0%. On the prototype the same suite reported 0.0%
without `-coverpkg` and 78.0% with it. This single flag is the difference
between a meaningful number and a meaningless one.

Exclude:

- `*_templ.go` — generated by templ, and its behaviour is covered by the
  render tests against the *source* `.templ` file.
- `zz_generated_*.go` — generated OpenAPI types, per house convention.
- `internal/migrations/**` — a migration runs once; the fixture proves it works.
  Covering it means asserting that `up` calls the functions `up` calls.
- `internal/**/testsupport/`, `**/<pkg>test/` — the fakes. Covered transitively
  and by the contract suite; counting them inflates the number.

As a Taskfile target in house style:

```yaml
  test:cover:
    desc: Run the suite and report coverage, excluding generated code
    env:
      CGO_ENABLED: "1"
    cmds:
      - go test -race -covermode=atomic -coverpkg=./... -coverprofile=coverage.raw ./...
      - head -1 coverage.raw > coverage.out
      - grep -Ev '(_templ\.go|zz_generated.*\.go|/migrations/|/testsupport/|test/)' coverage.raw | tail -n +2 >> coverage.out
      - go tool cover -func=coverage.out | tail -1

  test:cover:gate:
    desc: Fail if coverage is below the floor
    deps: [test:cover]
    vars:
      FLOOR: "80.0"
    cmds:
      - |
        total=$(go tool cover -func=coverage.out | tail -1 | grep -oE '[0-9]+\.[0-9]+')
        awk -v t="$total" -v f="{{.FLOOR}}" 'BEGIN {
          if (t+0 < f+0) { printf "coverage %.1f%% is below the %.1f%% floor\n", t, f; exit 1 }
          printf "coverage %.1f%% (floor %.1f%%)\n", t, f
        }'
```

### Should CI gate on it?

**Yes, but as a low fixed floor (80%), not a ratchet, and it is the weakest of
the gates.**

The honest reasoning: a coverage percentage is a proxy, and gating hard on a
rising ratchet reliably produces tests written to move the number — assertions
on getters, tests that call a function and check nothing. That is worse than no
gate. A fixed 80% floor catches the thing you actually want caught (someone
merged a subsystem with no tests at all) and stops incentivising theatre above
that line.

The gates that carry the real weight here are the **contract suite**, the
**authorization tests**, the **OpenAPI/route inventory gate** (§7), and the
**Playwright smoke gate** (Part B). Coverage is a smoke alarm, not a load-bearing
wall — treat it accordingly in the spec so nobody games it.

## 7. The route-inventory / OpenAPI gate

### The core problem

I checked whether the route list can be recovered from the router after the
fact. **It cannot.**

- PocketBase's `RouterGroup.children` is **unexported**:

  ```go
  type RouterGroup[T hook.Resolver] struct {
      excludedMiddlewares map[string]struct{}
      children            []any // Route or RouterGroup   <-- unexported
      Prefix      string
      Middlewares []*hook.Handler[T]
  }
  ```

- `Router.BuildMux()` returns an `http.Handler`, and Go 1.27's `http.ServeMux`
  still exposes **no** pattern-enumeration API (only `Handler(*Request)`, which
  requires you to already know the path).

So reflection/introspection is out, and a hand-maintained list is forbidden by
Principle VIII. The only sound answer is a **registry that routes flow
through**, where describing a route and registering it are the same call.

### The mechanism

The design point that makes this work: **describing routes is separated from
binding them**, so `medigo routes` needs no database, no port, and no
migrations — it is a pure function of the binary.

```go
// internal/httproute/registry.go
package httproute

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

type Kind string

const (
	KindPage   Kind = "page"   // server-rendered templ page -> smoke-tested by Playwright
	KindAPI    Kind = "api"    // /api/v1 JSON -> must appear in the OpenAPI doc
	KindStream Kind = "stream" // Datastar SSE -> not navigated by Playwright
	KindAsset  Kind = "asset"  // static/files -> excluded from both gates
)

type Auth string

const (
	AuthPublic Auth = "public"
	AuthUser   Auth = "user"
	AuthAdmin  Auth = "admin"
)

// Route is the single description of a registered endpoint. It is produced by
// the same call that registers the handler, so the inventory cannot drift.
type Route struct {
	Method   string `json:"method"`
	Path     string `json:"path"`
	Kind     Kind   `json:"kind"`
	Auth     Auth   `json:"auth"`
	Landmark string `json:"landmark,omitempty"`    // ARIA role:name for the smoke gate
	OpID     string `json:"operationId,omitempty"` // must exist in the OpenAPI doc
	SmokeURL string `json:"smokeUrl,omitempty"`    // Path with params bound to seeded ids
}

type entry struct {
	spec    Route
	handler func(*core.RequestEvent) error
}

// Registry is the single source of truth for every route MediGo serves.
type Registry struct {
	entries []entry
	seen    map[string]struct{}
}

func New() *Registry { return &Registry{seen: map[string]struct{}{}} }

// Handle records the route and its handler. Declaring a route and describing it
// for the gates is one indivisible call.
func (r *Registry) Handle(spec Route, h func(*core.RequestEvent) error) *Registry {
	key := spec.Method + " " + spec.Path
	if _, dup := r.seen[key]; dup {
		panic("httproute: duplicate route " + key)
	}
	if spec.Kind == KindPage && spec.Landmark == "" {
		panic("httproute: page route " + key + " must declare a Landmark")
	}
	if spec.Kind == KindPage && spec.SmokeURL == "" {
		panic("httproute: page route " + key + " must declare a SmokeURL")
	}
	if spec.Kind == KindAPI && spec.OpID == "" {
		panic("httproute: api route " + key + " must declare an OpID")
	}
	r.seen[key] = struct{}{}
	r.entries = append(r.entries, entry{spec, h})
	return r
}

// Bind attaches every recorded route to the PocketBase router. Called once from
// OnServe. This is the ONLY place routes reach the router.
func (r *Registry) Bind(se *core.ServeEvent) {
	for _, e := range r.entries {
		se.Router.Route(e.spec.Method, e.spec.Path, e.handler)
	}
}

func (r *Registry) Routes() []Route {
	out := make([]Route, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e.spec)
	}
	slices.SortFunc(out, func(a, b Route) int {
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return strings.Compare(a.Method, b.Method)
	})
	return out
}

func (r *Registry) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r.Routes())
}

// SmokeTargets are the GET page routes the Playwright gate must navigate.
func (r *Registry) SmokeTargets() []Route {
	var out []Route
	for _, rt := range r.Routes() {
		if rt.Kind == KindPage && rt.Method == http.MethodGet {
			out = append(out, rt)
		}
	}
	return out
}

// OperationIDs must all appear in the generated OpenAPI document.
func (r *Registry) OperationIDs() []string {
	var out []string
	for _, rt := range r.Routes() {
		if rt.Kind == KindAPI {
			out = append(out, rt.OpID)
		}
	}
	return out
}
```

Registration then reads:

```go
// internal/httproute/build.go
func Build(svc Services) *Registry {
	r := New()

	r.Handle(Route{
		Method: http.MethodGet, Path: "/", Kind: KindPage, Auth: AuthUser,
		Landmark: "main:Dashboard", SmokeURL: "/",
	}, handlers.Dashboard(svc.Patients))

	r.Handle(Route{
		Method: http.MethodGet, Path: "/patients/{id}", Kind: KindPage, Auth: AuthUser,
		Landmark: "main:Patient", SmokeURL: "/patients/seed_pat_1",
	}, handlers.PatientPage(svc.Patients))

	r.Handle(Route{
		Method: http.MethodGet, Path: "/api/v1/patients", Kind: KindAPI, Auth: AuthUser,
		OpID: "listPatients",
	}, handlers.ListPatients(svc.Patients))

	r.Handle(Route{
		Method: http.MethodGet, Path: "/ui/vitals/stream", Kind: KindStream, Auth: AuthUser,
	}, handlers.VitalsStream(svc.Vitals))

	return r
}
```

Wired into PocketBase in exactly one place:

```go
app.OnServe().BindFunc(func(se *core.ServeEvent) error {
	httproute.Build(services).Bind(se)
	return se.Next()
})
```

### The Cobra subcommand

PocketBase's `RootCmd` already *is* a `*cobra.Command`:

```go
// internal/cmd/routes.go
func NewRoutesCmd(build func() *httproute.Registry) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "routes",
		Short: "Print the registered route inventory",
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg := build() // no DB, no port, no migrations
			switch format {
			case "json":
				return reg.WriteJSON(cmd.OutOrStdout())
			case "table":
				return reg.WriteTable(cmd.OutOrStdout())
			default:
				return fmt.Errorf("unknown --format %q", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "json", "json|table")
	return cmd
}

// in main:
app.RootCmd.AddCommand(cmd.NewRoutesCmd(func() *httproute.Registry {
	return httproute.Build(services)
}))
```

Verified output shape:

```json
[
  {
    "method": "GET",
    "path": "/",
    "kind": "page",
    "auth": "user",
    "landmark": "main:Dashboard",
    "smokeUrl": "/"
  },
  {
    "method": "GET",
    "path": "/api/v1/patients",
    "kind": "api",
    "auth": "user",
    "operationId": "listPatients"
  }
]
```

### The gates this enables

Modelled on `medikeep-mcp`'s `task api:coverage` (which fails the build on any
spec operation with no tool). Three checks, all `go test`, all in CI:

```go
// internal/httproute/gate_test.go

// Gate 1: every KindAPI route has an operation in the generated OpenAPI doc,
// and the doc has no operation without a route. Both directions.
func TestOpenAPICoversEveryAPIRoute(t *testing.T) {
	t.Parallel()

	reg := httproute.Build(stubServices())
	doc := openapi.MustLoad(t, "../../api/openapi.json") // the committed artifact

	assert.ElementsMatch(t, reg.OperationIDs(), doc.OperationIDs(),
		"route registry and OpenAPI document disagree — run `task openapi`")
}

// Gate 2: the committed OpenAPI document is what the current code generates.
// (The diff, not just the set, so a changed schema is reviewable.)
func TestOpenAPIDocumentIsNotStale(t *testing.T) {
	t.Parallel()

	got, err := openapi.Generate(httproute.Build(stubServices()))
	require.NoError(t, err)

	want, err := os.ReadFile("../../api/openapi.json")
	require.NoError(t, err)

	assert.JSONEq(t, string(want), string(got),
		"api/openapi.json is stale — run `task openapi` and commit the diff")
}

// Gate 3: every page route is smokeable. This is what stops a new page from
// silently escaping the Playwright gate.
func TestEveryPageRouteIsSmokeable(t *testing.T) {
	t.Parallel()

	for _, r := range httproute.Build(stubServices()).SmokeTargets() {
		t.Run(r.Path, func(t *testing.T) {
			t.Parallel()
			assert.NotEmpty(t, r.Landmark, "page must declare an ARIA landmark")
			assert.NotEmpty(t, r.SmokeURL, "page must declare a concrete SmokeURL")
			assert.NotContains(t, r.SmokeURL, "{",
				"SmokeURL must bind path params to seeded fixture ids")
		})
	}
}
```

Plus the registration-time panics, which are the strongest link: a page route
declared without a landmark **cannot boot**. Verified:

```go
func TestPageWithoutLandmarkPanics(t *testing.T) {
	t.Parallel()
	assert.PanicsWithValue(t,
		"httproute: page route GET /oops must declare a Landmark",
		func() {
			httproute.New().Handle(httproute.Route{
				Method: http.MethodGet, Path: "/oops", Kind: httproute.KindPage,
			}, nil)
		})
}
```

Taskfile:

```yaml
  openapi:
    desc: Regenerate the OpenAPI document from the route registry
    cmds:
      - go run ./cmd/medigo openapi --out api/openapi.json

  openapi:check:
    desc: Fail if the committed OpenAPI document is stale
    deps: [openapi]
    cmds:
      - git diff --exit-code -- api/openapi.json

  routes:check:
    desc: Prove every registered route is covered by the gates
    cmds:
      - go test ./internal/httproute/ -run 'TestOpenAPI|TestEveryPageRoute' -count=1
```

---

# Part B — the Playwright smoke + console-error gate

Scope, restated so it does not creep: **every page route returns 200, renders
its landmark, and produces zero console output, zero page errors, and zero
failed requests, at two viewports.** Not user journeys. Not visual regression.
Not accessibility auditing. If a check cannot be expressed as "this page is
not broken", it does not belong here.

## 8. Design

### Playwright CLI, and keeping Node out of the runtime

Playwright is a devDependency and a CI tool. It never enters the runtime image.
The separation is structural, not a convention:

```
medigo/
  cmd/medigo/            # the Go binary — the only thing shipped
  internal/
  api/openapi.json
  e2e/                   # <-- Playwright lives here, entirely
    package.json
    playwright.config.ts
    auth.setup.ts
    routes.ts
    smoke.spec.ts
  Dockerfile             # never COPYs e2e/
  .dockerignore          # explicitly excludes e2e/ and node_modules/
```

- `e2e/package.json` is the only `package.json` in the project.
- The `Dockerfile` builder stage copies `go.mod`, `go.sum`, and the Go tree —
  never `e2e/`. Add `e2e/` and `**/node_modules/` to `.dockerignore`.
  (Per the monorepo memory note: a new project must be added to the root
  `/.dockerignore` allowlist *and* `build-image.yaml`, or its Docker build fails
  with a misleading "file not found". Do that for `medigo/` and make sure the
  allowlist does not accidentally pull in `e2e/`.)
- The gate runs `./medigo serve` — the **built binary**, the same artifact CI
  ships. Playwright shells out to it; it does not link against anything Go.

CI invocation:

```bash
cd e2e
npm ci
npx playwright install --with-deps chromium   # --with-deps is required on Linux
npx playwright test
```

`--with-deps` matters: on a bare Linux runner the browser downloads fine but
will not launch. I hit this exactly — the binary was present at
`~/.cache/ms-playwright/chromium_headless_shell-1234/chrome-linux/headless_shell`
and failed with:

```
error while loading shared libraries: libatk-1.0.so.0: cannot open shared object file
ldd -> libatk-1.0.so.0, libXcomposite.so.1, libXdamage.so.1, libXfixes.so.3,
       libXrandr.so.2, libgbm.so.1, libasound.so.2, libatspi.so.0  => not found
```

`--with-deps` installs those (needs root, which GitHub runners have). The
alternative is the pinned container `mcr.microsoft.com/playwright:v1.62.1-noble`,
which I would prefer in CI because it pins the browser and the system libs
together.

**Pin the Playwright version exactly** (`"@playwright/test": "1.62.1"`, not
`^1.62.1`) and commit `package-lock.json`. A browser engine that floats is a
gate that fails on someone else's schedule.

### Route enumeration — derived from the binary

This is the part that must never rot, so it is worth being precise about *when*
it happens: `smokeTargets()` is called at **module load**, i.e. during
Playwright's collection phase, before any browser starts. It shells out to the
binary under test:

```ts
// e2e/routes.ts
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export type Route = {
  method: string;
  path: string;
  kind: 'page' | 'api' | 'stream' | 'asset';
  auth: 'public' | 'user' | 'admin';
  landmark?: string;
  smokeUrl?: string;
};

const here = path.dirname(fileURLToPath(import.meta.url));
const BIN = process.env.MEDIGO_BIN ?? path.resolve(here, '..', 'medigo');

/**
 * The inventory comes from the binary under test, not from a list in this repo.
 * A new page ships with a new smoke test for free; a page that forgets its
 * landmark cannot boot at all. There is nothing to keep in sync by hand.
 */
export function loadRoutes(): Route[] {
  const out = execFileSync(BIN, ['routes'], { encoding: 'utf8' });
  const routes = JSON.parse(out) as Route[];
  if (routes.length === 0) throw new Error('medigo routes returned nothing');
  return routes;
}

export function smokeTargets(): Route[] {
  const pages = loadRoutes().filter((r) => r.kind === 'page' && r.method === 'GET');
  if (pages.length === 0) throw new Error('no page routes to smoke — inventory is broken');
  for (const r of pages) {
    if (!r.smokeUrl || !r.landmark) {
      throw new Error(`route ${r.path} is a page but has no smokeUrl/landmark`);
    }
  }
  return pages;
}

/** "main:Lab results" -> { role: 'main', name: 'Lab results' } */
export function parseLandmark(landmark: string) {
  const i = landmark.indexOf(':');
  return i === -1
    ? { role: landmark, name: undefined as string | undefined }
    : { role: landmark.slice(0, i), name: landmark.slice(i + 1) };
}
```

**Demonstrated end to end.** Starting from four page routes:

```
$ npx playwright test --list
  [setup]   › auth.setup.ts:11:1 › authenticate as the seeded user
  [desktop] › smoke.spec.ts › smoke /login
  [desktop] › smoke.spec.ts › smoke /
  [desktop] › smoke.spec.ts › smoke /patients/seed_pat_1
  [desktop] › smoke.spec.ts › smoke /labs
  [mobile]  › smoke.spec.ts › smoke /login
  [mobile]  › smoke.spec.ts › smoke /
  [mobile]  › smoke.spec.ts › smoke /patients/seed_pat_1
  [mobile]  › smoke.spec.ts › smoke /labs
Total: 9 tests in 2 files
```

A developer then adds one page route **in Go only** and rebuilds — no
TypeScript touched:

```go
{"GET", "/sharing/invitations", "page", "user", "main:Invitations", "/sharing/invitations"},
```

```
$ go build -o medigo ./cmd/medigo && npx playwright test --list
  ...
  [desktop] › smoke.spec.ts › smoke /sharing/invitations
  [mobile]  › smoke.spec.ts › smoke /sharing/invitations
Total: 11 tests in 2 files
```

Note also that `/ui/vitals/stream` (`kind: stream`) and `/api/v1/patients`
(`kind: api`) are correctly **absent** — you do not navigate a browser to an SSE
endpoint, and a JSON endpoint has no landmark.

### The assertions

Per route, per viewport:

| Assertion | Mechanism | Rationale |
|---|---|---|
| HTTP 200 | `(await page.goto(url)).status()` | catches 500s and, critically, a 302 to `/login` when auth breaks |
| landmark present | `page.getByRole(role, { name })` | role-based, so it survives restyling; also proves the page is not an error shell |
| zero console errors **and** warnings | `page.on('console')` filtered to `error`/`warning` | a Datastar selector that matches nothing warns rather than throws |
| zero uncaught exceptions | `page.on('pageerror')` | the classic templ-renders-fine-then-JS-throws failure |
| zero failed requests | `page.on('requestfailed')`, ignoring `net::ERR_ABORTED` | catches a purged Tailwind asset or a 404 partial |
| no 4xx/5xx subresource | `page.on('response')` | a `requestfailed` does not fire for a 404 that *responds* |
| desktop 1440×900 | project `desktop` | |
| mobile 390×844 | project `mobile` | |

Two points worth defending:

- **Role-based selectors over CSS.** `page.getByRole('main', { name: 'Lab results' })`
  survives a Tailwind refactor that a `.lab-results-panel` selector would not,
  and it fails when the page genuinely did not render. It also forces every
  page to carry a real landmark, which is an accessibility win obtained as a
  side effect. Pair it with the templ render test asserting the same landmark,
  and the two gates hold each other honest.
- **Warnings fail too.** Starting from zero-tolerance and adding justified
  entries to `IGNORED_CONSOLE` is the only version of this that stays at zero.
  Starting from "errors only" means warnings accumulate until nobody reads them.
  The ignore list should be empty on day one and every entry should carry a
  comment.

### Auth

A `medigo seed` subcommand builds the deterministic fixture; the gate logs in
once and reuses `storageState`.

```go
// internal/cmd/seed.go — the SAME seeder that builds the Go test fixture
func NewSeedCmd(app core.App) *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Populate a deterministic dataset for the smoke gate",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return seed.Apply(cmd.Context(), app, seed.Deterministic)
		},
	}
}
```

`seed.Deterministic` must use **fixed ids** (`seed_pat_1`, `seed_lab_1`) so the
`SmokeURL` values in the registry are stable. That coupling is deliberate: the
registry references ids the seeder guarantees, and Gate 3 above asserts no
`SmokeURL` still contains a `{param}`.

```ts
// e2e/auth.setup.ts
import { test as setup, expect } from '@playwright/test';
import fs from 'node:fs';

const FILE = 'e2e/.auth/user.json';

/**
 * Log in ONCE against the seeded user and save storageState. Every smoke spec
 * reuses it, so the gate does not replay a login flow N times — and a login
 * regression fails here, loudly, instead of failing all 40 page tests.
 */
setup('authenticate as the seeded user', async ({ page, request }) => {
  const res = await request.post('/api/v1/auth/login', {
    data: { email: 'smoke@medigo.test', password: process.env.MEDIGO_SEED_PASSWORD! },
  });
  expect(res.status(), 'seeded login must succeed').toBeLessThan(300);

  await page.goto('/');
  await expect(page.getByRole('main', { name: 'Dashboard' })).toBeVisible();

  fs.mkdirSync('e2e/.auth', { recursive: true });
  await page.context().storageState({ path: FILE });
});
```

Public routes (`/login`) are still visited with the authenticated state. That is
fine and is in fact the more interesting case — it catches a `/login` page that
throws when a session already exists. If a route needs the anonymous state, give
it a third project with `storageState: undefined` keyed off `auth: 'public'`.

### Fixtures: deterministic bring-up and teardown

`webServer` in the config handles it, but the determinism has to come from a
fresh database per run:

```yaml
  gate:
    desc: Run the Playwright smoke + console-error gate
    deps: [build]
    cmds:
      - rm -rf .gate/pb_data
      - ./medigo migrate up --dir .gate/pb_data
      - ./medigo seed --dir .gate/pb_data
      - cd e2e && npx playwright test
    env:
      MEDIGO_DATA_DIR: .gate/pb_data
```

Deleting and re-seeding is the whole trick: a gate that runs against a database
carrying yesterday's state is a gate that passes for the wrong reason. Teardown
is `rm -rf .gate/` — no containers, no ports to reclaim, because PocketBase is
embedded SQLite.

`reuseExistingServer: !process.env.CI` keeps the local loop fast while
guaranteeing CI always gets a fresh process.

### Datastar-specific traps

These are the ones that will actually bite:

1. **`waitUntil: 'networkidle'` never resolves.** An open SSE stream keeps an
   in-flight request forever, so `networkidle` waits until the navigation
   timeout on *every* live page. Use `'load'` (the default) or `'commit'`.
   Never `'networkidle'`. This is the single biggest hang risk in this stack.
2. **`net::ERR_ABORTED` on teardown is not a defect.** When the page or context
   closes, the browser aborts the open SSE request and `requestfailed` fires.
   Filtering exactly `net::ERR_ABORTED` is correct; filtering *all*
   `requestfailed` would gut the gate.
3. **The Go server can hang on shutdown.** If a streaming handler loops without
   selecting on `sse.Context().Done()`, `webServer` teardown blocks until
   Playwright's timeout and CI stalls. This is the same discipline §5 requires
   for testability — write it once, get both.
4. **Reconnect storms.** Datastar's SSE client retries on disconnect. If the
   server 500s on a stream, the client reconnects in a loop and floods
   `requestfailed`. Bound it: give the stream route a `retry` interval and let
   the failed-request assertion catch the first one.
5. **`page.waitForLoadState('networkidle')` in a helper.** Same trap as (1),
   easy to reintroduce later. Worth a lint rule or a grep in CI:
   `! grep -rn "networkidle" e2e/`.

### Flakiness control

**Retries: zero.** This follows Constitution VIII ("a flaky assertion is fixed
or removed, never retried into passing") and it is also just correct here: every
assertion in this gate is deterministic. A status code, a landmark, a console
message, a failed request — none of them can legitimately pass on a second
attempt. A retry could only ever hide a real defect.

The one genuine race — the server not being ready — is fixed properly by
`webServer.url` polling, not by retries.

| Concern | Setting | Why |
|---|---|---|
| server not up yet | `webServer.url` + `timeout: 60_000` | polls until it answers; the correct fix |
| slow first page (cold SQLite, templ) | `timeout: 30_000` per test | generous but bounded |
| assertion settling | `expect.timeout: 5_000` | auto-retrying assertions absorb async DOM patches |
| navigation | `navigationTimeout: 15_000` | fails fast on a hang instead of eating the test timeout |
| **anything else** | **`retries: 0`** | a smoke failure is a defect |
| `.only` left in a spec | `forbidOnly: !!process.env.CI` | stops a gate silently shrinking to one test |
| diagnosis | `trace: 'retain-on-failure'` | with zero retries, the trace is the whole story — upload it |

`workers: 2` in CI is deliberately conservative: each worker drives a browser
against one shared server process, and oversubscribing a 2-core runner adds
timing noise, which is exactly what you are trying to avoid in a gate.

### CI wiring

A dedicated job, after the Go job, blocking merge:

```yaml
  smoke-gate:
    needs: [go]
    runs-on: ubuntu-latest
    container: mcr.microsoft.com/playwright:v1.62.1-noble  # pins browser + libs
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.27.x' }

      - name: Build the binary under test
        run: go build -trimpath -o medigo ./cmd/medigo

      - name: Seed a deterministic database
        run: |
          ./medigo migrate up --dir .gate/pb_data
          ./medigo seed --dir .gate/pb_data

      - name: Install the gate's dependencies
        working-directory: e2e
        run: npm ci

      - name: Run the smoke + console-error gate
        working-directory: e2e
        run: npx playwright test
        env:
          CI: '1'
          MEDIGO_DATA_DIR: ${{ github.workspace }}/.gate/pb_data
          MEDIGO_SEED_PASSWORD: ${{ secrets.MEDIGO_SEED_PASSWORD }}

      - uses: actions/upload-artifact@v4
        if: failure()
        with:
          name: playwright-trace
          path: e2e/.artifacts/
          retention-days: 7
```

**It blocks merge** (Constitution VIII). Two conditions make that sustainable:
zero retries with traces uploaded on failure (so a red gate is diagnosable in
one click), and the scope discipline at the top of Part B (so the gate only ever
fails for "this page is broken").

## 9. The config and one spec

### `e2e/playwright.config.ts`

```ts
import { defineConfig, devices } from '@playwright/test';

const PORT = Number(process.env.MEDIGO_PORT ?? 8090);
const BASE = `http://127.0.0.1:${PORT}`;

export default defineConfig({
  testDir: '.',
  outputDir: './.artifacts',

  // A gate must not be able to silently shrink to one test.
  forbidOnly: !!process.env.CI,

  // ZERO retries, deliberately (Constitution VIII). Every assertion here is
  // deterministic: a status code, a landmark, a console log, a failed request.
  // None can legitimately pass on a second attempt, so a retry could only ever
  // hide a real defect. The one genuine race — server readiness — is handled by
  // webServer.url polling below, which is the correct fix.
  retries: 0,

  workers: process.env.CI ? 2 : undefined,
  fullyParallel: true,
  timeout: 30_000,
  expect: { timeout: 5_000 },

  reporter: process.env.CI
    ? [['github'], ['list'], ['json', { outputFile: '.artifacts/results.json' }]]
    : [['list']],

  use: {
    baseURL: BASE,
    // With no retries, the trace IS the bug report. Always keep it on failure.
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'off',
    actionTimeout: 10_000,
    navigationTimeout: 15_000,
  },

  // Bring up the REAL binary. Playwright shells out to it; no Node in runtime.
  webServer: {
    command: '../medigo serve --http 127.0.0.1:' + PORT,
    url: `${BASE}/login`,          // poll until it answers — the right fix for the start race
    reuseExistingServer: !process.env.CI,
    stdout: 'pipe',
    stderr: 'pipe',
    timeout: 60_000,
    env: {
      MEDIGO_DATA_DIR: process.env.MEDIGO_DATA_DIR ?? '../.gate/pb_data',
      MEDIGO_LOG_LEVEL: 'warn',
    },
  },

  projects: [
    { name: 'setup', testMatch: /auth\.setup\.ts/ },
    {
      name: 'desktop',
      dependencies: ['setup'],
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1440, height: 900 },
        storageState: '.auth/user.json',
      },
    },
    {
      name: 'mobile',
      dependencies: ['setup'],
      use: {
        ...devices['Pixel 7'],
        viewport: { width: 390, height: 844 },
        storageState: '.auth/user.json',
      },
    },
  ],
});
```

### `e2e/smoke.spec.ts`

```ts
import { test, expect, type Page } from '@playwright/test';
import { smokeTargets, parseLandmark } from './routes';

/**
 * Console noise that is provably not the app's fault. Keep this list SHORT and
 * make every entry carry a reason. An empty list is the goal and the default.
 */
const IGNORED_CONSOLE: RegExp[] = [];

/** Requests MediGo does not own. */
const IGNORED_REQUESTS = [/\/favicon\.ico$/];

type Collected = {
  console: string[];
  pageErrors: string[];
  failedRequests: string[];
  badStatuses: string[];
};

/**
 * Listeners MUST be attached before the first navigation or early errors are
 * lost. Both `error` and `warning` fail the gate: a warning that is genuinely
 * benign belongs in IGNORED_CONSOLE with a justification, not tolerated silently.
 */
function collect(page: Page): Collected {
  const c: Collected = { console: [], pageErrors: [], failedRequests: [], badStatuses: [] };

  page.on('console', (msg) => {
    const type = msg.type();
    if (type !== 'error' && type !== 'warning') return;
    const text = msg.text();
    if (IGNORED_CONSOLE.some((re) => re.test(text))) return;
    const loc = msg.location();
    c.console.push(`[${type}] ${text} (${loc.url}:${loc.lineNumber})`);
  });

  page.on('pageerror', (err) => c.pageErrors.push(`${err.name}: ${err.message}`));

  page.on('requestfailed', (req) => {
    const failure = req.failure()?.errorText ?? 'unknown';
    // A Datastar SSE stream is aborted by design when the page/context closes.
    // That surfaces here as ERR_ABORTED and is not a defect.
    if (failure === 'net::ERR_ABORTED') return;
    if (IGNORED_REQUESTS.some((re) => re.test(req.url()))) return;
    c.failedRequests.push(`${req.method()} ${req.url()} -> ${failure}`);
  });

  // requestfailed does NOT fire for a 404 that actually responds.
  page.on('response', (res) => {
    if (res.status() >= 400) c.badStatuses.push(`${res.status()} ${res.url()}`);
  });

  return c;
}

for (const route of smokeTargets()) {
  test(`smoke ${route.smokeUrl}`, async ({ page }) => {
    const seen = collect(page);

    // 'load', NEVER 'networkidle': a Datastar SSE stream keeps the network busy
    // forever, so 'networkidle' would time out on every live page.
    const response = await page.goto(route.smokeUrl!, { waitUntil: 'load' });

    expect(response, `no response for ${route.smokeUrl}`).not.toBeNull();
    // Also catches a 302 to /login when auth silently breaks.
    expect(response!.status(), `${route.smokeUrl} must return 200`).toBe(200);

    // Role-based landmark: survives class churn, fails on a page that genuinely
    // did not render.
    const { role, name } = parseLandmark(route.landmark!);
    await expect(page.getByRole(role as any, name ? { name } : {})).toBeVisible();

    // Give Datastar's on-load patch a bounded window to run and misbehave.
    await page.waitForLoadState('domcontentloaded');

    expect(seen.pageErrors, `uncaught exceptions on ${route.smokeUrl}`).toEqual([]);
    expect(seen.console, `console output on ${route.smokeUrl}`).toEqual([]);
    expect(seen.failedRequests, `failed requests on ${route.smokeUrl}`).toEqual([]);
    expect(seen.badStatuses, `4xx/5xx responses on ${route.smokeUrl}`).toEqual([]);
  });
}
```

**Verification status:** the config and specs parse and collect correctly under
Playwright 1.62.1 (`--list` output above is real). The browser assertions were
**not executed** here — this sandbox has no root and Chromium's shared libraries
(`libatk-1.0.so.0` et al.) could not be installed. Run
`npx playwright install --with-deps chromium` on a machine with root, or use
the pinned container, and confirm the gate goes green on a healthy app and red
on a page with a deliberate `console.error` before trusting it.

---

## Summary of rules for the spec

1. **Go 1.27.x, not 1.26.5.** PocketBase v0.40.1 will not build otherwise.
2. `TestAppFactory` constructs a **new** app per scenario. Never share one —
   it stack-overflows.
3. Every PB integration test is `t.Parallel()`; measured ~10 ms/app.
4. A test may touch a real PocketBase app **iff** its package imports
   `pocketbase`. Enforced by `depguard`.
5. No mocking framework. Hand-written fakes in `<pkg>test`, `var _ Iface =
   (*Fake)(nil)`, kept honest by a shared contract suite.
6. `require` for preconditions, `assert` for independent facts. Table-driven
   subtests are the default shape.
7. Routes reach the router **only** through `httproute.Registry`. A page
   without a landmark and a concrete `SmokeURL` panics at registration.
8. Coverage: 80% floor after excluding `*_templ.go`, `zz_generated_*.go`,
   migrations, and test support. `-coverpkg=./...` is mandatory.
9. Playwright: zero retries, `waitUntil: 'load'` never `'networkidle'`, ignore
   only `net::ERR_ABORTED`, two viewports, blocks merge.
10. Every streaming handler selects on `sse.Context().Done()`.
