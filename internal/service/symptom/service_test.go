package symptom_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/symptom"
	"medikube/internal/service/symptom/symptomtest"
)

const ownerID = "mkuserowner0001"
const patientID = "mkpatowner00001"

func newService(t *testing.T) (*symptom.Service, *symptomtest.FakeRepository) {
	t.Helper()

	repo := symptomtest.NewFakeRepository()
	svc, err := symptom.New(repo, symptomtest.Authorizer{OwnerID: ownerID})
	require.NoError(t, err)

	return svc, repo
}

func owner() access.Actor { return access.Actor{UserID: ownerID} }

func draft() clinical.Symptom {
	return clinical.Symptom{
		PatientID:  patientID,
		Name:       "Dizziness",
		Severity:   clinical.SeverityModerate,
		OccurredAt: clinical.NewInstant(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)),
	}
}

func TestServiceCreate(t *testing.T) {
	t.Parallel()

	t.Run("recording the same name again creates a second episode", func(t *testing.T) {
		t.Parallel()

		svc, _ := newService(t)

		first, err := svc.Create(t.Context(), owner(), draft())
		require.NoError(t, err)

		second, err := svc.Create(t.Context(), owner(), draft())
		require.NoError(t, err)

		assert.NotEqual(t, first.ID, second.ID)
		assert.Equal(t, 2, second.EpisodeCount)
	})

	t.Run("a non-owner is refused", func(t *testing.T) {
		t.Parallel()

		svc, _ := newService(t)

		_, err := svc.Create(t.Context(), access.Actor{UserID: "somebody-else"}, draft())
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("an empty patient is refused as a validation failure", func(t *testing.T) {
		t.Parallel()

		svc, _ := newService(t)

		d := draft()
		d.PatientID = ""

		_, err := svc.Create(t.Context(), owner(), d)
		require.Error(t, err)

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
	})
}

func TestServiceAggregate(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t)

	names := []string{"Dizziness", "dizziness", "DIZZINESS", "Headache"}
	dates := []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
	}

	for i, name := range names {
		d := draft()
		d.Name = name
		d.OccurredAt = clinical.NewInstant(dates[i])
		_, err := svc.Create(t.Context(), owner(), d)
		require.NoError(t, err)
	}

	last, err := svc.Get(t.Context(), owner(), (func() string {
		page, listErr := svc.List(t.Context(), owner(), symptom.Query{PatientID: patientID, Limit: 100})
		require.NoError(t, listErr)
		require.NotEmpty(t, page.Items)

		for _, item := range page.Items {
			if item.Name == "DIZZINESS" {
				return item.ID
			}
		}

		t.Fatal("no dizziness episode found")

		return ""
	})())
	require.NoError(t, err)

	assert.Equal(t, 3, last.EpisodeCount, "names differing only in case group together")
	assert.Equal(t, dates[2], last.LastOccurredAt.Time(), "the newest occurrence is the latest of the three")
}

func TestServiceUpdateAndDelete(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), draft())
	require.NoError(t, err)

	newName := "Vertigo"
	updated, err := svc.Update(t.Context(), owner(), created.ID, created.Version, symptom.Patch{Name: &newName})
	require.NoError(t, err)
	assert.Equal(t, "Vertigo", updated.Name)

	_, err = svc.Update(t.Context(), owner(), created.ID, created.Version, symptom.Patch{})
	require.ErrorIs(t, err, domain.ErrVersionMismatch, "the version has already moved on")

	require.NoError(t, svc.Delete(t.Context(), owner(), created.ID, updated.Version))

	_, err = svc.Get(t.Context(), owner(), created.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
}
