package api_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/obs"
	"medikube/internal/testsupport"
)

// T202, FR-005, research D-17. The account-existence oracle, closed and
// asserted BY ITS MECHANISM.
//
// Two byte-identical 401 bodies are not enough. The naive shape — look the
// address up, miss, return — answers an unknown address in about 200µs and a
// wrong password in about 70ms, because only the second pays for a bcrypt
// comparison. Measured on this codebase against PocketBase v0.40.1 that is a
// 339× difference behind two identical bodies, and it is measurable over the
// public internet.
//
// What closes it is that the unknown-address path performs a comparison too,
// against a FIXED dummy hash whose work factor equals the collection's. What
// this file asserts is that comparison — through a counting seam on the
// comparer, never through a clock. Constitution VIII forbids a flaky gate
// assertion, and a latency has no threshold anybody can defend; a count is
// deterministic. The latency itself is reported by timing_bench_test.go, which
// `go test ./...` does not run.

// signInRefusals is every way MediKube's own sign-in route can refuse.
func signInRefusals() []struct {
	name     string
	email    string
	password string
	dummy    int
} {
	return []struct {
		name     string
		email    string
		password string
		dummy    int
	}{
		{
			name:     "an address with no account",
			email:    "nobody@example.test",
			password: testsupport.Password,
			// The comparison an address with no account still pays for.
			dummy: 1,
		},
		{
			name:     "an account with another password",
			email:    testsupport.AccountAEmail,
			password: "not-the-password",
			dummy:    0,
		},
		{
			name:     "an address that is not an address at all",
			email:    "][",
			password: testsupport.Password,
			dummy:    1,
		},
		{
			name:     "an empty address",
			email:    "",
			password: testsupport.Password,
			dummy:    1,
		},
	}
}

// THE mechanism assertion. Every refusal costs exactly ONE comparison, and the
// one an unknown address pays for is the dummy.
func TestEverySignInRefusalCostsExactlyOneComparisonOnMediKubesOwnRoute(t *testing.T) {
	t.Parallel()

	for _, refusal := range signInRefusals() {
		t.Run(refusal.name, func(t *testing.T) {
			t.Parallel()

			instance := newRig(t)
			seam := instance.instance.Accounts.Authenticator

			// The fixture's own reads do not count: only what the request under
			// test did.
			seam.Forget()

			answer := instance.anonymous().post(loginURL, body(
				"email", quoted(refusal.email),
				"password", quoted(refusal.password),
			))

			require.Equal(t, http.StatusUnauthorized, answer.Status, answer.Body)

			assert.Equal(t, 1, seam.Comparisons(),
				"a refusal cost %d bcrypt comparisons; the two refusal paths must cost the same",
				seam.Comparisons())
			assert.Equal(t, refusal.dummy, seam.DummyComparisons(),
				"the dummy comparison did not happen exactly %d time(s), so the two paths do not cost the same",
				refusal.dummy)
		})
	}
}

// The counting seam is on MEDIKUBE'S path and not on PocketBase's.
//
// This is the assertion that fails if `login` is ever rebuilt to delegate the
// LOOKUP as well: PocketBase's own dummyPasswordCheck is unexported, samples an
// arbitrary row and returns without comparing anything at all when the table is
// empty, so a test pointed at the native route would keep passing while
// MediKube's handler had no defence left.
func TestMediKubesOwnRouteIsWhatPaysForTheComparison(t *testing.T) {
	t.Parallel()

	instance := newRig(t)
	seam := instance.instance.Accounts.Authenticator

	seam.Forget()

	require.Equal(t, http.StatusOK, instance.anonymous().post(loginURL, body(
		"email", quoted(testsupport.AccountAEmail),
		"password", quoted(testsupport.Password),
	)).Status)

	assert.Equal(t, 1, seam.Comparisons(),
		"a successful sign-in through MediKube's route did not reach MediKube's comparer at all")
	assert.Zero(t, seam.DummyComparisons())
}

// The dummy hash's work factor must equal the collection's, or the two
// comparisons stop costing the same and the equalisation quietly becomes
// decorative. That is asserted where the hash lives, against the collection's
// own password field — internal/store/identity's
// TestTheDummyHashCostsWhatEveryRealHashCosts — because the constant and the
// column are both there and neither is visible from here. It is named rather
// than repeated so that nobody deletes it believing this file covers it.

// The two refusals are indistinguishable in every part of the response a caller
// can read — the status, the headers and the body — and not only in the body.
// A 404 once differed from a genuine one by four missing headers in this
// repository; a 401 can differ the same way.
func TestTheTwoSignInRefusalsAreIndistinguishableInEveryPartOfTheResponse(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	unknown := instance.anonymous().post(loginURL, body(
		"email", quoted("nobody@example.test"), "password", quoted(testsupport.Password)))

	wrong := instance.anonymous().post(loginURL, body(
		"email", quoted(testsupport.AccountAEmail), "password", quoted("not-the-password")))

	require.Equal(t, http.StatusUnauthorized, unknown.Status)
	assert.Equal(t, unknown.Status, wrong.Status)
	assert.Equal(t, withoutCorrelationID(unknown.Body), withoutCorrelationID(wrong.Body))
	assert.Equal(t, len(unknown.Body), len(wrong.Body),
		"the two bodies are different lengths, which is visible without reading either")

	assert.Equal(t, headerNames(unknown), headerNames(wrong),
		"the two refusals carry different headers")

	for _, name := range headerNames(unknown) {
		// The correlation id is the ONE thing FR-033 lets two otherwise
		// identical refusals differ in — it is per request, not per outcome,
		// and it is what lets a person quote a reference to an operator.
		if name == obs.CorrelationHeader {
			continue
		}

		assert.Equalf(t, unknown.Header.Values(name), wrong.Header.Values(name),
			"%s differs between the two refusals", name)
	}

	assert.Contains(t, headerNames(unknown), obs.CorrelationHeader,
		"neither refusal carries a correlation id, so the exclusion above hides nothing")
}

func headerNames(r response) []string {
	names := make([]string, 0, len(r.Header))
	for name := range r.Header {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}
