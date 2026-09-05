package vitals_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/vitals"
	"medikube/internal/service/vitals/vitalstest"
)

const ownerID = "mkuserowner0001"
const patientID = "mkpatowner00001"

func f(v float64) *float64 { return &v }

func newService(t *testing.T) *vitals.Service {
	t.Helper()

	repo := vitalstest.NewFakeRepository()
	svc, err := vitals.New(repo, vitalstest.Authorizer{OwnerID: ownerID})
	require.NoError(t, err)

	return svc
}

func owner() access.Actor { return access.Actor{UserID: ownerID} }

func draft() clinical.Vitals {
	return clinical.Vitals{
		PatientID:  patientID,
		RecordedAt: clinical.NewInstant(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)),
		WeightKg:   f(70),
	}
}

func TestServiceCreate(t *testing.T) {
	t.Parallel()

	t.Run("a valid set is stored", func(t *testing.T) {
		t.Parallel()

		svc := newService(t)

		created, err := svc.Create(t.Context(), owner(), draft())
		require.NoError(t, err)
		assert.NotEmpty(t, created.ID)
	})

	t.Run("a set with no measurement is refused", func(t *testing.T) {
		t.Parallel()

		svc := newService(t)

		d := draft()
		d.WeightKg = nil

		_, err := svc.Create(t.Context(), owner(), d)
		require.Error(t, err)
	})

	t.Run("a non-owner is refused", func(t *testing.T) {
		t.Parallel()

		svc := newService(t)

		_, err := svc.Create(t.Context(), access.Actor{UserID: "somebody-else"}, draft())
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestServiceUpdateAndDelete(t *testing.T) {
	t.Parallel()

	svc := newService(t)

	created, err := svc.Create(t.Context(), owner(), draft())
	require.NoError(t, err)

	newWeight := f(72)
	updated, err := svc.Update(t.Context(), owner(), created.ID, created.Version, vitals.Patch{WeightKg: &newWeight})
	require.NoError(t, err)
	require.NotNil(t, updated.WeightKg)
	assert.InDelta(t, 72, *updated.WeightKg, 0.0001)

	_, err = svc.Update(t.Context(), owner(), created.ID, created.Version, vitals.Patch{})
	require.ErrorIs(t, err, domain.ErrVersionMismatch)

	require.NoError(t, svc.Delete(t.Context(), owner(), created.ID, updated.Version))

	_, err = svc.Get(t.Context(), owner(), created.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
}
