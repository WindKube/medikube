package practitionertest

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"medikube/internal/domain"
	"medikube/internal/domain/directory"
	"medikube/internal/service/practitioner"
)

// Accounts are the two owners a contract run writes rows for, plus a way to
// mint a facility owned by one of them.
//
// SeedFacility is a callback rather than a fixed id because a facility is a
// row in another collection this package has no repository interface for: the
// fake mints it in its own facilities map and the PocketBase implementation
// writes an actual facilities record, and the contract has to work either way.
type Accounts struct {
	Owner    string
	Stranger string

	// SeedFacility mints a facility owned by ownerID and returns its id.
	SeedFacility func(t *testing.T, ownerID string) string
}

// Factory builds one empty repository, per test.
type Factory func(t *testing.T) (practitioner.Repository, Accounts)

// RunRepositoryContract is the contract every practitioner.Repository passes.
func RunRepositoryContract(t *testing.T, newRepository Factory) {
	t.Helper()

	require.NotNil(t, newRepository, "the contract has no repository to run against")

	suite.Run(t, &repositoryContract{newRepository: newRepository})
}

type repositoryContract struct {
	suite.Suite

	newRepository Factory
	repository    practitioner.Repository
	accounts      Accounts
}

func (c *repositoryContract) SetupTest() {
	repository, accounts := c.newRepository(c.T())

	c.Require().NotNil(repository, "the factory returned no repository")
	c.Require().NotEmpty(accounts.Owner, "the factory named no owner")
	c.Require().NotEmpty(accounts.Stranger, "the factory named no stranger to be scoped against")
	c.Require().NotEqual(accounts.Owner, accounts.Stranger,
		"the owner and the stranger are the same account, so every scoping assertion below would pass vacuously")
	c.Require().NotNil(accounts.SeedFacility, "the factory named no way to seed a facility")

	c.repository, c.accounts = repository, accounts
}

func (c *repositoryContract) ctx() context.Context { return c.T().Context() }

func (c *repositoryContract) draft(owner, name string) directory.Practitioner {
	return directory.Practitioner{OwnerID: owner, Name: name}
}

func (c *repositoryContract) create(entity directory.Practitioner) directory.Practitioner {
	c.T().Helper()

	stored, err := c.repository.Create(c.ctx(), entity)
	c.Require().NoError(err)

	return stored
}

func (c *repositoryContract) list(query practitioner.Query) domain.Page[directory.Practitioner] {
	c.T().Helper()

	page, err := c.repository.List(c.ctx(), c.accounts.Owner, query)
	c.Require().NoError(err)

	return page
}

func ids(page domain.Page[directory.Practitioner]) []string {
	found := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		found = append(found, item.ID)
	}

	return found
}

// TestCreateStoresEveryFieldAndMintsAnIdentity is the round trip.
func (c *repositoryContract) TestCreateStoresEveryFieldAndMintsAnIdentity() {
	facilityID := c.accounts.SeedFacility(c.T(), c.accounts.Owner)

	full := directory.Practitioner{
		OwnerID:    c.accounts.Owner,
		Name:       "Dr. Amara Okonkwo",
		Specialty:  directory.SpecialtyCardiology,
		FacilityID: facilityID,
		Phone:      "+1 555 0100",
		Email:      "amara@example.test",
		Website:    "https://example.test",
		Notes:      "prefers morning appointments",
	}

	stored := c.create(full)

	c.Require().NotEmpty(stored.ID, "a stored practitioner with no identity cannot be addressed again")
	c.Assert().NotEmpty(stored.Version, "no version means If-Match can never succeed")
	c.Assert().False(stored.CreatedAt.IsZero())
	c.Assert().False(stored.UpdatedAt.IsZero())

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)

	c.Assert().Equal(stored.ID, read.ID)
	c.Assert().Equal(stored.Version, read.Version)
	c.Assert().Equal(c.accounts.Owner, read.OwnerID)
	c.Assert().Equal(full.Name, read.Name)
	c.Assert().Equal(full.Specialty, read.Specialty)
	c.Assert().Equal(full.FacilityID, read.FacilityID)
	c.Assert().Equal(full.Phone, read.Phone)
	c.Assert().Equal(full.Email, read.Email)
	c.Assert().Equal(full.Website, read.Website)
	c.Assert().Equal(full.Notes, read.Notes)
}

