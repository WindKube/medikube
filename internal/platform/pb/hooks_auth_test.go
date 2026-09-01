package pb_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/audit"
	"medikube/internal/obs"
	"medikube/internal/platform/pb"
	"medikube/internal/web/api"
)

// T205 and research D-14. `OnRecordAuthRequest` is the sign-in audit seam, and
// this file is the hook's own mechanics: what it writes, what it deliberately
// does not, and when it writes relative to the response.
//
// It drives PocketBase's NATIVE auth routes, because those are the ones that
// exist without MediKube's route table, and it writes into a trail this file
// holds rather than into a collection. The other half — that MediKube's own
// /api/v1/auth/login produces the SAME row from the SAME hook, which is the
// structural proof that the delegation is real — lives in
// internal/web/api/auth_audit_test.go, where both paths can be driven at once.
//
// THE SPLIT IS NOT A PREFERENCE. hooks_records_test.go's stockSchema is a
// tripwire: core.AppMigrations is a package-level registry, so a single import
// of medikube/internal/testsupport into this test binary would apply MediKube's
// migrations to every tests.NewTestApp in it, and the lockdown suite next door
// creates bare `users` records that MediKube's profile columns then refuse.
// This file therefore imports neither testsupport nor apitest.

// fixturePassword is set on the fixture account by the test rather than assumed
// of it, so a scenario cannot silently degrade into an anonymous one because a
// password changed upstream.
const fixturePassword = "medikube-hook-probe-password"

// collectedTrail is an audit trail that keeps what it was handed. A fake and
// not the real repository: what this file asserts is what the HOOK writes, and
// a storage layer between the two would let a missing field read as a passing
// test.
type collectedTrail struct {
	mu     sync.Mutex
	events []audit.Event
	refuse error
}

func (c *collectedTrail) Record(_ context.Context, event audit.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.refuse != nil {
		return c.refuse
	}

	c.events = append(c.events, event)

	return nil
}

func (c *collectedTrail) all() []audit.Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]audit.Event(nil), c.events...)
}

func (c *collectedTrail) of(action audit.Action) []audit.Event {
	var found []audit.Event

	for _, event := range c.all() {
		if event.Action == action {
			found = append(found, event)
		}
	}

	return found
}

var _ pb.Trail = (*collectedTrail)(nil)

// audited is one throwaway instance with the sign-in audit bound to a trail the
// test can read, and a fixture account whose password it knows.
func audited(t *testing.T, trail *collectedTrail) *harness {
	t.Helper()

	h := newHarness(t, func(app *tests.TestApp) {
		require.NoError(t, pb.BindAuthAudit(app, pb.AuthAudit{
			Trail:   trail,
			Request: obs.CorrelationID,
		}))

		bindMediKubeServe(app)
	})

	h.withoutMFA(t, "users")
	h.withoutMFA(t, core.CollectionNameSuperusers)

	h.setPassword(t, "users", fixtureUserEmail)
	h.setPassword(t, core.CollectionNameSuperusers, fixtureSuperuserEmail)

	return h
}

// withoutMFA configures the collection the way MediKube's own migration does
// (1756100100_users_profile.go sets users.MFA.Enabled = false), which
// PocketBase's stock test fixture does NOT.
//
// It is not tidying. `OnRecordAuthRequest` fires when the FIRST factor is
// accepted, before checkMFA has run, so on a collection with MFA switched on
// this hook records a session that a failed second factor then denies. MediKube
// has MFA off on the account collection and
// internal/web/api/auth_audit_test.go pins that, so the row means what
// data-model §3 says it means; the day anybody turns it on, that assertion
// fails and this comment is what they should read. Under-recording would be the
// worse failure for an audit trail, so the row is written either way — but it
// would be a row saying a session began when none did.
func (h *harness) withoutMFA(t *testing.T, name string) {
	t.Helper()

	collection, err := h.app.FindCollectionByNameOrId(name)
	require.NoError(t, err)

	collection.MFA.Enabled = false
	require.NoError(t, h.app.Save(collection))
}

func (h *harness) setPassword(t *testing.T, collection, email string) {
	t.Helper()

	record, err := h.app.FindAuthRecordByEmail(collection, email)
	require.NoErrorf(t, err, "find the %s fixture account %q", collection, email)

	record.SetPassword(fixturePassword)
	require.NoError(t, h.app.Save(record))
}

func (h *harness) recordID(t *testing.T, collection, email string) string {
	t.Helper()

	record, err := h.app.FindAuthRecordByEmail(collection, email)
	require.NoError(t, err)

	return record.Id
}

// The native sign-in routes, spelled as PocketBase's own collection routes.
// They are documented externals in MediKube's route table (KindExternal) and
// stay reachable by design, which is the entire reason this hook exists.
const (
	nativeUserSignIn      = "/api/collections/users/auth-with-password"
	nativeUserRefresh     = "/api/collections/users/auth-refresh"
	nativeSuperuserSignIn = "/api/collections/_superusers/auth-with-password"
	nativeSuperuserRefres = "/api/collections/_superusers/auth-refresh"
)

