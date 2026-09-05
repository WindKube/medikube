package stream

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/realtime"
	"medikube/internal/records/recordstest"
)

// FR-007: "the ended session MUST NOT be usable again from anywhere it was
// still open." An open stream is exactly somewhere it was still open, and it
// was the one place nothing checked.
//
// Authorization ran per event FOR THE RECORD and never FOR THE IDENTITY: the
// actor is built once by web.WithActor at subscribe and was then frozen for the
// life of the connection. Revoking the session stopped every ordinary request
// at PocketBase's loadAuthToken and did nothing at all to an open stream —
// measured on a real socket, rows written after the revocation were delivered
// in full and rendered.
//
// The mechanical half is here; internal/web/stream's external suite revokes a
// real session against a real instance.

// ended asserts the stream closed without sending another event. A beat that
// passed its identity check just before the session ended may still be in
// flight; the loop returns the moment a check fails, so nothing can follow one.
func (r *rig) ended() {
	r.t.Helper()

	for {
		select {
		case f, open := <-r.frames:
			if !open {
				return
			}

			require.Equalf(r.t, "datastar-patch-signals", f.Event, "the stream sent an event after its session ended: %+v", f)
		case <-time.After(5 * time.Second):
			r.t.Fatalf("the stream neither ended nor sent anything (handler error: %v)", r.failure())
		}
	}
}

func TestARevokedSessionReceivesNothingMoreAndTheStreamEnds(t *testing.T) {
	t.Parallel()

	rig := newRig(t)

	id := seeded(t, rig)

	rig.drainHeartbeat()

	// The control: this session was working a moment ago, so what stops it
	// below is the revocation and not a broken pipeline.
	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: id, OwnerID: recordstest.OwnerID})
	require.Equal(t, rowSelector(kind.Medication, id), rig.next().selector())

	rig.sessions.end()

	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: id, OwnerID: recordstest.OwnerID})

	rig.ended()

	assert.NoError(t, rig.failure(),
		"a session ending is the specified end of the stream and not a failure to report")
	assert.Empty(t, rig.authorizer.consultations()[1:],
		"the record checkpoint was consulted for a subscriber whose session had already ended")
}

// The identity is re-checked on the beat as well as on the event, which is what
// bounds how long a revoked session can hold a connection open on an account
// nobody is writing to. Without it a stream on a quiet account would outlive
// the revocation until the next write, and there is no write on a quiet
// account.
func TestARevokedSessionEndsOnTheNextBeatWithNoEventAtAll(t *testing.T) {
	t.Parallel()

	rig := newRig(t, withHeartbeat(20*time.Millisecond))

	rig.drainHeartbeat()

	rig.sessions.end()

	rig.ended()

	assert.NoError(t, rig.failure())
}

// The beat re-checks the identity, so a quiet account cannot hold a revoked
// session open until somebody writes.
func TestTheBeatReChecksTheIdentity(t *testing.T) {
	t.Parallel()

	rig := newRig(t, withHeartbeat(20*time.Millisecond))

	rig.drainHeartbeat()

	rig.sessions.end()

	rig.ended()

	assert.Positive(t, rig.sessions.checks(), "the identity was never re-checked at all")
}

// A re-check that could not be MADE is not a session that ended. Both stop the
// stream — nothing may keep carrying somebody's records when nothing can say
// whose they still are — but this one is reported, because a stream ending
// because the database is unreachable is not routine.
func TestAnIdentityCheckThatCouldNotBeMadeEndsTheStreamAndIsReported(t *testing.T) {
	t.Parallel()

	rig := newRig(t)

	id := seeded(t, rig)

	rig.drainHeartbeat()

	broken := errors.New("the session could not be re-checked")
	rig.sessions.failWith(broken)

	rig.publish(realtime.Event{Kind: kind.Medication, RecordID: id, OwnerID: recordstest.OwnerID})

	rig.ended()

	assert.ErrorIs(t, rig.failure(), broken)
}

// A stream that cannot re-check its identity must not open at all. Refusing
// before the header block is committed is what makes the refusal a status a
// client can read, rather than a stream that looks healthy and is never
// re-checked again.
func TestAStreamThatCouldNotReCheckItsIdentityNeverOpens(t *testing.T) {
	t.Parallel()

	rig := newRig(t, withNoSession())

	assert.NotEqual(t, http.StatusOK, rig.response.StatusCode,
		"a stream that cannot notice its session ending was opened anyway")
	assert.ErrorIs(t, rig.failure(), ErrNoSession)
}

// The production implementation refuses the same way, and for the two reasons
// that are structural rather than the caller's: nothing to re-check, and
// nothing to re-check it against.
func TestTheProductionSessionRefusesARequestItCannotReCheck(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		token string
	}{
		{name: "a request carrying no token"},
		{name: "a request with no application behind it", token: "a.b.c"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			e := new(core.RequestEvent)
			e.Request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/streams/records", nil)

			if testCase.token != "" {
				e.Request.Header.Set("Authorization", testCase.token)
			}

			session, err := tokenSessions{}.Open(e)

			require.ErrorIs(t, err, ErrNoSession)
			assert.Nil(t, session)
		})
	}
}

// The production implementation reads the token exactly as PocketBase's own
// loadAuthToken does, prefix optional (apis/middlewares.go:211). A token the
// router accepted and this could not re-check would be a session that never
// ends.
func TestTheTokenIsReadTheWayPocketBaseReadsIt(t *testing.T) {
	t.Parallel()

	const token = "eyJhbGciOiJIUzI1NiJ9.e30.signature"

	cases := []struct {
		name   string
		header string
		want   string
	}{
		{name: "bare", header: token, want: token},
		{name: "the Bearer scheme", header: "Bearer " + token, want: token},
		{name: "the scheme in any case", header: "bEaReR " + token, want: token},
		{name: "no header at all", header: "", want: ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, bearer(testCase.header))
		})
	}
}
