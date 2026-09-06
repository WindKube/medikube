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

// criteriaListDTO is a single kind's list envelope, read generically: every
// row is left as raw JSON so a test picks out the one member (`basis`) it
// cares about without restating the rest of the kind's own shape.
type criteriaListDTO struct {
	Items    []json.RawMessage `json:"items"`
	Criteria map[string]any    `json:"criteria"`
}

func (r response) criteriaList(t *testing.T) criteriaListDTO {
	t.Helper()

	var page criteriaListDTO
	r.decode(t, &page)

	return page
}

func basisOf(t *testing.T, row json.RawMessage) []string {
	t.Helper()

	var found struct {
		Basis []string `json:"basis"`
	}
	require.NoError(t, json.Unmarshal(row, &found))

	return found.Basis
}

// T180/T186. Every single-kind list echoes the narrowing it resolved, and
// every row states why it qualified for a narrowing that carries a row-level
// basis (contracts/records-clinical.md §1).
func TestListOfKindEchoesCriteriaAndPopulatesBasis(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	patient := testsupport.AccountAPatientSelfID

	t.Run("allergy critical", func(t *testing.T) {
		url := "/api/v1/records/" + kind.Allergy.Segment() + "?patient=" + patient + "&critical=true"

		got := owner.get(url)
		require.Equal(t, http.StatusOK, got.Status, got.Body)

		page := got.criteriaList(t)
		assert.Equal(t, patient, page.Criteria["patient"])
		assert.ElementsMatch(t, []any{"true"}, page.Criteria["critical"])
		require.NotEmpty(t, page.Items)

		for _, item := range page.Items {
			assert.Equal(t, []string{"critical"}, basisOf(t, item))
		}
	})

	t.Run("procedure scheduled", func(t *testing.T) {
		url := "/api/v1/records/" + kind.Procedure.Segment() + "?patient=" + patient + "&scheduled=true"

		got := owner.get(url)
		require.Equal(t, http.StatusOK, got.Status, got.Body)

		page := got.criteriaList(t)
		require.NotEmpty(t, page.Items)

		for _, item := range page.Items {
			assert.NotEmpty(t, basisOf(t, item))
		}
	})

	t.Run("insurance expiring", func(t *testing.T) {
		url := "/api/v1/records/" + kind.Insurance.Segment() +
			"?patient=" + testsupport.AccountAPatientParentID + "&expiring_within_days=60"

		got := owner.get(url)
		require.Equal(t, http.StatusOK, got.Status, got.Body)

		page := got.criteriaList(t)
		require.NotEmpty(t, page.Items)

		for _, item := range page.Items {
			assert.Contains(t, basisOf(t, item), "expiring")
		}
	})

	t.Run("equipment service due", func(t *testing.T) {
		url := "/api/v1/records/" + kind.Equipment.Segment() + "?patient=" + patient + "&service_due_within_days=30"

		got := owner.get(url)
		require.Equal(t, http.StatusOK, got.Status, got.Body)

		page := got.criteriaList(t)
		require.NotEmpty(t, page.Items)

		for _, item := range page.Items {
			assert.NotEmpty(t, basisOf(t, item))
		}
	})

	// US9-1: with both an active and a resolved condition on record, the
	// active view lists only the active one and says why it was selected.
	t.Run("condition active", func(t *testing.T) {
		collection := "/api/v1/records/" + kind.Condition.Segment()

		created := owner.post(collection,
			`{"patient":"`+patient+`","diagnosis":"Seasonal rhinitis","status":"active"}`)
		require.Equal(t, http.StatusCreated, created.Status, created.Body)

		got := owner.get(collection + "?patient=" + patient + "&active=true")
		require.Equal(t, http.StatusOK, got.Status, got.Body)

		page := got.criteriaList(t)
		assert.ElementsMatch(t, []any{"true"}, page.Criteria["active"])
		require.NotEmpty(t, page.Items)

		for _, item := range page.Items {
			var row struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			}
			require.NoError(t, json.Unmarshal(item, &row))
			assert.NotEqual(t, testsupport.ResolvedConditionID, row.ID)
			assert.Contains(t, []string{"active", "chronic"}, row.Status)
			assert.Equal(t, []string{"active"}, basisOf(t, item))
		}
	})

	t.Run("no narrowing echoes an empty criteria beyond patient", func(t *testing.T) {
		url := "/api/v1/records/" + kind.Allergy.Segment() + "?patient=" + patient

		got := owner.get(url)
		require.Equal(t, http.StatusOK, got.Status, got.Body)

		page := got.criteriaList(t)
		assert.Equal(t, patient, page.Criteria["patient"])
		assert.NotContains(t, page.Criteria, "critical")
	})
}
