package obs

import (
	"context"
	"sync/atomic"
)

// edgeKey is the context key the per-request ledger travels under.
type edgeKey struct{}

// Edge is the ledger one request carries from the outermost net/http wrapper,
// through PocketBase's router, and back out again.
//
// It exists because those two ends sit on opposite sides of the router and have
// to agree on two things: which correlation id this request has (one request,
// one id — FR-054) and whether its one line has already been written (one
// request, one line — FR-053, Principle VI).
//
// A plain context value cannot carry the second fact back out. Every middleware
// inside the router works on a *copy* of the request with a derived context
// (RequestLogger does exactly that), and the wrapper still holds the original,
// so a value written inside is invisible outside. A pointer in the context is
// visible from both ends because both ends reach the same struct.
type Edge struct {
	correlation string
	logged      atomic.Bool
}

// NewEdge opens the ledger for one request and returns the context carrying it.
//
// inbound is the correlation header as it arrived. It is honoured when it is
// safe to put in a log field and replaced with a fresh id when it is not, which
// is the same rule the request logger applies — and applying it here, once, is
// what stops the two ends minting different ids for the same request.
func NewEdge(ctx context.Context, inbound string) (context.Context, *Edge) {
	edge := &Edge{correlation: correlationID(inbound)}

	return context.WithValue(ctx, edgeKey{}, edge), edge
}

// EdgeFrom returns the ledger ctx carries, or nil.
//
// Nil is an ordinary answer rather than a failure: it is what every code path
// that does not go through the outermost wrapper sees, which is every
// tests.ApiScenario in the repository. Every method below is nil-safe for that
// reason, so a caller never has to ask.
func EdgeFrom(ctx context.Context) *Edge {
	edge, _ := ctx.Value(edgeKey{}).(*Edge)

	return edge
}

// CorrelationID returns the id the edge minted for this request, or the empty
// string when there is no ledger — in which case the caller mints its own.
func (e *Edge) CorrelationID() string {
	if e == nil {
		return ""
	}

	return e.correlation
}

// MarkLogged records that the one line for this request has been written, so
// that the outermost wrapper knows not to write a second.
func (e *Edge) MarkLogged() {
	if e == nil {
		return
	}

	e.logged.Store(true)
}

// Logged reports whether the request's line has been written already.
func (e *Edge) Logged() bool { return e != nil && e.logged.Load() }
