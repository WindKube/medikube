package testsupport

import (
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

// NewEdgeHandler builds the http.Handler an instance of MediKube actually
// serves: the mux PocketBase's router builds, wrapped in whatever the OnServe
// chain wrapped it in.
//
// tests.ApiScenario cannot produce this and no amount of table rows will make
// it. It constructs the serve event by hand with only App and Router set
// (tests/api.go:189-195), so ServeEvent.Server is nil, and then it calls
// BuildMux and drives the mux directly — so anything installed *outside* the
// mux is invisible to it, and so is every response the mux answers before
// routing. Both of the holes this harness exists for live there: a CORS
// preflight, which apis.CORS is only ever bound for by apis.Serve and never by
// apis.NewRouter, and net/http's own path-normalising redirects, which
// ServeMux decides before it looks for a registered handler.
//
// The terminal function below is apis.Serve's own (apis/serve.go:217-223),
// which is what makes ServeEvent.Server.Handler exist and therefore what makes
// pb.ServeOptions.Outermost run.
//
// The app must already carry its OnServe bindings; this triggers them once. The
// returned handler is safe to drive as many times as the test likes — it is one
// built mux, not one per request — and the OnServe accumulation NewApp warns
// about cannot happen because apis.NewRouter is called exactly once.
func NewEdgeHandler(t testing.TB, app core.App) http.Handler {
	t.Helper()

	pbRouter, err := apis.NewRouter(app)
	require.NoError(t, err, "building the PocketBase router")

	se := new(core.ServeEvent)
	se.App = app
	se.Router = pbRouter
	// ReadHeaderTimeout is PocketBase's own (apis/serve.go:155). This server
	// never listens — only its Handler is read — but a zero value here is a
	// gosec finding and a bad example.
	se.Server = &http.Server{ReadHeaderTimeout: time.Minute}

	err = app.OnServe().Trigger(se, func(e *core.ServeEvent) error {
		handler, buildErr := e.Router.BuildMux()
		if buildErr != nil {
			return buildErr
		}

		e.Server.Handler = handler

		return nil
	})
	require.NoError(t, err, "triggering the OnServe chain")
	require.NotNil(t, se.Server.Handler, "the OnServe chain never reached the terminal handler")

	return se.Server.Handler
}
