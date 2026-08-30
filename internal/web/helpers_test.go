package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/rs/zerolog"

	"medikube/internal/config"
	"medikube/internal/logging"
	"medikube/internal/testsupport"
)

// discardLogger is MediKube's real logger writing nowhere. It goes through
// internal/logging rather than zerolog.New so the field names and the redaction
// on the path are the ones the application has.
func discardLogger() zerolog.Logger {
	return logging.NewTo(io.Discard, config.LogConfig{Level: zerolog.LevelInfoValue}, "test")
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
