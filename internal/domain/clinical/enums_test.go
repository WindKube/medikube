package clinical

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// FR-016 and data-model §2. The published sets are pinned here as literals
// rather than read from the implementation: a test that asks the code what it
// accepts and then asserts the code accepts it proves nothing. These spellings
// are the ones in data-model §2 and they are also the stored column values, so
// a change here is a migration, not a rename.
var (
	publishedMedicationTypes = []MedicationType{
		"prescription", "otc", "supplement", "herbal",
	}

	publishedMedicationRoutes = []MedicationRoute{
		"oral", "sublingual", "topical", "transdermal", "inhalation", "nasal",
		"ophthalmic", "otic", "rectal", "vaginal", "intramuscular",
		"subcutaneous", "intravenous", "other",
	}

	publishedTherapyStatuses = []TherapyStatus{
		"active", "on_hold", "completed", "stopped", "cancelled",
	}
)

func TestMedicationTypeAcceptsExactlyThePublishedValues(t *testing.T) {
	t.Parallel()

	for _, value := range publishedMedicationTypes {
		t.Run("accepts "+string(value), func(t *testing.T) {
			t.Parallel()

			assert.True(t, value.Valid(), "%q is published in data-model §2 and must be accepted", value)
		})
	}

	// Near misses, other vocabularies and casing. FR-016 refuses these rather
	// than storing them as free text, and casing matters because the stored
	// value is compared as-is by every filter and index.
	rejected := []MedicationType{
		"", " ", "prescription ", " prescription", "Prescription", "PRESCRIPTION",
		"prescriptions", "OTC", "Otc", "over-the-counter", "over_the_counter",
		"overthecounter", "supplements", "Supplement", "herbal_supplement",
		"Herbal", "vitamin", "generic", "unknown", "other",
	}
	for _, value := range rejected {
		t.Run("refuses "+string(value), func(t *testing.T) {
			t.Parallel()

			assert.False(t, value.Valid(), "%q is not a published kind and must be refused", value)
		})
	}
}

func TestMedicationRouteAcceptsExactlyThePublishedValues(t *testing.T) {
	t.Parallel()

	for _, value := range publishedMedicationRoutes {
		t.Run("accepts "+string(value), func(t *testing.T) {
			t.Parallel()

			assert.True(t, value.Valid(), "%q is published in data-model §2 and must be accepted", value)
		})
	}

	rejected := []MedicationRoute{
		"", " ", "oral ", "Oral", "ORAL", "by_mouth", "by mouth", "orally",
		"sub_lingual", "under_the_tongue", "Topical", "trans_dermal",
		"inhaled", "inhalational", "Nasal", "eye", "ocular", "aural",
		"per_rectum", "intra_muscular", "im", "sub_cutaneous", "subcut", "sc",
		"intra_venous", "iv", "IV", "injection", "Other", "OTHER",
	}
	for _, value := range rejected {
		t.Run("refuses "+string(value), func(t *testing.T) {
			t.Parallel()

			assert.False(t, value.Valid(), "%q is not a published route and must be refused", value)
		})
	}
}

func TestTherapyStatusAcceptsExactlyThePublishedValues(t *testing.T) {
	t.Parallel()

	for _, value := range publishedTherapyStatuses {
		t.Run("accepts "+string(value), func(t *testing.T) {
			t.Parallel()

			assert.True(t, value.Valid(), "%q is published in data-model §2 and must be accepted", value)
		})
	}

	// "canceled" is the one that would otherwise slip through review: it is the
	// US spelling of a value the whole application stores as "cancelled".
	rejected := []TherapyStatus{
		"", " ", "active ", "Active", "ACTIVE", "current", "currently_taking",
		"on-hold", "onhold", "on hold", "hold", "paused", "On_Hold",
		"complete", "finished", "Completed", "discontinued", "Stopped",
		"canceled", "Cancelled", "CANCELLED", "cancelled ", "draft", "archived",
	}
	for _, value := range rejected {
		t.Run("refuses "+string(value), func(t *testing.T) {
			t.Parallel()

			assert.False(t, value.Valid(), "%q is not a published state and must be refused", value)
		})
	}
}

// The published set is what the form offers, what the filter narrows by and
// what the OpenAPI enum lists. A value added to the type but not to this list —
// or the reverse — is drift between the vocabulary and its consumers, so the
// order and the membership are both pinned.
func TestThePublishedSetsAreExactlyWhatTheAccessorsReturn(t *testing.T) {
	t.Parallel()

	t.Run("medication types", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, publishedMedicationTypes, MedicationTypes())
	})

	t.Run("medication routes", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, publishedMedicationRoutes, MedicationRoutes())
		assert.Len(t, MedicationRoutes(), 14, "data-model §2 publishes fourteen routes")
	})

	t.Run("therapy statuses", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, publishedTherapyStatuses, TherapyStatuses())
	})
}

// The vocabulary is a package-level value and the accessors hand it out. A
// caller that sorted the result for display would otherwise resort the form,
// the OpenAPI enum and every other caller with it.
func TestTheAccessorsHandOutACopy(t *testing.T) {
	t.Parallel()

	t.Run("medication types", func(t *testing.T) {
		t.Parallel()

		MedicationTypes()[0] = "tampered"
		assert.Equal(t, publishedMedicationTypes, MedicationTypes())
	})

	t.Run("medication routes", func(t *testing.T) {
		t.Parallel()

		MedicationRoutes()[0] = "tampered"
		assert.Equal(t, publishedMedicationRoutes, MedicationRoutes())
	})

	t.Run("therapy statuses", func(t *testing.T) {
		t.Parallel()

		TherapyStatuses()[0] = "tampered"
		assert.Equal(t, publishedTherapyStatuses, TherapyStatuses())
	})
}
