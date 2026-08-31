package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/audit"
	"medikube/internal/httproute"
	"medikube/internal/store"
	"medikube/internal/testsupport"
	"medikube/internal/web/apitest"
)

// T205 and research D-14, asserted where both paths to a session can be driven
// at once.
//
// There are TWO ways to a valid session in this application: MediKube's
// /api/v1/auth/login and PocketBase's native auth-with-password, which
// contracts/README.md keeps reachable deliberately. The `login` audit row is
// written from OnRecordAuthRequest so that ONE piece of code covers both.
//
// This file is the structural proof that the delegation is real. Reimplement
// authentication inside MediKube's handler and the native path stops producing
// a row; "fix" that by writing the row from the handler and the native path is
// STILL unaudited. Neither failure can be papered over by moving the write —
// which is the whole reason the row is not written where it would be easiest.
//
// The hook's own mechanics are asserted next door in
// internal/platform/pb/hooks_auth_test.go, against a fake trail. What can only
// be asserted here is that the two paths and the one trail meet.

// signInPath is one of the two ways in, addressed through the route table
// rather than by a path spelled here.
type signInPath struct {
	name string
	path string
	body func(email, password string) string
}

func signInPaths(t *testing.T) []signInPath {
	t.Helper()

	native := routeFor(t, "nativeUserAuthWithPassword")

	return []signInPath{
		{
			name: "MediKube's own route",
			path: loginURL,
			body: func(email, password string) string {
				return body("email", quoted(email), "password", quoted(password))
			},
		},
		{
			name: "PocketBase's native route, reachable by design",
			path: native,
			body: func(email, password string) string {
				return body("identity", quoted(email), "password", quoted(password))
			},
		},
	}
}

func routeFor(t *testing.T, opID string) string {
	t.Helper()

	for _, route := range httproute.Inventory().Routes() {
		if route.OpID == opID {
			return route.Path
		}
	}

	require.FailNowf(t, "missing route", "the route table no longer declares %s", opID)

	return ""
}

func rowsOfAction(t *testing.T, instance *apitest.Instance, action audit.Action) []audit.Event {
	t.Helper()

	var found []audit.Event

	for _, event := range apitest.Events(t, instance.App) {
		if event.Action == action {
			found = append(found, event)
		}
	}

	return found
}

func TestASignInThroughEitherPathWritesTheSameLoginRow(t *testing.T) {
	t.Parallel()

	for _, path := range signInPaths(t) {
		t.Run(path.name, func(t *testing.T) {
			t.Parallel()

			instance := newRig(t)

			answer := instance.anonymous().post(path.path, path.body(testsupport.AccountAEmail, testsupport.Password))
			require.Equal(t, http.StatusOK, answer.Status, answer.Body)

			rows := rowsOfAction(t, instance.instance, audit.ActionLogin)
			require.Lenf(t, rows, 1, "%s produced %d login rows", path.name, len(rows))

			assert.Equal(t, audit.ActorKindUser, rows[0].ActorKind)
			assert.Equal(t, testsupport.AccountAID, rows[0].ActorID, "the row does not name who signed in")
			assert.Equal(t, audit.TargetKindUser, rows[0].TargetKind)
			assert.Equal(t, testsupport.AccountAID, rows[0].TargetID)
			assert.NotEmpty(t, rows[0].RequestID, "the row correlates to no log line (FR-054)")

			assert.Empty(t, rowsOfAction(t, instance.instance, audit.ActionLoginFailed),
				"a successful sign-in was recorded as a failure")
			assert.Empty(t, rowsOfAction(t, instance.instance, audit.ActionAdminSession),
				"an ordinary sign-in opened an admin session")
		})
	}
}

// FR-006. Every refused sign-in leaves EXACTLY ONE row whichever path it came
// through — the service writes MediKube's, the hook writes PocketBase's, and
// neither writes the other's. A second writer is invisible to any test that
// only asks whether a row exists.
func TestARefusedSignInWritesExactlyOneRowWhicheverPathItCameThrough(t *testing.T) {
	t.Parallel()

	for _, path := range signInPaths(t) {
		for name, attempt := range map[string]struct {
			email, password, aimedAt string
		}{
			"a known address with the wrong password": {
				email: testsupport.AccountAEmail, password: "not-the-password",
				aimedAt: testsupport.AccountAID,
			},
			"an address with no account": {
				email: "nobody@example.test", password: testsupport.Password,
				aimedAt: "",
			},
		} {
			t.Run(path.name+", "+name, func(t *testing.T) {
				t.Parallel()

				instance := newRig(t)

				answer := instance.anonymous().post(path.path, path.body(attempt.email, attempt.password))
				require.NotEqual(t, http.StatusOK, answer.Status, "the credentials were accepted")

				rows := rowsOfAction(t, instance.instance, audit.ActionLoginFailed)
				require.Lenf(t, rows, 1, "%d login_failed rows for one refusal", len(rows))

				assert.Equal(t, audit.ActorKindUser, rows[0].ActorKind)
				assert.Equal(t, audit.TargetKindUser, rows[0].TargetKind)

				// contracts/auth.md: the row names the account somebody aimed
				// at and NEVER the address they typed. Writing the typed string
				// would put a real person's address — possibly a stranger's,
				// possibly a typo of one — into a two-year medical trail.
				assert.Equal(t, attempt.aimedAt, rows[0].TargetID)

				assert.Empty(t, rowsOfAction(t, instance.instance, audit.ActionLogin),
					"a refused sign-in was recorded as a success")
			})
		}
	}
}

