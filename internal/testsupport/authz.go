package testsupport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
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

	// Headers are sent on every leg. A write whose precondition is missing is
	// refused with 422 for everybody, which satisfies "the stranger did not
	// succeed" while asserting nothing about ownership — so a case for PATCH
	// or DELETE puts the owner's own If-Match here and makes the refusal be
	// about who is asking.
	Headers map[string]string

	// The three expected outcomes. Zero means the default: 200, 404, 401.
	OwnerStatus    int
	StrangerStatus int
	GuestStatus    int

	// MissingPath addresses an identifier that has never existed. When it is
	// set, the matrix asserts the stranger's whole response is byte-identical
	// to the owner's response for this path — which is what "do not confirm the
	// identifier exists" actually means, as opposed to merely returning the
	// same status code. The HEADER SET is compared too: this repository has
	// already shipped two responses that were distinguishable by headers
	// alone, which no body comparison would have caught.
	//
	// A body that carries a correlation id or a timestamp needs
	// OwnershipMatrix.Normalise; without one the comparison fails on the
	// per-request member rather than on a disclosure.
	MissingPath string

	// StrangerIsolated marks an operation that addresses no record of anybody's
	// — a list, a create. A stranger is not refused one: they get their own
	// answer, and what "refused" means for them is that nothing of the owner's
	// is in it. The Secrets check is then the whole of the assertion, which is
	// why a case that sets this and names nothing is refused twice over.
	StrangerIsolated bool

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

	// Normalise removes the parts of a response that differ between two
	// requests by construction — the correlation id above all, which
	// contracts/records.md and FR-033 name as the ONE permitted difference
	// between a refusal and a genuine miss.
	//
	// It is a function rather than a list of member names because the shape is
	// the caller's: an envelope, a page, an HTML error view. Nil is the
	// identity, which is what a body with nothing volatile in it wants.
	//
	// It applies to bodies AND to header values, so a header carrying the same
	// id is normalised the same way rather than needing a second hook.
	Normalise func(string) string

	// VolatileHeaders are dropped before the header sets are compared. A
	// header whose value cannot match between two requests goes here by name;
	// one whose value merely contains a correlation id is handled by
	// Normalise.
	VolatileHeaders []string

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

	t.Run("a stranger is refused", func(t *testing.T) {
		status, body, headers := send(t, matrix.Handler, one, one.Path, matrix.Stranger)

		assert.NotEqual(t, http.StatusForbidden, status,
			"403 tells a stranger the identifier exists, which turns guessing into enumeration; the answer is 404")

		if one.StrangerIsolated {
			assert.Less(t, status, http.StatusBadRequest,
				"the operation addresses no record, so a stranger is answered rather than refused")
			assert.Equal(t, orStatus(one.StrangerStatus, defaultOwnerStatus), status)
		} else {
			assert.GreaterOrEqual(t, status, http.StatusBadRequest,
				"a stranger reached someone else's record")
			assert.Equal(t, orStatus(one.StrangerStatus, defaultStrangerStatus), status)
		}

		assertNoSecrets(t, "the stranger's response", body, one.Secrets)

		if one.MissingPath != "" {
			missingStatus, missingBody, missingHeaders := send(t, matrix.Handler, one, one.MissingPath, matrix.Owner)

			assert.Equal(t, missingStatus, status,
				"a record that exists and one that never did answer differently")
			assert.Equal(t, normalise(matrix, missingBody), normalise(matrix, body),
				"the refusal is distinguishable from a genuine miss, so the identifier is confirmed by the body")
			assert.Equal(t,
				comparableHeaders(matrix, missingHeaders), comparableHeaders(matrix, headers),
				"the refusal is distinguishable from a genuine miss by its headers alone")
		}
	})

	t.Run("a guest is refused", func(t *testing.T) {
		status, body, _ := send(t, matrix.Handler, one, one.Path, nil)

		assert.Equal(t, orStatus(one.GuestStatus, defaultGuestStatus), status)
		assertNoSecrets(t, "the guest's response", body, one.Secrets)
	})

	// LAST, and that ordering is load-bearing rather than tidy. The owner leg
	// of a delete destroys the subject, so run first it would leave the
	// stranger refused because the record was gone — a green that proves
	// nothing and cannot be told from a green that proves everything. Run
	// last, it is the control: the record was still reachable by its owner
	// after two refusals, so the refusals were about who was asking.
	t.Run("the owner succeeds", func(t *testing.T) {
		status, _, _ := send(t, matrix.Handler, one, one.Path, matrix.Owner)

		assert.Equal(t, orStatus(one.OwnerStatus, defaultOwnerStatus), status,
			"the owner cannot reach their own record, so the refusals above were not about ownership")
	})
}

// send drives one request through the handler in memory. No socket is opened,
// which is what makes a table of these cheap enough that every endpoint gets
// one.
func send(t *testing.T, handler http.Handler, one OwnershipCase, path string, who Identity) (int, string, http.Header) {
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

	for name, value := range one.Headers {
		request.Header.Set(name, value)
	}

	if who != nil {
		who(request)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder.Code, recorder.Body.String(), recorder.Header().Clone()
}

// normalise applies the matrix's own normalisation, or none.
func normalise(matrix OwnershipMatrix, value string) string {
	if matrix.Normalise == nil {
		return value
	}

	return matrix.Normalise(value)
}

// comparableHeaders is one response's headers with the volatile ones dropped
// and the rest normalised, so the comparison is of what the two responses SAY
// rather than of when they were sent.
func comparableHeaders(matrix OwnershipMatrix, headers http.Header) map[string][]string {
	comparable := make(map[string][]string, len(headers))

	for name, values := range headers {
		if slices.ContainsFunc(matrix.VolatileHeaders, func(volatile string) bool {
			return http.CanonicalHeaderKey(volatile) == name
		}) {
			continue
		}

		normalised := make([]string, 0, len(values))
		for _, value := range values {
			normalised = append(normalised, normalise(matrix, value))
		}

		comparable[name] = normalised
	}

	return comparable
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
