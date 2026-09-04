package architecture

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T301. scenarios_test.go asserts every scenario is claimed at least once and
// that a claim names a real test; this asserts the sharper property SC-004
// needs — a scenario is claimed EXACTLY once, so two tests cannot both claim
// it while a third scenario has none. The failure mode this catches that the
// other test cannot: two entries (one in scenarioTests, one in
// missingScenarios, or two people editing the same map on two branches)
// naming the same id.

func TestScenarioIDsAreUniqueAndClaimedExactlyOnce(t *testing.T) {
	t.Parallel()

	scenarios := acceptanceScenarios(t)
	require.NotEmpty(t, scenarios)

	seen := map[string]int{}
	for _, id := range scenarios {
		seen[id]++
	}

	var duplicated []string
	for id, count := range seen {
		if count > 1 {
			duplicated = append(duplicated, id)
		}
	}
	assert.Emptyf(t, duplicated, "spec.md's own scenario numbering produced a duplicate id — the parser is wrong: %v", duplicated)

	claims := map[string]int{}
	for id := range scenarioTests {
		claims[id]++
	}
	for id := range missingScenarios {
		claims[id]++
	}

	var unclaimed, overclaimed []string
	for _, id := range scenarios {
		switch claims[id] {
		case 0:
			unclaimed = append(unclaimed, id)
		case 1:
			// exactly right
		default:
			overclaimed = append(overclaimed, id)
		}
	}

	assert.Emptyf(t, unclaimed, "these scenarios are claimed by nothing: %v", unclaimed)
	assert.Emptyf(t, overclaimed, "these scenarios are claimed more than once: %v", overclaimed)

	// And the reverse: an entry naming an id spec.md does not have is a stale
	// claim rather than a proof of anything.
	valid := make(map[string]bool, len(scenarios))
	for _, id := range scenarios {
		valid[id] = true
	}

	for id := range scenarioTests {
		assert.Truef(t, valid[id], "scenarioTests claims %s, which is not a scenario id spec.md currently has", id)
	}

	for id := range missingScenarios {
		assert.Truef(t, valid[id], "missingScenarios claims %s, which is not a scenario id spec.md currently has", id)
	}
}
