package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
)

// T108, FR-092. Every operation injuries serve, run three ways: as the owning
// account, as a signed-in stranger and as nobody. It mirrors
// immunization_http_test.go's own matrix, one file per kind for the same
// reason: FR-092 wants the proof for THIS kind rather than a shared
// assumption that whatever held for another kind holds here too.
//
// The subtests are not run in parallel with each other: they share one
// instance and one row count (the create case adds a row, the delete case
// removes one), so the list assertions would be flaky against a total that
// depended on run order.
//
// The cross-kind list is left out for the same reason immunization's matrix
// leaves it out: with three kinds registered, internal/records/handler.go's
// List refuses any selection wider than one kind with a 400 that does not
// distinguish owner from stranger (ErrCrossKindPaging) — a limitation of the
// record family as a whole, not of injury.

const (
	injurySampleID    = "mkinjamara00001"
	injuryRemovableID = "mkinjamara00002"
)

func injuryCollectionURL() string { return "/api/v1/records/" + kind.Injury.Segment() }

func injuryRecordURL(id string) string { return injuryCollectionURL() + "/" + id }

// injuryHTTPDTO mirrors api.Injury as a client sees it, declared here rather
// than imported so the wire shape is a contract an assertion has to restate,
// not the struct the handler currently returns. It also carries laterality,
// per FR-041/US4-4's requirement that the side is visible wherever an injury
// is shown — the wire representation included.
type injuryHTTPDTO struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Laterality string `json:"laterality"`
	Severity   string `json:"severity"`
	Status     string `json:"status"`
}

func (r response) injury(t *testing.T) injuryHTTPDTO {
	t.Helper()

	var dto injuryHTTPDTO
	r.decode(t, &dto)

	return dto
}

func TestEveryInjuryOperationIsOwnerScoped(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)
	stranger := owner.as(testsupport.AccountBEmail)
	guest := owner.anonymous()

	target := injuryRecordURL(injurySampleID)
	removable := injuryRecordURL(injuryRemovableID)

	current := owner.get(target)
	require.Equal(t, http.StatusOK, current.Status, current.Body)
	version := current.etag(t)

	removableCurrent := owner.get(removable)
	require.Equal(t, http.StatusOK, removableCurrent.Status, removableCurrent.Body)
	removableVersion := removableCurrent.etag(t)

	secrets := []string{injurySampleID, "Sprained ankle", "fell while running", testsupport.AccountAID}
	removableSecrets := []string{injuryRemovableID, "Cut on hand", "healed without complication", testsupport.AccountAID}

	t.Run("list one kind", func(t *testing.T) {
		mine := owner.get(injuryCollectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100")
		require.Equal(t, http.StatusOK, mine.Status, mine.Body)
		assert.Len(t, mine.list(t).Items, 2)

		theirs := stranger.get(injuryCollectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100")
		assert.Equal(t, http.StatusNotFound, theirs.Status, theirs.Body)
		assertNoImmunizationSecrets(t, "the stranger's list", theirs.Body, secrets)

		nobody := guest.get(injuryCollectionURL() + "?patient=" + testsupport.AccountAPatientSelfID + "&limit=100")
		assert.Equal(t, http.StatusUnauthorized, nobody.Status, nobody.Body)
	})

	t.Run("record an injury", func(t *testing.T) {
		body := `{"patient":"` + testsupport.AccountAPatientSelfID + `","name":"Twisted knee","body_part":"knee","laterality":"left"}`

		created := owner.post(injuryCollectionURL(), body)
		require.Equal(t, http.StatusCreated, created.Status, created.Body)
		assert.Equal(t, "Twisted knee", created.injury(t).Name)
		assert.Equal(t, "left", created.injury(t).Laterality)

		nobody := guest.post(injuryCollectionURL(), body)
		assert.Equal(t, http.StatusUnauthorized, nobody.Status, nobody.Body)
	})

	t.Run("read one", func(t *testing.T) {
		read := owner.get(target)
		require.Equal(t, http.StatusOK, read.Status, read.Body)
		assert.Equal(t, "Sprained ankle", read.injury(t).Name)
		assert.Equal(t, "right", read.injury(t).Laterality)

		refused := stranger.get(target)
		assert.Equal(t, http.StatusNotFound, refused.Status, refused.Body)
		assertNoImmunizationSecrets(t, "the stranger's read", refused.Body, secrets)

		missing := owner.get(injuryRecordURL(missingID))
		assert.Equal(t, missing.Status, refused.Status)
		assert.Equal(t, withoutCorrelationID(missing.Body), withoutCorrelationID(refused.Body),
			"a stranger's refusal must read exactly like a genuine miss")

		nobody := guest.get(target)
		assert.Equal(t, http.StatusUnauthorized, nobody.Status, nobody.Body)
	})

	t.Run("change one", func(t *testing.T) {
		refused := stranger.patch(target, `{"laterality":"bilateral"}`, version)
		assert.Equal(t, http.StatusNotFound, refused.Status, refused.Body)
		assertNoImmunizationSecrets(t, "the stranger's change", refused.Body, secrets)

		nobody := guest.patch(target, `{"laterality":"bilateral"}`, version)
		assert.Equal(t, http.StatusUnauthorized, nobody.Status, nobody.Body)

		changed := owner.patch(target, `{"laterality":"bilateral"}`, version)
		require.Equal(t, http.StatusOK, changed.Status, changed.Body)
		assert.Equal(t, "bilateral", changed.injury(t).Laterality)
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
