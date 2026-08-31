package main

import (
	json "encoding/json/v2"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/logging"
	"medikube/internal/obs"
	"medikube/internal/platform/pb"
	"medikube/internal/realtime"
	"medikube/internal/records"
	"medikube/internal/web"
)

// syncBuffer is the log destination. samber/do shuts services down in parallel
// and PocketBase logs from its own goroutines, and zerolog leaves serialisation
// to the writer, so this needs a lock that os.Stdout does not.
type syncBuffer struct {
	mu    sync.Mutex
	lines []string
	rest  string
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.rest += string(p)
	for {
		cut := strings.IndexByte(b.rest, '\n')
		if cut < 0 {
			break
		}

		b.lines = append(b.lines, b.rest[:cut])
		b.rest = b.rest[cut+1:]
	}

	return len(p), nil
}

func (b *syncBuffer) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return slices.Clone(b.lines)
}

// testConfig is a validated configuration pointed at a directory that does not
// exist yet. FR-063 is about exactly that directory.
func testConfig(t *testing.T, dataDir string) config.Config {
	t.Helper()

	cfg := config.Config{
		Env:        "development",
		DataDir:    dataDir,
		HTTPAddr:   "127.0.0.1:0",
		PublicURL:  "http://127.0.0.1:8090",
		DrainDelay: time.Second,
		DrainMax:   10 * time.Second,
		Log:        config.LogConfig{Level: "debug"},
		Auth:       config.AuthConfig{SessionTTL: 168 * time.Hour},
		Retention:  config.RetentionConfig{AuditDays: 730},
		Files: config.FilesConfig{
			MaxUploadBytes: 1 << 20,
			AllowedMIME:    []string{"application/pdf"},
		},
	}

	// Built by hand rather than loaded, so it is run through the same
	// validation config.Load would have applied. A test that served an
	// invalid configuration would be proving nothing about the real boot.
	require.NoError(t, cfg.Validate())

	return cfg
}

// serving boots the composition root against cfg, puts it behind a real TCP
// listener and returns its address. It is the recipe FR-063 needs: an
// unbootstrapped app, an empty directory, and a real socket rather than
// tests.ApiScenario — which calls apis.NewRouter directly and therefore never
// registers CORS, the admin-UI route or the listener at all.
func serving(t *testing.T, cfg config.Config, logs *syncBuffer) string {
	t.Helper()

	log := logging.NewTo(logs, cfg.Log, "test")

	app, container, err := build(cfg, log)
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	// Priority 10000 runs after MediKube's own OnServe binding and after its
	// boot assertions, so the listener is installed by a handler that cannot
	// mask a refusal to start.
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Id:       "medikubeTestListener",
		Priority: 10000,
		Func: func(se *core.ServeEvent) error {
			se.Listener = listener

			return se.Next()
		},
	})

	require.NoError(t, app.Bootstrap())

	served := make(chan error, 1)
	go func() {
		served <- apis.Serve(app, apis.ServeConfig{ShowStartBanner: false, HttpAddr: cfg.HTTPAddr})
	}()

	t.Cleanup(func() {
		terminate := new(core.TerminateEvent)
		terminate.App = app

		require.NoError(t, app.OnTerminate().Trigger(terminate, func(*core.TerminateEvent) error { return nil }))

		select {
		case err := <-served:
			assert.NoError(t, err)
		case <-time.After(30 * time.Second):
			t.Error("apis.Serve did not return after the terminate event: the instance does not shut down")
		}

		assert.NoError(t, container.Shutdown())
	})

	base := "http://" + listener.Addr().String()

	require.Eventually(t, func() bool {
		res, reqErr := http.Get(base + "/api/v1/healthz") //nolint:noctx // a liveness poll, not a request under test
		if reqErr != nil {
			return false
		}
		_ = res.Body.Close()

		return true
	}, 20*time.Second, 20*time.Millisecond, "the instance never started answering on %s", base)

	return base
}

func get(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	return res, body
}

