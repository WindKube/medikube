package patient_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainaudit "medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/domain/person"
	"medikube/internal/service/patient"
	"medikube/internal/service/patient/patienttest"
)

func newChartService(t *testing.T, counter *patienttest.Counter, activity *patienttest.Activity) (*patient.Service, *patienttest.Repository) {
	t.Helper()

	repo := patienttest.NewRepository()
	auditor := patienttest.NewAuditor()
	authorizer := patienttest.NewAuthorizer(repo, auditor)
	photos := patienttest.NewPhotoStore(15<<20, []string{"image/jpeg", "image/png", "image/webp"})

	svc, err := patient.New(repo, photos, authorizer, patienttest.NewActivePatientStore(), auditor, counter, activity)
	require.NoError(t, err)

	return svc, repo
}

// FR-030, US4-2: a kind with no rows for this patient still gets a tile, at
// zero — the count is never omitted because it happens to be nothing.
func TestSummaryCountsIncludeAKindWithZeroRecords(t *testing.T) {
	t.Parallel()

	counter := patienttest.NewCounter(kind.Medication)
	activity := patienttest.NewActivity()

	svc, repo := newChartService(t, counter, activity)

	created := repo.Seed(person.Patient{OwnerID: patienttest.OwnerID, FirstName: "Amara", LastName: "Okonkwo"})

	chart, err := svc.Summary(t.Context(), owner(), created.ID)
	require.NoError(t, err)

	require.Len(t, chart.Counts, 1)
	assert.Equal(t, kind.Medication, chart.Counts[0].Kind)
	assert.Equal(t, 0, chart.Counts[0].Count)
	assert.Equal(t, 0, chart.TotalRecords)
}

// FR-028, SC-007: the total is the sum of every kind's own count.
func TestSummaryTotalRecordsIsTheSumOfEveryKind(t *testing.T) {
	t.Parallel()

	counter := patienttest.NewCounter(kind.Medication)
	activity := patienttest.NewActivity()

	svc, repo := newChartService(t, counter, activity)

	created := repo.Seed(person.Patient{OwnerID: patienttest.OwnerID, FirstName: "Amara", LastName: "Okonkwo"})

	counter.SetCount(kind.Medication, created.ID, 12)

	chart, err := svc.Summary(t.Context(), owner(), created.ID)
	require.NoError(t, err)

	assert.Equal(t, 12, chart.TotalRecords)
}

// FR-029: recent activity is passed through newest-first, exactly as the
// reader answers it.
func TestSummaryRecentActivityIsTheReadersOwnOrder(t *testing.T) {
	t.Parallel()

	counter := patienttest.NewCounter(kind.Medication)
	activity := patienttest.NewActivity()

	svc, repo := newChartService(t, counter, activity)

	created := repo.Seed(person.Patient{OwnerID: patienttest.OwnerID, FirstName: "Amara", LastName: "Okonkwo"})

	older := domainaudit.Event{OccurredAt: time.Now().Add(-time.Hour), Action: domainaudit.ActionCreate,
		TargetKind: domainaudit.TargetKindMedication, TargetID: "mkfakemed00001", RequestID: "req0000000003"}
	newer := domainaudit.Event{OccurredAt: time.Now(), Action: domainaudit.ActionDelete,
		TargetKind: domainaudit.TargetKindMedication, TargetID: "mkfakemed00001", RequestID: "req0000000004"}

	activity.Seed(created.ID, older)
	activity.Seed(created.ID, newer)

	chart, err := svc.Summary(t.Context(), owner(), created.ID)
	require.NoError(t, err)

	require.Len(t, chart.RecentActivity, 2)
	assert.Equal(t, domainaudit.ActionDelete, chart.RecentActivity[0].Action)
	assert.Equal(t, domainaudit.ActionCreate, chart.RecentActivity[1].Action)
}

// FR-027: a header detail nobody recorded stays absent rather than reading as
// a recorded zero — Summary hands back the domain entity unchanged, and the
// same absence-preserving rule internal/web/api's own DTO test asserts for
// getPatient holds here because Summary reuses Repository.Get verbatim.
func TestSummaryPatientCarriesNoFabricatedDetail(t *testing.T) {
	t.Parallel()

	counter := patienttest.NewCounter(kind.Medication)
	activity := patienttest.NewActivity()

	svc, repo := newChartService(t, counter, activity)

	created := repo.Seed(person.Patient{OwnerID: patienttest.OwnerID, FirstName: "Amara", LastName: "Okonkwo"})

	chart, err := svc.Summary(t.Context(), owner(), created.ID)
	require.NoError(t, err)

	assert.Empty(t, chart.Patient.BloodType)
	assert.Zero(t, chart.Patient.HeightCM)
	assert.Zero(t, chart.Patient.WeightKG)
}

// A stranger's request is refused exactly as Get's is.
func TestSummaryRefusesAStranger(t *testing.T) {
	t.Parallel()

	counter := patienttest.NewCounter(kind.Medication)
	activity := patienttest.NewActivity()

	svc, repo := newChartService(t, counter, activity)

	created := repo.Seed(person.Patient{OwnerID: patienttest.OwnerID, FirstName: "Amara", LastName: "Okonkwo"})

	_, err := svc.Summary(t.Context(), stranger(), created.ID)
	require.Error(t, err)
}