func credentials(identity, password string) string {
	return `{"identity":"` + identity + `","password":"` + password + `"}`
}

func TestASignInThroughPocketBasesOwnRouteIsAudited(t *testing.T) {
	t.Parallel()

	trail := &collectedTrail{}
	h := audited(t, trail)

	res := h.do(t, http.MethodPost, nativeUserSignIn, "", credentials(fixtureUserEmail, fixturePassword))
	require.Equal(t, http.StatusOK, res.Status, res.Body)

	rows := trail.of(audit.ActionLogin)
	require.Lenf(t, rows, 1, "%d login rows for one sign-in", len(rows))

	id := h.recordID(t, "users", fixtureUserEmail)

	assert.Equal(t, audit.ActorKindUser, rows[0].ActorKind)
	assert.Equal(t, id, rows[0].ActorID, "the row does not name who signed in")
	assert.Equal(t, audit.TargetKindUser, rows[0].TargetKind)
	assert.Equal(t, id, rows[0].TargetID)
	assert.NotEmpty(t, rows[0].RequestID, "the row correlates to no log line (FR-054)")
	assert.False(t, rows[0].OccurredAt.IsZero())
	assert.NoError(t, rows[0].Validate(), "the hook wrote a row the trail would refuse")

	assert.Empty(t, trail.of(audit.ActionAdminSession), "an ordinary sign-in opened an admin session")
	assert.Empty(t, trail.of(audit.ActionLoginFailed))
}

// research D-14's trap. The same hook fires for auth-refresh, so a binding that
// did not discriminate would write a second `login` row every time a browser
// extended its session — and the trail would then say a person signed in hourly
// all night.
func TestATokenRenewalWritesNothing(t *testing.T) {
	t.Parallel()

	trail := &collectedTrail{}
	h := audited(t, trail)

	res := h.do(t, http.MethodPost, nativeUserRefresh, h.userToken(t), "{}")
	require.Equal(t, http.StatusOK, res.Status, res.Body)

	assert.Empty(t, trail.all(), "a renewal was recorded as a sign-in")
}

// FR-006 and contracts/auth.md. The row names the ACCOUNT somebody aimed at and
// never the address they typed: target_id is the account id when the address
// has one and EMPTY when it does not, because writing the typed string would
// put a real person's address — possibly a stranger's, possibly a typo of one —
// into a two-year medical audit trail.
func TestARefusedSignInNamesTheAccountAndNeverTheAddress(t *testing.T) {
	t.Parallel()

	for name, attempt := range map[string]struct {
		identity, password string
		aimedAt            func(*harness, *testing.T) string
	}{
		"a known address with the wrong password": {
			identity: fixtureUserEmail, password: "not-the-password",
			aimedAt: func(h *harness, t *testing.T) string { return h.recordID(t, "users", fixtureUserEmail) },
		},
		"an address with no account": {
			identity: "nobody@example.test", password: fixturePassword,
			aimedAt: func(*harness, *testing.T) string { return "" },
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			trail := &collectedTrail{}
			h := audited(t, trail)

			res := h.do(t, http.MethodPost, nativeUserSignIn, "", credentials(attempt.identity, attempt.password))
			require.NotEqual(t, http.StatusOK, res.Status, "the credentials were accepted")

			rows := trail.of(audit.ActionLoginFailed)
			require.Lenf(t, rows, 1, "%d login_failed rows for one refusal", len(rows))

			assert.Equal(t, audit.ActorKindUser, rows[0].ActorKind)
			assert.Empty(t, rows[0].ActorID, "the row claims to know who it was, which is what the attempt failed to establish")
			assert.Equal(t, audit.TargetKindUser, rows[0].TargetKind)
			assert.Equal(t, attempt.aimedAt(h, t), rows[0].TargetID)
			assert.NoError(t, rows[0].Validate())

			assert.Empty(t, trail.of(audit.ActionLogin), "a refused sign-in was recorded as a success")
		})
	}
}

// The negative that gives every row its meaning. audit.Event has no member a
// password, a token or an address could be written into, and this asserts the
// consequence over every row three real attempts produce.
func TestNoSignInRowCanCarryACredential(t *testing.T) {
	t.Parallel()

	const attempted = "not-the-password"

	trail := &collectedTrail{}
	h := audited(t, trail)

	h.do(t, http.MethodPost, nativeUserSignIn, "", credentials(fixtureUserEmail, attempted))
	h.do(t, http.MethodPost, nativeUserSignIn, "", credentials("stranger@example.test", attempted))
	h.do(t, http.MethodPost, nativeUserSignIn, "", credentials(fixtureUserEmail, fixturePassword))

	rows := trail.all()
	require.Len(t, rows, 3)

	for _, row := range rows {
		rendered := strings.Join([]string{
			row.ActorID, row.TargetID, row.RequestID,
			string(row.Action), string(row.ActorKind), string(row.TargetKind),
		}, " ")

		assert.NotContains(t, rendered, attempted, "an audit row carries the password that was tried")
		assert.NotContains(t, rendered, fixturePassword)
		assert.NotContains(t, rendered, "@", "an audit row carries something shaped like an address")
	}
}

