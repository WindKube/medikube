package stream

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainidentity "medikube/internal/domain/identity"
	service "medikube/internal/service/identity"
	pbidentity "medikube/internal/store/identity"

	// The migrations register themselves from their own init and
	// tests.NewTestApp runs core.AppMigrations against the instance. Without
	// this import the accounts below would be written into PocketBase's stock
	// schema, which has none of the columns the account repository writes.
	_ "medikube/internal/store/migrations"
)

// T217's seam, and the only test in the repository that runs BOTH halves of it.
//
// internal/store/identity ends a session by moving the record's `tokenKey`.
// This file's tokenSessions asks whether a session is still live by handing the
// token back to FindAuthRecordByToken, which rebuilds its signing key as
// `record.TokenKey() + collection.AuthToken.Secret`. The two are the same field
// or one of them is decorative — and the decorative one would be silent: the
// store's own tests would pass, this package's own tests would pass (they run
// against a fake Session), and a live feed of somebody's medical records would
// outlive the password change that was meant to close it.
//
// Nothing here restates the mechanism. The rotation is the real adapter's and
// the check is the real production Session, so a change to either that stopped
// them agreeing fails here.

//nolint:gosec // an in-memory test credential
const rotationPassword = "a-perfectly-ordinary-passphrase"

type rotationRig struct {
	app  *tests.TestApp
	auth *pbidentity.Authenticator
	repo *pbidentity.Repository
	user domainidentity.User
}

func newRotationRig(t *testing.T) rotationRig {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	repo, err := pbidentity.NewRepository(app)
	require.NoError(t, err)

	auth, err := pbidentity.NewAuthenticator(app)
	require.NoError(t, err)

	user, err := repo.Create(t.Context(), domainidentity.User{
		Email:      "streaming@example.test",
		Name:       "Streaming Person",
		Role:       domainidentity.DefaultRole,
		UnitSystem: domainidentity.DefaultUnitSystem,
		Locale:     domainidentity.DefaultLocale,
		DateFormat: domainidentity.DefaultDateFormat,
		Theme:      domainidentity.DefaultTheme,
	}, rotationPassword)
	require.NoError(t, err)

	return rotationRig{app: app, auth: auth, repo: repo, user: user}
}

// openStream is a stream opened the way a browser opens one: an auth token, in
// the Authorization header, through the production Session.
func (r rotationRig) openStream(t *testing.T) Session {
	t.Helper()

	record, err := r.app.FindRecordById("users", r.user.ID)
	require.NoError(t, err)

	token, err := record.NewAuthToken()
	require.NoError(t, err)

	e := new(core.RequestEvent)
	e.App = r.app
	e.Request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/streams/records", nil)
	e.Request.Header.Set("Authorization", "Bearer "+token)

	session, err := tokenSessions{}.Open(e)
	require.NoError(t, err)

	return session
}

// TestAnAccountChangeThatEndsASessionEndsAnOpenStreamWithIt.
//
// Both revocations are here and each is asserted on its own, because they reach
// the token key by different routes: a password change is rotated by PocketBase
// inside onRecordSaveExecute, and a sign-out is rotated by MediKube calling
// RefreshTokenKey — nothing about a sign-out changes the password, so the
// automatic refresh never fires for it. Either one can break while the other
// works.
func TestAnAccountChangeThatEndsASessionEndsAnOpenStreamWithIt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		end  func(t *testing.T, rig rotationRig)
	}{
		{
			name: "a password change",
			end: func(t *testing.T, rig rotationRig) {
				t.Helper()
				require.NoError(t, rig.auth.SetPassword(t.Context(), rig.user.ID,
					"a-second-perfectly-ordinary-passphrase"))
			},
		},
		{
			name: "a sign-out",
			end: func(t *testing.T, rig rotationRig) {
				t.Helper()
				require.NoError(t, rig.auth.EndSessions(t.Context(), rig.user.ID))
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rig := newRotationRig(t)

			phone := rig.openStream(t)
			laptop := rig.openStream(t)

			require.NoError(t, phone.Live(t.Context()), "a stream was over before anything happened to it")
			require.NoError(t, laptop.Live(t.Context()))

			testCase.end(t, rig)

			assert.ErrorIs(t, phone.Live(t.Context()), ErrSessionEnded,
				"the stream on the device the change was made from is still open")
			assert.ErrorIs(t, laptop.Live(t.Context()), ErrSessionEnded,
				"another device's live feed of this person's records outlived the session that opened it (FR-007, FR-010)")
		})
	}
}

// The negative, and the half that would be missed.
//
// A stream that ended on every write would be a stream nobody could keep open:
// the same save path carries a theme change and a password change, so a
// rotation reached unconditionally would close every open feed on every
// preference change — and every assertion above would still pass.
func TestChangingAPreferenceLeavesAnOpenStreamOpen(t *testing.T) {
	t.Parallel()

	rig := newRotationRig(t)

	open := rig.openStream(t)
	require.NoError(t, open.Live(t.Context()))

	changed := rig.user
	changed.Theme = domainidentity.ThemeDark

	_, err := rig.repo.Update(t.Context(), changed)
	require.NoError(t, err)

	assert.NoError(t, open.Live(t.Context()),
		"changing a preference closed every open stream the account had")
}

// A stream opened after the revocation is a new session and streams normally.
// Without this the two above are satisfied by an adapter that broke
// authentication outright.
func TestAStreamOpenedAfterTheRevocationIsLive(t *testing.T) {
	t.Parallel()

	rig := newRotationRig(t)

	before := rig.openStream(t)
	require.NoError(t, before.Live(t.Context()))

	require.NoError(t, rig.auth.EndSessions(t.Context(), rig.user.ID))
	require.ErrorIs(t, before.Live(t.Context()), ErrSessionEnded)

	after := rig.openStream(t)
	assert.NoError(t, after.Live(t.Context()), "the account can no longer open a stream at all")
}

// Redeeming a recovery link is the third route to the same key, and it is the
// one the store's contract observes: a link is signed with the record's own
// key, so a change that ends the sessions spends every outstanding link in the
// same write. Asserted here against the production Session so that "the link
// died" and "the stream died" are known to be the same event and not two
// mechanisms that happen to agree today.
func TestThePasswordChangeThatEndsTheStreamSpendsTheRecoveryLinkToo(t *testing.T) {
	t.Parallel()

	rig := newRotationRig(t)

	record, err := rig.app.FindRecordById("users", rig.user.ID)
	require.NoError(t, err)

	link, err := record.NewPasswordResetToken()
	require.NoError(t, err)

	open := rig.openStream(t)
	require.NoError(t, open.Live(t.Context()))

	redeemed, err := rig.auth.Redeem(t.Context(), service.TokenPasswordReset, link)
	require.NoError(t, err, "a freshly minted link did not resolve, so the case below would pass vacuously")
	require.Equal(t, rig.user.ID, redeemed.ID)

	require.NoError(t, rig.auth.SetPassword(t.Context(), rig.user.ID,
		"a-third-perfectly-ordinary-passphrase"))

	assert.ErrorIs(t, open.Live(t.Context()), ErrSessionEnded)

	_, err = rig.auth.Redeem(t.Context(), service.TokenPasswordReset, link)
	assert.ErrorIs(t, err, service.ErrInvalidToken,
		"the stream ended and the link did not, so the two are not reading the same key")
}
