package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/testsupport/seed"
)

// T044. The owner/stranger/anonymous triangle contracts/records.md asks every
// kind to answer, at the smallest size that still proves the wiring: read one
// as its owner, as a signed-in stranger, and as nobody.
func allergyURL(id string) string { return "/api/v1/records/" + kind.Allergy.Segment() + "/" + id }

func TestAllergyReadIsOwnerScopedAndAnonymousRefused(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	stranger := owner.as(testsupport.AccountBEmail)
	guest := owner.anonymous()

	target := allergyURL(seed.CriticalAllergyID)

	mine := owner.get(target)
	require.Equal(t, http.StatusOK, mine.Status, mine.Body)
	assert.Contains(t, mine.Body, seed.CriticalAllergyID)

	strangerAnswer := stranger.get(target)
	missing := stranger.get(allergyURL(missingID))
	require.Equal(t, http.StatusNotFound, strangerAnswer.Status, strangerAnswer.Body)
	assert.Equal(t, withoutCorrelationID(missing.Body), withoutCorrelationID(strangerAnswer.Body),
		"a stranger's refusal must be byte-identical to a genuine miss (FR-033)")

	anon := guest.get(target)
	require.Equal(t, http.StatusUnauthorized, anon.Status, anon.Body)
	assert.NotContains(t, anon.Body, testsupport.AccountAPatientSelfID,
		"an unauthenticated refusal named the patient it was asked about")
}
