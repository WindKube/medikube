package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"medikube/internal/store"
	"medikube/internal/web"
)

// AuthResponseHookID names MediKube's auth-response writer so that a second
// Bind replaces rather than appends. PocketBase's hook.Bind appends when no Id
// is given, and an instance wired twice would try to write two responses to one
// request.
const AuthResponseHookID = "medikubeAuthResponse"

// AuthResponsePriority puts the writer ahead of anything else bound on the same
// hook, which in this phase is the sign-in audit row (T221).
//
// The ordering decides only which failure wins, not whether the chain runs: the
// writer ALWAYS ends with e.Next(), so every other handler bound here still
// gets its turn. Returning without it would silently skip them — the audit row
// among them — and a sign-in that is not audited looks exactly like one that is
// (research D-14). session_test.go's probe is what fails if that line goes.
const AuthResponsePriority = -10

// sessionIntentKey is where a handler leaves the answer it wants written. It is
// per REQUEST, on the event, and never on the writer: the writer is one value
// shared by every request in the process, and a field on it would be two
// concurrent sign-ins overwriting each other's response.
const sessionIntentKey = "medikubeSessionIntent"

// accountCacheControl is what an account response is served with. `private`
// keeps it out of every shared cache and `no-store` keeps it off disk: the body
// is somebody's own address and display name.
const accountCacheControl = "private, no-store"

// ErrNoAccountRecord is a session asked for an account that is not there. It is
// an internal failure and never the caller's: the service resolved the account
// a moment earlier, so a miss here is the row disappearing underneath the
// request.
var ErrNoAccountRecord = errors.New("api: the account a session was asked for is not in the account collection")

// SessionWriter is how MediKube answers with a session, and the ONE place a
// token becomes a cookie.
//
// MediKube mints no token. Every session in this application is minted by
// apis.RecordAuthResponse, which is also what fires OnRecordAuthRequest — the
// hook that writes the `login` audit row for MediKube's own route AND for
// PocketBase's native one, which stays reachable by design (research D-13,
// D-14). A handler that minted its own token would leave the native path
// unaudited and would look exactly like coverage.
//
// What MediKube does own is the RESPONSE. PocketBase's native body is
// {"record": <the whole record>, "token": "<jwt>"} — the credential readable by
// any script, with `role`, `verified` and `disabled_at` beside it. The seam is
// apis/record_helpers.go's `if e.Written() { return nil }`: a handler bound on
// the hook that writes first leaves the native writer nothing to do. This type
// is that handler.
type SessionWriter struct {
	app     core.App
	cookies web.SessionCookie
}

// sessionIntent is one request's answer, left on the event by the handler and
// picked up by the hook.
//
// The token is deliberately not in it: the hook reads that off the event, from
// the mint that has only just happened, so there is no window in which a
// handler holds a credential.
type sessionIntent struct {
	status   int
	location string

	// user is the body to write. Nil is an operation that answers with a fresh
	// cookie and no body at all — a password change (contracts/account.md).
	user *Me
}

// NewSessionWriter binds the writer and returns the seam the handlers use.
//
// Binding here rather than in a separate step is deliberate: the seam and the
// hook are one mechanism, and a composition root that built the first and
// forgot the second would hand every caller PocketBase's body with the token in
// it — a leak with no error, no log line and a passing status code.
func NewSessionWriter(app core.App, cookies web.SessionCookie) (*SessionWriter, error) {
	if app == nil {
		return nil, errors.New("api: the session writer is wired with no application, so no session could be minted")
	}

	writer := &SessionWriter{app: app, cookies: cookies}

	app.OnRecordAuthRequest(store.AccountCollection).Bind(&hook.Handler[*core.RecordAuthRequestEvent]{
		Id:       AuthResponseHookID,
		Priority: AuthResponsePriority,
		Func:     writer.write,
	})

	return writer, nil
}

