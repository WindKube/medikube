package injurytest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/injury"
)

type Accounts struct {
	Patient         string
	StrangerPatient string
}

type Factory func(t *testing.T) (injury.Repository, Accounts)

func RunRepositoryContract(t *testing.T, newRepository Factory) {
	t.Helper()

	require.NotNil(t, newRepository, "the contract has no repository to run against")

	suite.Run(t, &repositoryContract{newRepository: newRepository})
}

type repositoryContract struct {
	suite.Suite

	newRepository Factory
	repository    injury.Repository
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

func (c *repositoryContract) draft(patientID, name string) clinical.Injury {
	return clinical.Injury{PatientID: patientID, Name: name, BodyPart: "wrist", Status: clinical.ConditionStatusActive}
}

func (c *repositoryContract) create(entity clinical.Injury) clinical.Injury {
	c.T().Helper()

	stored, err := c.repository.Create(c.ctx(), entity)
	c.Require().NoError(err)

	return stored
}

func (c *repositoryContract) list(query injury.Query) domain.Page[clinical.Injury] {
	c.T().Helper()

	page, err := c.repository.List(c.ctx(), c.accounts.Patient, query)
	c.Require().NoError(err)

	return page
}

func ids(page domain.Page[clinical.Injury]) []string {
	found := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		found = append(found, item.ID)
	}

	return found
}

func (c *repositoryContract) TestCreateStoresEveryFieldAndMintsAnIdentity() {
	occurredOn, err := domain.NewDate(2026, time.March, 1)
	c.Require().NoError(err)

	full := clinical.Injury{
		PatientID:     c.accounts.Patient,
		Name:          "Broken wrist",
		Type:          clinical.InjuryTypeFracture,
		BodyPart:      "left wrist",
		Laterality:    clinical.LateralityLeft,
		OccurredOn:    occurredOn,
		Mechanism:     "fell off a scooter",
		Severity:      clinical.SeverityModerate,
		Status:        clinical.ConditionStatusHealing,
		RecoveryNotes: "cast for six weeks",
	}

	stored := c.create(full)

	c.Require().NotEmpty(stored.ID)
	c.Assert().NotEmpty(stored.Version)

	read, err := c.repository.Get(c.ctx(), stored.ID)
	c.Require().NoError(err)

	c.Assert().Equal(c.accounts.Patient, read.PatientID)
	c.Assert().Equal(full.Name, read.Name)
	c.Assert().Equal(full.Type, read.Type)
	c.Assert().Equal(full.BodyPart, read.BodyPart)
	c.Assert().Equal(full.Laterality, read.Laterality)
	c.Assert().Equal(full.OccurredOn, read.OccurredOn)
	c.Assert().Equal(full.Mechanism, read.Mechanism)
	c.Assert().Equal(full.Severity, read.Severity)
	c.Assert().Equal(full.Status, read.Status)
	c.Assert().Equal(full.RecoveryNotes, read.RecoveryNotes)
}

func (c *repositoryContract) TestAnAbsentOptionalFieldStaysAbsent() {
	stored := c.create(c.draft(c.accounts.Patient, "Sprained ankle"))

	read, err := c.repository.Get(c.ctx(), stored.ID)
	c.Require().NoError(err)

	c.Assert().True(read.OccurredOn.IsZero())
	c.Assert().Empty(read.Type)
	c.Assert().Empty(read.Laterality)
}

func (c *repositoryContract) TestGetOfAnIdentityThatNeverExistedIsNotFound() {
	_, err := c.repository.Get(c.ctx(), "nosuchrecord01")

	c.Require().ErrorIs(err, domain.ErrNotFound)
}

func (c *repositoryContract) TestListReturnsOnlyThePatientsRows() {
	mine := c.create(c.draft(c.accounts.Patient, "Cut hand"))
	theirs := c.create(c.draft(c.accounts.StrangerPatient, "Twisted knee"))

	page := c.list(injury.Query{})

	c.Assert().Equal([]string{mine.ID}, ids(page))
	c.Assert().NotContains(ids(page), theirs.ID)
}

func (c *repositoryContract) TestListNarrowsByStatus() {
	active := c.create(c.draft(c.accounts.Patient, "Fractured rib"))

	resolved := c.draft(c.accounts.Patient, "Old bruise")
	resolved.Status = clinical.ConditionStatusResolved
	resolvedID := c.create(resolved).ID

	page := c.list(injury.Query{Statuses: []clinical.ConditionStatus{clinical.ConditionStatusActive}})
	c.Assert().Equal([]string{active.ID}, ids(page))
	c.Assert().NotContains(ids(page), resolvedID)
}

