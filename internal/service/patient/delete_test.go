package patient_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/service/patient"
	"medikube/internal/service/patient/patienttest"
)

// FR-051, US6-4: a self-record is refused, not deleted, and the refusal is
// distinguishable from every other conflict — internal/web maps it to its
// own 409 with the account-closure explanation rather than the generic
// "conflict" text (internal/web/errors_test.go).
func TestDeleteRefusesASelfRecord(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newService(t)

	draft := personDraft()
	draft.OwnerID = patienttest.OwnerID
	draft.IsSelfRecord = true
	self := repo.Seed(draft)

	err := svc.Delete(t.Context(), owner(), self.ID, self.Version)
	assert.ErrorIs(t, err, patient.ErrSelfRecordProtected)

	_, getErr := repo.Get(t.Context(), owner().UserID, self.ID)
	assert.NoError(t, getErr, "the refused delete left the row in place")
}

// The If-Match precondition: a stale version is refused rather than applied.
// A missing precondition is web.IfMatch's own 422 (internal/web/etag.go),
// asserted at the edge in patients_delete_test.go rather than duplicated
// here.
func TestDeleteRefusesAStaleVersion(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	err = svc.Delete(t.Context(), owner(), created.ID, "not-the-real-version")
	assert.ErrorIs(t, err, domain.ErrVersionMismatch)
}

// FR-050, US6-6, SC-005: a stranger's delete is domain.ErrNotFound, exactly
// as every other patient operation refuses one, and nothing is removed.
func TestDeleteRefusesAStranger(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	err = svc.Delete(t.Context(), stranger(), created.ID, created.Version)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	_, getErr := repo.Get(t.Context(), owner().UserID, created.ID)
	assert.NoError(t, getErr)
}

// A successful delete removes the row.
func TestDeleteRemovesTheRow(t *testing.T) {
	t.Parallel()

	svc, repo, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	err = svc.Delete(t.Context(), owner(), created.ID, created.Version)
	require.NoError(t, err)

	_, getErr := repo.Get(t.Context(), owner().UserID, created.ID)
	assert.ErrorIs(t, getErr, domain.ErrNotFound)
}