// write is the hook. It answers ONLY for MediKube's own operations.
//
// A request with no intent on it is PocketBase's native sign-in, which
// contracts/README.md keeps reachable deliberately and which answers in
// PocketBase's own shape. Rewriting that one here would be MediKube quietly
// changing a documented external's contract.
func (w *SessionWriter) write(ev *core.RecordAuthRequestEvent) error {
	intent, mine := ev.Get(sessionIntentKey).(*sessionIntent)
	if !mine {
		return ev.Next()
	}

	w.cookies.Issue(ev.RequestEvent, ev.Token)

	if intent.location != "" {
		ev.Response.Header().Set("Location", intent.location)
	}

	ev.Response.Header().Set("Cache-Control", accountCacheControl)

	if err := w.body(ev, intent); err != nil {
		return err
	}

	// The rest of the chain, always. The audit row is bound here (research
	// D-14) and PocketBase's own terminal reads Written() and stands down.
	return ev.Next()
}

func (w *SessionWriter) body(ev *core.RecordAuthRequestEvent, intent *sessionIntent) error {
	if intent.user == nil {
		return ev.NoContent(intent.status)
	}

	return web.WriteJSON(ev.RequestEvent, intent.status, Session{
		User: *intent.user,
		// Read off the collection the token was just minted from rather than
		// off MediKube's configuration, so the instant in the body is the one
		// the token actually carries. internal/platform/pb's applySessionTTL
		// writes MEDIKUBE_AUTH_SESSION_TTL into this same number at boot, and
		// internal/web/session_test.go pins the cookie's Max-Age to it too.
		ExpiresAt: wireInstant(time.Now().UTC().Add(
			time.Duration(ev.Record.Collection().AuthToken.Duration) * time.Second)),
	})
}

// Issue completes an operation by handing the caller a session.
//
// method is PocketBase's authMethod. core.MFAMethodPassword is a fresh sign-in
// — it is what a `login` audit row is discriminated on, so a refresh must NOT
// use it or every renewal writes a second sign-in into a two-year medical trail
// (research D-14). The empty string is a renewal.
//
// contracts/auth.md spells the same value core.RequestInfoContextPasswordAuth.
// The two constants are byte-identical; this one names the parameter it is
// actually passed to, and session_test.go asserts they agree so the day
// PocketBase separates them is a failing test rather than a silent MFA bypass.
func (w *SessionWriter) Issue(
	e *core.RequestEvent,
	userID, method string,
	intent sessionIntent,
) error {
	record, err := w.account(e.Request.Context(), userID)
	if err != nil {
		return err
	}

	// The rule context PocketBase's own password route sets. It changes nothing
	// while the account collection's auth rule is the empty string, and it is
	// set anyway so that a rule which one day reads @request.context sees the
	// same value on MediKube's route as on the native one.
	if method == core.MFAMethodPassword {
		e.Set(core.RequestEventKeyInfoContext, core.RequestInfoContextPasswordAuth)
	}

	e.Set(sessionIntentKey, &intent)

	return apis.RecordAuthResponse(e, record, method, nil)
}

// Clear ends the browser's half of a sign-out.
//
// It is never the whole of one: the session itself ends by rotating the
// record's token key, which is what makes it unusable from everywhere else it
// was open (FR-007). A clear on its own hands the person a browser that has
// forgotten a credential that still works.
func (w *SessionWriter) Clear(e *core.RequestEvent) {
	w.cookies.Clear(e)
}

func (w *SessionWriter) account(ctx context.Context, userID string) (*core.Record, error) {
	record, err := w.app.FindRecordById(store.AccountCollection, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoAccountRecord, err)
	}

	if record == nil {
		return nil, ErrNoAccountRecord
	}

	return record, nil
}

// signedIn is the answer register, login and refresh share.
func signedIn(status int, me Me) sessionIntent {
	return sessionIntent{status: status, user: &me}
}

// reissued is the answer a password change gives: a fresh cookie, no body
// (contracts/account.md).
func reissued() sessionIntent {
	return sessionIntent{status: http.StatusNoContent}
}
