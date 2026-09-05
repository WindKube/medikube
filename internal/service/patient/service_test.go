package patient_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/person"
	"medikube/internal/service/patient"
	"medikube/internal/service/patient/patienttest"
)

// The fake Repository and PhotoStore are held to the same Liskov contract as
// internal/store/patient's PocketBase adapter (Principle I).
func TestFakeRepositorySatisfiesTheContract(t *testing.T) {
	t.Parallel()

	patienttest.RepositoryContract(t, func(t *testing.T) patient.Repository {
		t.Helper()

		return patienttest.NewRepository()
	})
}

func TestFakePhotoStoreSatisfiesTheContract(t *testing.T) {
	t.Parallel()

	patienttest.PhotoStoreContract(t, func(t *testing.T) patient.PhotoStore {
		t.Helper()

		return patienttest.NewPhotoStore(15<<20, []string{"image/jpeg", "image/png", "image/webp"})
	})
}

func newService(t *testing.T) (*patient.Service, *patienttest.Repository, *patienttest.Auditor) {
	t.Helper()

	repo := patienttest.NewRepository()
	auditor := patienttest.NewAuditor()
	authorizer := patienttest.NewAuthorizer(repo, auditor)
	photos := patienttest.NewPhotoStore(15<<20, []string{"image/jpeg", "image/png", "image/webp"})

	svc, err := patient.New(repo, photos, authorizer)
	require.NoError(t, err)

	return svc, repo, auditor
}

func owner() access.Actor {
	return access.Actor{UserID: patienttest.OwnerID, RequestID: "req0000000001"}
}

func stranger() access.Actor {
	return access.Actor{UserID: patienttest.StrangerID, RequestID: "req0000000002"}
}

// US1-2: a valid creation is saved and owned by the actor, never by anything
// a request could name.
func TestCreateSavesAndOwnsThePatient(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), person.Patient{
		FirstName: "Amara", LastName: "Okonkwo", BirthDate: mustDate(t, "1988-04-12"),
		OwnerID: "someone-else", IsSelfRecord: true,
	})

	require.NoError(t, err)
	assert.Equal(t, patienttest.OwnerID, created.OwnerID, "OwnerID must come from the actor, never the draft")
	assert.False(t, created.IsSelfRecord, "IsSelfRecord must never be settable through the ordinary Create path")
}

// US1-3: four simultaneous faults return four fields[] entries in one
// *domain.ValidationError.
func TestCreateReportsEveryInvalidFieldAtOnce(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	future := mustDate(t, "2099-01-01")

	_, err := svc.Create(t.Context(), owner(), person.Patient{
		FirstName: "", LastName: "", BirthDate: future, HeightCM: 1000,
	})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Len(t, invalid.Fields, 4)
}

// US1-4: the second self-record for one owner is refused 409, and never
// through the ordinary Create/Update paths since neither can set
// IsSelfRecord at all — this is CreateSelfRecord's own rule.
func TestASecondSelfRecordIsRefusedWithConflict(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	_, err := svc.CreateSelfRecord(t.Context(), patienttest.OwnerID, "Amara Okonkwo")
	require.NoError(t, err)

	_, err = svc.CreateSelfRecord(t.Context(), patienttest.OwnerID, "Amara Okonkwo")
	assert.ErrorIs(t, err, domain.ErrConflict)
}

// research D-10: the display name splits on the last space, and the birth
// date is never fabricated.
func TestCreateSelfRecordSplitsTheDisplayNameAndFabricatesNothing(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.CreateSelfRecord(t.Context(), patienttest.OwnerID, "Amara Chidinma Okonkwo")
	require.NoError(t, err)

	assert.Equal(t, "Amara Chidinma", created.FirstName)
	assert.Equal(t, "Okonkwo", created.LastName)
	assert.True(t, created.BirthDate.IsZero(), "a self-record's birth date must never be invented")
	assert.True(t, created.IsSelfRecord)
	assert.Equal(t, person.RelationshipSelf, created.RelationshipToOwner)

	t.Run("a name with no space is entirely the first name", func(t *testing.T) {
		t.Parallel()

		svc, _, _ := newService(t)

		created, err := svc.CreateSelfRecord(t.Context(), patienttest.OwnerID, "Cher")
		require.NoError(t, err)
		assert.Equal(t, "Cher", created.FirstName)
		assert.Empty(t, created.LastName)
	})
}