// TestAnAbsentOptionalFieldStaysAbsent.
func (c *repositoryContract) TestAnAbsentOptionalFieldStaysAbsent() {
	stored := c.create(c.draft(c.accounts.Owner, "Dr. No Specialty"))

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)

	c.Assert().Empty(read.Specialty)
	c.Assert().Empty(read.FacilityID)
	c.Assert().Empty(read.Phone)
	c.Assert().Empty(read.Email)
	c.Assert().Empty(read.Website)
	c.Assert().Empty(read.Notes)
}

// TestGetIsScopedToTheOwner is FR-037 at the storage layer.
func (c *repositoryContract) TestGetIsScopedToTheOwner() {
	stored := c.create(c.draft(c.accounts.Owner, "Dr. Metformin"))

	_, err := c.repository.Get(c.ctx(), c.accounts.Stranger, stored.ID)
	c.Require().ErrorIs(err, domain.ErrNotFound,
		"another account's practitioner is answered exactly as one that does not exist")
}

func (c *repositoryContract) TestGetOfAnIdentityThatNeverExistedIsNotFound() {
	_, err := c.repository.Get(c.ctx(), c.accounts.Owner, "nosuchrecord01")

	c.Require().ErrorIs(err, domain.ErrNotFound)
}

// TestListReturnsOnlyTheOwnersRows.
func (c *repositoryContract) TestListReturnsOnlyTheOwnersRows() {
	mine := c.create(c.draft(c.accounts.Owner, "Dr. Atorvastatin"))
	theirs := c.create(c.draft(c.accounts.Stranger, "Dr. Bisoprolol"))

	page := c.list(practitioner.Query{})

	c.Assert().Equal([]string{mine.ID}, ids(page))
	c.Assert().NotContains(ids(page), theirs.ID)
}

// TestListNarrowsBySpecialtyAndFacility is FR-039's picker narrowing.
func (c *repositoryContract) TestListNarrowsBySpecialtyAndFacility() {
	facilityID := c.accounts.SeedFacility(c.T(), c.accounts.Owner)

	cardio := c.draft(c.accounts.Owner, "Dr. Cardio")
	cardio.Specialty = directory.SpecialtyCardiology
	cardio.FacilityID = facilityID
	cardioID := c.create(cardio).ID

	derm := c.draft(c.accounts.Owner, "Dr. Derm")
	derm.Specialty = directory.SpecialtyDermatology
	dermID := c.create(derm).ID

	bySpecialty := c.list(practitioner.Query{Specialty: directory.SpecialtyCardiology})
	c.Assert().Equal([]string{cardioID}, ids(bySpecialty))

	byFacility := c.list(practitioner.Query{FacilityID: facilityID})
	c.Assert().Equal([]string{cardioID}, ids(byFacility))

	c.Assert().Contains(ids(c.list(practitioner.Query{})), dermID)
}

// TestListSearchesNameCaseInsensitively is FR-039's type-ahead.
func (c *repositoryContract) TestListSearchesNameCaseInsensitively() {
	match := c.create(c.draft(c.accounts.Owner, "Dr. Salbutamol"))
	unrelated := c.create(c.draft(c.accounts.Owner, "Dr. Omeprazole"))

	page := c.list(practitioner.Query{Search: "SALBUTA"})

	c.Assert().Equal([]string{match.ID}, ids(page))
	c.Assert().NotContains(ids(page), unrelated.ID)
}

// TestListSortsByNameInBothDirections.
func (c *repositoryContract) TestListSortsByNameInBothDirections() {
	lower := c.create(c.draft(c.accounts.Owner, "amoxicillin"))
	upper := c.create(c.draft(c.accounts.Owner, "Betahistine"))

	ascending := c.list(practitioner.Query{Sort: []domain.SortKey{{Field: practitioner.FieldName}}})
	c.Assert().Equal([]string{lower.ID, upper.ID}, ids(ascending))

	descending := c.list(practitioner.Query{Sort: []domain.SortKey{{Field: practitioner.FieldName, Desc: true}}})
	c.Assert().Equal([]string{upper.ID, lower.ID}, ids(descending))
}

// TestCountIsOnlyProducedWhenAsked.
func (c *repositoryContract) TestCountIsOnlyProducedWhenAsked() {
	for _, name := range []string{"Dr. A", "Dr. B", "Dr. C"} {
		c.create(c.draft(c.accounts.Owner, name))
	}

	c.create(c.draft(c.accounts.Stranger, "Dr. D"))

	silent := c.list(practitioner.Query{Limit: 2})
	c.Assert().Nil(silent.Total)

	counted := c.list(practitioner.Query{Limit: 2, Count: true})
	c.Require().NotNil(counted.Total)
	c.Assert().Equal(3, *counted.Total)
}

