package insurance_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/insurance"
	"medikube/internal/service/insurance/insurancetest"
)

func actor() access.Actor {
	return access.Actor{UserID: insurancetest.OwnerID, RequestID: "req-insurance"}
}

func stranger() access.Actor {
	return access.Actor{UserID: insurancetest.StrangerID, RequestID: "req-insurance-stranger"}
}

func newService(t *testing.T) *insurance.Service {
	t.Helper()

	service, err := insurance.New(insurancetest.NewRepository(), insurancetest.NewAuthorizer(insurancetest.OwnerID))
	require.NoError(t, err)

	return service
}

func minimalPolicy() clinical.Insurance {
	effectiveOn, _ := domain.ParseDate("2026-01-01")

	return clinical.Insurance{
		PatientID:   insurancetest.PatientID,
		Type:        clinical.InsuranceTypeMedical,
		Company:     "Acme Health",
		MemberName:  "Jamie Doe",
		MemberID:    "M1",
		EffectiveOn: effectiveOn,
	}
}

func TestServiceNew(t *testing.T) {
	t.Parallel()

	_, err := insurance.New(nil, insurancetest.NewAuthorizer(insurancetest.OwnerID))
	assert.Error(t, err)

	_, err = insurance.New(insurancetest.NewRepository(), nil)
	assert.Error(t, err)
}

func TestCreateRequiresAPatient(t *testing.T) {
	t.Parallel()

	service := newService(t)

	_, err := service.Create(t.Context(), actor(), clinical.Insurance{})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, insurance.FieldPatient, invalid.Fields[0].Field)
}

func TestCreateGet(t *testing.T) {
	t.Parallel()

	service := newService(t)

	result, err := service.Create(t.Context(), actor(), minimalPolicy())
	require.NoError(t, err)
	assert.NotEmpty(t, result.Insurance.ID)
	assert.Nil(t, result.Displaced)

	found, err := service.Get(t.Context(), actor(), result.Insurance.ID)
	require.NoError(t, err)
	assert.Equal(t, result.Insurance.ID, found.ID)

	_, err = service.Get(t.Context(), stranger(), result.Insurance.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// TestSettingPrimaryDisplacesTheOldOne is FR-045: marking a second policy
// primary displaces the first, in one write, and the write reports it.
func TestSettingPrimaryDisplacesTheOldOne(t *testing.T) {
	t.Parallel()

	service := newService(t)

	first := minimalPolicy()
	first.IsPrimary = true
	firstResult, err := service.Create(t.Context(), actor(), first)
	require.NoError(t, err)
	assert.Nil(t, firstResult.Displaced)

	second := minimalPolicy()
	second.IsPrimary = true
	secondResult, err := service.Create(t.Context(), actor(), second)
	require.NoError(t, err)
	require.NotNil(t, secondResult.Displaced)
	assert.Equal(t, firstResult.Insurance.ID, secondResult.Displaced.ID)

	reread, err := service.Get(t.Context(), actor(), firstResult.Insurance.ID)
	require.NoError(t, err)
	assert.False(t, reread.IsPrimary, "the first policy should no longer be primary")
}

func TestUpdateAndDeleteCheckVersion(t *testing.T) {
	t.Parallel()

	service := newService(t)

	created, err := service.Create(t.Context(), actor(), minimalPolicy())
	require.NoError(t, err)

	_, err = service.Update(t.Context(), actor(), created.Insurance.ID, "stale", insurance.Patch{})
	assert.ErrorIs(t, err, domain.ErrVersionMismatch)

	newCompany := "New Health Co"
	updated, err := service.Update(t.Context(), actor(), created.Insurance.ID, created.Insurance.Version, insurance.Patch{Company: &newCompany})
	require.NoError(t, err)
	assert.Equal(t, newCompany, updated.Insurance.Company)

	err = service.Delete(t.Context(), actor(), created.Insurance.ID, "stale")
	assert.ErrorIs(t, err, domain.ErrVersionMismatch)

	require.NoError(t, service.Delete(t.Context(), actor(), created.Insurance.ID, updated.Insurance.Version))
}

func TestExpiringBasis(t *testing.T) {
	t.Parallel()

	today := clinical.Today()

	past, err := domain.ParseDate("2020-01-01")
	require.NoError(t, err)

	far, err := domain.ParseDate("2099-01-01")
	require.NoError(t, err)

	cases := []struct {
		name      string
		expiresOn clinical.Date
		want      []string
	}{
		{name: "no expiry recorded", expiresOn: clinical.Date{}, want: nil},
		{name: "already expired", expiresOn: past, want: nil},
		{name: "expiring within the window", expiresOn: today, want: []string{insurance.BasisExpiring}},
		{name: "far in the future", expiresOn: far, want: nil},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := insurance.ExpiringBasis(clinical.Insurance{ExpiresOn: tt.expiresOn}, 60)
			assert.Equal(t, tt.want, got)
		})
	}
}
