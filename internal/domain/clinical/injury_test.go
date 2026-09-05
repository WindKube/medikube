package clinical

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

func validInjury() Injury {
	return Injury{PatientID: "patient-1", Name: "Broken wrist", BodyPart: "wrist"}
}

func TestInjuryValidateRequiresNameAndBodyPart(t *testing.T) {
	t.Parallel()

	var invalid *domain.ValidationError
	require.ErrorAs(t, Injury{PatientID: "p"}.Validate(), &invalid)

	names := fieldNames(invalid)
	assert.Contains(t, names, "name")
	assert.Contains(t, names, "body_part")

	assert.NoError(t, validInjury().Validate())
}

// TestInjuryTypeAcceptsOnlyThePublishedVocabulary is FR-040/US4-3: the
// vocabulary is fixed and there is no code path anywhere that adds a value to
// it (InjuryTypes() is a closed, hand-written list — this test is what would
// fail if one ever tried).
func TestInjuryTypeAcceptsOnlyThePublishedVocabulary(t *testing.T) {
	t.Parallel()

	for _, value := range InjuryTypes() {
		assert.True(t, value.Valid(), "%q is published and must be accepted", value)
	}

	assert.True(t, InjuryTypeOther.Valid(), "the catch-all must be accepted")
	assert.False(t, InjuryType("dinosaur-bite").Valid(), "an unpublished type must be refused")

	entity := validInjury()
	entity.Type = "dinosaur-bite"

	var invalid *domain.ValidationError
	require.ErrorAs(t, entity.Validate(), &invalid)
	assert.Contains(t, fieldNames(invalid), "type")
}

// TestLateralityIncludesNotApplicable is FR-041.
func TestLateralityIncludesNotApplicable(t *testing.T) {
	t.Parallel()

	assert.True(t, LateralityNotApplicable.Valid())

	entity := validInjury()
	entity.Laterality = LateralityNotApplicable

	assert.NoError(t, entity.Validate())
}

func TestInjuryValidateRefusesAFutureOccurredOn(t *testing.T) {
	t.Parallel()

	future, err := domain.NewDate(2099, 1, 1)
	require.NoError(t, err)

	entity := validInjury()
	entity.OccurredOn = future

	var invalid *domain.ValidationError
	require.ErrorAs(t, entity.Validate(), &invalid)
	assert.Contains(t, fieldNames(invalid), "occurred_on")
}