// US1-6: absent details stay absent. A patient created with only the three
// required fields carries zero values for everything else, and those zero
// values are what person.Patient.Validate() treats as "not set" rather than
// as a recorded zero.
func TestAbsentDetailsStayAbsent(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), person.Patient{
		FirstName: "Chiamaka", LastName: "Okonkwo", BirthDate: mustDate(t, "2015-09-03"),
	})

	require.NoError(t, err)
	assert.Empty(t, created.Sex)
	assert.Empty(t, created.BloodType)
	assert.Zero(t, created.HeightCM)
	assert.Zero(t, created.WeightKG)
	assert.Empty(t, created.Address)
}

// US1-7: a stale save is refused. The version check is the repository's own
// (RunOwnershipMatrix's HTTP-level 412 is asserted at internal/web/api), and
// this asserts the service does not swallow it.
func TestUpdateRefusesAStaleVersion(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), person.Patient{
		FirstName: "Amara", LastName: "Okonkwo", BirthDate: mustDate(t, "1988-04-12"),
	})
	require.NoError(t, err)

	last := "Adeyemi"

	_, err = svc.Update(t.Context(), owner(), created.ID, "not-the-real-version", patient.Patch{LastName: &last})
	assert.ErrorIs(t, err, domain.ErrVersionMismatch)
}

func TestUpdateAppliesOnlyTheSuppliedFields(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), person.Patient{
		FirstName: "Amara", LastName: "Okonkwo", BirthDate: mustDate(t, "1988-04-12"), Address: "1 Lekki Rd",
	})
	require.NoError(t, err)

	last := "Adeyemi"

	updated, err := svc.Update(t.Context(), owner(), created.ID, created.Version, patient.Patch{LastName: &last})
	require.NoError(t, err)
	assert.Equal(t, "Adeyemi", updated.LastName)
	assert.Equal(t, "Amara", updated.FirstName, "a field not in the patch is untouched")
	assert.Equal(t, "1 Lekki Rd", updated.Address)
}

// FR-042: a stranger's Get is domain.ErrNotFound, never domain.ErrForbidden,
// and it is audited exactly once.
func TestGetRefusesAStrangerAsNotFoundAndAudits(t *testing.T) {
	t.Parallel()

	svc, _, auditor := newService(t)

	created, err := svc.Create(t.Context(), owner(), person.Patient{
		FirstName: "Amara", LastName: "Okonkwo", BirthDate: mustDate(t, "1988-04-12"),
	})
	require.NoError(t, err)

	_, err = svc.Get(t.Context(), stranger(), created.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	require.NotErrorIs(t, err, domain.ErrForbidden)

	events := auditor.Events()
	require.Len(t, events, 1)
	assert.Equal(t, "access_denied", string(events[0].Action))
}

func TestGetRefusesAnAnonymousActorAsUnauthenticated(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), person.Patient{
		FirstName: "Amara", LastName: "Okonkwo", BirthDate: mustDate(t, "1988-04-12"),
	})
	require.NoError(t, err)

	_, err = svc.Get(t.Context(), access.Anonymous("req0000000003"), created.ID)
	assert.ErrorIs(t, err, domain.ErrUnauthenticated)
}

func TestListIsScopedToTheActorsOwnPatients(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	_, err := svc.Create(t.Context(), owner(), person.Patient{FirstName: "Amara", LastName: "Okonkwo", BirthDate: mustDate(t, "1988-04-12")})
	require.NoError(t, err)

	_, err = svc.Create(t.Context(), stranger(), person.Patient{FirstName: "Boris", LastName: "Novak", BirthDate: mustDate(t, "1990-07-22")})
	require.NoError(t, err)

	page, err := svc.List(t.Context(), owner(), patient.Query{})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Amara", page.Items[0].FirstName)
}

func mustDate(t *testing.T, text string) domain.Date {
	t.Helper()

	date, err := domain.ParseDate(text)
	require.NoError(t, err)

	return date
}