// FR-063, and the phase checkpoint: the instance starts against an empty
// storage location by creating everything it needs.
func TestTheCompositionRootServesAgainstAnEmptyDataDirectory(t *testing.T) {
	t.Parallel()

	dataDir := filepath.Join(t.TempDir(), "pb_data")
	_, err := os.Stat(dataDir)
	require.ErrorIs(t, err, os.ErrNotExist, "the directory must not exist, or this proves nothing")

	var logs syncBuffer

	base := serving(t, testConfig(t, dataDir), &logs)

	res, body := get(t, base+"/api/v1/healthz")
	assert.NotEqual(t, http.StatusBadGateway, res.StatusCode)
	assert.NotEmpty(t, body)

	for _, name := range []string{"data.db", "auxiliary.db"} {
		info, statErr := os.Stat(filepath.Join(dataDir, name))
		require.NoError(t, statErr, "%s was not created, so the instance did not build its own storage", name)
		assert.Positive(t, info.Size())
	}
}

// "Safe to restart repeatedly without manual intervention" is the other half
// of FR-063, and the half a first-boot test cannot see: the second boot runs
// against a schema that already exists, settings that are already written and
// a users collection whose session lifetime has already been set.
func TestTheInstanceRestartsAgainstItsOwnDataDirectory(t *testing.T) {
	t.Parallel()

	dataDir := filepath.Join(t.TempDir(), "pb_data")
	cfg := testConfig(t, dataDir)

	for _, boot := range []string{"first", "second", "third"} {
		t.Run(boot, func(t *testing.T) {
			var logs syncBuffer

			base := serving(t, cfg, &logs)

			res, _ := get(t, base+"/api/v1/healthz")
			assert.NotEqual(t, http.StatusBadGateway, res.StatusCode)
		})
	}
}

// The lockdown is the phase's security invariant and the composition root is
// where it could be lost: a binder that unbound it, a middleware bound in the
// wrong order, a route table that shadowed it. pb.BindServe refuses to start
// on the first two; this is the end-to-end proof through a real socket.
func TestTheLockdownHoldsOnTheAssembledInstance(t *testing.T) {
	t.Parallel()

	var logs syncBuffer

	base := serving(t, testConfig(t, filepath.Join(t.TempDir(), "pb_data")), &logs)

	locked := pb.LockedRoutes()
	require.NotEmpty(t, locked)

	for _, route := range locked {
		if route.Method != http.MethodGet {
			continue
		}

		t.Run(route.Pattern(), func(t *testing.T) {
			t.Parallel()

			// The collection is spelled by the kind table, never by a literal.
			url := base + strings.ReplaceAll(route.Path, "{collection}", kind.Medication.Collection())
			url = strings.ReplaceAll(url, "{id}", "anything")

			res, body := get(t, url)

			assert.Equal(t, http.StatusNotFound, res.StatusCode,
				"PocketBase's own record API answered; the lockdown is not holding")
			assertEnvelope(t, body, web.CodeNotFound)
		})
	}
}

// A refusal from the lockdown and a refusal from the mux must be the same
// answer, or the difference tells an anonymous caller which collections exist
// (FR-033).
func TestALockedRouteAndAnUnknownPathAreAnsweredIdentically(t *testing.T) {
	t.Parallel()

	var logs syncBuffer

	base := serving(t, testConfig(t, filepath.Join(t.TempDir(), "pb_data")), &logs)

	lockedRes, lockedBody := get(t, base+"/api/collections/"+kind.Medication.Collection()+"/records")
	unknownRes, unknownBody := get(t, base+"/api/collections/no-such-collection-at-all/records")

	require.Equal(t, http.StatusNotFound, lockedRes.StatusCode)
	require.Equal(t, http.StatusNotFound, unknownRes.StatusCode)

	assert.Equal(t, requestIDless(t, lockedBody), requestIDless(t, unknownBody),
		"the two refusals differ, so the response is an oracle for which collections exist")

	for _, header := range []string{"Content-Security-Policy", "X-Frame-Options", "Referrer-Policy"} {
		assert.Equalf(t, lockedRes.Header.Get(header), unknownRes.Header.Get(header),
			"%s differs between a locked route and an unknown path", header)
		assert.NotEmptyf(t, lockedRes.Header.Get(header), "%s is missing from the assembled instance", header)
	}
}

