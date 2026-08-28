package testsupport

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T091. Five later phases run every one of their record endpoints through
// RunOwnershipMatrix, so the matrix passing is the only evidence any of them
// has that a stranger is refused. A matrix that cannot fail is worse than no
// matrix at all: it is a green tick over an open door.
//
// So this file drives the matrix against handlers that are each broken in one
// specific way and requires it to go red for each of them — in a child process,
// running the real exported function, because a failure recorded on this
// process's own *testing.T is a failure of this test.

const (
	// The child is this same test binary, re-entered with the flavour to break.
	selfTestEnv  = "MEDIKUBE_AUTHZ_SELFTEST"
	selfTestName = "TestAHandlerThatLeaksMakesTheMatrixFail"

	ownerToken    = "owner-token"
	strangerToken = "stranger-token"

	ownedID   = "mkmedamara00001"
	missingID = "doesnotexist001"
	ownerName = "Lisinopril"
)

// flaw is one way a real handler goes wrong, and the assertion that has to
// notice. Each is a bug somebody has actually shipped.
type flaw struct {
	name string
	// wants is a fragment of the message the matrix must print, so the test
	// asserts the *right* leg failed and not merely that something did.
	wants string
}

var flaws = []flaw{
	{
		// The classic: authorization checked at the route and not against the
		// record, so any signed-in caller reads anyone's records.
		name:  "any signed-in caller is served",
		wants: "a stranger reached someone else's record",
	},
	{
		// Refused, but the refusal confirms the identifier exists.
		name:  "the stranger is told 403",
		wants: "403 tells a stranger the identifier exists",
	},
	{
		// 404, and the body says which record was not found.
		name:  "the 404 body echoes the identifier",
		wants: "discloses",
	},
	{
		// Two different 404s: one for "not yours" and one for "no such thing".
		// Indistinguishable status codes, distinguishable bodies.
		name:  "the refusal reads differently from a genuine miss",
		wants: "the refusal is distinguishable from a genuine miss",
	},
	{
		// No credential required at all.
		name:  "a guest is served",
		wants: "Not equal",
	},
	{
		// Not a broken handler but a broken case: one that names nothing that
		// must not leak runs its refusal legs against a status code only, and
		// "the stranger got a 404" is compatible with a 404 whose body is the
		// record. The matrix refuses the case rather than reporting a pass.
		name:  "the case names no secret",
		wants: "names nothing that must not leak",
	},
}

func TestAHandlerThatLeaksMakesTheMatrixFail(t *testing.T) {
	if flavour := os.Getenv(selfTestEnv); flavour != "" {
		// The child. This is the real exported function against a broken
		// handler, and it is *supposed* to fail — that failure is the parent's
		// evidence.
		RunOwnershipMatrix(t, brokenMatrix(flavour))

		return
	}

	for _, broken := range flaws {
		t.Run(broken.name, func(t *testing.T) {
			t.Parallel()

			output, err := runSelfTest(t, broken.name)

			require.Errorf(t, err,
				"the matrix PASSED against a handler where %s — it cannot catch this and five phases are relying on it to:\n%s",
				broken.name, output)
			assert.Containsf(t, output, broken.wants,
				"the matrix failed, but not on the assertion that should have caught %q:\n%s",
				broken.name, output)
		})
	}
}

// TestAnHonestHandlerPassesTheMatrix is the control. Without it the file above
// would be satisfied by a matrix that failed unconditionally, which proves
// nothing either.
func TestAnHonestHandlerPassesTheMatrix(t *testing.T) {
	t.Parallel()

	RunOwnershipMatrix(t, matrix(honestHandler()))
}

// brokenMatrix is the honest matrix with exactly one thing wrong with it.
func brokenMatrix(flaw string) OwnershipMatrix {
	broken := matrix(handlerFor(flaw))

	if flaw == "the case names no secret" {
		broken.Cases[0].Secrets = nil
	}

	return broken
}

func runSelfTest(t *testing.T, flavour string) (string, error) {
	t.Helper()

	// os.Args[0] is this test binary. Re-entering it is the only way to run the
	// exported RunOwnershipMatrix against a real *testing.T and still survive
	// its failure.
	child := exec.Command(os.Args[0], "-test.run=^"+selfTestName+"$", "-test.v") //nolint:gosec // the binary is this process
	child.Env = append(os.Environ(), selfTestEnv+"="+flavour)

	output, err := child.CombinedOutput()

	return string(output), err
}

func matrix(handler http.Handler) OwnershipMatrix {
	return OwnershipMatrix{
		Handler:  handler,
		Owner:    BearerToken(ownerToken),
		Stranger: BearerToken(strangerToken),
		Cases: []OwnershipCase{
			{
				Name:        "read one",
				Method:      http.MethodGet,
				Path:        "/records/" + ownedID,
				MissingPath: "/records/" + missingID,
				Secrets:     []string{ownedID, ownerName},
			},
		},
	}
}

// honestHandler is what a correct MediKube endpoint does: no credential is a
// 401, a record somebody else owns is indistinguishable from a record that
// never existed, and neither answer carries anything of the owner's.
func honestHandler() http.Handler {
	return handlerFor("")
}

func handlerFor(flaw string) http.Handler {
	const (
		miss   = `{"status":404,"message":"The requested resource wasn't found."}`
		refuse = `{"status":401,"message":"The request requires valid record authorization token."}`
	)

	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")

		token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		id := strings.TrimPrefix(request.URL.Path, "/records/")

		switch {
		case token == "" && flaw != "a guest is served":
			write(response, http.StatusUnauthorized, refuse)

		case id != ownedID:
			write(response, http.StatusNotFound, miss)

		case token == ownerToken || flaw == "any signed-in caller is served" || flaw == "a guest is served":
			write(response, http.StatusOK, fmt.Sprintf(`{"id":%q,"name":%q}`, ownedID, ownerName))

		case flaw == "the stranger is told 403":
			write(response, http.StatusForbidden, `{"status":403,"message":"You are not allowed to perform this request."}`)

		case flaw == "the 404 body echoes the identifier":
			write(response, http.StatusNotFound, fmt.Sprintf(`{"status":404,"message":"no record %s"}`, ownedID))

		case flaw == "the refusal reads differently from a genuine miss":
			write(response, http.StatusNotFound, `{"status":404,"message":"That record is not yours."}`)

		default:
			write(response, http.StatusNotFound, miss)
		}
	})
}

func write(response http.ResponseWriter, status int, body string) {
	response.WriteHeader(status)
	_, _ = response.Write([]byte(body))
}
