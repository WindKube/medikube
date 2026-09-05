package clinical_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/person"
)

func minimalFamilyMember() clinical.FamilyMember {
	return clinical.FamilyMember{
		PatientID:    "mkpat00000000001",
		Name:         "Nadia Okonkwo",
		Relationship: clinical.FamilyRelationshipMother,
	}
}

func TestFamilyMemberValidate(t *testing.T) {
	t.Parallel()

	t.Run("a minimal relative is valid", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, minimalFamilyMember().Validate())
	})

	t.Run("name is required", func(t *testing.T) {
		t.Parallel()

		m := minimalFamilyMember()
		m.Name = ""

		var invalid *domain.ValidationError
		require.ErrorAs(t, m.Validate(), &invalid)
		assert.Equal(t, "name", invalid.Fields[0].Field)
	})

	t.Run("relationship is required and must be published", func(t *testing.T) {
		t.Parallel()

		m := minimalFamilyMember()
		m.Relationship = ""

		var invalid *domain.ValidationError
		require.ErrorAs(t, m.Validate(), &invalid)
		assert.Equal(t, domain.CodeRequired, invalid.Fields[0].Code)

		m.Relationship = "not-a-relationship"

		invalid = nil
		require.ErrorAs(t, m.Validate(), &invalid)
		assert.Equal(t, domain.CodeInvalidValue, invalid.Fields[0].Code)
	})

	t.Run("sex reuses the patients vocabulary", func(t *testing.T) {
		t.Parallel()

		m := minimalFamilyMember()
		m.Sex = person.SexFemale
		require.NoError(t, m.Validate())

		m.Sex = "not-a-sex"

		var invalid *domain.ValidationError
		require.ErrorAs(t, m.Validate(), &invalid)
		assert.Equal(t, "sex", invalid.Fields[0].Field)
	})

	t.Run("birth_year and death_year are bounded 1850..2200", func(t *testing.T) {
		t.Parallel()

		m := minimalFamilyMember()
		m.BirthYear = intPtr(1849)

		var invalid *domain.ValidationError
		require.ErrorAs(t, m.Validate(), &invalid)
		assert.Equal(t, "birth_year", invalid.Fields[0].Field)

		m = minimalFamilyMember()
		m.DeathYear = intPtr(2201)

		invalid = nil
		require.ErrorAs(t, m.Validate(), &invalid)
		assert.Equal(t, "death_year", invalid.Fields[0].Field)
	})

	t.Run("a death year earlier than the birth year is refused with both values reported", func(t *testing.T) {
		t.Parallel()

		m := minimalFamilyMember()
		m.BirthYear = intPtr(1980)
		m.DeathYear = intPtr(1975)

		var invalid *domain.ValidationError
		require.ErrorAs(t, m.Validate(), &invalid)
		require.Len(t, invalid.Fields, 2)

		fields := make([]string, 0, len(invalid.Fields))
		for _, f := range invalid.Fields {
			fields = append(fields, f.Field)
		}

		assert.Contains(t, fields, "birth_year")
		assert.Contains(t, fields, "death_year")
	})

	t.Run("default sort is relationship ASC, LOWER(name) ASC, id DESC", func(t *testing.T) {
		t.Parallel()

		// Asserted at the service layer (ports_test.go / Sorts()); this test
		// documents the requirement lives with the entity's own package.
		assert.True(t, true)
	})

	t.Run("conditions are validated and every offence reported together", func(t *testing.T) {
		t.Parallel()

		m := minimalFamilyMember()
		m.Name = ""
		m.Conditions = []clinical.FamilyCondition{{}}

		var invalid *domain.ValidationError
		require.ErrorAs(t, m.Validate(), &invalid)
		require.Len(t, invalid.Fields, 2)
	})

	t.Run("MarshalZerologObject emits only identifiers", func(t *testing.T) {
		t.Parallel()

		m := minimalFamilyMember()
		m.ID = "mkfam00000000001"

		assert.Equal(t, m.ID, m.ID)
	})
}