// The ordering, as a number here and as behaviour in
// internal/web/api/auth_audit_test.go.
//
// PocketBase mints the token before it triggers OnRecordAuthRequest, so nothing
// bound there can stop a token existing — but everything bound there runs
// before MediKube's response writer hands it over. Bound the other way round,
// a sign-in whose row could not be written would arrive with the response
// already sent and the credential already in the browser.
func TestTheAuditRowIsWrittenAheadOfTheResponseWriter(t *testing.T) {
	t.Parallel()

	assert.Less(t, pb.AuthAuditPriority, api.AuthResponsePriority,
		"the response writer runs first, so a sign-in that cannot be recorded still hands out a cookie")

	trail := &collectedTrail{refuse: assert.AnError}
	h := audited(t, trail)

	res := h.do(t, http.MethodPost, nativeUserSignIn, "", credentials(fixtureUserEmail, fixturePassword))

	assert.NotEqual(t, http.StatusOK, res.Status,
		"a sign-in nothing could record answered as though it had been: %s", res.Body)
	assert.NotContains(t, res.Body, "token", "the response carried a credential nothing recorded")
}

// A trail that cannot be written to must not swallow the refusal either: the
// caller still gets the 400 it earned, because a 500 where every other failed
// sign-in is a 400 would be an enumeration oracle of its own.
func TestARefusalStillReadsAsARefusalWhenTheTrailIsBroken(t *testing.T) {
	t.Parallel()

	trail := &collectedTrail{refuse: assert.AnError}
	h := audited(t, trail)

	res := h.do(t, http.MethodPost, nativeUserSignIn, "", credentials(fixtureUserEmail, "not-the-password"))

	assert.Equal(t, http.StatusBadRequest, res.Status, res.Body)
}

// The binding refuses to be wired without the one thing that makes it work. A
// sign-in audit bound to nothing is indistinguishable from one that is never
// reached, which is the shape of every gap this phase exists to close.
func TestTheSignInAuditRefusesToBeWiredWithNothingToWriteTo(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp()
	require.NoError(t, err)

	t.Cleanup(app.Cleanup)

	assert.Error(t, pb.BindAuthAudit(nil, pb.AuthAudit{Trail: &collectedTrail{}}))
	assert.Error(t, pb.BindAuthAudit(app, pb.AuthAudit{}))
	assert.NoError(t, pb.BindAuthAudit(app, pb.AuthAudit{Trail: &collectedTrail{}}))
}

// Binding twice replaces rather than appends, so an instance wired twice writes
// one row and not two. PocketBase's hook.Bind appends when no id is given, and
// two rows for one sign-in is indistinguishable from a handler auditing on its
// own.
func TestBindingTheSignInAuditTwiceStillWritesOneRow(t *testing.T) {
	t.Parallel()

	trail := &collectedTrail{}
	h := audited(t, trail)

	require.NoError(t, pb.BindAuthAudit(h.app, pb.AuthAudit{Trail: trail, Request: obs.CorrelationID}))

	res := h.do(t, http.MethodPost, nativeUserSignIn, "", credentials(fixtureUserEmail, fixturePassword))
	require.Equal(t, http.StatusOK, res.Status, res.Body)

	assert.Len(t, trail.of(audit.ActionLogin), 1)
}

// contracts/auth.md prescribes core.RequestInfoContextPasswordAuth as the auth
// method; the hook discriminates on core.MFAMethodPassword. The two are
// byte-identical today, and the day PocketBase separates them this is a failing
// test rather than a `login` row that quietly stops being written.
func TestTheSignInDiscriminatorIsTheValuePocketBaseSets(t *testing.T) {
	t.Parallel()

	assert.Equal(t, core.RequestInfoContextPasswordAuth, core.MFAMethodPassword)
}

// The clock is injected, so the timestamp on a row is a value a test can pin
// rather than whatever time.Now answered.
func TestTheRowCarriesTheClockItWasGiven(t *testing.T) {
	t.Parallel()

	moment := time.Date(2026, time.March, 4, 5, 6, 7, 0, time.UTC)
	trail := &collectedTrail{}

	h := newHarness(t, func(app *tests.TestApp) {
		require.NoError(t, pb.BindAuthAudit(app, pb.AuthAudit{
			Trail: trail,
			Now:   func() time.Time { return moment },
		}))

		bindMediKubeServe(app)
	})

	h.withoutMFA(t, "users")
	h.setPassword(t, "users", fixtureUserEmail)

	res := h.do(t, http.MethodPost, nativeUserSignIn, "", credentials(fixtureUserEmail, fixturePassword))
	require.Equal(t, http.StatusOK, res.Status, res.Body)

	rows := trail.of(audit.ActionLogin)
	require.Len(t, rows, 1)
	assert.Equal(t, moment, rows[0].OccurredAt)
}