// TestListPagesWithACursorAndNeverRepeatsOrSkips is FR-022's traversal
// property, reused here for the directory's own page.
func (c *repositoryContract) TestListPagesWithACursorAndNeverRepeatsOrSkips() {
	names := []string{"Aspirin", "Bisacodyl", "Cetirizine", "Diazepam", "Enoxaparin"}

	seeded := make([]string, 0, len(names))
	for _, name := range names {
		seeded = append(seeded, c.create(c.draft(c.accounts.Owner, name)).ID)
	}

	query := practitioner.Query{Sort: []domain.SortKey{{Field: practitioner.FieldName}}, Limit: 2}

	var seen []string

	for page := 1; ; page++ {
		c.Require().LessOrEqual(page, len(names)+2, "the traversal is not terminating")

		got := c.list(query)
		seen = append(seen, ids(got)...)

		if got.NextCursor == nil {
			break
		}

		query.Cursor = *got.NextCursor
	}

	c.Assert().ElementsMatch(seeded, seen)
}

// TestCreateRefusesTheSameNameAndSpecialtyTwice is FR-038, including the
// "no specialty at all" case research D-25 depends on.
func (c *repositoryContract) TestCreateRefusesTheSameNameAndSpecialtyTwice() {
	cases := []struct {
		name      string
		specialty directory.Specialty
	}{
		{"with a specialty", directory.SpecialtyCardiology},
		{"with no specialty at all", ""},
	}

	for _, testCase := range cases {
		c.Run(testCase.name, func() {
			first := c.draft(c.accounts.Owner, "Dr. Duplicate")
			first.Specialty = testCase.specialty
			c.create(first)

			second := c.draft(c.accounts.Owner, "dr. duplicate")
			second.Specialty = testCase.specialty

			_, err := c.repository.Create(c.ctx(), second)
			c.Assert().ErrorIs(err, domain.ErrConflict)
		})
	}
}

// TestCreateAllowsTheSameNameUnderADifferentSpecialty.
func (c *repositoryContract) TestCreateAllowsTheSameNameUnderADifferentSpecialty() {
	first := c.draft(c.accounts.Owner, "Dr. Repeatable")
	first.Specialty = directory.SpecialtyCardiology
	c.create(first)

	second := c.draft(c.accounts.Owner, "Dr. Repeatable")
	second.Specialty = directory.SpecialtyDermatology

	_, err := c.repository.Create(c.ctx(), second)
	c.Assert().NoError(err)
}

// TestCreateAllowsTheSameNameUnderAnotherOwner.
func (c *repositoryContract) TestCreateAllowsTheSameNameUnderAnotherOwner() {
	c.create(c.draft(c.accounts.Owner, "Dr. Shared Name"))

	_, err := c.repository.Create(c.ctx(), c.draft(c.accounts.Stranger, "Dr. Shared Name"))
	c.Assert().NoError(err)
}

// TestCreateRefusesAFacilityTheOwnerDoesNotHold is FR-042.
func (c *repositoryContract) TestCreateRefusesAFacilityTheOwnerDoesNotHold() {
	foreign := c.accounts.SeedFacility(c.T(), c.accounts.Stranger)

	draft := c.draft(c.accounts.Owner, "Dr. Wrong Facility")
	draft.FacilityID = foreign

	_, err := c.repository.Create(c.ctx(), draft)
	c.Assert().ErrorIs(err, domain.ErrNotFound)
}

// TestUpdateReplacesTheStoredValuesWhenTheVersionMatches.
func (c *repositoryContract) TestUpdateReplacesTheStoredValuesWhenTheVersionMatches() {
	stored := c.create(c.draft(c.accounts.Owner, "Dr. Rifampicin"))

	changed := stored
	changed.Phone = "+1 555 0199"

	updated, err := c.repository.Update(c.ctx(), changed, stored.Version)
	c.Require().NoError(err)
	c.Assert().Equal("+1 555 0199", updated.Phone)

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)
	c.Assert().Equal("+1 555 0199", read.Phone)
	c.Assert().Equal(updated.Version, read.Version)
}

func (c *repositoryContract) TestUpdateRefusesAVersionThatIsNotTheCurrentOne() {
	stored := c.create(c.draft(c.accounts.Owner, "Dr. Sertraline"))

	changed := stored
	changed.Phone = "+1 555 0100"

	_, err := c.repository.Update(c.ctx(), changed, "not-the-version-that-was-read")
	c.Require().ErrorIs(err, domain.ErrVersionMismatch)

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)
	c.Assert().Empty(read.Phone, "the refused update was applied anyway")
}

