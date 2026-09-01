package obs

import (
	"go/build"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"medikube/internal/testsupport/phileak"
)

const pocketbaseModule = "github.com/pocketbase/pocketbase"

// upgradeProcedure is named in every failure here, because none of these
// failures mean the code is wrong. They mean PocketBase moved.
const upgradeProcedure = "PocketBase's own database configuration has moved; work entry 4 of docs/pocketbase-upgrade-checklist.md before changing anything here"

// ---------------------------------------------------------------------------
// The drift check, half one: the copied string against the real one.
// ---------------------------------------------------------------------------

// TestTheCopiedPragmaStringIsStillPocketBasesOwn reads the pragma string out of
// PocketBase's own source and compares it with the copy.
//
// It is the textual half, and it catches what the behavioural half below cannot
// see: a pragma reordered (the busy_timeout-first comment says the order
// matters), and _defensive, which is a connection flag rather than a pragma and
// so appears in no pragma listing.
//
// It does not skip when it cannot find the source. "I could not check" is a
// different fact from "it has not drifted", and a check that quietly downgrades
// itself to the first while reporting the second is the whole failure mode this
// phase exists to prevent.
func TestTheCopiedPragmaStringIsStillPocketBasesOwn(t *testing.T) {
	t.Parallel()

	source := readPocketBaseSource(t, filepath.Join("core", "db_connect.go"))

	upstream := extractPragmaLiteral(t, source)

	require.NotEmpty(t, upstream)
	assert.Equal(t, upstream, pocketbasePragmas, upgradeProcedure)
}

// readPocketBaseSource locates one file of the pinned PocketBase in the module
// cache. The version comes from the build rather than from a constant, so this
// cannot end up reading a version the binary is not built against.
func readPocketBaseSource(t *testing.T, rel string) string {
	t.Helper()

	info, ok := debug.ReadBuildInfo()
	require.True(t, ok, "this test binary carries no build information, so the pinned PocketBase cannot be located")

	var version string

	for _, dep := range info.Deps {
		if dep.Path != pocketbaseModule {
			continue
		}

		version = dep.Version

		if dep.Replace != nil {
			version = dep.Replace.Version
		}
	}

	require.NotEmpty(t, version, "%s is not among this binary's dependencies", pocketbaseModule)

	// GOMODCACHE, then the GOPATH resolution go/build already does, which
	// covers the environment variable and the $HOME/go default alike.
	cache := os.Getenv("GOMODCACHE")
	if cache == "" {
		cache = filepath.Join(build.Default.GOPATH, "pkg", "mod")
	}

	path := filepath.Join(cache, filepath.FromSlash(pocketbaseModule)+"@"+version, rel)

	body, err := os.ReadFile(path) //nolint:gosec // the path is assembled from the build's own dependency list
	require.NoError(t, err,
		"%s could not be read, so the pragma string was compared against nothing. %s", path, upgradeProcedure)

	return string(body)
}

// extractPragmaLiteral pulls the `pragmas := "..."` literal out of
// DefaultDBConnect. It is deliberately anchored on the assignment rather than
// on a substring of the value: a search for "busy_timeout" would find the
// comment above it and a search for "?_pragma" would find whatever this string
// had been changed into, which is the thing being detected.
func extractPragmaLiteral(t *testing.T, source string) string {
	t.Helper()

	const marker = "pragmas := "

	start := strings.Index(source, marker)
	require.GreaterOrEqual(t, start, 0,
		"DefaultDBConnect no longer assigns a variable called pragmas. %s", upgradeProcedure)

	rest := source[start+len(marker):]

	end := strings.IndexByte(rest, '\n')
	require.GreaterOrEqual(t, end, 0)

	literal, err := strconv.Unquote(strings.TrimSpace(rest[:end]))
	require.NoError(t, err,
		"the pragmas assignment is no longer a single quoted string literal. %s", upgradeProcedure)

	return literal
}

// ---------------------------------------------------------------------------
// The drift check, half two: the two connections, compared by what they do.
// ---------------------------------------------------------------------------

// pragmasThatCannotMatch is the exemption table, keyed by the pragma exempted
// and valued by the reason and the task that owns it. Everything not named here
// must read identically on both connections.
//
// It is one entry long on purpose. A second entry is a claim that MediKube's
// database is configured differently from PocketBase's in some way somebody
// decided was acceptable, and that is a decision with a name on it.
var pragmasThatCannotMatch = map[string]string{
	"database_list": "T247: it reports the file path, and the two connections are deliberately opened on two different files — one file would let the first connection's persisted journal_mode satisfy the second and the comparison would prove nothing",
}

