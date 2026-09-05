package api_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storeidentity "medikube/internal/store/identity"
	"medikube/internal/testsupport"
	"medikube/internal/web"
)

// T196, FR-008 and the second half of FR-007.
//
// Two different ends of a session, and they have to be told apart or neither is
// really asserted: one runs out, the other is taken away. A suite that only
// proved "a bad token is refused" would pass with a session that never expired
// AND with one that expired but could not be revoked, because a forged token is
// refused by the signature long before either question is reached.
//
// So every case below carries the same control: a token minted the same way,
// for the same account, through the same transport, that IS accepted. Without
// it a broken fixture, a mis-signed token or an unauthenticated route would
// read as the refusal under test.

// usersCollection is PocketBase's own auth collection, which MediKube amends
// rather than replaces.
const usersCollection = "users"

func TestASessionOlderThanItsLifetimeIsRefused(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	fresh := tokenExpiringAt(t, caller, testsupport.AccountAEmail, time.Now().Add(time.Hour))
	expired := tokenExpiringAt(t, caller, testsupport.AccountAEmail, time.Now().Add(-time.Second))

	// The control, and it is doing real work: it proves the hand-signed token
	// is signed with the right key and carries the right claims, so that the
	// refusal below is the expiry and not a token this test built wrong.
	require.Equal(t, http.StatusOK, byHeader(caller, fresh).Status,
		"a token that differs from the expired one only in its exp was refused, so this test is measuring its own arithmetic")
	require.Equal(t, http.StatusOK, byCookie(caller, fresh).Status)

	for _, tc := range []struct {
		name     string
		response response
	}{
		{name: "presented as a bearer token", response: byHeader(caller, expired)},
		{name: "presented as the browser's cookie", response: byCookie(caller, expired)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, http.StatusUnauthorized, tc.response.Status, tc.response.Body)

			envelope := tc.response.envelope(t)
			assert.Equal(t, "unauthenticated", envelope.Error.Code)
			assert.Equal(t, "sign in to continue", envelope.Error.Message)
			assert.NotEmpty(t, envelope.Error.RequestID)
		})
	}

	// An expired session and no session at all are the same refusal here.
	// FR-008's "and told why" is the page layer's: PocketBase's loadAuthToken
	// swallows an expired token and leaves the request anonymous
	// (apis/middlewares.go:201-205), so nothing at this layer can tell the two
	// apart, and the sign-in page's ?reason=expired has to classify the cookie
	// itself. An API that guessed would be guessing.
	assert.Equal(t,
		withoutCorrelationID(byHeader(caller, "").Body),
		withoutCorrelationID(byHeader(caller, expired).Body),
		"an expired session is answered differently from an absent one, which tells a caller a session once existed")
}

// FR-007: "the ended session MUST NOT be usable again from anywhere it was
// still open" — and not merely at the end of its lifetime.
//
// The token here is a real one, minted with the configured seven-day lifetime,
// and the assertion that makes this test about revocation rather than about
// expiry is that its own exp is still days away at the moment it is refused.
// Without that clause a session that was only ever refused because the clock
// caught up would pass.
func TestEndingASessionRefusesItAtOnceRatherThanAtItsExpiry(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)
	token := testsupport.UserToken(t, caller.app, testsupport.AccountAEmail)

	require.Equal(t, http.StatusOK, byHeader(caller, token).Status)
	require.Equal(t, http.StatusOK, byCookie(caller, token).Status)

	// What signing out does, through the code that does it: SignOut calls
	// EndSessions, and EndSessions rotates the record's token key — the one
	// write that invalidates every token ever signed for the account
	// (research D-16, contracts/auth.md's logout). Driving MediKube's own
	// adapter rather than the record keeps this test pointed at the path a
	// sign-out actually takes.
	record, err := caller.app.FindAuthRecordByEmail(usersCollection, testsupport.AccountAEmail)
	require.NoError(t, err)

	before := record.TokenKey()

	authenticator, err := storeidentity.NewAuthenticator(caller.app)
	require.NoError(t, err)
	require.NoError(t, authenticator.EndSessions(t.Context(), testsupport.AccountAID))

	record, err = caller.app.FindAuthRecordByEmail(usersCollection, testsupport.AccountAEmail)
	require.NoError(t, err)
	require.NotEqual(t, before, record.TokenKey(), "nothing was rotated, so nothing was revoked")

	assert.Equal(t, http.StatusUnauthorized, byHeader(caller, token).Status)
	assert.Equal(t, http.StatusUnauthorized, byCookie(caller, token).Status,
		"the browser's cookie outlived the sign-out that the API token honoured")

	assert.WithinRange(t, expiryOf(t, token), time.Now().Add(24*time.Hour), time.Now().Add(365*24*time.Hour),
		"the refused token had already expired on its own, so this test never observed the revocation")

	// The other half of FR-010: the session that did the rotating is minted
	// from the saved record and still works. A revocation that also locked the
	// person out of the browser they were sitting at would pass every
	// assertion above.
	assert.Equal(t, http.StatusOK, byCookie(caller, mustToken(t, record)).Status)
}

