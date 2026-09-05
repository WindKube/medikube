package facility_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/directory"
	"medikube/internal/service/facility"
	"medikube/internal/service/facility/facilitytest"
	"medikube/internal/store"
	pbfacility "medikube/internal/store/facility"

	// The migrations register themselves from their own init and
	// tests.NewTestApp runs core.AppMigrations against the instance. Without
	// this import every case below would run against PocketBase's stock
	// schema, where the collection this package reads does not exist.
	_ "medikube/internal/store/migrations"
)

const (
	ownerEmail    = "owner@example.test"
	strangerEmail = "stranger@example.test"
)

// harness is one instance, one repository and two accounts — mirroring
// internal/store/medication's own harness so the two implementations are
// exercised the same way.
type harness struct {
	app      *tests.TestApp
	repo     *pbfacility.Repo
	owner    string
	stranger string
}

func newHarness(t *testing.T) harness {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err, "the instance has no cursor key material")

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbfacility.New(app, codec)
	require.NoError(t, err)

	return harness{
		app:      app,
		repo:     repo,
		owner:    seedAccount(t, app, ownerEmail),
		stranger: seedAccount(t, app, strangerEmail),
	}
}

func seedAccount(t *testing.T, app core.App, email string) string {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)

	record := core.NewRecord(users)
	record.SetEmail(email)
	record.SetPassword("correct-horse-battery-staple")
	record.Set("name", "Test Person")
	record.Set("role", "user")
	record.Set("unit_system", "metric")
	record.Set("locale", "en")
	record.Set("date_format", "iso")
	record.Set("theme", "system")

	require.NoError(t, app.Save(record))

	return record.Id
}

func (h harness) draft(owner, name string) directory.Facility {
	return directory.Facility{OwnerID: owner, Kind: directory.FacilityKindPractice, Name: name}
}

func (h harness) create(t *testing.T, entity directory.Facility) directory.Facility {
	t.Helper()

	stored, err := h.repo.Create(t.Context(), entity)
	require.NoError(t, err)

	return stored
}

// T125. The PocketBase repository passes the same suite the in-memory fake
// passes, which is the whole of Principle II: two implementations, one
// contract.
func TestThePocketBaseRepositoryPassesTheSameContractTheFakeDoes(t *testing.T) {
	t.Parallel()

	facilitytest.RunRepositoryContract(t, func(t *testing.T) (facility.Repository, facilitytest.Accounts) {
		t.Helper()

		h := newHarness(t)

		return h.repo, facilitytest.Accounts{Owner: h.owner, Stranger: h.stranger}
	})
}

// TestTwoFacilitiesSharingANameAreBothStoredAndBothListed is FR-035 and
// US5-3, asserted against the real database and not only against the fake:
// there is deliberately no uniqueness constraint on name, and a chain's
// second branch must be offered beside the first.
func TestTwoFacilitiesSharingANameAreBothStoredAndBothListed(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	first := h.draft(h.owner, "Boots")
	first.Street = "1 High Street"

	second := h.draft(h.owner, "Boots")
	second.Street = "2 Station Road"

	firstStored := h.create(t, first)
	secondStored := h.create(t, second)

	require.NotEqual(t, firstStored.ID, secondStored.ID)

	page, err := h.repo.List(t.Context(), h.owner, facility.Query{})
	require.NoError(t, err)

	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}

	assert.ElementsMatch(t, []string{firstStored.ID, secondStored.ID}, ids,
		"both branches of the same chain must be stored and both must appear in the list")
}

// TestASearchNeverReachesPastTheOwner mirrors internal/store/medication's own
// case: the owner scope is a separate conjunct outside the `q` disjunction,
// and a search over the owner's own account id must never leak into another
// account's rows.
func TestASearchNeverReachesPastTheOwner(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	mine := h.create(t, h.draft(h.owner, "Riverside Pharmacy"))

	theirs := h.draft(h.stranger, "Riverside Pharmacy")
	theirsStored, err := h.repo.Create(t.Context(), theirs)
	require.NoError(t, err)

	page, err := h.repo.List(t.Context(), h.owner, facility.Query{Search: "riverside"})
	require.NoError(t, err)

	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}

	assert.Equal(t, []string{mine.ID}, ids)
	assert.NotContains(t, ids, theirsStored.ID,
		"the search reached past the owner scope into another account's records")
}

// TestOwnerAnswersTheAccountEvenForAnotherCaller is the seam the service's
// cross-owner audit stands on: Owner is not scoped by a caller argument at
// all, only by the record's own identity.
func TestOwnerAnswersTheAccountEvenForAnotherCaller(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	stored := h.create(t, h.draft(h.owner, "Owner Lookup Clinic"))

	owner, err := h.repo.Owner(t.Context(), stored.ID)
	require.NoError(t, err)
	assert.Equal(t, h.owner, owner)
}

// TestUsageCountsPractitionersThatReferenceTheFacility. FR-039's Usage:
// a practitioner whose facility is this row is what a delete would silently
// orphan if the reference cascaded rather than being unset.
func TestUsageCountsPractitionersThatReferenceTheFacility(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	stored := h.create(t, h.draft(h.owner, "Referenced Clinic"))

	practitioners, err := h.app.FindCollectionByNameOrId("practitioners")
	require.NoError(t, err)

	record := core.NewRecord(practitioners)
	record.Set("owner", h.owner)
	record.Set("name", "Dr Amara")
	record.Set("facility", stored.ID)
	require.NoError(t, h.app.Save(record))

	usage, err := h.repo.Usage(t.Context(), h.owner, stored.ID)
	require.NoError(t, err)

	assert.Equal(t, 1, usage.Practitioners)
}
