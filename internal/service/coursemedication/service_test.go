package coursemedication_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/coursemedication"
)

type fakeRepository struct {
	rows map[string]clinical.CourseMedication // key: treatment+"|"+medication
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{rows: map[string]clinical.CourseMedication{}}
}

func key(treatmentID, medicationID string) string { return treatmentID + "|" + medicationID }

func (f *fakeRepository) List(_ context.Context, treatmentID string) ([]clinical.CourseMedication, error) {
	var rows []clinical.CourseMedication

	for k, row := range f.rows {
		if row.TreatmentID == treatmentID {
			_ = k

			rows = append(rows, row)
		}
	}

	return rows, nil
}

func (f *fakeRepository) Upsert(
	_ context.Context, entity clinical.CourseMedication, _ string,
) (clinical.CourseMedication, bool, error) {
	k := key(entity.TreatmentID, entity.MedicationID)

	existing, found := f.rows[k]
	if found {
		entity.ID = existing.ID
		f.rows[k] = entity

		return entity, false, nil
	}

	entity.ID = "cm_" + k
	f.rows[k] = entity

	return entity, true, nil
}

func (f *fakeRepository) Delete(_ context.Context, treatmentID, medicationID, _ string) error {
	delete(f.rows, key(treatmentID, medicationID))

	return nil
}

type fakeTreatments struct {
	byID map[string]clinical.Treatment
}

func (f fakeTreatments) Get(_ context.Context, id string) (clinical.Treatment, error) {
	t, found := f.byID[id]
	if !found {
		return clinical.Treatment{}, domain.ErrNotFound
	}

	return t, nil
}

type fakeMedications struct {
	byID map[string]clinical.Medication
}

func (f fakeMedications) Get(_ context.Context, id string) (clinical.Medication, error) {
	m, found := f.byID[id]
	if !found {
		return clinical.Medication{}, domain.ErrNotFound
	}

	return m, nil
}

type fakeAuthorizer struct {
	calls  []string
	denied map[string]bool
}

func (f *fakeAuthorizer) Patient(
	_ context.Context, _ access.Actor, patientID string, _ access.Permission,
) (access.Grant, error) {
	f.calls = append(f.calls, patientID)

	if f.denied[patientID] {
		return access.Grant{}, nil
	}

	return access.Grant{Level: access.PermOwn}, nil
}

type fakeAuditor struct {
	events []audit.Event
}

func (f *fakeAuditor) Record(_ context.Context, event audit.Event) error {
	f.events = append(f.events, event)

	return nil
}

const (
	patientA    = "patientA"
	patientB    = "patientB"
	treatmentID = "treatment1"
	medicationA = "medicationA"
	medicationB = "medicationB"
)

func newService(t *testing.T, authorizer *fakeAuthorizer) (*coursemedication.Service, *fakeRepository) {
	t.Helper()

	svc, repo, _ := newServiceWithAuditor(t, authorizer)

	return svc, repo
}

func newServiceWithAuditor(
	t *testing.T, authorizer *fakeAuthorizer,
) (*coursemedication.Service, *fakeRepository, *fakeAuditor) {
	t.Helper()

	repo := newFakeRepository()
	auditor := &fakeAuditor{}

	treatments := fakeTreatments{byID: map[string]clinical.Treatment{
		treatmentID: {ID: treatmentID, PatientID: patientA, Version: "v1"},
	}}
	medications := fakeMedications{byID: map[string]clinical.Medication{
		medicationA: {ID: medicationA, PatientID: patientA, Dosage: "5mg"},
		medicationB: {ID: medicationB, PatientID: patientB},
	}}

	svc, err := coursemedication.New(repo, treatments, medications, authorizer, auditor)
	require.NoError(t, err)

	return svc, repo, auditor
}

func TestUpsertTwiceYieldsOneRowAndCreatedOnlyOnce(t *testing.T) {
	t.Parallel()

	authorizer := &fakeAuthorizer{}
	svc, repo := newService(t, authorizer)

	item, created, err := svc.Upsert(t.Context(), access.Actor{}, treatmentID, medicationA,
		coursemedication.Patch{}, "v1")
	require.NoError(t, err)
	assert.True(t, created)

	item2, created2, err := svc.Upsert(t.Context(), access.Actor{}, treatmentID, medicationA,
		coursemedication.Patch{}, "v1")
	require.NoError(t, err)
	assert.False(t, created2, "the second upsert of the same pair must not create a second row")
	assert.Equal(t, item.CourseMedication.ID, item2.CourseMedication.ID)

	rows, err := repo.List(t.Context(), treatmentID)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
}