// byCookie presents the session the way a browser does and in the only way a
// browser can: no Authorization header anywhere, because a plain navigation
// cannot set one.
func byCookie(c *caller, token string) response {
	return c.anonymous().do(http.MethodGet, meURL, "",
		map[string]string{"Cookie": web.SessionCookieName + "=" + token})
}

// byHeader presents the same session the way an API client does.
func byHeader(c *caller, token string) response {
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = token
	}

	return c.anonymous().do(http.MethodGet, meURL, "", headers)
}

func mustToken(t *testing.T, record *core.Record) string {
	t.Helper()

	token, err := record.NewAuthToken()
	require.NoError(t, err)

	return token
}

// tokenExpiringAt mints an auth token for a seeded account with an expiry the
// test chooses.
//
// PocketBase cannot: NewAuthToken takes the collection's lifetime and
// NewStaticAuthToken falls back to it for any duration that is not positive
// (core/record_tokens.go:41-71), so an already-expired token has to be signed
// here. It is signed with PocketBase's own key — the record's token key and the
// collection's auth secret, which is what FindAuthRecordByToken rebuilds
// (core/record_query.go:514-534) — and the control in every caller is a token
// from this same function that IS accepted, which is what proves the signing is
// right rather than merely rejected.
//
// The claims are core/record_tokens.go's, spelled through PocketBase's own
// constants so that a renamed claim is a compile error rather than a token
// nobody accepts.
func tokenExpiringAt(t *testing.T, c *caller, email string, expires time.Time) string {
	t.Helper()

	record, err := c.app.FindAuthRecordByEmail(usersCollection, email)
	require.NoError(t, err)

	return signedJWT(t, record.TokenKey()+record.Collection().AuthToken.Secret, map[string]any{
		core.TokenClaimType:         core.TokenTypeAuth,
		core.TokenClaimId:           record.Id,
		core.TokenClaimCollectionId: record.Collection().Id,
		core.TokenClaimRefreshable:  true,
		"exp":                       expires.Unix(),
	})
}

func signedJWT(t *testing.T, key string, claims map[string]any) string {
	t.Helper()

	header := segment(t, map[string]any{"alg": "HS256", "typ": "JWT"})
	payload := segment(t, claims)

	mac := hmac.New(sha256.New, []byte(key))
	_, err := mac.Write([]byte(header + "." + payload))
	require.NoError(t, err)

	return header + "." + payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func segment(t *testing.T, value map[string]any) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	require.NoError(t, err)

	return base64.RawURLEncoding.EncodeToString(encoded)
}

// expiryOf reads the exp claim without verifying anything, which is all a test
// needs to say "this token had not run out".
func expiryOf(t *testing.T, token string) time.Time {
	t.Helper()

	parts := strings.Split(token, ".")
	require.Len(t, parts, 3, "not a JWT: %s", token)

	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)

	var claims struct {
		Exp int64 `json:"exp"`
	}

	require.NoError(t, json.Unmarshal(raw, &claims))
	require.NotZero(t, claims.Exp, "the token carries no expiry at all")

	return time.Unix(claims.Exp, 0)
}