// TestAnInstrumentedConnectionIsConfiguredExactlyLikePocketBasesOwn is the
// behavioural half of the drift check, and it is the one that runs everywhere:
// it reads no source and makes no assumption about where a module lives.
//
// It compares EVERY pragma SQLite reports, not a list somebody wrote down. That
// is what makes it catch an upstream ADDITION — a pragma PocketBase starts
// setting that this copy does not — which a check driven by the copied string's
// own contents structurally cannot see.
func TestAnInstrumentedConnectionIsConfiguredExactlyLikePocketBasesOwn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	theirs, err := core.DefaultDBConnect(filepath.Join(dir, "pocketbase.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = theirs.Close() })

	connect := instrumentedDBConnect(sdktrace.NewTracerProvider())

	ours, err := connect(filepath.Join(dir, "medikube.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = ours.Close() })

	theirSettings := pragmaSnapshot(t, theirs)
	ourSettings := pragmaSnapshot(t, ours)

	require.Greater(t, len(theirSettings), 40,
		"the walk read almost no pragmas; it is not reading a SQLite connection and would agree with anything")
	require.Equal(t, len(theirSettings), len(ourSettings),
		"the two connections do not even report the same set of pragmas")

	compared := 0

	for _, name := range sortedKeys(theirSettings) {
		if reason, exempt := pragmasThatCannotMatch[name]; exempt {
			require.NotEmpty(t, reason)

			continue
		}

		compared++

		assert.Equalf(t, theirSettings[name], ourSettings[name],
			"PRAGMA %s differs between PocketBase's connection and MediKube's. %s", name, upgradeProcedure)
	}

	require.Greater(t, compared, 40,
		"almost every pragma was exempted; the comparison is no longer comparing anything")

	for name := range pragmasThatCannotMatch {
		require.Contains(t, theirSettings, name,
			"%s is exempted and SQLite no longer reports it: strike it out of pragmasThatCannotMatch", name)
	}
}

// TestTheInstrumentedConnectionKeepsDefensiveModeOn covers the one member of
// the DSN that no pragma listing reports: _defensive=1 is an sqlite3_db_config
// flag, so the comparison above is blind to it.
//
// Its observable effect is that PRAGMA writable_schema=ON is refused — which is
// what stops a SQL injection or a stray migration from rewriting sqlite_schema
// directly, and is worth a good deal in a database holding medical records.
func TestTheInstrumentedConnectionKeepsDefensiveModeOn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	connect := instrumentedDBConnect(sdktrace.NewTracerProvider())

	ours, err := connect(filepath.Join(dir, "medikube.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = ours.Close() })

	undefended, err := dbx.Open(sqliteDriver, filepath.Join(dir, "undefended.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = undefended.Close() })

	// The control. Without it this asserts that a setting is off on a
	// connection where it was never turned on, which is true of any string.
	_, err = undefended.NewQuery("PRAGMA writable_schema=ON").Execute()
	require.NoError(t, err)
	require.Equal(t, "1", readPragma(t, undefended, "writable_schema"),
		"a connection opened WITHOUT _defensive refused the write too, so this proves nothing about the flag")

	_, err = ours.NewQuery("PRAGMA writable_schema=ON").Execute()
	require.NoError(t, err)

	assert.Equal(t, "0", readPragma(t, ours, "writable_schema"),
		"sqlite_schema became writable, so _defensive is not set. %s", upgradeProcedure)
}

// pragmaSnapshot reads every pragma this SQLite build reports, as text.
//
// A pragma that errors is recorded as its error rather than skipped: two
// connections that disagree about which pragmas are legal disagree about their
// configuration, and a skip would hide exactly that.
func pragmaSnapshot(t *testing.T, db *dbx.DB) map[string]string {
	t.Helper()

	var names []struct {
		Name string `db:"name"`
	}

	require.NoError(t, db.NewQuery("SELECT name FROM pragma_pragma_list ORDER BY name").All(&names))

	snapshot := make(map[string]string, len(names))

	for _, listed := range names {
		snapshot[listed.Name] = readPragmaRows(db, listed.Name)
	}

	return snapshot
}

func readPragmaRows(db *dbx.DB, name string) string {
	rows, err := db.NewQuery("PRAGMA " + name).Rows()
	if err != nil {
		return "error: " + err.Error()
	}

	defer func() { _ = rows.Close() }()

	var lines []string

	for rows.Next() {
		values := dbx.NullStringMap{}
		if err := rows.ScanMap(values); err != nil {
			return "error: " + err.Error()
		}

		var cells []string
		for _, column := range sortedKeys(values) {
			cells = append(cells, column+"="+values[column].String)
		}

		lines = append(lines, strings.Join(cells, ","))
	}

	if err := rows.Err(); err != nil {
		return "error: " + err.Error()
	}

	return strings.Join(lines, "|")
}

func readPragma(t *testing.T, db *dbx.DB, name string) string {
	t.Helper()

	value := readPragmaRows(db, name)
	_, setting, found := strings.Cut(value, "=")
	require.True(t, found, "PRAGMA %s answered %q", name, value)

	return setting
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// ---------------------------------------------------------------------------
// What the instrumentation is allowed to say, and when it exists at all.
// ---------------------------------------------------------------------------

// TestAnUntracedBuildOpensItsDatabaseThroughPocketBaseItself is why the copied
// string is not a liability for most deployments.
//
// Nil is not "no instrumentation configured yet". Nil is what makes PocketBase
// call core.DefaultDBConnect — the real one, with the real pragmas — so a
// deployment that has not configured an OTLP endpoint never touches the copy
// and cannot be hurt by it drifting.
func TestAnUntracedBuildOpensItsDatabaseThroughPocketBaseItself(t *testing.T) {
	t.Parallel()

	for name, tracing := range map[string]*Tracing{
		"nothing built it at all":      nil,
		"built, and no endpoint given": {},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.False(t, tracing.Active())
			assert.Nil(t, InstrumentedDBConnect(tracing),
				"an untraced build replaced PocketBase's own connection function with MediKube's copy of it for no benefit")
		})
	}
}

// TestNoQueryTextReachesASpan is FR-038 at the database boundary.
//
// otelsql puts the statement on every span as db.query.text unless it is told
// not to, and the destination is a trace backend MediKube does not own. The
// sentinel is inlined into the SQL exactly as a hand-built filter would inline
// a search term.
//
// It runs through internal/testsupport/phileak rather than through a span
// recorder of its own, and that is not ceremony. A recorder here would be the
// second partial PHI gate cross-artifact finding M6 describes — one that asserts
// over spans, looks in review exactly like the assertion, and leaves the sink
// nobody captured uncaptured. The harness asserts over all four.
func TestNoQueryTextReachesASpan(t *testing.T) {
	t.Parallel()

	const sentinel = "Amoxicillin-500mg-sentinel"

	capture := phileak.New(t)
	capture.WatchMetrics(NewMetrics().Registry())

	connect := instrumentedDBConnect(capture.TracerProvider())

	db, err := connect(filepath.Join(t.TempDir(), "traced.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	_, err = db.NewQuery("CREATE TABLE probe (note text)").Execute()
	require.NoError(t, err)

	_, err = db.NewQuery("INSERT INTO probe (note) VALUES ('" + sentinel + "')").Execute()
	require.NoError(t, err)

	var found []struct {
		Note string `db:"note"`
	}

	require.NoError(t, db.NewQuery("SELECT note FROM probe WHERE note = '"+sentinel+"'").All(&found))
	require.Len(t, found, 1, "the query never ran, so there is nothing here a span could have leaked")

	// The guard on the guard: the sentinel cannot be absent from a stream that
	// was never written to. Flushing is what puts the ended spans in front of
	// the assertion.
	require.NoError(t, capture.TracerProvider().ForceFlush(t.Context()))

	traces := sinkNamed(t, capture, phileak.SinkTraces)
	require.Contains(t, traces, "sql.conn.query",
		"the instrumentation recorded no database spans at all, so finding no sentinel in them proves nothing")

	capture.AssertNoSentinels(t, sentinel)
}

func sinkNamed(t *testing.T, capture *phileak.Capture, name string) string {
	t.Helper()

	for _, sink := range capture.Sinks() {
		if sink.Name == name {
			return sink.Text
		}
	}

	t.Fatalf("the capture holds no %s sink; it holds %v", name, capture.Names())

	return ""
}

// TestTheBuilderIsTheSQLiteOneAndNotTheStandardFallback covers the second of
// research D-30's two day-costing details: dbx.NewFromDB falls back to the ANSI
// builder for a driver name it does not recognise, and nothing fails at boot.
func TestTheBuilderIsTheSQLiteOneAndNotTheStandardFallback(t *testing.T) {
	t.Parallel()

	connect := instrumentedDBConnect(sdktrace.NewTracerProvider())

	ours, err := connect(filepath.Join(t.TempDir(), "medikube.db"))
	require.NoError(t, err)

	t.Cleanup(func() { _ = ours.Close() })

	require.Contains(t, dbx.BuilderFuncMap, sqliteDriver,
		"dbx no longer maps %q to a builder, so NewFromDB is silently on the ANSI fallback. %s", sqliteDriver, upgradeProcedure)

	standard := dbx.NewFromDB(ours.DB(), "a driver dbx has never heard of")

	assert.Equal(t, sqliteDriver, ours.DriverName())
	assert.NotEqual(t, standard.QuoteTableName("a.b"), ours.QuoteTableName("a.b"),
		"MediKube's builder quotes identifiers the way the ANSI fallback does, which is what an unrecognised driver name produces")
}
