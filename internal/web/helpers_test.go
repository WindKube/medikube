package web

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/pocketbase/pocketbase/ui"
	"github.com/rs/zerolog"

	"medikube/internal/config"
	"medikube/internal/logging"
	"medikube/internal/obs"
	"medikube/internal/platform/pb"
	"medikube/internal/testsupport"
)

// discardLogger is MediKube's real logger writing nowhere. It goes through
// internal/logging rather than zerolog.New so the field names and the redaction
// on the path are the ones the application has.
func discardLogger() zerolog.Logger { return logTo(io.Discard) }

// logTo is the same logger pointed at a sink a test can read, for the
// assertions that are about the log stream itself rather than about a response.
func logTo(sink io.Writer) zerolog.Logger {
	return logging.NewTo(sink, config.LogConfig{Level: zerolog.LevelInfoValue}, "test")
}

// logSink collects the log stream and counts the lines in it.
//
// It locks because zerolog leaves serialisation to the destination, and the
// handler under test writes from whichever goroutine net/http gave the request.
type logSink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *logSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.Write(p)
}

// lines returns the log lines written so far, without the trailing blank one.
func (s *logSink) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	trimmed := strings.TrimRight(s.buf.String(), "\n")
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}

// serveBinders composes several binders into the one seam
// pb.ServeOptions.Routes has, the way cmd/medikube does.
type serveBinders []testsupport.ServeBinder

func (b serveBinders) Bind(se *core.ServeEvent) error {
	for _, one := range b {
		if err := one.Bind(se); err != nil {
			return err
		}
	}

	return nil
}

// cors binds PocketBase's CORS middleware exactly as apis.Serve binds it
// (apis/serve.go:76-79).
//
// apis.NewRouter does NOT bind it, which is why no tests.ApiScenario in this
// repository has ever seen a preflight: the middleware that answers one is only
// ever installed on the path scenarios do not take. A harness that wants to see
// what a browser sees has to put it back.
func cors() testsupport.ServeBinder {
	return binder(func(se *core.ServeEvent) {
		se.Router.Bind(apis.CORS(apis.CORSConfig{
			AllowOrigins: []string{"*"},
			AllowMethods: []string{
				http.MethodGet, http.MethodHead, http.MethodPut,
				http.MethodPatch, http.MethodPost, http.MethodDelete,
			},
		}))
	})
}

// pocketBaseAdminPolicy is apis/serve.go:25's `defaultCSP`, verbatim. It is
// unexported there, so a copy is the only way a test can name it; the
// end-to-end proof that the real one survives is a request to /_/ against the
// real binary, and this stands in for it here.
const pocketBaseAdminPolicy = "default-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' http://127.0.0.1:* https://tile.openstreetmap.org data: blob:; " +
	"connect-src 'self' http://127.0.0.1:* https://nominatim.openstreetmap.org; " +
	"script-src 'self' http://127.0.0.1:*; frame-ancestors 'none'"

// adminUI registers PocketBase's superuser admin UI the way apis.Serve
// registers it (apis/serve.go:82-99): the real static handler over the real
// embedded dist, behind the set-if-empty policy that is the entire reason
// [Outermost] fills the header in at commit instead of before delegating.
//
// apis.NewRouter does not register this route either, so without it the harness
// answers 404 for /_/ and the one exemption in the header set goes untested.
func adminUI() testsupport.ServeBinder {
	return binder(func(se *core.ServeEvent) {
		se.Router.GET("/_/{path...}", apis.Static(ui.DistDirFS, false)).
			BindFunc(func(e *core.RequestEvent) error {
				if e.Response.Header().Get("Content-Security-Policy") == "" {
					e.Response.Header().Set("Content-Security-Policy", pocketBaseAdminPolicy)
				}

				return e.Next()
			}).
			Bind(apis.Gzip())
	})
}

// served builds the http.Handler an instance of MediKube actually serves, with
// the whole production edge on it: the middleware order pb.BindServe installs,
// the lockdown, PocketBase's CORS where apis.Serve puts it, MediKube's security
// headers, and [Outermost] wrapped around the built mux.
//
// It exists because tests.ApiScenario structurally cannot reach two kinds of
// response — see testsupport.NewEdgeHandler — and both of them were, until
// this harness, answered with no security headers at all.
func served(t *testing.T, log zerolog.Logger, routes ...testsupport.ServeBinder) http.Handler {
	t.Helper()

	app := testsupport.NewApp(t)

	pb.BindServe(app, pb.ServeOptions{
		Middlewares: []*hook.Handler[*core.RequestEvent]{
			obs.RequestLogger(log),
			Errors(nil),
		},
		Routes:    append(serveBinders{cors(), adminUI(), SecurityBinder{}}, routes...),
		Outermost: Outermost(log),
	})

	return testsupport.NewEdgeHandler(t, app)
}

// call drives one request through a handler and returns the response.
func call(t *testing.T, handler http.Handler, method, target string, headers ...string) *http.Response {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		request.Header.Set(headers[i], headers[i+1])
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder.Result()
}

// serveBinder adapts a plain function to the interface both
// testsupport.NewAppFactory and pb.ServeOptions.Routes take. It is declared
// once here because six test files in this package need the same four lines.
type serveBinder func(se *core.ServeEvent) error

func (b serveBinder) Bind(se *core.ServeEvent) error { return b(se) }

// binder registers routes and middleware on the serve event and does NOT call
// se.Next(): testsupport.NewAppWith owns the chain, and a binder that continued
// it would pre-empt the scenario's own terminal function.
func binder(build func(se *core.ServeEvent)) testsupport.ServeBinder {
	return serveBinder(func(se *core.ServeEvent) error {
		build(se)

		return nil
	})
}

// route is the shortest binder: one method and path, served by handler.
func route(method, path string, handler func(e *core.RequestEvent) error) testsupport.ServeBinder {
	return binder(func(se *core.ServeEvent) {
		se.Router.Route(method, path, handler)
	})
}

// middleware binds one handler on the root router.
func middleware(handlers ...*hook.Handler[*core.RequestEvent]) testsupport.ServeBinder {
	return binder(func(se *core.ServeEvent) {
		for _, h := range handlers {
			se.Router.Bind(h)
		}
	})
}

// event builds a *core.RequestEvent around a recorder, for everything that
// speaks to a response without needing a router, a database or a port.
//
// The request carries the test's context, so a helper that reads it — every one
// that reaches obs.CorrelationID does — sees a real one.
func event(t *testing.T, method, target string, body ...string) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()

	var reader io.Reader
	if len(body) > 0 {
		reader = strings.NewReader(body[0])
	}

	request := httptest.NewRequestWithContext(t.Context(), method, target, reader)
	recorder := httptest.NewRecorder()

	e := &core.RequestEvent{}
	e.Request = request
	e.Response = &router.ResponseWriter{ResponseWriter: recorder}

	return e, recorder
}

// headerSet renders a response's headers as a comparable map, so an assertion
// can be made about the whole set rather than about the three names its author
// remembered. Values are joined because a repeated header is one header with
// two values and a test that dropped the second would not notice.
func headerSet(header http.Header) map[string]string {
	set := make(map[string]string, len(header))
	for name, values := range header {
		set[http.CanonicalHeaderKey(name)] = strings.Join(values, ", ")
	}

	return set
}

// rereadable wraps a body exactly as PocketBase's router does
// (tools/router/router.go:136). It rewinds on EOF, which is what makes
// json.UnmarshalRead see the document twice and fail on every request.
func rereadable(body string) io.ReadCloser {
	return &router.RereadableReadCloser{ReadCloser: io.NopCloser(strings.NewReader(body))}
}