// research D-14's trap. OnRecordAuthRequest fires for auth-refresh as well as
// for a sign-in, so a naive binding writes a second `login` row every time a
// browser extends its session, and the trail then says a person signed in
// hourly all night.
func TestARenewalIsNotASecondSignIn(t *testing.T) {
	t.Parallel()

	for name, path := range map[string]string{
		"MediKube's own renewal":      refreshURL,
		"PocketBase's native renewal": routeFor(t, "nativeUserAuthRefresh"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			instance := newRig(t)
			require.Empty(t, rowsOfAction(t, instance.instance, audit.ActionLogin),
				"the fixture already holds a login row")

			answer := instance.as(testsupport.AccountAEmail).post(path, "{}")
			require.Equal(t, http.StatusOK, answer.Status, answer.Body)

			assert.Empty(t, rowsOfAction(t, instance.instance, audit.ActionLogin),
				"a renewal was recorded as a sign-in, so a browser extending its session writes a row an hour")
		})
	}
}

// The negative that gives every row above its meaning. audit.Event has no
// member a credential could be written into, and this asserts the consequence
// over every row three real attempts produce.
func TestNoSignInRowCarriesAnAddressOrACredential(t *testing.T) {
	t.Parallel()

	const attempted = "not-the-password"

	for _, path := range signInPaths(t) {
		t.Run(path.name, func(t *testing.T) {
			t.Parallel()

			instance := newRig(t)
			anonymous := instance.anonymous()

			anonymous.post(path.path, path.body(testsupport.AccountAEmail, attempted))
			anonymous.post(path.path, path.body("stranger@example.test", attempted))
			anonymous.post(path.path, path.body(testsupport.AccountAEmail, testsupport.Password))

			events := apitest.Events(t, instance.instance.App)
			require.NotEmpty(t, events)

			for _, event := range events {
				rendered := strings.Join([]string{
					event.ActorID, event.TargetID, event.RequestID,
					string(event.Action), string(event.ActorKind), string(event.TargetKind),
				}, " ")

				assert.NotContains(t, rendered, attempted, "an audit row carries the password that was tried")
				assert.NotContains(t, rendered, "@", "an audit row carries something shaped like an address")
			}
		})
	}
}

// The ordering, as behaviour rather than as a number: a sign-in whose row
// cannot be written hands out NO session. PocketBase mints the token before it
// triggers the hook, so nothing can stop a token existing — but the row is
// written before the response writer hands it over, and that is the difference
// between an unrecorded sign-in and a failed one.
func TestASignInNothingCouldRecordHandsOverNoSession(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	// Rebinding by the same hook id REPLACES rather than appends, so this is
	// the production wiring with one part broken and nothing else changed.
	require.NoError(t, breakTheTrail(instance))

	for _, path := range signInPaths(t) {
		t.Run(path.name, func(t *testing.T) {
			answer := instance.anonymous().post(path.path, path.body(testsupport.AccountAEmail, testsupport.Password))

			assert.NotEqual(t, http.StatusOK, answer.Status,
				"a sign-in nothing could record answered as though it had been: %s", answer.Body)
			assert.Nil(t, answer.sessionCookie(t), "the browser was handed a session nothing recorded")
			assert.NotContains(t, answer.Body, "token")
		})
	}
}

// The `login` row says a session BEGAN, and that is only true while the account
// collection has no second factor for the row to outrun.
//
// OnRecordAuthRequest fires when the FIRST factor is accepted, before
// PocketBase's checkMFA has run, so on a collection with MFA switched on the
// hook would record a session that a failed second factor then denies.
// MediKube's own migration sets users.MFA.Enabled = false and this is what
// makes that assumption load-bearing: turn it on and this fails, rather than
// the trail quietly starting to claim sessions that never began.
func TestTheAccountCollectionHasNoSecondFactorForTheLoginRowToOutrun(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	collection, err := instance.instance.App.FindCollectionByNameOrId(store.AccountCollection)
	require.NoError(t, err)

	assert.False(t, collection.MFA.Enabled,
		"a second factor was enabled on the account collection: the login audit row is written when the "+
			"first factor is accepted and would now claim sessions that never began "+
			"(internal/platform/pb/hooks.go's recordSignIn)")
}
