package symptom_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/service/symptom"
	"medikube/internal/service/symptom/symptomtest"
)

// FR-032, US6-2: treated_by_medications and caused_by_medications are two
// distinct sets over the same target kind — a medication may be named in
// either, or both, without one field's membership constraining the other.

type fakeRoleResolver struct {
	found map[string]bool
}

func (f fakeRoleResolver) Resolve(_ context.Context, _ kind.Kind, ids []string) ([]clinical.PatientRef, error) {
	refs := make([]clinical.PatientRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, clinical.PatientRef{ID: id, PatientID: patientID, Found: f.found[id]})
	}

	return refs, nil
}

type fakeRoleAuthorizer struct{}

func (fakeRoleAuthorizer) Patient(
	_ context.Context, _ access.Actor, _ string, _ access.Permission,
) (access.Grant, error) {
	return access.Grant{Level: access.PermOwn}, nil
}

func newServiceWithLinks(t *testing.T, found map[string]bool) *symptom.Service {
	t.Helper()

	repo := symptomtest.NewFakeRepository()

	svc, err := symptom.New(repo, symptomtest.Authorizer{OwnerID: ownerID},
		symptom.WithLinks(fakeRoleResolver{found: found}, fakeRoleAuthorizer{}))
	require.NoError(t, err)

	return svc
}

func TestCreateKeepsTreatedByAndCausedByAsDistinctSets(t *testing.T) {
	t.Parallel()

	svc := newServiceWithLinks(t, map[string]bool{"m1": true, "m2": true})

	created, err := svc.Create(t.Context(), owner(), func() clinical.Symptom {
		d := draft()
		d.TreatedByMedicationIDs = []string{"m1"}
		d.CausedByMedicationIDs = []string{"m2"}

		return d
	}())
	require.NoError(t, err)

	assert.Equal(t, []string{"m1"}, created.TreatedByMedicationIDs)
	assert.Equal(t, []string{"m2"}, created.CausedByMedicationIDs)
}

func TestCreateAllowsTheSameMedicationInBothRoles(t *testing.T) {
	t.Parallel()

	svc := newServiceWithLinks(t, map[string]bool{"m1": true})

	created, err := svc.Create(t.Context(), owner(), func() clinical.Symptom {
		d := draft()
		d.TreatedByMedicationIDs = []string{"m1"}
		d.CausedByMedicationIDs = []string{"m1"}

		return d
	}())
	require.NoError(t, err)

	assert.Equal(t, []string{"m1"}, created.TreatedByMedicationIDs)
	assert.Equal(t, []string{"m1"}, created.CausedByMedicationIDs)
}
