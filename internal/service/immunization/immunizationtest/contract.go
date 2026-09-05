package immunizationtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/immunization"
)

// Accounts mirrors medicationtest.Accounts.
type Accounts struct {
	Patient         string
	StrangerPatient string
}

type Factory func(t *testing.T) (immunization.Repository, Accounts)

// RunRepositoryContract is the contract every immunization.Repository passes,
// mirroring medicationtest.RunRepositoryContract's shape and rationale.
func RunRepositoryContract(t *testing.T, newRepository Factory) {
	t.Helper()

	require.NotNil(t, newRepository, "the contract has no repository to run against")

	suite.Run(t, &repositoryContract{newRepository: newRepository})
}

type repositoryContract struct {
	suite.Suite

	newRepository Factory
	repository    immunization.Repository
	accounts      Accounts
}

func (c *repositoryContract) SetupTest() {
	repository, accounts := c.newRepository(c.T())

	c.Require().NotNil(repository)
	c.Require().NotEmpty(accounts.Patient)
	c.Require().NotEmpty(accounts.StrangerPatient)
	c.Require().NotEqual(accounts.Patient, accounts.StrangerPatient)

	c.repository, c.accounts = repository, accounts
}

func (c *repositoryContract) ctx() context.Context { return c.T().Context() }

// draft is the least a stored immunization needs: administered_on is required
// (FR-038, data-model §4.8), unlike medication's optional started_on, so there
// is no "undated" fixture for this kind and no null-primary-date sort case
// (recordstest.RepositoryContractOptions.NullPrimaryDateSkip documents why).
func (c *repositoryContract) draft(patientID, vaccine string) clinical.Immunization {
	administeredOn, err := domain.NewDate(2026, time.January, 1)
	c.Require().NoError(err)

	return clinical.Immunization{PatientID: patientID, VaccineName: vaccine, AdministeredOn: administeredOn}
}

func (c *repositoryContract) create(entity clinical.Immunization) clinical.Immunization {
	c.T().Helper()

	stored, err := c.repository.Create(c.ctx(), entity)
	c.Require().NoError(err)

	return stored
}

func (c *repositoryContract) list(query immunization.Query) domain.Page[clinical.Immunization] {
	c.T().Helper()

	page, err := c.repository.List(c.ctx(), c.accounts.Patient, query)
	c.Require().NoError(err)

	return page
}

func ids(page domain.Page[clinical.Immunization]) []string {
	found := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		found = append(found, item.ID)
	}

	return found
}

func (c *repositoryContract) TestCreateStoresEveryFieldAndMintsAnIdentity() {
	administeredOn, err := domain.NewDate(2026, time.March, 1)
	c.Require().NoError(err)

	expiresOn, err := domain.NewDate(2027, time.March, 1)
	c.Require().NoError(err)

	dose := 2

	full := clinical.Immunization{
		PatientID:      c.accounts.Patient,
		VaccineName:    "Influenza",
		TradeName:      "Fluvirin",
		AdministeredOn: administeredOn,
		DoseNumber:     &dose,
		LotNumber:      "AB1234",
		Manufacturer:   "Acme Biologics",
		Site:           clinical.ImmunizationSiteLeftArm,
		Route:          clinical.ImmunizationRouteIntramuscular,
		ExpiresOn:      expiresOn,
	}

	stored := c.create(full)

	c.Require().NotEmpty(stored.ID)
	c.Assert().NotEmpty(stored.Version)
	c.Assert().False(stored.CreatedAt.IsZero())
	c.Assert().False(stored.UpdatedAt.IsZero())

	read, err := c.repository.Get(c.ctx(), stored.ID)
	c.Require().NoError(err)

	c.Assert().Equal(c.accounts.Patient, read.PatientID)
	c.Assert().Equal(full.VaccineName, read.VaccineName)
	c.Assert().Equal(full.TradeName, read.TradeName)
	c.Assert().Equal(full.AdministeredOn, read.AdministeredOn)
	c.Require().NotNil(read.DoseNumber)
	c.Assert().Equal(*full.DoseNumber, *read.DoseNumber)
	c.Assert().Equal(full.LotNumber, read.LotNumber)
	c.Assert().Equal(full.Manufacturer, read.Manufacturer)
	c.Assert().Equal(full.Site, read.Site)
	c.Assert().Equal(full.Route, read.Route)
	c.Assert().Equal(full.ExpiresOn, read.ExpiresOn)
}

func (c *repositoryContract) TestAnAbsentOptionalFieldStaysAbsent() {
	stored := c.create(c.draft(c.accounts.Patient, "Tetanus"))

	read, err := c.repository.Get(c.ctx(), stored.ID)
	c.Require().NoError(err)

	c.Assert().Nil(read.DoseNumber)
	c.Assert().Empty(read.LotNumber)
}

func (c *repositoryContract) TestGetOfAnIdentityThatNeverExistedIsNotFound() {
	_, err := c.repository.Get(c.ctx(), "nosuchrecord01")

	c.Require().ErrorIs(err, domain.ErrNotFound)
}

func (c *repositoryContract) TestListReturnsOnlyThePatientsRows() {
	mine := c.create(c.draft(c.accounts.Patient, "MMR"))
	theirs := c.create(c.draft(c.accounts.StrangerPatient, "Varicella"))

	page := c.list(immunization.Query{})

	c.Assert().Equal([]string{mine.ID}, ids(page))
	c.Assert().NotContains(ids(page), theirs.ID)
}