// Principle VI: one stream, and every line on it is JSON with the pinned field
// names. This reads the whole of a real boot — PocketBase's migration lines
// included, which is what the two log bridges exist for.
func TestEveryLineTheInstanceWritesIsAZerologLine(t *testing.T) {
	t.Parallel()

	var logs syncBuffer

	base := serving(t, testConfig(t, filepath.Join(t.TempDir(), "pb_data")), &logs)
	get(t, base+"/api/v1/healthz")

	lines := logs.Lines()
	require.NotEmpty(t, lines, "a whole boot produced no log lines at all; the stream is not connected")

	for i, line := range lines {
		var fields map[string]any

		require.NoErrorf(t, json.Unmarshal([]byte(line), &fields),
			"log line %d is not JSON, so something wrote outside the one stream: %s", i, line)

		for _, field := range []string{"ts", "level", "message", "service", "release"} {
			if field == "message" {
				// zerolog's message key is `msg` here, pinned by
				// internal/logging's init.
				assert.Containsf(t, fields, "msg", "log line %d has no msg: %s", i, line)

				continue
			}

			assert.Containsf(t, fields, field, "log line %d has no %s: %s", i, field, line)
		}
	}
}

// The request logger writes one line per request and that line carries the
// correlation id the client was given (FR-054). Asserted here rather than only
// in internal/obs because the composition root is what binds it, and a
// middleware list is easy to get wrong in exactly one place.
func TestOneRequestProducesOneCorrelatedLogLine(t *testing.T) {
	t.Parallel()

	var logs syncBuffer

	base := serving(t, testConfig(t, filepath.Join(t.TempDir(), "pb_data")), &logs)

	before := len(logs.Lines())

	res, _ := get(t, base+"/api/v1/healthz")
	correlation := res.Header.Get(obs.CorrelationHeader)
	require.NotEmpty(t, correlation, "the response carries no correlation id, so nobody can quote one")

	var matched int

	require.Eventually(t, func() bool {
		matched = 0
		for _, line := range logs.Lines()[before:] {
			if strings.Contains(line, correlation) {
				matched++
			}
		}

		return matched > 0
	}, 5*time.Second, 10*time.Millisecond, "no log line carries the correlation id the client was given")

	assert.Equal(t, 1, matched, "one request, one line")
}

// Every route MediKube serves has a handler, and every handler is a route it
// serves. httproute.New refuses both halves; this is the proof that the
// composition root satisfies it rather than the proof that the check exists.
func TestTheCompositionRootWiresEveryRouteMediKubeServes(t *testing.T) {
	t.Parallel()

	// The resolver is never called: which operations have handlers is decided
	// by which groups have landed, not by whether an instance could resolve a
	// kind registry. The application is real but unbootstrapped, because the
	// account surface binds a hook on it and holds it for later — it reads
	// nothing here.
	cfg := testConfig(t, filepath.Join(t.TempDir(), "pb_data"))

	table, err := operations(
		pb.New(cfg, pb.Options{}),
		cfg,
		func() (*records.Handler, error) { return nil, nil },
		realtime.New(),
	)
	require.NoError(t, err)

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindExternal {
			assert.NotContainsf(t, table, route.OpID,
				"%s is PocketBase's own route and a MediKube handler under that name would shadow it", route.OpID)

			continue
		}

		assert.Containsf(t, table, route.OpID, "%s (%s) is registered with no handler", route.OpID, route.Pattern())
	}

	// httproute.New refuses both halves of the mismatch. Reaching it means the
	// composition root satisfies the check rather than merely declaring it.
	_, err = httproute.New(table)
	require.NoError(t, err)
}

// The shrinking list. Every operation named here answers 501 today because its
// handler is a later task, and the moment one is implemented this test fails
// until the name is struck out — which is the only mechanism that stops a
// finished handler from sitting behind a stub nobody noticed.
//
// It is in the test rather than in handlers.go on purpose: handlers.go derives
// the stub set from the route table, so the inventory of what is still missing
// has no home in the program. Here it has one, and it is a diff.
func TestTheOperationsStillAnsweringNotImplementedAreExactlyThese(t *testing.T) {
	t.Parallel()

	pending := []string{
		// internal/web/api/health.go — US5, T285.
		"healthz", "readyz",
		// internal/web/page/overview.go — US4, T264.
		"overviewPage",
	}

	assert.ElementsMatch(t, pending, unimplemented(),
		"an operation was implemented and left in the stub list, or a stub appeared that nobody declared")
}

