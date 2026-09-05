package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
	"medikube/internal/testsupport/seed"
)

// T108, FR-092. Every operation immunizations serve, run three ways: as the
// owning account, as a signed-in stranger and as nobody. It mirrors
// TestEveryRecordOperationIsOwnerScoped in records_authz_test.go — it is its
// own file, per kind, rather than an extension of that one, because FR-092
// wants the proof for THIS kind rather than a shared assumption that whatever
// held for medication holds for everything registered after it.
//
// The subtests below are not run in parallel with each other: they share one
// instance and one row count (the create case adds a row, the delete case
// removes one), so the list assertions would be flaky against a total that
// depended on run order.

func immunizationCollectionURL() string { return "/api/v1/records/" + kind.Immunization.Segment() }

func immunizationRecordURL(id string) string { return immunizationCollectionURL() + "/" + id }

// immunizationHTTPDTO mirrors api.Immunization as a client sees it, declared
// here rather than imported so the wire shape is a contract an assertion has
// to restate, not the struct the handler currently returns.
type immunizationHTTPDTO struct {
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`
	VaccineName    string  `json:"vaccine_name"`
	TradeName      string  `json:"trade_name"`
	AdministeredOn *string `json:"administered_on"`
	DoseNumber     *int    `json:"dose_number"`
}

func (r response) immunization(t *testing.T) immunizationHTTPDTO {
	t.Helper()

	var dto immunizationHTTPDTO
	r.decode(t, &dto)

	return dto
}

func TestEveryImmunizationOperationIsOwnerScoped(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	stranger := owner.as(testsupport.AccountBEmail)
	guest := owner.anonymous()

	target := immunizationRecordURL(seed.ImmunizationSampleID)
	removable := immunizationRecordURL("mkimmamara00002")

	current := owner.get(target)
	require.Equal(t, http.StatusOK, current.Status, current.Body)
	version := current.etag(t)

	removableCurrent := owner.get(removable)
	require.Equal(t, http.StatusOK, removableCurrent.Status, removableCurrent.Body)
	removableVersion := removableCurrent.etag(t)

	secrets := []string{seed.ImmunizationSampleID, "Influenza", "Fluarix", testsupport.AccountAID}
	removableSecrets := []string{"mkimmamara00002", "Tetanus", testsupport.AccountAID}

	// The cross-kind list is deliberately left out of this matrix: with three
	// kinds now registered, ErrCrossKindPaging refuses ANY selection wider
	// than one kind with a 400 that does not distinguish owner from stranger
	// (internal/records/handler.go's List, case default) — the same
	// limitation records_authz_test.go's own "list every kind" case already
	// hits. Cross-kind pagination is a capability of the record family as a
	// whole rather than of this kind, so it is out of scope for a per-kind
	// authorization file.
	t.Run("list one kind", func(t *testing.T) {
		mine := owner.get(immunizationCollectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100")
		require.Equal(t, http.StatusOK, mine.Status, mine.Body)
		assert.Len(t, mine.list(t).Items, 2)

		theirs := stranger.get(immunizationCollectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100")
		assert.Equal(t, http.StatusNotFound, theirs.Status, theirs.Body)
		assertNoImmunizationSecrets(t, "the stranger's list", theirs.Body, secrets)

		nobody := guest.get(immunizationCollectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100")
		assert.Equal(t, http.StatusUnauthorized, nobody.Status, nobody.Body)
	})

	t.Run("record a vaccination", func(t *testing.T) {
		body := `{"patient":"` + testsupport.AccountAPatientSelfID + `","vaccine_name":"Shingles","administered_on":"2026-01-01"}`

		created := owner.post(immunizationCollectionURL(), body)
		require.Equal(t, http.StatusCreated, created.Status, created.Body)
		assert.Equal(t, "Shingles", created.immunization(t).VaccineName)

		nobody := guest.post(immunizationCollectionURL(), body)
		assert.Equal(t, http.StatusUnauthorized, nobody.Status, nobody.Body)
	})

	t.Run("read one", func(t *testing.T) {
		read := owner.get(target)
		require.Equal(t, http.StatusOK, read.Status, read.Body)
		assert.Equal(t, "Influenza", read.immunization(t).VaccineName)

		refused := stranger.get(target)
		assert.Equal(t, http.StatusNotFound, refused.Status, refused.Body)
		assertNoImmunizationSecrets(t, "the stranger's read", refused.Body, secrets)

		missing := owner.get(immunizationRecordURL(missingID))
		assert.Equal(t, missing.Status, refused.Status)
		assert.Equal(t, withoutCorrelationID(missing.Body), withoutCorrelationID(refused.Body),
			"a stranger's refusal must read exactly like a genuine miss")

		nobody := guest.get(target)
		assert.Equal(t, http.StatusUnauthorized, nobody.Status, nobody.Body)
	})

	t.Run("change one", func(t *testing.T) {
		refused := stranger.patch(target, `{"trade_name":"Fluzone"}`, version)
		assert.Equal(t, http.StatusNotFound, refused.Status, refused.Body)
		assertNoImmunizationSecrets(t, "the stranger's change", refused.Body, secrets)

		nobody := guest.patch(target, `{"trade_name":"Fluzone"}`, version)
		assert.Equal(t, http.StatusUnauthorized, nobody.Status, nobody.Body)

		changed := owner.patch(target, `{"trade_name":"Fluzone"}`, version)
		require.Equal(t, http.StatusOK, changed.Status, changed.Body)
		assert.Equal(t, "Fluzone", changed.immunization(t).TradeName)
	})

	t.Run("delete one", func(t *testing.T) {
		refused := stranger.delete(removable, removableVersion)
		assert.Equal(t, http.StatusNotFound, refused.Status, refused.Body)
		assertNoImmunizationSecrets(t, "the stranger's delete", refused.Body, removableSecrets)

		nobody := guest.delete(removable, removableVersion)
		assert.Equal(t, http.StatusUnauthorized, nobody.Status, nobody.Body)

		gone := owner.delete(removable, removableVersion)
		assert.Equal(t, http.StatusNoContent, gone.Status, gone.Body)
	})
}

// assertNoImmunizationSecrets is testsupport's assertNoSecrets, unexported
// there, restated for a body already read into a string.
func assertNoImmunizationSecrets(t *testing.T, where, body string, secrets []string) {
	t.Helper()

	for _, secret := range secrets {
		assert.NotContains(t, body, secret, "%s discloses %q", where, secret)
	}
}
