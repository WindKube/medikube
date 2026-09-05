package api_test

import (
	json "encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/person"
	"medikube/internal/web"
	"medikube/internal/web/api"
)

// T056. contracts/patients.md's four DTOs under Go 1.27's encoding/json/v2:
// unknown fields are rejected, duplicate keys are rejected, dates round-trip
// as YYYY-MM-DD, and a member never filled in marshals as null rather than
// being silently dropped (FR-024) — the shape internal/web/json_semantics_test.go
// pins generically, exercised here against the real production types.

func TestPatientCreateRejectsAnUnknownMember(t *testing.T) {
	t.Parallel()

	var create api.PatientCreate
	err := web.DecodeBytes([]byte(`{"first_name":"Amara","last_name":"Okonkwo","birth_date":"1988-04-12","owner":"mkuser0000001"}`), &create)

	require.Error(t, err, "owner is not a member of PatientCreate; FR-002 forbids re-attribution by shape")

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Contains(t, invalid.Fields, domain.FieldError{Field: "owner", Code: domain.CodeUnknownField,
		Message: "the field is not one this operation accepts"})
}

func TestPatientPatchRejectsAnUnknownMember(t *testing.T) {
	t.Parallel()

	var patch api.PatientPatch
	err := web.DecodeBytes([]byte(`{"is_self_record":true}`), &patch)

	require.Error(t, err)
}

func TestPatientCreateRejectsADuplicateMember(t *testing.T) {
	t.Parallel()

	var create api.PatientCreate
	err := web.DecodeBytes([]byte(`{"first_name":"Amara","first_name":"Chiamaka","last_name":"Okonkwo","birth_date":"1988-04-12"}`), &create)

	require.Error(t, err, "the last value silently won over the first")
}

func TestPatientCreatesBirthDateRoundTripsAsCalendarText(t *testing.T) {
	t.Parallel()

	var create api.PatientCreate
	require.NoError(t, web.DecodeBytes(
		[]byte(`{"first_name":"Amara","last_name":"Okonkwo","birth_date":"1988-04-12"}`), &create))

	assert.Equal(t, "1988-04-12", create.BirthDate)

	draft, err := create.Draft()
	require.NoError(t, err)
	assert.Equal(t, "1988-04-12", draft.BirthDate.String())
}

func TestPatientCreateRefusesABirthDateThatIsNotACalendarDay(t *testing.T) {
	t.Parallel()

	create := api.PatientCreate{FirstName: "Amara", LastName: "Okonkwo", BirthDate: "not-a-date"}

	_, err := create.Draft()
	require.Error(t, err)

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
}

// TestPatientPatchDistinguishesAbsentClearedAndGiven is web.Optional's whole
// point (contracts/patients.md's `**float64` note): a member nobody sent, an
// explicit null and a value are three different instructions and PatientPatch
// carries all three without collapsing any two of them.
func TestPatientPatchDistinguishesAbsentClearedAndGiven(t *testing.T) {
	t.Parallel()

	t.Run("absent", func(t *testing.T) {
		t.Parallel()

		var patch api.PatientPatch
		require.NoError(t, web.DecodeBytes([]byte(`{}`), &patch))

		patched, err := patch.ToServicePatch()
		require.NoError(t, err)
		assert.Nil(t, patched.HeightCM, "an absent member must not be read as a clear")
	})

	t.Run("explicit null clears it", func(t *testing.T) {
		t.Parallel()

		var patch api.PatientPatch
		require.NoError(t, web.DecodeBytes([]byte(`{"height_cm":null}`), &patch))

		patched, err := patch.ToServicePatch()
		require.NoError(t, err)
		require.NotNil(t, patched.HeightCM, "a sent null must still reach the service as an instruction")
		assert.Nil(t, *patched.HeightCM, "a cleared member carries no value")
	})

	t.Run("a value is carried through", func(t *testing.T) {
		t.Parallel()

		var patch api.PatientPatch
		require.NoError(t, web.DecodeBytes([]byte(`{"height_cm":175}`), &patch))

		patched, err := patch.ToServicePatch()
		require.NoError(t, err)
		require.NotNil(t, patched.HeightCM)
		require.NotNil(t, *patched.HeightCM)
		assert.InDelta(t, 175, **patched.HeightCM, 0)
	})
}

// TestAPatientNeverFilledInMarshalsNullNotZeroOrOmitted is FR-024/US1-6: a
// member with nothing recorded renders as null, not as "0", "" or absent —
// any of which would read as a value somebody entered.
func TestAPatientNeverFilledInMarshalsNullNotZeroOrOmitted(t *testing.T) {
	t.Parallel()

	summary := api.NewPatientSummary(person.Patient{ID: "mkpatient0001", FirstName: "Chiamaka", LastName: "Okonkwo"}, nil)
	raw, err := json.Marshal(summary)
	require.NoError(t, err)

	assert.Contains(t, string(raw), `"birth_date":null`)
	assert.Contains(t, string(raw), `"age":null`)
	assert.Contains(t, string(raw), `"photo_url":null`)
}
