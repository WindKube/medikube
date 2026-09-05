package person

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// data-model §3. Pinned as literals rather than read from the implementation:
// asking the code what it accepts and then asserting it accepts that proves
// nothing. These spellings are also the stored column values.
var (
	publishedSexes = []Sex{"female", "male", "intersex", "unspecified"}

	publishedBloodTypes = []BloodType{
		"a_pos", "a_neg", "b_pos", "b_neg", "ab_pos", "ab_neg", "o_pos", "o_neg",
	}

	publishedRelationshipsToOwner = []RelationshipToOwner{
		"self", "spouse", "partner", "parent", "child", "sibling", "ward", "other",
	}
)

func TestSexAcceptsExactlyThePublishedValues(t *testing.T) {
	t.Parallel()

	for _, value := range publishedSexes {
		t.Run("accepts "+string(value), func(t *testing.T) {
			t.Parallel()
			assert.True(t, value.Valid(), "%q is published in data-model §3 and must be accepted", value)
		})
	}

	rejected := []Sex{"", "FEMALE", "Male", "other", "unknown", "u", "f", "m"}
	for _, value := range rejected {
		t.Run("refuses "+string(value), func(t *testing.T) {
			t.Parallel()
			assert.False(t, value.Valid(), "%q is not a published value and must be refused", value)
		})
	}
}

func TestBloodTypeAcceptsExactlyThePublishedValues(t *testing.T) {
	t.Parallel()

	for _, value := range publishedBloodTypes {
		t.Run("accepts "+string(value), func(t *testing.T) {
			t.Parallel()
			assert.True(t, value.Valid(), "%q is published in data-model §3 and must be accepted", value)
		})
	}

	rejected := []BloodType{"", "AB_POS", "ab", "a+", "O_POS", "unknown"}
	for _, value := range rejected {
		t.Run("refuses "+string(value), func(t *testing.T) {
			t.Parallel()
			assert.False(t, value.Valid(), "%q is not a published value and must be refused", value)
		})
	}
}

func TestRelationshipToOwnerAcceptsExactlyThePublishedValues(t *testing.T) {
	t.Parallel()

	for _, value := range publishedRelationshipsToOwner {
		t.Run("accepts "+string(value), func(t *testing.T) {
			t.Parallel()
			assert.True(t, value.Valid(), "%q is published in data-model §3 and must be accepted", value)
		})
	}

	rejected := []RelationshipToOwner{"", "SELF", "Cousin", "grandparent", "unknown"}
	for _, value := range rejected {
		t.Run("refuses "+string(value), func(t *testing.T) {
			t.Parallel()
			assert.False(t, value.Valid(), "%q is not a published value and must be refused", value)
		})
	}
}