func (c *repositoryContract) TestUpdateIsScopedToTheOwner() {
	stored := c.create(c.draft(c.accounts.Owner, "Dr. Tamsulosin"))

	hijacked := stored
	hijacked.OwnerID = c.accounts.Stranger
	hijacked.Name = "Taken over"

	_, err := c.repository.Update(c.ctx(), hijacked, stored.Version)
	c.Require().ErrorIs(err, domain.ErrNotFound)

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)
	c.Assert().Equal("Dr. Tamsulosin", read.Name)
}

// TestUpdateRefusesACollisionWithAnotherRow is FR-038 on the write that would
// create one.
func (c *repositoryContract) TestUpdateRefusesACollisionWithAnotherRow() {
	c.create(c.draft(c.accounts.Owner, "Dr. Existing"))
	stored := c.create(c.draft(c.accounts.Owner, "Dr. Movable"))

	changed := stored
	changed.Name = "Dr. Existing"

	_, err := c.repository.Update(c.ctx(), changed, stored.Version)
	c.Assert().ErrorIs(err, domain.ErrConflict)
}

// TestDeleteRemovesTheRowWhenTheVersionMatches.
func (c *repositoryContract) TestDeleteRemovesTheRowWhenTheVersionMatches() {
	stored := c.create(c.draft(c.accounts.Owner, "Dr. Valsartan"))

	c.Require().NoError(c.repository.Delete(c.ctx(), c.accounts.Owner, stored.ID, stored.Version))

	_, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Assert().ErrorIs(err, domain.ErrNotFound)
}

func (c *repositoryContract) TestDeleteRefusesAVersionThatIsNotTheCurrentOne() {
	stored := c.create(c.draft(c.accounts.Owner, "Dr. Zopiclone"))

	err := c.repository.Delete(c.ctx(), c.accounts.Owner, stored.ID, "not-the-version-that-was-read")
	c.Require().ErrorIs(err, domain.ErrVersionMismatch)

	_, err = c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Assert().NoError(err, "the refused delete removed the row anyway")
}

func (c *repositoryContract) TestDeleteIsScopedToTheOwner() {
	stored := c.create(c.draft(c.accounts.Owner, "Dr. Xylometazoline"))

	err := c.repository.Delete(c.ctx(), c.accounts.Stranger, stored.ID, stored.Version)
	c.Require().ErrorIs(err, domain.ErrNotFound)

	_, err = c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Assert().NoError(err, "a stranger deleted a row they were told does not exist")
}

func (c *repositoryContract) TestDeleteOfAnIdentityThatNeverExistedIsNotFound() {
	err := c.repository.Delete(c.ctx(), c.accounts.Owner, "nosuchrecord01", "any-version")

	c.Require().ErrorIs(err, domain.ErrNotFound)
}

// TestOwnerAnswersTheAccountOrNotFound is the seam authorizeRecord relies on.
func (c *repositoryContract) TestOwnerAnswersTheAccountOrNotFound() {
	stored := c.create(c.draft(c.accounts.Owner, "Dr. Owned"))

	owner, err := c.repository.Owner(c.ctx(), stored.ID)
	c.Require().NoError(err)
	c.Assert().Equal(c.accounts.Owner, owner)

	_, err = c.repository.Owner(c.ctx(), "nosuchrecord01")
	c.Assert().ErrorIs(err, domain.ErrNotFound)
}

// TestUsageAnswersZeroForAFreshRow. Both implementations agree on the case
// nothing references the practitioner yet; the referenced case is exercised
// where the referencing collections actually exist (T126).
func (c *repositoryContract) TestUsageAnswersZeroForAFreshRow() {
	stored := c.create(c.draft(c.accounts.Owner, "Dr. Unused"))

	usage, err := c.repository.Usage(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)
	c.Assert().Zero(usage.Patients)
	c.Assert().Zero(usage.Records)
}

// TestEveryRefusalIsOneOfTheDomainSentinels.
func (c *repositoryContract) TestEveryRefusalIsOneOfTheDomainSentinels() {
	_, err := c.repository.Get(c.ctx(), c.accounts.Owner, "nosuchrecord01")

	c.Require().Error(err)
	c.Assert().True(
		errors.Is(err, domain.ErrNotFound) ||
			errors.Is(err, domain.ErrVersionMismatch) ||
			errors.Is(err, domain.ErrConflict),
		"a refusal that matches no sentinel reaches the client as an internal error")
}
