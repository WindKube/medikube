package directory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// data-model §1. Pinned as literals rather than read from the implementation.
var publishedFacilityKinds = []FacilityKind{
	"practice", "pharmacy", "hospital", "lab", "imaging", "other",
}

// data-model §2, verbatim (research D-23).
var publishedSpecialties = []Specialty{
	"allergy_immunology", "anesthesiology", "cardiology", "dentistry", "dermatology",
	"emergency_medicine", "endocrinology", "family_medicine", "gastroenterology",
	"general_surgery", "genetics", "geriatrics", "gynecology", "hematology",
	"hepatology", "infectious_disease", "internal_medicine", "nephrology",
	"neurology", "neurosurgery", "nutrition", "obstetrics", "occupational_therapy",
	"oncology", "ophthalmology", "optometry", "oral_surgery", "orthopedics",
	"otolaryngology", "pain_medicine", "palliative_care", "pathology", "pediatrics",
	"physical_therapy", "plastic_surgery", "podiatry", "psychiatry", "psychology",
	"pulmonology", "radiology", "rheumatology", "urology", "other",
}

func TestFacilityKindAcceptsExactlyThePublishedValues(t *testing.T) {
	t.Parallel()

	for _, value := range publishedFacilityKinds {
		t.Run("accepts "+string(value), func(t *testing.T) {
			t.Parallel()
			assert.True(t, value.Valid(), "%q is published in data-model §1 and must be accepted", value)
		})
	}

	rejected := []FacilityKind{"", "PRACTICE", "clinic", "unknown"}
	for _, value := range rejected {
		t.Run("refuses "+string(value), func(t *testing.T) {
			t.Parallel()
			assert.False(t, value.Valid(), "%q is not a published value and must be refused", value)
		})
	}
}

// FR-033: the catch-all must exist and must never be dropped by an ordinary
// edit to the vocabulary.
func TestFacilityKindOtherIsThePublishedCatchAll(t *testing.T) {
	t.Parallel()
	assert.Contains(t, FacilityKinds(), FacilityKindOther)
}

func TestSpecialtyAcceptsExactlyThePublishedValues(t *testing.T) {
	t.Parallel()

	for _, value := range publishedSpecialties {
		t.Run("accepts "+string(value), func(t *testing.T) {
			t.Parallel()
			assert.True(t, value.Valid(), "%q is published in data-model §2 and must be accepted", value)
		})
	}

	rejected := []Specialty{"UROLOGY", "podiatrist", "unknown_specialty"}
	for _, value := range rejected {
		t.Run("refuses "+string(value), func(t *testing.T) {
			t.Parallel()
			assert.False(t, value.Valid(), "%q is not a published value and must be refused", value)
		})
	}
}

// FR-033: "MUST NOT be extended by ordinary use" — the catch-all is a fixed
// member of the vocabulary, not a fallback synthesised elsewhere.
func TestSpecialtyOtherIsThePublishedCatchAll(t *testing.T) {
	t.Parallel()
	assert.Contains(t, Specialties(), SpecialtyOther)
}

// research D-23: the Go slice and the generated select vocabulary are built
// from the same source, so they cannot disagree. This asserts the slice this
// package publishes is exactly the one pinned above from data-model §2.
func TestSpecialtiesMatchesThePublishedVocabularyExactly(t *testing.T) {
	t.Parallel()
	require.ElementsMatch(t, publishedSpecialties, Specialties())
}

func TestFacilityKindsMatchesThePublishedVocabularyExactly(t *testing.T) {
	t.Parallel()
	require.ElementsMatch(t, publishedFacilityKinds, FacilityKinds())
}