func TestAnUnimplementedOperationAnswersNotImplementedInTheMediKubeEnvelope(t *testing.T) {
	t.Parallel()

	var logs syncBuffer

	base := serving(t, testConfig(t, filepath.Join(t.TempDir(), "pb_data")), &logs)

	res, body := get(t, base+"/api/v1/healthz")

	assert.Equal(t, http.StatusNotImplemented, res.StatusCode)
	assertEnvelope(t, body, web.CodeInternal)
	assert.NotContains(t, string(body), "not implemented",
		"the stub's own words reached the client; only Message(code) may")
}

// PocketBase's serve command defaults to 127.0.0.1:8090 and to a wildcard CORS
// origin. MEDIKUBE_HTTP_ADDR and MEDIKUBE_ALLOWED_ORIGINS are validated
// configuration, and a composition root that ignored them would leave two
// documented variables doing nothing.
func TestTheServeCommandTakesItsDefaultsFromTheValidatedConfiguration(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t, filepath.Join(t.TempDir(), "pb_data"))
	cfg.HTTPAddr = "127.0.0.1:9999"
	cfg.AllowedOrigins = []string{"https://example.test"}

	app, container, err := build(cfg, logging.NewTo(io.Discard, cfg.Log, "test"))
	require.NoError(t, err)

	t.Cleanup(func() { assert.NoError(t, container.Shutdown()) })

	require.NoError(t, registerCommands(app, cfg))

	// The commands are examined in place rather than returned from a helper.
	// Naming *cobra.Command anywhere — a signature, a variable — is what
	// promotes spf13/cobra to a direct require, which plan.md's dependency
	// table forbids because the version is PocketBase's to choose.
	var registered []string

	for _, command := range app.RootCmd.Commands() {
		registered = append(registered, command.Name())

		if command.Name() != "serve" {
			continue
		}

		assert.Equal(t, "127.0.0.1:9999", command.PersistentFlags().Lookup("http").Value.String())
		assert.Equal(t, "[https://example.test]", command.PersistentFlags().Lookup("origins").Value.String())

		// ParseFlags rather than PersistentFlags().Parse: cobra's own entry
		// point, and the spelling internal/store's filter-DSL walk does not
		// read as search.Provider.Parse.
		require.NoError(t, command.ParseFlags([]string{"--http", "0.0.0.0:1234"}))
		assert.Equal(t, "0.0.0.0:1234", command.PersistentFlags().Lookup("http").Value.String(),
			"an explicit flag still outranks the environment")
	}

	// superuser is the only door to a first superuser, because pb.BindServe
	// nils PocketBase's first-run installer.
	assert.Subset(t, registered, []string{"serve", "superuser"},
		"the command surface this build registers")
}

func TestConfigurationThatDoesNotValidateIsABootFailureRatherThanAServer(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t, filepath.Join(t.TempDir(), "pb_data"))
	cfg.DataDir = ""

	var logs syncBuffer

	_, _, err := build(cfg, logging.NewTo(&logs, cfg.Log, "test"))
	require.Error(t, err, "a container built from an unvalidated configuration is a running instance with no storage")
}

func assertEnvelope(t *testing.T, body []byte, code string) {
	t.Helper()

	var envelope web.Envelope

	require.NoError(t, json.Unmarshal(body, &envelope), "body is not MediKube's error envelope: %s", body)
	assert.Equal(t, code, envelope.Error.Code)
	assert.Equal(t, web.Message(code), envelope.Error.Message)
	assert.NotEmpty(t, envelope.Error.RequestID, "an error a person cannot quote a reference for")
}

// requestIDless is the body with the one member that legitimately differs
// between two responses removed.
func requestIDless(t *testing.T, body []byte) string {
	t.Helper()

	var envelope web.Envelope

	require.NoError(t, json.Unmarshal(body, &envelope))
	envelope.Error.RequestID = ""

	rendered, err := json.Marshal(envelope)
	require.NoError(t, err)

	return string(rendered)
}
