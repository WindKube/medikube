package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/testsupport"
)

func searchURL(query string) string { return "/api/v1/search?" + query }

func decodedSearch(t *testing.T, r response) searchDTO {
	t.Helper()

	var decoded searchDTO
	require.NoError(t, json.Unmarshal(r.rawBody, &decoded))

	return decoded
}

// searchDTO mirrors api.SearchResponse for the shape this test needs.
type searchDTO struct {
	Groups []struct {
		Kind  string `json:"kind"`
		Items []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"items"`
	} `json:"groups"`
	Criteria struct {
		QPresent bool `json:"q_present"`
	} `json:"criteria"`
	EmptyReason *string `json:"empty_reason"`
}

// T167, FR-069, FR-075. An absent `?patient=` is 400 patient_required with no
// fallback to the caller's active patient, the way every other patient-scoped
// list refuses it.
func TestSearchRequiresAPatient(t *testing.T) {
	t.Parallel()

	c := newCaller(t)

	resp := c.get(searchURL("q=paracetamol"))
	require.Equal(t, http.StatusBadRequest, resp.Status, resp.Body)
	assert.Equal(t, "patient_required", resp.envelope(t).Error.Code)
}

// FR-033. A patient another account owns answers 404, byte-identical apart
// from request_id to a patient id that never existed — search is patient-
// scoped data like any other.
func TestSearchOnAnUnreachablePatientIsNotFound(t *testing.T) {
	t.Parallel()

	c := newCaller(t)

	stranger := c.get(searchURL("patient=" + testsupport.AccountBPatientSelfID + "&q=paracetamol"))
	require.Equal(t, http.StatusNotFound, stranger.Status, stranger.Body)

	missing := c.get(searchURL("patient=mkpatnobody0001&q=paracetamol"))
	require.Equal(t, http.StatusNotFound, missing.Status, missing.Body)

	assert.Equal(t, withoutCorrelationID(stranger.Body), withoutCorrelationID(missing.Body))
}

// FR-072/FR-073. A term matching only another account's records answers
// exactly like a term matching nobody's: no groups, and the "no matches"
// reason — never a hint that the record exists but belongs to somebody else.
func TestSearchAcrossAnotherAccountsRecordsIsByteIdenticalToNoMatches(t *testing.T) {
	t.Parallel()

	c := newCaller(t)

	// "Atorvastatin" only exists on account B's data (seed.go), and never on
	// account A's patient this request names.
	othersOnly := c.get(searchURL("patient=" + testsupport.AccountAPatientSelfID + "&q=atorvastatin"))
	require.Equal(t, http.StatusOK, othersOnly.Status, othersOnly.Body)

	nonsense := c.get(searchURL("patient=" + testsupport.AccountAPatientSelfID + "&q=zzqxnonsenseterm"))
	require.Equal(t, http.StatusOK, nonsense.Status, nonsense.Body)

	assert.Equal(t, withoutCorrelationID(othersOnly.Body), withoutCorrelationID(nonsense.Body))

	decoded := decodedSearch(t, othersOnly)
	assert.Empty(t, decoded.Groups)
	require.NotNil(t, decoded.EmptyReason)
	assert.Equal(t, "no_matches", *decoded.EmptyReason)
}

// The read side's own contract: a term matching the owner's own record comes
// back grouped by kind, and criteria echoes q_present rather than the term.
func TestSearchFindsTheOwnersOwnRecord(t *testing.T) {
	t.Parallel()

	c := newCaller(t)

	resp := c.get(searchURL("patient=" + testsupport.AccountAPatientSelfID + "&q=paracetamol"))
	require.Equal(t, http.StatusOK, resp.Status, resp.Body)

	decoded := decodedSearch(t, resp)
	require.NotEmpty(t, decoded.Groups)
	assert.True(t, decoded.Criteria.QPresent)
	assert.Nil(t, decoded.EmptyReason)

	var found bool
	for _, group := range decoded.Groups {
		for _, item := range group.Items {
			if item.ID == testsupport.NameOnlyMedicationID {
				found = true
			}
		}
	}
	assert.True(t, found, "the seeded Paracetamol medication was not in the result: %s", resp.Body)

	assert.NotContains(t, resp.Body, "q\":\"paracetamol\"", "the search term must never be echoed back whole (FR-075)")
}

// An unknown kind in `?kinds=` is 400 bad_request and never echoes the value
// it could not resolve.
func TestSearchRefusesAnUnknownKind(t *testing.T) {
	t.Parallel()

	c := newCaller(t)

	resp := c.get(searchURL("patient=" + testsupport.AccountAPatientSelfID + "&q=paracetamol&kinds=not-a-real-kind"))
	require.Equal(t, http.StatusBadRequest, resp.Status, resp.Body)
	assert.NotContains(t, resp.Body, "not-a-real-kind")
}
