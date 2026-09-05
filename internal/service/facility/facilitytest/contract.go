package facilitytest

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"medikube/internal/domain"
	"medikube/internal/domain/directory"
	"medikube/internal/service/facility"
)

// Accounts are the two owners a contract run writes rows for.
type Accounts struct {
	Owner    string
	Stranger string
}

// Factory builds one empty repository, per test.
type Factory func(t *testing.T) (facility.Repository, Accounts)

// RunRepositoryContract is the contract every facility.Repository passes — the
// in-memory fake here and the PocketBase repository in internal/store,
// against a real instance.
func RunRepositoryContract(t *testing.T, newRepository Factory) {
	t.Helper()

	require.NotNil(t, newRepository, "the contract has no repository to run against")

	suite.Run(t, &repositoryContract{newRepository: newRepository})
}

type repositoryContract struct {
	suite.Suite

	newRepository Factory
	repository    facility.Repository
	accounts      Accounts
}

func (c *repositoryContract) SetupTest() {
	repository, accounts := c.newRepository(c.T())

	c.Require().NotNil(repository, "the factory returned no repository")
	c.Require().NotEmpty(accounts.Owner, "the factory named no owner")
	c.Require().NotEmpty(accounts.Stranger, "the factory named no stranger to be scoped against")
	c.Require().NotEqual(accounts.Owner, accounts.Stranger,
		"the owner and the stranger are the same account, so every scoping assertion below would pass vacuously")

	c.repository, c.accounts = repository, accounts
}

func (c *repositoryContract) ctx() context.Context { return c.T().Context() }

// draft is the minimum a stored facility needs: a name, a kind and an owner.
func (c *repositoryContract) draft(owner, name string) directory.Facility {
	return directory.Facility{
		OwnerID: owner,
		Kind:    directory.FacilityKindPractice,
		Name:    name,
	}
}

func (c *repositoryContract) create(entity directory.Facility) directory.Facility {
	c.T().Helper()

	stored, err := c.repository.Create(c.ctx(), entity)
	c.Require().NoError(err)

	return stored
}

func (c *repositoryContract) list(query facility.Query) domain.Page[directory.Facility] {
	c.T().Helper()

	page, err := c.repository.List(c.ctx(), c.accounts.Owner, query)
	c.Require().NoError(err)

	return page
}

func ids(page domain.Page[directory.Facility]) []string {
	found := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		found = append(found, item.ID)
	}

	return found
}

// TestCreateStoresEveryFieldAndMintsAnIdentity is the round trip. Every field
// data-model §1 declares is written and read back.
func (c *repositoryContract) TestCreateStoresEveryFieldAndMintsAnIdentity() {
	full := directory.Facility{
		OwnerID:      c.accounts.Owner,
		Kind:         directory.FacilityKindPharmacy,
		Name:         "Boots",
		Brand:        "Boots UK",
		Street:       "123 High Street",
		City:         "Leeds",
		Region:       "West Yorkshire",
		PostalCode:   "LS1 1AA",
		Country:      "United Kingdom",
		Phone:        "0113 000 0000",
		Fax:          "0113 000 0001",
		Email:        "branch@example.test",
		Website:      "https://example.test",
		PortalURL:    "https://portal.example.test",
		Hours:        "9am-6pm",
		Open24h:      false,
		DriveThrough: true,
		Services:     "vaccinations",
		Notes:        "ask for the pharmacist",
	}

	stored := c.create(full)

	c.Require().NotEmpty(stored.ID, "a stored facility with no identity cannot be addressed again")
	c.Assert().NotEmpty(stored.Version, "no version means If-Match can never succeed")
	c.Assert().False(stored.CreatedAt.IsZero())
	c.Assert().False(stored.UpdatedAt.IsZero())

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)

	c.Assert().Equal(c.accounts.Owner, read.OwnerID)
	c.Assert().Equal(full.Kind, read.Kind)
	c.Assert().Equal(full.Name, read.Name)
	c.Assert().Equal(full.Brand, read.Brand)
	c.Assert().Equal(full.Street, read.Street)
	c.Assert().Equal(full.City, read.City)
	c.Assert().Equal(full.Region, read.Region)
	c.Assert().Equal(full.PostalCode, read.PostalCode)
	c.Assert().Equal(full.Country, read.Country)
	c.Assert().Equal(full.Phone, read.Phone)
	c.Assert().Equal(full.Fax, read.Fax)
	c.Assert().Equal(full.Email, read.Email)
	c.Assert().Equal(full.Website, read.Website)
	c.Assert().Equal(full.PortalURL, read.PortalURL)
	c.Assert().Equal(full.Hours, read.Hours)
	c.Assert().Equal(full.Open24h, read.Open24h)
	c.Assert().Equal(full.DriveThrough, read.DriveThrough)
	c.Assert().Equal(full.Services, read.Services)
	c.Assert().Equal(full.Notes, read.Notes)
}