func (c *repositoryContract) TestListSearchesVaccineAndTradeName() {
	byName := c.create(c.draft(c.accounts.Patient, "Hepatitis B"))

	alternative := c.draft(c.accounts.Patient, "HBV")
	alternative.TradeName = "Hepatitis B booster"
	byTrade := c.create(alternative)

	unrelated := c.create(c.draft(c.accounts.Patient, "Polio"))

	page := c.list(immunization.Query{Search: "hepatitis"})

	c.Assert().ElementsMatch([]string{byName.ID, byTrade.ID}, ids(page))
	c.Assert().NotContains(ids(page), unrelated.ID)
}

func (c *repositoryContract) TestListSortsByVaccineNameInBothDirections() {
	lower := c.create(c.draft(c.accounts.Patient, "bcg"))
	upper := c.create(c.draft(c.accounts.Patient, "Cholera"))

	ascending := c.list(immunization.Query{Sort: []domain.SortKey{{Field: immunization.FieldVaccineName}}})
	c.Assert().Equal([]string{lower.ID, upper.ID}, ids(ascending))

	descending := c.list(immunization.Query{Sort: []domain.SortKey{{Field: immunization.FieldVaccineName, Desc: true}}})
	c.Assert().Equal([]string{upper.ID, lower.ID}, ids(descending))
}

func (c *repositoryContract) TestListSortsByAdministeredOnInBothDirections() {
	earlier, err := domain.NewDate(2026, time.January, 5)
	c.Require().NoError(err)

	later, err := domain.NewDate(2026, time.June, 9)
	c.Require().NoError(err)

	early := c.draft(c.accounts.Patient, "DTaP")
	early.AdministeredOn = earlier
	earlyID := c.create(early).ID

	late := c.draft(c.accounts.Patient, "HPV")
	late.AdministeredOn = later
	lateID := c.create(late).ID

	descending := c.list(immunization.Query{Sort: []domain.SortKey{{Field: immunization.FieldAdministeredOn, Desc: true}}})
	c.Assert().Equal([]string{lateID, earlyID}, ids(descending))

	ascending := c.list(immunization.Query{Sort: []domain.SortKey{{Field: immunization.FieldAdministeredOn}}})
	c.Assert().Equal([]string{earlyID, lateID}, ids(ascending))
}

func (c *repositoryContract) TestListPagesWithACursorAndNeverRepeatsOrSkips() {
	names := []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon"}

	throughout := make([]string, 0, len(names))
	for _, name := range names {
		throughout = append(throughout, c.create(c.draft(c.accounts.Patient, name)).ID)
	}

	query := immunization.Query{Sort: []domain.SortKey{{Field: immunization.FieldVaccineName}}, Limit: 2}

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

	for _, id := range throughout {
		c.Assert().Contains(seen, id)
	}
}

func (c *repositoryContract) TestCountIsOnlyProducedWhenAsked() {
	for _, name := range []string{"One", "Two", "Three"} {
		c.create(c.draft(c.accounts.Patient, name))
	}

	silent := c.list(immunization.Query{Limit: 2})
	c.Assert().Nil(silent.Total)

	counted := c.list(immunization.Query{Limit: 2, Count: true})
	c.Require().NotNil(counted.Total)
	c.Assert().Equal(3, *counted.Total)
}

func (c *repositoryContract) TestUpdateReplacesTheStoredValuesWhenTheVersionMatches() {
	stored := c.create(c.draft(c.accounts.Patient, "Yellow fever"))

	changed := stored
	changed.LotNumber = "ZZ9999"

	updated, err := c.repository.Update(c.ctx(), changed, stored.Version)
	c.Require().NoError(err)
	c.Assert().Equal("ZZ9999", updated.LotNumber)

	read, err := c.repository.Get(c.ctx(), stored.ID)
	c.Require().NoError(err)
	c.Assert().Equal("ZZ9999", read.LotNumber)
	c.Assert().Equal(updated.Version, read.Version)
}

func (c *repositoryContract) TestUpdateRefusesAVersionThatIsNotTheCurrentOne() {
	stored := c.create(c.draft(c.accounts.Patient, "Shingles"))

	changed := stored
	changed.LotNumber = "X"

	_, err := c.repository.Update(c.ctx(), changed, "not-the-version-that-was-read")
	c.Require().ErrorIs(err, domain.ErrVersionMismatch)

	read, err := c.repository.Get(c.ctx(), stored.ID)
	c.Require().NoError(err)
	c.Assert().Empty(read.LotNumber)
}

func (c *repositoryContract) TestDeleteRemovesTheRowWhenTheVersionMatches() {
	stored := c.create(c.draft(c.accounts.Patient, "Meningococcal"))

	c.Require().NoError(c.repository.Delete(c.ctx(), stored.ID, stored.Version))

	_, err := c.repository.Get(c.ctx(), stored.ID)
	c.Assert().ErrorIs(err, domain.ErrNotFound)
}

func (c *repositoryContract) TestDeleteRefusesAVersionThatIsNotTheCurrentOne() {
	stored := c.create(c.draft(c.accounts.Patient, "Pneumococcal"))

	err := c.repository.Delete(c.ctx(), stored.ID, "not-the-version-that-was-read")
	c.Require().ErrorIs(err, domain.ErrVersionMismatch)
}

func (c *repositoryContract) TestEveryRefusalIsOneOfTheDomainSentinels() {
	_, err := c.repository.Get(c.ctx(), "nosuchrecord01")

	c.Require().Error(err)
	c.Assert().True(
		errors.Is(err, domain.ErrNotFound) ||
			errors.Is(err, domain.ErrVersionMismatch) ||
			errors.Is(err, domain.ErrConflict))
}
