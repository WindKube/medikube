package obs

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// throughEdge drives one request through the request logger with a ledger
// already open on it, the way the outermost net/http wrapper opens one, and
// returns the ledger and the response.
func throughEdge(t *testing.T, inbound string) (*Edge, *httptest.ResponseRecorder) {
	t.Helper()

	buf, base := capture(t)
	t.Cleanup(func() {
		require.NotEmpty(t, buf.String(), "the request logger wrote nothing, so nothing here proves anything")
	})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	if inbound != "" {
		request.Header.Set(CorrelationHeader, inbound)
	}

	ctx, edge := NewEdge(request.Context(), request.Header.Get(CorrelationHeader))
	request = request.WithContext(ctx)

	recorder := httptest.NewRecorder()

	e := &core.RequestEvent{}
	e.Request = request
	e.Response = &router.ResponseWriter{ResponseWriter: recorder}

	var chain hook.Hook[*core.RequestEvent]
	chain.Bind(RequestLogger(base))

	require.NoError(t, chain.Trigger(e, func(e *core.RequestEvent) error {
		return e.NoContent(http.StatusNoContent)
	}))

	return edge, recorder
}

// One request, one id (FR-054). The outermost wrapper mints the id before
// PocketBase's router is entered — it has to, because it answers responses the
// router never sees — and the request logger has to take that one back rather
// than mint a second from the same header. Two ids for one request is an
// operator holding the id a person quoted and finding nothing under it.
func TestTheRequestLoggerTakesTheEdgesIDRatherThanMintingASecond(t *testing.T) {
	t.Parallel()

	t.Run("a minted id", func(t *testing.T) {
		t.Parallel()

		edge, recorder := throughEdge(t, "")

		assert.Equal(t, edge.CorrelationID(), recorder.Header().Get(CorrelationHeader),
			"the request logger minted a second id for a request that already had one")
	})

	t.Run("an id the edge honoured from the header", func(t *testing.T) {
		t.Parallel()

		edge, recorder := throughEdge(t, "from-the-proxy")

		require.Equal(t, "from-the-proxy", edge.CorrelationID())
		assert.Equal(t, edge.CorrelationID(), recorder.Header().Get(CorrelationHeader))
	})

	t.Run("free text the edge refused", func(t *testing.T) {
		t.Parallel()

		edge, recorder := throughEdge(t, `a value with spaces and "quotes"`)

		require.NotEqual(t, `a value with spaces and "quotes"`, edge.CorrelationID(),
			"free text was taken as a correlation id")
		assert.Equal(t, edge.CorrelationID(), recorder.Header().Get(CorrelationHeader),
			"the two ends disagreed about which id replaced the refused one")
	})
}

// One request, one line (FR-053, Principle VI). The wrapper writes the line for
// requests that never reached the router, and the only thing stopping it
// writing a second for every request that did is this claim.
func TestTheRequestLoggerClaimsTheLedgerOnceItHasWrittenTheLine(t *testing.T) {
	t.Parallel()

	edge, _ := throughEdge(t, "")

	assert.True(t, edge.Logged(),
		"the request logger left the ledger unclaimed, so the outermost wrapper logs the same request again")
}

// Nil is the ordinary answer everywhere the outermost wrapper is not: every
// tests.ApiScenario in the repository drives the mux directly. A ledger that
// panicked there would take the whole HTTP tier with it.
func TestTheLedgerIsNilSafeWhereThereIsNoOutermostWrapper(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)

	edge := EdgeFrom(request.Context())
	require.Nil(t, edge, "a request that never passed the wrapper should carry no ledger")

	assert.Empty(t, edge.CorrelationID())
	assert.False(t, edge.Logged())
	assert.NotPanics(t, edge.MarkLogged)
}

func TestTheLedgerAppliesTheSameInboundIDRuleTheRequestLoggerDoes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		inbound string
		kept    bool
	}{
		{"an opaque id from a reverse proxy", "7f62bc058a7764e6bffcf2c75c86fa4c", true},
		{"the punctuation an id may carry", "req-1.2_3", true},
		{"a newline, which is a log injection", "id\nlevel=fatal", false},
		{"a quote, which is the same thing in JSON", `id","x":"`, false},
		{"a space", "two words", false},
		{"longer than the bound", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"nothing at all", "", false},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			_, edge := NewEdge(t.Context(), one.inbound)

			if one.kept {
				assert.Equal(t, one.inbound, edge.CorrelationID())

				return
			}

			assert.NotEqual(t, one.inbound, edge.CorrelationID())
			assert.Len(t, edge.CorrelationID(), 32, "the replacement is a fresh 16-byte hex id")
		})
	}
}
