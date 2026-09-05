package clinical_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
)

func intPtr(v int) *int { return &v }

func TestValidateFamilyConditions(t *testing.T) {
	t.Parallel()

	t.Run("a minimal condition is valid", func(t *testing.T) {
		t.Parallel()

		err := clinical.ValidateFamilyConditions([]clinical.FamilyCondition{{Name: "Breast cancer"}})
		require.NoError(t, err)
	})

	t.Run("a name is required", func(t *testing.T) {
		t.Parallel()

		err := clinical.ValidateFamilyConditions([]clinical.FamilyCondition{{}})

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, domain.CodeRequired, invalid.Fields[0].Code)
	})

	t.Run("diagnosed_age is bounded 0..130", func(t *testing.T) {
		t.Parallel()

		err := clinical.ValidateFamilyConditions([]clinical.FamilyCondition{
			{Name: "Heart arrhythmia", DiagnosedAge: intPtr(131)},
		})

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, domain.CodeOutOfRange, invalid.Fields[0].Code)
	})

	t.Run("severity and status must be from the shared ladders", func(t *testing.T) {
		t.Parallel()

		err := clinical.ValidateFamilyConditions([]clinical.FamilyCondition{
			{Name: "Diabetes", Severity: "not-a-severity", Status: "not-a-status"},
		})

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Len(t, invalid.Fields, 2)
	})

	t.Run("notes is bounded at 2000 characters", func(t *testing.T) {
		t.Parallel()

		err := clinical.ValidateFamilyConditions([]clinical.FamilyCondition{
			{Name: "Diabetes", Notes: strings.Repeat("a", 2001)},
		})

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, domain.CodeTooLong, invalid.Fields[0].Code)
	})

	t.Run("the list is bounded at 50 entries", func(t *testing.T) {
		t.Parallel()

		conditions := make([]clinical.FamilyCondition, 51)
		for i := range conditions {
			conditions[i] = clinical.FamilyCondition{Name: "Condition"}
		}

		err := clinical.ValidateFamilyConditions(conditions)

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, domain.CodeTooLong, invalid.Fields[0].Code)
	})

	t.Run("every offending entry is reported together", func(t *testing.T) {
		t.Parallel()

		err := clinical.ValidateFamilyConditions([]clinical.FamilyCondition{
			{},
			{Name: "Valid"},
			{Name: "Another", DiagnosedAge: intPtr(-1)},
		})

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Len(t, invalid.Fields, 2)
	})
}
