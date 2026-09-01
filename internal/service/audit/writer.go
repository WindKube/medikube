package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	domainaudit "medikube/internal/domain/audit"
)

// Repository is the storage seam, declared by the consumer (Principle II).
// One method, because the trail only ever grows.
type Repository interface {
	Append(ctx context.Context, event domainaudit.Event) error
}

// RequestIDFunc reads the correlation handle of the request behind a context.
//
// It is a seam and not an import: the handle is minted at the HTTP edge, which
// lives in internal/obs on the PocketBase side of the boundary, and a service
// that imported it would drag the platform into every unit test. The
// composition root passes internal/obs's own reader through WithRequestID.
type RequestIDFunc func(ctx context.Context) string

// Option adjusts one writer.
type Option func(*Writer)

// WithRequestID gives the writer a way to find the handle of the request a row
// belongs to, for the callers that did not resolve it themselves.
func WithRequestID(resolve RequestIDFunc) Option {
	return func(w *Writer) { w.requestID = resolve }
}

// Writer is the one way a row reaches the trail.
//
// It validates before it appends, and that ordering is the point: the columns
// are bounded and the three vocabularies are closed, so a row refused by the
// database would be refused AFTER the thing it records has already happened,
// with nowhere useful to report it. Refusing here turns that into an error the
// caller can join to the operation it belongs to.
type Writer struct {
	repository Repository
	requestID  RequestIDFunc
}

func New(repository Repository, options ...Option) (*Writer, error) {
	if repository == nil {
		return nil, errors.New("audit: the writer is wired with no repository, so every row it accepted would be discarded silently")
	}

	w := &Writer{repository: repository}

	for _, option := range options {
		option(w)
	}

	return w, nil
}

// Record writes one event.
func (w *Writer) Record(ctx context.Context, event domainaudit.Event) error {
	event.RequestID = w.correlate(ctx, event.RequestID)

	if err := event.Validate(); err != nil {
		return fmt.Errorf("audit: the event is not one that can be recorded: %w", err)
	}

	return w.repository.Append(ctx, event)
}

// correlate settles FR-054's handle, and it never answers the empty string.
//
// The order is caller, request, run, fresh. The caller wins because a caller
// that resolved the handle already resolved it against the thing it is
// auditing; the request comes next because that is the common case; the run is
// what a cron tick, a job, a migration or a backfill has instead of a request,
// and it is the whole reason this is not a one-line read.
//
// The last branch is not a fallback so much as a refusal to write a row the
// store would reject: request_id is Required, so an empty handle turns an audit
// row into a validation failure raised after the thing it records has already
// happened — nightly, in production, where nobody is watching the purge.
func (w *Writer) correlate(ctx context.Context, carried string) string {
	if carried != "" {
		return carried
	}

	if w.requestID != nil && ctx != nil {
		if id := w.requestID(ctx); id != "" {
			return id
		}
	}

	if id := RunIDFrom(ctx); id != "" {
		return id
	}

	return newHandle()
}

// runIDKey is the context key a run's handle is carried under.
type runIDKey struct{}

// StartRun opens a background run and gives it a correlation handle, so every
// audit row and every log line it produces carries one handle an operator can
// join on. A caller that has already minted the handle its log lines carry
// passes it in rather than acquiring a second one for the same run.
func StartRun(ctx context.Context, id string) (context.Context, string) {
	if id == "" {
		id = newHandle()
	}

	return context.WithValue(ctx, runIDKey{}, id), id
}

// RunIDFrom returns the handle of the run behind ctx, or the empty string when
// there is no run — which is every request, and is not a failure.
func RunIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	id, _ := ctx.Value(runIDKey{}).(string)

	return id
}

// handleBytes is sixteen, which is internal/obs's own width for a request's
// correlation id. The two are deliberately the same shape so that an operator
// greps for one thing and finds both a request's lines and a run's.
//
// The minter is duplicated rather than shared because internal/obs is on the
// PocketBase side of the import boundary and this package is not (Principle
// II). The width is asserted against the column that holds it, not against obs.
const handleBytes = 16

func newHandle() string {
	var raw [handleBytes]byte

	// crypto/rand.Read either fills the buffer or the program is already over;
	// since Go 1.24 it does not return an error at all.
	_, _ = rand.Read(raw[:])

	return hex.EncodeToString(raw[:])
}