func TestEffectiveFallsBackToTheMedicationAndCarriesSource(t *testing.T) {
	t.Parallel()

	authorizer := &fakeAuthorizer{}
	svc, _ := newService(t, authorizer)

	item, _, err := svc.Upsert(t.Context(), access.Actor{}, treatmentID, medicationA, coursemedication.Patch{}, "v1")
	require.NoError(t, err)

	assert.Equal(t, "5mg", item.Effective.Dosage.Value)
	assert.Equal(t, clinical.SourceMedication, item.Effective.Dosage.Source)

	dosage := "3mg"

	item2, _, err := svc.Upsert(t.Context(), access.Actor{}, treatmentID, medicationA,
		coursemedication.Patch{Dosage: &dosage}, "v1")
	require.NoError(t, err)
	assert.Equal(t, "3mg", item2.Effective.Dosage.Value)
	assert.Equal(t, clinical.SourceCourse, item2.Effective.Dosage.Source)
}

func TestUpsertRefusesAMedicationOfAnotherPatient(t *testing.T) {
	t.Parallel()

	authorizer := &fakeAuthorizer{}
	svc, _ := newService(t, authorizer)

	_, _, err := svc.Upsert(t.Context(), access.Actor{}, treatmentID, medicationB, coursemedication.Patch{}, "v1")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestUpsertAuthorizesBothTheTreatmentAndTheMedicationOnEveryCall(t *testing.T) {
	t.Parallel()

	authorizer := &fakeAuthorizer{}
	svc, _ := newService(t, authorizer)

	_, _, err := svc.Upsert(t.Context(), access.Actor{}, treatmentID, medicationA, coursemedication.Patch{}, "v1")
	require.NoError(t, err)
	assert.Equal(t, []string{patientA, patientA}, authorizer.calls,
		"the treatment's patient and the medication's patient are each authorized, even though they are the same patient")

	authorizer.calls = nil

	require.NoError(t, svc.Delete(t.Context(), access.Actor{}, treatmentID, medicationA, "v1"))
	assert.Equal(t, []string{patientA, patientA}, authorizer.calls)
}

func TestUpsertRefusesWhenTheAuthorizerDeniesThePatient(t *testing.T) {
	t.Parallel()

	authorizer := &fakeAuthorizer{denied: map[string]bool{patientA: true}}
	svc, _ := newService(t, authorizer)

	_, _, err := svc.Upsert(t.Context(), access.Actor{}, treatmentID, medicationA, coursemedication.Patch{}, "v1")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestUpsertRefusesAnUnknownTreatmentOrMedication(t *testing.T) {
	t.Parallel()

	authorizer := &fakeAuthorizer{}
	svc, _ := newService(t, authorizer)

	_, _, err := svc.Upsert(t.Context(), access.Actor{}, "doesNotExist", medicationA, coursemedication.Patch{}, "v1")
	require.ErrorIs(t, err, domain.ErrNotFound)

	_, _, err = svc.Upsert(t.Context(), access.Actor{}, treatmentID, "doesNotExist", coursemedication.Patch{}, "v1")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

// T209, FR-084, FR-085. Attaching, re-attaching and detaching a course
// medication are relationship changes the generic per-kind audit hooks never
// see (treatment_medications is a join, not a kind.Kind), so the service
// must write its own row for each — and that row must carry no dose,
// frequency, timing or any other course-specific value.
func TestUpsertAndDeleteEachProduceOneAuditEventWithNoCourseDetail(t *testing.T) {
	t.Parallel()

	authorizer := &fakeAuthorizer{}
	svc, _, auditor := newServiceWithAuditor(t, authorizer)

	dosage := "500mg twice daily"

	_, created, err := svc.Upsert(t.Context(), access.Actor{UserID: "u1", RequestID: "r1"},
		treatmentID, medicationA, coursemedication.Patch{Dosage: &dosage}, "v1")
	require.NoError(t, err)
	assert.True(t, created)
	require.Len(t, auditor.events, 1)
	assert.Equal(t, audit.ActionCreate, auditor.events[0].Action)

	_, created, err = svc.Upsert(t.Context(), access.Actor{UserID: "u1", RequestID: "r2"},
		treatmentID, medicationA, coursemedication.Patch{Dosage: &dosage}, "v1")
	require.NoError(t, err)
	assert.False(t, created)
	require.Len(t, auditor.events, 2)
	assert.Equal(t, audit.ActionUpdate, auditor.events[1].Action)

	require.NoError(t, svc.Delete(t.Context(), access.Actor{UserID: "u1", RequestID: "r3"}, treatmentID, medicationA, "v1"))
	require.Len(t, auditor.events, 3)
	assert.Equal(t, audit.ActionDelete, auditor.events[2].Action)

	for _, event := range auditor.events {
		assert.Equal(t, audit.TargetKindTreatment, event.TargetKind)
		assert.Equal(t, treatmentID, event.TargetID)
		assert.Equal(t, patientA, event.PatientID)
		assert.NotContains(t, fmt.Sprintf("%+v", event), dosage,
			"the course medication's own dosage must never reach the audit trail")
	}
}
