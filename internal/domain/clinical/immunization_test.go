package clinical

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

func validImmunization(t *testing.T) Immunization {
	t.Helper()

	administeredOn, err := domain.NewDate(2026, time.March, 1)
	require.NoError(t, err)

	return Immunization{PatientID: "patient-1", VaccineName: "Influenza", AdministeredOn: administeredOn}
}

func TestImmunizationValidateRequiresVaccineNameAndAdministeredOn(t *testing.T) {
	t.Parallel()

	t.Run("both absent", func(t *testing.T) {
		t.Parallel()

		var invalid *domain.ValidationError
		require.ErrorAs(t, Immunization{PatientID: "p"}.Validate(), &invalid)

		fields := fieldNames(invalid)
		assert.Contains(t, fields, "vaccine_name")
		assert.Contains(t, fields, "administered_on")
	})

	t.Run("both present is accepted", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, validImmunization(t).Validate())
	})
}

func TestImmunizationValidateRefusesADoseNumberThatIsNotPositive(t *testing.T) {
	t.Parallel()

	for _, dose := range []int{0, -1, -5} {
		t.Run(strconv.Itoa(dose), func(t *testing.T) {
			t.Parallel()

			entity := validImmunization(t)
			entity.DoseNumber = &dose

			var invalid *domain.ValidationError
			require.ErrorAs(t, entity.Validate(), &invalid)
			assert.Contains(t, fieldNames(invalid), "dose_number")
		})
	}

	t.Run("a positive dose is accepted", func(t *testing.T) {
		t.Parallel()

		entity := validImmunization(t)
		dose := 2
		entity.DoseNumber = &dose

		assert.NoError(t, entity.Validate())
	})
}

func TestImmunizationValidateRefusesAnExpiryBeforeTheAdministeredDate(t *testing.T) {
	t.Parallel()

	entity := validImmunization(t)

	before, err := domain.NewDate(2026, time.February, 1)
	require.NoError(t, err)
	entity.ExpiresOn = before

	var invalid *domain.ValidationError
	require.ErrorAs(t, entity.Validate(), &invalid)
	assert.Contains(t, fieldNames(invalid), "expires_on")
}

func TestImmunizationValidateRefusesAFutureAdministeredDate(t *testing.T) {
	t.Parallel()

	entity := validImmunization(t)

	future, err := domain.NewDate(2099, time.January, 1)
	require.NoError(t, err)
	entity.AdministeredOn = future

	var invalid *domain.ValidationError
	require.ErrorAs(t, entity.Validate(), &invalid)
	assert.Contains(t, fieldNames(invalid), "administered_on")
}

func fieldNames(invalid *domain.ValidationError) []string {
	names := make([]string, 0, len(invalid.Fields))
	for _, field := range invalid.Fields {
		names = append(names, field.Field)
	}

	return names
}
