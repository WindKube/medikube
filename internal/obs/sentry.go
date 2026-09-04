package obs

import (
	"context"
	"fmt"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase/core"
	"github.com/rs/zerolog"

	"medikube/internal/config"
	"medikube/internal/logging"
)

// Reporter is MediKube's error-reporting destination.
//
// The zero value and a Reporter built without a DSN are the same thing: no
// client, no transport, no outbound connection. That is deliberate rather than
// defensive — FR-039 is a promise that the process makes no request nobody
// asked for, and the way to keep it is for there to be nothing to make one
// with, not for a constructed client to be asked politely to stay quiet.
type Reporter struct {
	client *sentry.Client
}

// StartSentry builds the reporter for cfg. Without a DSN it builds nothing at
// all and returns a Reporter that is inert for the life of the process.
//
// It does not install a global hub. sentry.CurrentHub() is process state that
// every package can reach, which is the shape that produced the double
// reporting FR-057 forbids; the composition root holds this one and hands it
// to the single caller — the 500 branch of the error mapper.
func StartSentry(cfg config.SentryConfig, release string, log zerolog.Logger) (*Reporter, error) {
	return startSentry(cfg, release, log, nil)
}

// StartSentryWithTransport is StartSentry with the transport seam exposed.
//
// Production never calls it — StartSentry does, with a nil transport, which is
// what leaves sentry-go to dial the real DSN. A suite that must observe an
// event without a network has no other way to see one: sentry.Client keeps no
// public accessor for what it sent, so phileak.SentryTransport, handed in
// here, is the only place an event a full production-shaped chain produced can
// be read back (T284).
func StartSentryWithTransport(
	cfg config.SentryConfig, release string, log zerolog.Logger, transport sentry.Transport,
) (*Reporter, error) {
	return startSentry(cfg, release, log, transport)
}

// startSentry is the seam the suite uses to observe an event without a network:
// a Transport is the only way sentry-go lets anything see what it would have
// sent.
func startSentry(cfg config.SentryConfig, release string, log zerolog.Logger, transport sentry.Transport) (*Reporter, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return &Reporter{}, nil
	}

	client, err := sentry.NewClient(sentryOptions(cfg, release, log, transport))
	if err != nil {
		return nil, fmt.Errorf("build the Sentry client: %w", err)
	}

	return &Reporter{client: client}, nil
}

// sentryOptions is the whole configuration of what leaves the process.
//
// DataCollection is the first of the two halves: it stops the SDK assembling
// cookies, headers, query parameters and bodies into an event at all. scrub is
// the second, and it is not redundant — prepareEvent leaves an already-set
// Request alone, so an event assembled anywhere else arrives at BeforeSend
// fully populated with the things DataCollection would have refused to gather.
func sentryOptions(cfg config.SentryConfig, release string, log zerolog.Logger, transport sentry.Transport) sentry.ClientOptions {
	return sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      cfg.Environment,
		Release:          release,
		SampleRate:       cfg.SampleRate,
		AttachStacktrace: true,
		Transport:        transport,

		Debug: cfg.Debug,
		// sentry-go's debug output is plain text on stdout, which is one more
		// stream than MediKube has (Principle VI).
		DebugWriter: sentryDebugWriter{log: log},

		// Negative means breadcrumbs are ignored outright. A breadcrumb is
		// assembled out of whatever passed through the process before the
		// failure, which is the least reviewable of the ways a note or a
		// medication name reaches a third party.
		MaxBreadcrumbs: -1,

		DataCollection: &sentry.DataCollection{
			UserInfo:    sentry.Set(false),
			Cookies:     &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
			HTTPBodies:  []sentry.BodyType{},
			QueryParams: &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
			HTTPHeaders: &sentry.HeaderCollectionConfig{
				Request:  &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
				Response: &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
			},
		},

		BeforeSend: scrub,
	}
}

// Active reports whether an operator has configured a destination.
func (r *Reporter) Active() bool { return r != nil && r.client != nil }

// Report sends Recordable(e, err) and reports whether it went anywhere. The
// correlation id travels as a tag so the issue and the log line share a handle.
func (r *Reporter) Report(e *core.RequestEvent, err error) bool {
	if !r.Active() || err == nil || e == nil || e.Request == nil {
		return false
	}

	ctx := e.Request.Context()
	cause := Recordable(e, err)

	scope := sentry.NewScope()

	if id := CorrelationID(ctx); id != "" {
		scope.SetTag(logging.CorrelationField, id)
	}

	// A 500 the code anticipated and a panic it did not are answered
	// identically to a client and must be distinguishable to an operator.
	if Panicked(err) {
		scope.SetLevel(sentry.LevelFatal)
	}

	hint := &sentry.EventHint{Context: ctx, OriginalException: cause}

	return r.client.CaptureException(cause, hint, scope) != nil
}

// Shutdown flushes whatever is still in flight and closes the client. It is
// nil-safe and inactive-safe, so the composition root's shutdown path does not
// have to ask whether Sentry was ever configured.
func (r *Reporter) Shutdown(ctx context.Context) error {
	if !r.Active() {
		return nil
	}

	if !r.client.FlushWithContext(ctx) {
		r.client.Close()

		return fmt.Errorf("flush the Sentry buffer: %w", ctx.Err())
	}

	r.client.Close()

	return nil
}

// scrub is the BeforeSend allowlist: it names what may leave rather than what
// may not, because a denylist is a list somebody has to remember to extend
// when sentry-go grows a field.
func scrub(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return nil
	}

	// The hostname names the machine somebody's records sit on, and sentry-go
	// fills it from os.Hostname() when the option is empty.
	event.ServerName = ""
	event.Breadcrumbs = nil
	event.Attachments = nil

	// Only the opaque record id survives. Everything else on a Sentry user is
	// a name, an address or a free-text map.
	event.User = sentry.User{ID: event.User.ID}

	if event.Request != nil {
		event.Request = &sentry.Request{
			Method: event.Request.Method,
			URL:    withoutQuery(event.Request.URL),
		}
	}

	// Outermost exception only: sentry-go serialises the whole Unwrap chain
	// and MaxErrorDepth cannot express "one" (zero means the default).
	if n := len(event.Exception); n > 1 {
		event.Exception = event.Exception[n-1:]
		event.Exception[0].Mechanism = nil
	}

	return event
}

// withoutQuery keeps the path and drops everything a search term reaches the
// process through. The path itself is a route and a 15-character record id,
// both of which an operator needs and neither of which discloses anything.
func withoutQuery(rawURL string) string {
	if cut := strings.IndexAny(rawURL, "?#"); cut >= 0 {
		return rawURL[:cut]
	}

	return rawURL
}

// sentryDebugWriter puts sentry-go's own diagnostics on the one stream.
type sentryDebugWriter struct {
	log zerolog.Logger
}

func (w sentryDebugWriter) Write(p []byte) (int, error) {
	w.log.Debug().Str("component", "sentry").Msg(strings.TrimRight(string(p), "\n"))

	return len(p), nil
}