// TestCreateAcceptsTwoFacilitiesSharingAName is FR-035 and US5-3: a chain's
// second branch is a second row with its own address, and there is
// deliberately no uniqueness constraint on the name to get in its way.
func (c *repositoryContract) TestCreateAcceptsTwoFacilitiesSharingAName() {
	first := c.draft(c.accounts.Owner, "Boots")
	first.Street = "1 High Street"

	second := c.draft(c.accounts.Owner, "Boots")
	second.Street = "2 Station Road"

	firstStored := c.create(first)
	secondStored := c.create(second)

	c.Assert().NotEqual(firstStored.ID, secondStored.ID)

	page := c.list(facility.Query{})
	c.Assert().ElementsMatch([]string{firstStored.ID, secondStored.ID}, ids(page),
		"both branches of the same chain must appear in the list")
}

func (c *repositoryContract) TestAnAbsentOptionalFieldStaysAbsent() {
	stored := c.create(c.draft(c.accounts.Owner, "Community Pharmacy"))

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)

	c.Assert().Empty(read.Brand)
	c.Assert().Empty(read.Street)
	c.Assert().Empty(read.Email)
	c.Assert().Empty(read.Website)
	c.Assert().Empty(read.Notes)
	c.Assert().False(read.Open24h)
	c.Assert().False(read.DriveThrough)
}

func (c *repositoryContract) TestGetIsScopedToTheOwner() {
	stored := c.create(c.draft(c.accounts.Owner, "St Mary's Practice"))

	_, err := c.repository.Get(c.ctx(), c.accounts.Stranger, stored.ID)
	c.Require().ErrorIs(err, domain.ErrNotFound,
		"another account's facility is answered exactly as one that does not exist")
}

func (c *repositoryContract) TestGetOfAnIdentityThatNeverExistedIsNotFound() {
	_, err := c.repository.Get(c.ctx(), c.accounts.Owner, "nosuchrecord01")

	c.Require().ErrorIs(err, domain.ErrNotFound)
}

func (c *repositoryContract) TestListReturnsOnlyTheOwnersRows() {
	mine := c.create(c.draft(c.accounts.Owner, "Riverside Clinic"))
	theirs := c.create(c.draft(c.accounts.Stranger, "Lakeside Clinic"))

	page := c.list(facility.Query{})

	c.Assert().Equal([]string{mine.ID}, ids(page))
	c.Assert().NotContains(ids(page), theirs.ID)
}

func (c *repositoryContract) TestListNarrowsByKind() {
	practice := c.create(c.draft(c.accounts.Owner, "Elm Street Surgery"))

	pharmacy := c.draft(c.accounts.Owner, "Elm Street Pharmacy")
	pharmacy.Kind = directory.FacilityKindPharmacy
	pharmacyStored := c.create(pharmacy)

	page := c.list(facility.Query{Kind: directory.FacilityKindPractice})
	c.Assert().Equal([]string{practice.ID}, ids(page))

	page = c.list(facility.Query{Kind: directory.FacilityKindPharmacy})
	c.Assert().Equal([]string{pharmacyStored.ID}, ids(page))
}

// TestListSearchesNameAndBrand is FR-036's text match: one substring, two
// columns.
func (c *repositoryContract) TestListSearchesNameAndBrand() {
	byName := c.create(c.draft(c.accounts.Owner, "Riverside Pharmacy"))

	byBrand := c.draft(c.accounts.Owner, "Branch 12")
	byBrand.Brand = "Riverside Group"
	byBrandStored := c.create(byBrand)

	unrelated := c.create(c.draft(c.accounts.Owner, "Unrelated Clinic"))

	page := c.list(facility.Query{Search: "riverside"})

	c.Assert().ElementsMatch([]string{byName.ID, byBrandStored.ID}, ids(page))
	c.Assert().NotContains(ids(page), unrelated.ID)
}

// TestListOrdersByKindThenName is contracts/facilities.md's one published
// ordering.
func (c *repositoryContract) TestListOrdersByKindThenName() {
	hospital := c.draft(c.accounts.Owner, "Zeta Hospital")
	hospital.Kind = directory.FacilityKindHospital
	hospitalStored := c.create(hospital)

	practiceB := c.draft(c.accounts.Owner, "Beta Surgery")
	practiceBStored := c.create(practiceB)

	practiceA := c.draft(c.accounts.Owner, "Alpha Surgery")
	practiceAStored := c.create(practiceA)

	page := c.list(facility.Query{})

	c.Assert().Equal([]string{hospitalStored.ID, practiceAStored.ID, practiceBStored.ID}, ids(page),
		"hospital sorts before practice, and within a kind the name decides")
}

