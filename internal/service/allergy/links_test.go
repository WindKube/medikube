package allergy_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/service/allergy"
	"medikube/internal/service/allergy/allergytest"
)

type fakeResolver struct {
	patient string
	found   map[string]bool
}

func (f fakeResolver) Resolve(_ context.Context, _ kind.Kind, ids []string) ([]clinical.PatientRef, error) {
	refs := make([]clinical.PatientRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, clinical.PatientRef{ID: id, PatientID: f.patient, Found: f.found[id]})
	}

	return refs, nil
}

type fakeLinkAuthorizer struct{}

func (fakeLinkAuthorizer) Record(
	_ context.Context, _ access.Actor, _ kind.Kind, _ string, _ access.Permission,
) (access.Grant, error) {
	return access.Grant{Level: access.PermOwn}, nil
}

func TestCreateValidatesTheMedicationsLinkWhenWired(t *testing.T) {
	t.Parallel()

	repo := allergytest.NewRepository()
	auth := allergytest.NewAuthorizer(allergytest.OwnerID)

	resolver := fakeResolver{patient: allergytest.PatientID, found: map[string]bool{"m1": true}}
	svc, err := allergy.New(repo, auth, allergy.WithLinks(resolver, fakeLinkAuthorizer{}))
	require.NoError(t, err)

	created, err := svc.Create(t.Context(), owner(), clinical.Allergy{
		PatientID: allergytest.PatientID, Allergen: "Peanuts", Severity: clinical.SeverityMild,
		MedicationIDs: []string{"m1"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"m1"}, created.MedicationIDs)
}

func TestCreateRefusesAMedicationOfAnotherPatientWhenWired(t *testing.T) {
	t.Parallel()

	repo := allergytest.NewRepository()
	auth := allergytest.NewAuthorizer(allergytest.OwnerID)

	resolver := fakeResolver{patient: "someone-elses-patient", found: map[string]bool{"m1": true}}
	svc, err := allergy.New(repo, auth, allergy.WithLinks(resolver, fakeLinkAuthorizer{}))
	require.NoError(t, err)

	_, err = svc.Create(t.Context(), owner(), clinical.Allergy{
		PatientID: allergytest.PatientID, Allergen: "Peanuts", Severity: clinical.SeverityMild,
		MedicationIDs: []string{"m1"},
	})
	require.ErrorIs(t, err, domain.ErrNotFound)
}
