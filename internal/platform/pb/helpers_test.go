package pb_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
	"medikube/internal/platform/pb"
)

// fixtureUserEmail and fixtureSuperuserEmail are the accounts PocketBase ships
// in tests/data. The ordinary user is what "an ordinary signed-in person" means
// in every lockdown assertion; the superuser is the one actor the lockdown lets
// through, because the admin UI is that actor's only interface.
const (
	fixtureUserEmail      = "test@example.com"
	fixtureSuperuserEmail = "test@example.com"
)

// harness boots one throwaway PocketBase instance, triggers OnServe exactly
// once, and hands back the mux the real server would have served.
//
// One instance per test, never shared. apis.NewRouter binds an *anonymous*
// OnServe handler (apis/extensions.go:24-26) and hook.Bind appends rather than
// replaces when the id is empty (tools/hook/hook.go:94-96), so a second
// NewRouter against the same app grows that chain permanently and every
// trigger recurses one frame deeper.
type harness struct {
	app *tests.TestApp
	se  *core.ServeEvent
	mux http.Handler
}

// newHarness reproduces apis.Serve's own sequence: build the router, hand the
// ServeEvent a real *http.Server, trigger OnServe, and only then build the mux
// — which is what makes a middleware bound inside OnServe actually reach a
// request (apis/serve.go:212-225).
func newHarness(t *testing.T, bind func(app *tests.TestApp)) *harness {
	t.Helper()

	app, err := tests.NewTestApp()
	require.NoError(t, err, "boot a throwaway PocketBase instance")
	t.Cleanup(app.Cleanup)

	if bind != nil {
		bind(app)
	}

	r, err := apis.NewRouter(app)
	require.NoError(t, err, "build the PocketBase router")

	se := new(core.ServeEvent)
	se.App = app
	se.Router = r
	// The literal apis.Serve builds at apis/serve.go:144-160, five-minute
	// WriteTimeout included, so an override has something real to override.
	se.Server = &http.Server{
		WriteTimeout:      5 * time.Minute,
		ReadTimeout:       5 * time.Minute,
		ReadHeaderTimeout: time.Minute,
	}
	se.InstallerFunc = apis.DefaultInstallerFunc

	require.NoError(t,
		app.OnServe().Trigger(se, func(*core.ServeEvent) error { return nil }),
		"trigger OnServe",
	)

	mux, err := se.Router.BuildMux()
	require.NoError(t, err, "build the mux")

	return &harness{app: app, se: se, mux: mux}
}

// bindMediKubeServe is the binding under test in every lockdown scenario: what
// internal/platform/pb puts on OnServe and nothing else.
func bindMediKubeServe(app *tests.TestApp) {
	pb.BindServe(app, pb.ServeOptions{})
}

// response is what an assertion actually needs: the status and the exact bytes,
// because FR-033 is a claim about the bytes.
type response struct {
	Status int
	Body   string
	Header http.Header
}

func (h *harness) do(t *testing.T, method, target, token, body string) response {
	t.Helper()

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)

	return response{
		Status: rec.Code,
		Body:   strings.TrimSpace(rec.Body.String()),
		Header: rec.Result().Header.Clone(),
	}
}

// authToken mints a token directly rather than going through
// auth-with-password, so a scenario cannot silently degrade into an anonymous
// one because a fixture password changed upstream.
func (h *harness) authToken(t *testing.T, collection, email string) string {
	t.Helper()

	record, err := h.app.FindAuthRecordByEmail(collection, email)
	require.NoErrorf(t, err, "find the %s fixture account %q", collection, email)

	token, err := record.NewAuthToken()
	require.NoError(t, err, "mint an auth token")

	return token
}

func (h *harness) userToken(t *testing.T) string {
	t.Helper()

	return h.authToken(t, "users", fixtureUserEmail)
}

func (h *harness) superuserToken(t *testing.T) string {
	t.Helper()

	return h.authToken(t, core.CollectionNameSuperusers, fixtureSuperuserEmail)
}

// collectionNames is what "every collection" means in the lockdown tests: not a
// list somebody maintains, but whatever the instance actually holds.
func (h *harness) collectionNames(t *testing.T) []string {
	t.Helper()

	collections, err := h.app.FindAllCollections()
	require.NoError(t, err, "enumerate the collections")
	require.NotEmpty(t, collections)

	names := make([]string, 0, len(collections))
	for _, c := range collections {
		names = append(names, c.Name)
	}

	return names
}

// genuine404 is the body PocketBase writes for a route that does not exist.
// Every lockdown refusal is compared against it, because "answered exactly as a
// request for one that has never existed" (FR-033) is a statement about bytes.
func (h *harness) genuine404(t *testing.T) response {
	t.Helper()

	res := h.do(t, http.MethodGet, "/api/this-route-has-never-existed", "", "")
	require.Equal(t, http.StatusNotFound, res.Status)

	return res
}

// testConfig is a Config that passes config.Validate, so a test asserting how
// the platform reads configuration cannot pass against a shape the real Load
// would have rejected.
func testConfig(t *testing.T, dataDir string) config.Config {
	t.Helper()

	cfg := config.Config{
		Env:        "production",
		DataDir:    dataDir,
		RateLimits: true,
		HTTPAddr:   "127.0.0.1:8090",
		PublicURL:  "https://medikube.example",
		DrainDelay: 5 * time.Second,
		DrainMax:   25 * time.Second,
		Log:        config.LogConfig{Level: "info"},
		Auth:       config.AuthConfig{SessionTTL: 168 * time.Hour},
		Retention:  config.RetentionConfig{AuditDays: 730},
		Sentry:     config.SentryConfig{Environment: "production", SampleRate: 1},
		Metrics:    config.MetricsConfig{Enabled: true, Addr: "127.0.0.1:9090"},
		OTel:       config.OTelConfig{Endpoint: "localhost:4318", SampleRatio: 1, Environment: "production"},
		Files: config.FilesConfig{
			MaxUploadBytes: 33554432,
			AllowedMIME:    []string{"application/pdf"},
			PhotoMaxBytes:  15728640,
			PhotoMimeTypes: []string{"image/jpeg", "image/png", "image/webp"},
			PhotoThumbs:    []string{"100x100t", "400x400f"},
		},
	}

	require.NoError(t, cfg.Validate(), "the test config must be one config.Load would accept")

	return cfg
}

func lockedPatternsUnderTest() []string {
	patterns := make([]string, 0, len(pb.LockedRoutes()))
	for _, locked := range pb.LockedRoutes() {
		patterns = append(patterns, locked.Pattern())
	}

	return patterns
}