func (c *repositoryContract) TestUpdateReplacesTheStoredValuesWhenTheVersionMatches() {
	stored := c.create(c.draft(c.accounts.Owner, "Old Name"))

	changed := stored
	changed.Name = "New Name"
	changed.City = "Manchester"

	updated, err := c.repository.Update(c.ctx(), changed, stored.Version)
	c.Require().NoError(err)

	c.Assert().Equal("New Name", updated.Name)
	c.Assert().Equal("Manchester", updated.City)

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)

	c.Assert().Equal("New Name", read.Name)
	c.Assert().Equal(updated.Version, read.Version,
		"the version an update returned is the one the next If-Match has to carry")
}

func (c *repositoryContract) TestUpdateRefusesAVersionThatIsNotTheCurrentOne() {
	stored := c.create(c.draft(c.accounts.Owner, "Sterling Clinic"))

	changed := stored
	changed.City = "Bristol"

	_, err := c.repository.Update(c.ctx(), changed, "not-the-version-that-was-read")
	c.Require().ErrorIs(err, domain.ErrVersionMismatch)

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)
	c.Assert().Empty(read.City, "the refused update was applied anyway")
}

func (c *repositoryContract) TestUpdateIsScopedToTheOwner() {
	stored := c.create(c.draft(c.accounts.Owner, "Camden Surgery"))

	hijacked := stored
	hijacked.OwnerID = c.accounts.Stranger
	hijacked.Name = "Taken over"

	_, err := c.repository.Update(c.ctx(), hijacked, stored.Version)
	c.Require().ErrorIs(err, domain.ErrNotFound)

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)
	c.Assert().Equal("Camden Surgery", read.Name)
	c.Assert().Equal(c.accounts.Owner, read.OwnerID, "the row changed hands")
}

func (c *repositoryContract) TestUpdateOfAnIdentityThatNeverExistedIsNotFound() {
	absent := c.draft(c.accounts.Owner, "Nowhere Clinic")
	absent.ID = "nosuchrecord01"

	_, err := c.repository.Update(c.ctx(), absent, "any-version")

	c.Require().ErrorIs(err, domain.ErrNotFound)
}

func (c *repositoryContract) TestDeleteRemovesTheRowWhenTheVersionMatches() {
	stored := c.create(c.draft(c.accounts.Owner, "Closing Down Clinic"))

	c.Require().NoError(c.repository.Delete(c.ctx(), c.accounts.Owner, stored.ID, stored.Version))

	_, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Assert().ErrorIs(err, domain.ErrNotFound)

	page := c.list(facility.Query{})
	c.Assert().NotContains(ids(page), stored.ID)
}

func (c *repositoryContract) TestDeleteRefusesAVersionThatIsNotTheCurrentOne() {
	stored := c.create(c.draft(c.accounts.Owner, "Everton Pharmacy"))

	err := c.repository.Delete(c.ctx(), c.accounts.Owner, stored.ID, "not-the-version-that-was-read")
	c.Require().ErrorIs(err, domain.ErrVersionMismatch)

	_, err = c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Assert().NoError(err, "the refused delete removed the row anyway")
}

func (c *repositoryContract) TestDeleteIsScopedToTheOwner() {
	stored := c.create(c.draft(c.accounts.Owner, "Hillside Clinic"))

	err := c.repository.Delete(c.ctx(), c.accounts.Stranger, stored.ID, stored.Version)
	c.Require().ErrorIs(err, domain.ErrNotFound)

	_, err = c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Assert().NoError(err, "a stranger deleted a row they were told does not exist")
}

func (c *repositoryContract) TestDeleteOfAnIdentityThatNeverExistedIsNotFound() {
	err := c.repository.Delete(c.ctx(), c.accounts.Owner, "nosuchrecord01", "any-version")

	c.Require().ErrorIs(err, domain.ErrNotFound)
}

// TestOwnerAnswersTheAccountAndNotFoundOtherwise is the seam
// authorizeRecord's cross-owner detection stands on: Owner never scopes by
// caller, only by identity.
func (c *repositoryContract) TestOwnerAnswersTheAccountAndNotFoundOtherwise() {
	stored := c.create(c.draft(c.accounts.Owner, "Owner Lookup Clinic"))

	owner, err := c.repository.Owner(c.ctx(), stored.ID)
	c.Require().NoError(err)
	c.Assert().Equal(c.accounts.Owner, owner)

	_, err = c.repository.Owner(c.ctx(), "nosuchrecord01")
	c.Assert().ErrorIs(err, domain.ErrNotFound)
}

func (c *repositoryContract) TestEveryRefusalIsOneOfTheDomainSentinels() {
	_, err := c.repository.Get(c.ctx(), c.accounts.Owner, "nosuchrecord01")

	c.Require().Error(err)
	c.Assert().True(
		errors.Is(err, domain.ErrNotFound) ||
			errors.Is(err, domain.ErrVersionMismatch) ||
			errors.Is(err, domain.ErrConflict),
		"a refusal that matches no sentinel reaches the client as an internal error")
}
