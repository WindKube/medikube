package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
)

// TestGetRecordCarriesReferencesForALinkedKind is T143/FR-006: the detail
// DTO's references.total and references.by_kind count every record pointing
// at this one. ResolvedConditionID is named as the condition on one seeded
// encounter and, since US6, on one seeded treatment too
// (internal/testsupport/seed/care.go), so this is a real back-relation read
// rather than a fixture with nothing to find.
func TestGetRecordCarriesReferencesForALinkedKind(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	answer := caller.get("/api/v1/records/" + kind.Condition.Collection() + "/" + testsupport.ResolvedConditionID)
	require.Equal(t, http.StatusOK, answer.Status, answer.Body)

	var body struct {
		References struct {
			Total  int            `json:"total"`
			ByKind map[string]int `json:"by_kind"`
		} `json:"references"`
	}
	require.NoError(t, json.Unmarshal([]byte(answer.Body), &body))

	assert.Equal(t, 2, body.References.Total)
	assert.Equal(t, 1, body.References.ByKind["encounter"])
	assert.Equal(t, 1, body.References.ByKind["treatment"])
}

// TestGetRecordReferencesAreZeroForAnUnreferencedRecord proves the field is
// still present, at {total: 0}, for a record nothing points at — the
// anti-vacuity control: without this, an always-empty field would satisfy
// the test above for the wrong reason.
func TestGetRecordReferencesAreZeroForAnUnreferencedRecord(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	answer := caller.get(recordURL(testsupport.NameOnlyMedicationID))
	require.Equal(t, http.StatusOK, answer.Status, answer.Body)

	var body struct {
		References struct {
			Total int `json:"total"`
		} `json:"references"`
	}
	require.NoError(t, json.Unmarshal([]byte(answer.Body), &body))

	assert.Equal(t, 0, body.References.Total)
}
