package identity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The other half of T202, and the half a call count cannot reach.
//
// Counting proves the dummy comparison HAPPENS. It cannot prove the comparison
// does any work: a `compare` that answered false without hashing would be
// called once, counted once, and would restore the whole 339× latency oracle
// with every count assertion still passing. Measured on this codebase: with a
// `return false` inserted on the dummy path, the entire suite stayed green.
//
// A comparison asked to SUCCEED is what separates them, which is why the plain
// value behind the fixed hash lives beside it.
func TestTheDummyComparisonIsARealComparisonAndNotAStub(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		supplied string
		matches  bool
	}{
		{name: "the value the fixed hash was made from", supplied: dummyPassword, matches: true},
		{name: "anything else", supplied: "a-perfectly-ordinary-passphrase"},
		{name: "nothing at all", supplied: ""},
		{name: "the hash itself", supplied: DummyPasswordHash},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			authenticator := &Authenticator{}

			assert.Equalf(t, testCase.matches, authenticator.compare(DummyPasswordHash, testCase.supplied),
				"the fixed dummy hash is not being bcrypt-compared: a comparison that costs nothing is the account-existence oracle research D-17 closes")
		})
	}
}

// The dummy path counts itself AND goes through the one comparison every other
// path goes through. Deleting the delegation leaves the dummy count intact and
// the total short, which is what this reads.
func TestTheDummyPathIsCountedTwiceOverAndComparesOnce(t *testing.T) {
	t.Parallel()

	authenticator := &Authenticator{}

	authenticator.compareDummy("something a caller typed")

	assert.Equal(t, 1, authenticator.DummyComparisons())
	assert.Equal(t, 1, authenticator.Comparisons(),
		"the dummy path was counted as a dummy and never reached the comparison itself")

	require.True(t, authenticator.compare(DummyPasswordHash, dummyPassword))

	assert.Equal(t, 2, authenticator.Comparisons())
	assert.Equal(t, 1, authenticator.DummyComparisons(), "an ordinary comparison was counted as a dummy")

	authenticator.Forget()

	assert.Equal(t, 0, authenticator.Comparisons())
	assert.Equal(t, 0, authenticator.DummyComparisons())
}
