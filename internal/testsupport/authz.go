package testsupport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The matrix's defaults, and each of them is a requirement rather than a
// convenience.
const (
	// A stranger is told the record is not there, never that it is theirs to
	// be refused. 403 confirms the identifier exists, which turns a list of
	// guesses into a list of real records.
	defaultStrangerStatus = http.StatusNotFound
	// A caller with no credentials is asked for them.
	defaultGuestStatus = http.StatusUnauthorized
	// The ordinary success. A case whose owner leg is a 201 or a 204 says so.
	defaultOwnerStatus = http.StatusOK
)

// Identity applies whatever proves who is calling to a request the matrix is
// about to send. It is a function rather than a token string because MediKube
// authenticates a browser with a session cookie and an API client with a
// header, and the matrix has to run both without knowing which is which.
type Identity func(*http.Request)

// BearerToken presents a PocketBase auth token. The prefix is optional to
// PocketBase (apis/middlewares.go:216) and written anyway, because the same
// case is run against MediKube's own routes by later phases.
func BearerToken(token string) Identity {
	return func(request *http.Request) {
		request.Header.Set("Authorization", "Bearer "+token)
	}
}

// SessionCookie presents a signed-in browser's cookie.
func SessionCookie(cookie *http.Cookie) Identity {
	return func(request *http.Request) {
		request.AddCookie(cookie)
	}
}

// OwnershipCase is one operation on one record, described once and run three
// ways: as the person who owns it, as a signed-in stranger, and as nobody.
//
// Phases 002-005 extend the table with their own kinds. They do not write a
// second matrix, because every authorization hole this catches is a hole that
// is only visible when all three legs are run against the same URL.
type OwnershipCase struct {
	// Name is the operation in words — "read one", "change the dose".
	Name string

	Method string

	// Path carries the owner's real identifier, not a placeholder. A matrix
	// that addressed a record that did not exist would have the stranger
	// refused for the wrong reason and would prove nothing.
	Path string

	Body        string
	ContentType string

	// The three expected outcomes. Zero means the default: 200, 404, 401.
	OwnerStatus    int
	StrangerStatus int
	GuestStatus    int

	// MissingPath addresses an identifier that has never existed. When it is
	// set, the matrix asserts the stranger's whole response is byte-identical
	// to the owner's response for this path — which is what "do not confirm the
	// identifier exists" actually means, as opposed to merely returning the
	// same status code.
	//
	// Leave it empty when the body carries a correlation id or a timestamp:
	// those differ per request and the comparison would fail on them rather
	// than on a disclosure.
	MissingPath string

	// Secrets must appear in neither the stranger's nor the guest's response.
	// The record's own identifier belongs here, and so does anything of the
	// owner's the response would carry — a name, a dose, a note.
	//
	// A case that names none is refused. "Nothing leaked" is not a claim a test
	// can make without saying what would have counted as a leak.
	Secrets []string
}

// OwnershipMatrix is the table and the three things needed to run it.
type OwnershipMatrix struct {
	// Handler is the application under test. router.Router.BuildMux() produces
	// one; so does an httptest server's handler.
	Handler http.Handler

	Owner    Identity
	Stranger Identity

	Cases []OwnershipCase
}

// RunOwnershipMatrix runs every case three ways and is the only ownership test
// any phase writes.
//
// Constitution Principle III requires an authorization test on every endpoint
// that touches a person's records. Written by hand each time, that test drifts:
// one endpoint checks the stranger gets refused, the next checks the owner
// succeeds, and the third checks neither. This is the shape, once.
func RunOwnershipMatrix(t *testing.T, matrix OwnershipMatrix) {
	t.Helper()

	require.NotNil(t, matrix.Handler, "the matrix has nothing to run against")
	require.NotNil(t, matrix.Owner, "the matrix needs an owner to succeed as")
	require.NotNil(t, matrix.Stranger, "the matrix needs a stranger to be refused as")
	require.NotEmpty(t, matrix.Cases, "an empty matrix asserts nothing")

	for _, one := range matrix.Cases {
		t.Run(one.Name, func(t *testing.T) {
			runOwnershipCase(t, matrix, one)
		})
	}
}

func runOwnershipCase(t *testing.T, matrix OwnershipMatrix, one OwnershipCase) {
	t.Helper()

	require.NotEmpty(t, one.Method, "%s: no method", one.Name)
	require.NotEmpty(t, one.Path, "%s: no path", one.Name)
	require.NotEmpty(t, one.Secrets,
		"%s: the case names nothing that must not leak, so its refusal legs assert only a status code", one.Name)

	t.Run("the owner succeeds", func(t *testing.T) {
		status, _ := send(t, matrix.Handler, one, one.Path, matrix.Owner)

		assert.Equal(t, orStatus(one.OwnerStatus, defaultOwnerStatus), status,
			"the owner cannot reach their own record, so nothing below this proves anything")
	})

	t.Run("a stranger is refused", func(t *testing.T) {
		status, body := send(t, matrix.Handler, one, one.Path, matrix.Stranger)

		assert.NotEqual(t, http.StatusForbidden, status,
			"403 tells a stranger the identifier exists, which turns guessing into enumeration; the answer is 404")
		assert.GreaterOrEqual(t, status, http.StatusBadRequest,
			"a stranger reached someone else's record")
		assert.Equal(t, orStatus(one.StrangerStatus, defaultStrangerStatus), status)

		assertNoSecrets(t, "the stranger's response", body, one.Secrets)

		if one.MissingPath != "" {
			missingStatus, missingBody := send(t, matrix.Handler, one, one.MissingPath, matrix.Owner)

			assert.Equal(t, missingStatus, status,
				"a record that exists and one that never did answer differently")
			assert.Equal(t, missingBody, body,
				"the refusal is distinguishable from a genuine miss, so the identifier is confirmed by the body")
		}
	})

	t.Run("a guest is refused", func(t *testing.T) {
		status, body := send(t, matrix.Handler, one, one.Path, nil)

		assert.Equal(t, orStatus(one.GuestStatus, defaultGuestStatus), status)
		assertNoSecrets(t, "the guest's response", body, one.Secrets)
	})
}

// send drives one request through the handler in memory. No socket is opened,
// which is what makes a table of these cheap enough that every endpoint gets
// one.
func send(t *testing.T, handler http.Handler, one OwnershipCase, path string, who Identity) (int, string) {
	t.Helper()

	var body io.Reader
	if one.Body != "" {
		body = strings.NewReader(one.Body)
	}

	// The test's context, so a request outlives neither the case nor a
	// cancelled run.
	request := httptest.NewRequestWithContext(t.Context(), one.Method, path, body)

	if one.Body != "" {
		request.Header.Set("Content-Type", orString(one.ContentType, "application/json"))
	}

	if who != nil {
		who(request)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder.Code, recorder.Body.String()
}

// assertNoSecrets names the secret and the response it appeared in. The secret
// itself is in the message because these are fixture values by construction —
// a real one would mean the fixture had become production data, which is a
// different failure and a louder one.
func assertNoSecrets(t *testing.T, where, body string, secrets []string) {
	t.Helper()

	for _, secret := range secrets {
		assert.NotContains(t, body, secret, "%s discloses %q", where, secret)
	}
}

func orStatus(value, fallback int) int {
	if value == 0 {
		return fallback
	}

	return value
}

func orString(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}