func (c *repositoryContract) TestListNarrowsByUnresolved() {
	active := c.draft(c.accounts.Patient, "Fresh burn")
	active.Status = clinical.ConditionStatusActive
	activeID := c.create(active).ID

	healing := c.draft(c.accounts.Patient, "Healing laceration")
	healing.Status = clinical.ConditionStatusHealing
	healingID := c.create(healing).ID

	resolved := c.draft(c.accounts.Patient, "Old sprain")
	resolved.Status = clinical.ConditionStatusResolved
	c.create(resolved)

	page := c.list(injury.Query{Statuses: []clinical.ConditionStatus{
		clinical.ConditionStatusActive, clinical.ConditionStatusHealing,
	}})

	c.Assert().ElementsMatch([]string{activeID, healingID}, ids(page))
}

func (c *repositoryContract) TestListSearchesName() {
	byName := c.create(c.draft(c.accounts.Patient, "Rotator cuff tear"))
	unrelated := c.create(c.draft(c.accounts.Patient, "Ankle sprain"))

	page := c.list(injury.Query{Search: "rotator"})

	c.Assert().Equal([]string{byName.ID}, ids(page))
	c.Assert().NotContains(ids(page), unrelated.ID)
}

func (c *repositoryContract) TestAnAbsentOccurredOnSortsLastInBothDirections() {
	earlier, err := domain.NewDate(2026, time.January, 5)
	c.Require().NoError(err)

	later, err := domain.NewDate(2026, time.June, 9)
	c.Require().NoError(err)

	early := c.draft(c.accounts.Patient, "Early injury")
	early.OccurredOn = earlier
	earlyID := c.create(early).ID

	late := c.draft(c.accounts.Patient, "Late injury")
	late.OccurredOn = later
	lateID := c.create(late).ID

	undated := c.create(c.draft(c.accounts.Patient, "Undated injury")).ID

	descending := c.list(injury.Query{Sort: []domain.SortKey{{Field: injury.FieldOccurredOn, Desc: true}}})
	c.Assert().Equal([]string{lateID, earlyID, undated}, ids(descending))

	ascending := c.list(injury.Query{Sort: []domain.SortKey{{Field: injury.FieldOccurredOn}}})
	c.Assert().Equal([]string{earlyID, lateID, undated}, ids(ascending))
}

func (c *repositoryContract) TestListPagesWithACursorAndNeverRepeatsOrSkips() {
	names := []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon"}

	throughout := make([]string, 0, len(names))
	for _, name := range names {
		throughout = append(throughout, c.create(c.draft(c.accounts.Patient, name)).ID)
	}

	query := injury.Query{Sort: []domain.SortKey{{Field: injury.FieldName}}, Limit: 2}

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

	silent := c.list(injury.Query{Limit: 2})
	c.Assert().Nil(silent.Total)

	counted := c.list(injury.Query{Limit: 2, Count: true})
	c.Require().NotNil(counted.Total)
	c.Assert().Equal(3, *counted.Total)
}

func (c *repositoryContract) TestUpdateReplacesTheStoredValuesWhenTheVersionMatches() {
	stored := c.create(c.draft(c.accounts.Patient, "Bruised shin"))

	changed := stored
	changed.Severity = clinical.SeveritySevere

	updated, err := c.repository.Update(c.ctx(), changed, stored.Version)
	c.Require().NoError(err)
	c.Assert().Equal(clinical.SeveritySevere, updated.Severity)

	read, err := c.repository.Get(c.ctx(), stored.ID)
	c.Require().NoError(err)
	c.Assert().Equal(clinical.SeveritySevere, read.Severity)
	c.Assert().Equal(updated.Version, read.Version)
}

func (c *repositoryContract) TestUpdateRefusesAVersionThatIsNotTheCurrentOne() {
	stored := c.create(c.draft(c.accounts.Patient, "Dislocated shoulder"))

	changed := stored
	changed.Severity = clinical.SeveritySevere

	_, err := c.repository.Update(c.ctx(), changed, "not-the-version-that-was-read")
	c.Require().ErrorIs(err, domain.ErrVersionMismatch)
}

func (c *repositoryContract) TestDeleteRemovesTheRowWhenTheVersionMatches() {
	stored := c.create(c.draft(c.accounts.Patient, "Concussion"))

	c.Require().NoError(c.repository.Delete(c.ctx(), stored.ID, stored.Version))

	_, err := c.repository.Get(c.ctx(), stored.ID)
	c.Assert().ErrorIs(err, domain.ErrNotFound)
}

func (c *repositoryContract) TestDeleteRefusesAVersionThatIsNotTheCurrentOne() {
	stored := c.create(c.draft(c.accounts.Patient, "Puncture wound"))

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
