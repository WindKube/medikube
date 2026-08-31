package medicationtest

import (
	"context"
	"errors"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/service/medication"
)

// Accounts are the two owners a contract run writes rows for. They are supplied
// by the implementation rather than fixed here because the PocketBase
// repository needs real ids: `owner` is a relation with a cascade, and a row
// pointing at an account that does not exist is refused by the column before it
// is refused by anything this suite asserts.
//
// The stranger is never written to by the contract itself. It exists so that
// every owner-scoped assertion has somebody to be scoped against.
type Accounts struct {
	Owner    string
	Stranger string
}

// Factory builds one empty repository, per test. Per test and not per run: a
// suite that shared a repository between two cases would have the second case
// reading rows the first left, and the ordering assertions would depend on the
// order the methods happen to run in.
type Factory func(t *testing.T) (medication.Repository, Accounts)

// RunRepositoryContract is the contract every medication.Repository passes —
// the in-memory fake here and the PocketBase repository in internal/store,
// against a real instance.
//
// It is one suite run twice rather than two suites, because the failure it
// exists to catch is the one where the fake and the store agree about
// everything the service does and disagree about owner scoping, the version
// check or the ordering of an absent date. Every one of those is invisible to a
// test written against either implementation alone.
func RunRepositoryContract(t *testing.T, newRepository Factory) {
	t.Helper()

	require.NotNil(t, newRepository, "the contract has no repository to run against")

	suite.Run(t, &repositoryContract{newRepository: newRepository})
}

type repositoryContract struct {
	suite.Suite

	newRepository Factory
	repository    medication.Repository
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

// draft is the minimum a stored medication needs: a name, an owner and a
// state. Every other field is optional by data-model §2 and the cases that care
// about one set it.
func (c *repositoryContract) draft(owner, name string) clinical.Medication {
	return clinical.Medication{
		OwnerID: owner,
		Name:    name,
		Status:  clinical.TherapyStatusActive,
	}
}

func (c *repositoryContract) create(medication clinical.Medication) clinical.Medication {
	c.T().Helper()

	stored, err := c.repository.Create(c.ctx(), medication)
	c.Require().NoError(err)

	return stored
}

func (c *repositoryContract) list(query medication.Query) domain.Page[clinical.Medication] {
	c.T().Helper()

	page, err := c.repository.List(c.ctx(), c.accounts.Owner, query)
	c.Require().NoError(err)

	return page
}

func ids(page domain.Page[clinical.Medication]) []string {
	found := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		found = append(found, item.ID)
	}

	return found
}

// TestCreateStoresEveryFieldAndMintsAnIdentity is the round trip. Every field
// data-model §2 declares is written and read back, because a mapper that drops
// one silently stores a medication missing the thing the person recorded.
func (c *repositoryContract) TestCreateStoresEveryFieldAndMintsAnIdentity() {
	startedOn, err := domain.NewDate(2026, time.March, 1)
	c.Require().NoError(err)

	endedOn, err := domain.NewDate(2026, time.April, 30)
	c.Require().NoError(err)

	full := clinical.Medication{
		OwnerID:         c.accounts.Owner,
		Name:            "Amoxicillin",
		AlternativeName: "Amoxil",
		Type:            clinical.MedicationTypePrescription,
		Dosage:          "500 mg",
		Frequency:       "three times a day",
		Route:           clinical.MedicationRouteOral,
		Indication:      "a chest infection",
		StartedOn:       startedOn,
		EndedOn:         endedOn,
		Status:          clinical.TherapyStatusCompleted,
		SideEffects:     "some nausea",
		Notes:           "finish the course",
	}

	stored := c.create(full)

	c.Require().NotEmpty(stored.ID, "a stored medication with no identity cannot be addressed again")
	c.Assert().NotEmpty(stored.Version, "no version means If-Match can never succeed")
	c.Assert().False(stored.CreatedAt.IsZero())
	c.Assert().False(stored.UpdatedAt.IsZero())

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)

	c.Assert().Equal(stored.ID, read.ID)
	c.Assert().Equal(stored.Version, read.Version,
		"the version a write returned and the version a read returns are the same value or If-Match is a coin toss")

	// Compared field by field rather than as a struct, so a failure names the
	// column that was dropped instead of printing two thirteen-field values.
	c.Assert().Equal(c.accounts.Owner, read.OwnerID)
	c.Assert().Equal(full.Name, read.Name)
	c.Assert().Equal(full.AlternativeName, read.AlternativeName)
	c.Assert().Equal(full.Type, read.Type)
	c.Assert().Equal(full.Dosage, read.Dosage)
	c.Assert().Equal(full.Frequency, read.Frequency)
	c.Assert().Equal(full.Route, read.Route)
	c.Assert().Equal(full.Indication, read.Indication)
	c.Assert().Equal(full.StartedOn, read.StartedOn)
	c.Assert().Equal(full.EndedOn, read.EndedOn)
	c.Assert().Equal(full.Status, read.Status)
	c.Assert().Equal(full.SideEffects, read.SideEffects)
	c.Assert().Equal(full.Notes, read.Notes)
}

// TestCreateMintsADistinctIdentityEveryTime — two courses of the same drug are
// legitimate (data-model §2: no unique index on name), and they are two rows.
func (c *repositoryContract) TestCreateMintsADistinctIdentityEveryTime() {
	first := c.create(c.draft(c.accounts.Owner, "Ibuprofen"))
	second := c.create(c.draft(c.accounts.Owner, "Ibuprofen"))

	c.Assert().NotEqual(first.ID, second.ID)
}

// TestAnAbsentOptionalFieldStaysAbsent. FR-024's "omit what was not recorded"
// is a property of the stored row before it is a property of any view: a
// repository that wrote a zero date as a real one would make the detail page
// show a start date the person never gave.
func (c *repositoryContract) TestAnAbsentOptionalFieldStaysAbsent() {
	stored := c.create(c.draft(c.accounts.Owner, "Paracetamol"))

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)

	c.Assert().True(read.StartedOn.IsZero())
	c.Assert().True(read.EndedOn.IsZero())
	c.Assert().Empty(read.AlternativeName)
	c.Assert().Empty(read.Type)
	c.Assert().Empty(read.Route)
	c.Assert().Empty(read.Notes)
}

// TestGetIsScopedToTheOwner is FR-033 at the storage layer. The service
// authorizes above this and the repository refuses anyway: two independent
// refusals, because one of them is one edit away from not being there.
func (c *repositoryContract) TestGetIsScopedToTheOwner() {
	stored := c.create(c.draft(c.accounts.Owner, "Metformin"))

	_, err := c.repository.Get(c.ctx(), c.accounts.Stranger, stored.ID)
	c.Require().ErrorIs(err, domain.ErrNotFound,
		"another account's medication is answered exactly as one that does not exist")
}

func (c *repositoryContract) TestGetOfAnIdentityThatNeverExistedIsNotFound() {
	_, err := c.repository.Get(c.ctx(), c.accounts.Owner, "nosuchrecord01")

	c.Require().ErrorIs(err, domain.ErrNotFound)
}

// TestListReturnsOnlyTheOwnersRows. The counterparty's rows are written through
// the same repository, so a scoping mistake has something to leak.
func (c *repositoryContract) TestListReturnsOnlyTheOwnersRows() {
	mine := c.create(c.draft(c.accounts.Owner, "Atorvastatin"))
	theirs := c.create(c.draft(c.accounts.Stranger, "Bisoprolol"))

	page := c.list(medication.Query{})

	c.Assert().Equal([]string{mine.ID}, ids(page))
	c.Assert().NotContains(ids(page), theirs.ID)
}

// TestListNarrowsByStatus is FR-022's narrowing by state.
func (c *repositoryContract) TestListNarrowsByStatus() {
	active := c.create(c.draft(c.accounts.Owner, "Ramipril"))

	stopped := c.draft(c.accounts.Owner, "Simvastatin")
	stopped.Status = clinical.TherapyStatusStopped
	stoppedID := c.create(stopped).ID

	onHold := c.draft(c.accounts.Owner, "Warfarin")
	onHold.Status = clinical.TherapyStatusOnHold
	onHoldID := c.create(onHold).ID

	page := c.list(medication.Query{Statuses: []clinical.TherapyStatus{clinical.TherapyStatusActive}})
	c.Assert().Equal([]string{active.ID}, ids(page))

	both := c.list(medication.Query{Statuses: []clinical.TherapyStatus{
		clinical.TherapyStatusStopped,
		clinical.TherapyStatusOnHold,
	}})
	c.Assert().ElementsMatch([]string{stoppedID, onHoldID}, ids(both))
}

// TestListSearchesNameAndAlternativeNameCaseInsensitively is FR-022's text
// match. Both columns, because a person who recorded the brand name and
// searched for the generic one is the case the alternative name exists for.
func (c *repositoryContract) TestListSearchesNameAndAlternativeNameCaseInsensitively() {
	byName := c.create(c.draft(c.accounts.Owner, "Salbutamol"))

	alternative := c.draft(c.accounts.Owner, "Ventolin")
	alternative.AlternativeName = "Salbutamol inhaler"
	byAlternative := c.create(alternative)

	unrelated := c.create(c.draft(c.accounts.Owner, "Omeprazole"))

	page := c.list(medication.Query{Search: "salbuta"})

	c.Assert().ElementsMatch([]string{byName.ID, byAlternative.ID}, ids(page))
	c.Assert().NotContains(ids(page), unrelated.ID)
}

// TestListSortsByNameInBothDirections pins the two orderings by name, including
// the case fold: the index is on LOWER(name), so "amoxicillin" sorts before
// "Betahistine" and a byte comparison would put it after.
func (c *repositoryContract) TestListSortsByNameInBothDirections() {
	lower := c.create(c.draft(c.accounts.Owner, "amoxicillin"))
	upper := c.create(c.draft(c.accounts.Owner, "Betahistine"))

	ascending := c.list(medication.Query{Sort: []domain.SortKey{{Field: medication.FieldName}}})
	c.Assert().Equal([]string{lower.ID, upper.ID}, ids(ascending))

	descending := c.list(medication.Query{Sort: []domain.SortKey{{Field: medication.FieldName, Desc: true}}})
	c.Assert().Equal([]string{upper.ID, lower.ID}, ids(descending))
}

// TestRowsSharingASortValueAreOrderedByIdentityDescending is why every index in
// data-model §2 ends in id. Two rows with the same name have no other order,
// and a repository that leaves them to the database orders them differently on
// two runs — which makes the page after them repeat one and skip the other.
func (c *repositoryContract) TestRowsSharingASortValueAreOrderedByIdentityDescending() {
	first := c.create(c.draft(c.accounts.Owner, "Codeine"))
	second := c.create(c.draft(c.accounts.Owner, "Codeine"))

	expected := []string{first.ID, second.ID}
	sort.Sort(sort.Reverse(sort.StringSlice(expected)))

	page := c.list(medication.Query{Sort: []domain.SortKey{{Field: medication.FieldName}}})

	c.Assert().Equal(expected, ids(page))
}

// TestAnAbsentStartDateSortsLastInBothDirections is contracts/records.md,
// stated there rather than left to the database precisely because SQLite's
// answer differs between the two directions: the absent date is the empty
// string, which sorts before every real one ascending and after every real one
// descending.
//
// A person whose medication has no recorded start date should not find it at
// the top of "most recently started", and should not find it at the top of
// "earliest started" either.
func (c *repositoryContract) TestAnAbsentStartDateSortsLastInBothDirections() {
	earlier, err := domain.NewDate(2026, time.January, 5)
	c.Require().NoError(err)

	later, err := domain.NewDate(2026, time.June, 9)
	c.Require().NoError(err)

	early := c.draft(c.accounts.Owner, "Enalapril")
	early.StartedOn = earlier
	earlyID := c.create(early).ID

	late := c.draft(c.accounts.Owner, "Furosemide")
	late.StartedOn = later
	lateID := c.create(late).ID

	undated := c.create(c.draft(c.accounts.Owner, "Gliclazide")).ID

	descending := c.list(medication.Query{Sort: []domain.SortKey{{Field: medication.FieldStartedOn, Desc: true}}})
	c.Assert().Equal([]string{lateID, earlyID, undated}, ids(descending))

	ascending := c.list(medication.Query{Sort: []domain.SortKey{{Field: medication.FieldStartedOn}}})
	c.Assert().Equal([]string{earlyID, lateID, undated}, ids(ascending))
}

// TestListOrdersByLastChange asserts the property rather than a permutation.
//
// Two rows written in the same millisecond share an `updated` value, so an
// exact expected sequence here would be a coin toss on a fast machine — the
// flaky gate assertion constitution VIII forbids. What is not a coin toss is
// that the sequence is monotonic in the value it claims to be sorted by, and
// that the identity tiebreaker decides when it is not.
func (c *repositoryContract) TestListOrdersByLastChange() {
	for _, name := range []string{"Levothyroxine", "Naproxen", "Prednisolone"} {
		c.create(c.draft(c.accounts.Owner, name))
	}

	descending := c.list(medication.Query{Sort: []domain.SortKey{{Field: medication.FieldUpdated, Desc: true}}})
	c.Require().Len(descending.Items, 3)

	for i := 1; i < len(descending.Items); i++ {
		previous, current := descending.Items[i-1], descending.Items[i]

		if previous.UpdatedAt.Equal(current.UpdatedAt) {
			c.Assert().Greater(previous.ID, current.ID,
				"rows sharing an update time are ordered by identity, descending")

			continue
		}

		c.Assert().True(previous.UpdatedAt.After(current.UpdatedAt),
			"the most recently changed row comes first")
	}

	ascending := c.list(medication.Query{Sort: []domain.SortKey{{Field: medication.FieldUpdated}}})
	c.Require().Len(ascending.Items, 3)

	for i := 1; i < len(ascending.Items); i++ {
		previous, current := ascending.Items[i-1], ascending.Items[i]

		if previous.UpdatedAt.Equal(current.UpdatedAt) {
			c.Assert().Greater(previous.ID, current.ID)

			continue
		}

		c.Assert().True(previous.UpdatedAt.Before(current.UpdatedAt))
	}
}

// TestListPagesWithACursorAndNeverRepeatsOrSkips is FR-023.
//
// The traversal continues while a row is inserted above the boundary, which is
// the case an offset gets wrong: with `LIMIT 3 OFFSET 3` the insert shifts
// every later page by one and the row on the boundary is served twice.
func (c *repositoryContract) TestListPagesWithACursorAndNeverRepeatsOrSkips() {
	names := []string{"Aspirin", "Bisacodyl", "Cetirizine", "Diazepam", "Enoxaparin", "Fluoxetine", "Gabapentin"}

	throughout := make([]string, 0, len(names))
	for _, name := range names {
		throughout = append(throughout, c.create(c.draft(c.accounts.Owner, name)).ID)
	}

	query := medication.Query{Sort: []domain.SortKey{{Field: medication.FieldName}}, Limit: 3}

	var (
		seen     []string
		inserted bool
	)

	for page := 1; ; page++ {
		c.Require().LessOrEqual(page, len(names)+2, "the traversal is not terminating")

		got := c.list(query)
		c.Require().LessOrEqual(len(got.Items), query.Limit, "a page longer than the limit was asked for")

		seen = append(seen, ids(got)...)

		if !inserted {
			// Sorts first under this ordering, and therefore behind the
			// boundary the first page ended on.
			c.create(c.draft(c.accounts.Owner, "Adrenaline"))
			inserted = true
		}

		if got.NextCursor == nil {
			break
		}

		c.Require().NotEmpty(*got.NextCursor, "an empty cursor is not a cursor")
		query.Cursor = *got.NextCursor
	}

	unique := slices.Clone(seen)
	slices.Sort(unique)
	c.Assert().Equal(len(seen), len(slices.Compact(unique)), "the traversal served a row twice")

	for _, id := range throughout {
		c.Assert().Contains(seen, id, "the traversal skipped a row that existed for the whole of it")
	}
}

// TestALastPageCarriesNoCursor. nil and not an empty string: the envelope's
// member is present either way, and "there is more" has to be answerable
// without a second request.
func (c *repositoryContract) TestALastPageCarriesNoCursor() {
	c.create(c.draft(c.accounts.Owner, "Hydrocortisone"))

	page := c.list(medication.Query{Limit: 25})

	c.Assert().Nil(page.NextCursor)
}

// TestACursorFromAnotherOrderingIsRefused. The boundary names a row's position
// in one sequence; continuing another sequence from it serves an arbitrary
// slice of the list, which is FR-023's failure with none of its symptoms.
func (c *repositoryContract) TestACursorFromAnotherOrderingIsRefused() {
	for _, name := range []string{"Insulin", "Ketoprofen", "Lansoprazole"} {
		c.create(c.draft(c.accounts.Owner, name))
	}

	first := c.list(medication.Query{Sort: []domain.SortKey{{Field: medication.FieldName}}, Limit: 2})
	c.Require().NotNil(first.NextCursor)

	_, err := c.repository.List(c.ctx(), c.accounts.Owner, medication.Query{
		Sort:   []domain.SortKey{{Field: medication.FieldName, Desc: true}},
		Limit:  2,
		Cursor: *first.NextCursor,
	})

	c.Assert().Error(err, "a boundary minted under one ordering was accepted under another")
}

// TestACursorThisRepositoryDidNotMintIsRefused. A caller that can write a
// boundary chooses a query the service never offered — including one over
// somebody else's rows.
func (c *repositoryContract) TestACursorThisRepositoryDidNotMintIsRefused() {
	c.create(c.draft(c.accounts.Owner, "Morphine"))

	_, err := c.repository.List(c.ctx(), c.accounts.Owner, medication.Query{Cursor: "not-a-cursor-this-instance-issued"})

	c.Assert().Error(err)
}

// TestCountIsOnlyProducedWhenAsked. Counting a large account's rows on every
// page is a cost nobody asked for, so the member is absent unless it was.
func (c *repositoryContract) TestCountIsOnlyProducedWhenAsked() {
	for _, name := range []string{"Nitrofurantoin", "Olanzapine", "Propranolol"} {
		c.create(c.draft(c.accounts.Owner, name))
	}

	c.create(c.draft(c.accounts.Stranger, "Quinine"))

	silent := c.list(medication.Query{Limit: 2})
	c.Assert().Nil(silent.Total)

	counted := c.list(medication.Query{Limit: 2, Count: true})
	c.Require().NotNil(counted.Total)
	c.Assert().Equal(3, *counted.Total, "the total counts the owner's whole list and not the page")
}

// TestUpdateReplacesTheStoredValuesWhenTheVersionMatches.
func (c *repositoryContract) TestUpdateReplacesTheStoredValuesWhenTheVersionMatches() {
	stored := c.create(c.draft(c.accounts.Owner, "Rifampicin"))

	changed := stored
	changed.Dosage = "600 mg"
	changed.Status = clinical.TherapyStatusStopped

	updated, err := c.repository.Update(c.ctx(), changed, stored.Version)
	c.Require().NoError(err)

	c.Assert().Equal("600 mg", updated.Dosage)
	c.Assert().Equal(clinical.TherapyStatusStopped, updated.Status)

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)

	c.Assert().Equal("600 mg", read.Dosage)
	c.Assert().Equal(clinical.TherapyStatusStopped, read.Status)
	c.Assert().Equal(updated.Version, read.Version,
		"the version an update returned is the one the next If-Match has to carry")
}

// TestUpdateRefusesAVersionThatIsNotTheCurrentOne is FR-026, and the second
// assertion is the half that matters: a refused write that wrote anyway is a
// silent overwrite with an error message.
func (c *repositoryContract) TestUpdateRefusesAVersionThatIsNotTheCurrentOne() {
	stored := c.create(c.draft(c.accounts.Owner, "Sertraline"))

	changed := stored
	changed.Dosage = "100 mg"

	_, err := c.repository.Update(c.ctx(), changed, "not-the-version-that-was-read")
	c.Require().ErrorIs(err, domain.ErrVersionMismatch)

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)
	c.Assert().Empty(read.Dosage, "the refused update was applied anyway")
}

// TestUpdateIsScopedToTheOwner. The stranger addresses a row that exists and
// is told it does not, and the row is untouched.
func (c *repositoryContract) TestUpdateIsScopedToTheOwner() {
	stored := c.create(c.draft(c.accounts.Owner, "Tamsulosin"))

	hijacked := stored
	hijacked.OwnerID = c.accounts.Stranger
	hijacked.Name = "Taken over"

	_, err := c.repository.Update(c.ctx(), hijacked, stored.Version)
	c.Require().ErrorIs(err, domain.ErrNotFound)

	read, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Require().NoError(err)
	c.Assert().Equal("Tamsulosin", read.Name)
	c.Assert().Equal(c.accounts.Owner, read.OwnerID, "the row changed hands")
}

// TestUpdateOfAnIdentityThatNeverExistedIsNotFound.
func (c *repositoryContract) TestUpdateOfAnIdentityThatNeverExistedIsNotFound() {
	absent := c.draft(c.accounts.Owner, "Ursodeoxycholic acid")
	absent.ID = "nosuchrecord01"

	_, err := c.repository.Update(c.ctx(), absent, "any-version")

	c.Require().ErrorIs(err, domain.ErrNotFound)
}

// TestDeleteRemovesTheRowWhenTheVersionMatches. FR-028: permanent, no recycle
// bin, so the assertion is that the second read is a miss.
func (c *repositoryContract) TestDeleteRemovesTheRowWhenTheVersionMatches() {
	stored := c.create(c.draft(c.accounts.Owner, "Valsartan"))

	c.Require().NoError(c.repository.Delete(c.ctx(), c.accounts.Owner, stored.ID, stored.Version))

	_, err := c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Assert().ErrorIs(err, domain.ErrNotFound)

	page := c.list(medication.Query{})
	c.Assert().NotContains(ids(page), stored.ID)
}

func (c *repositoryContract) TestDeleteRefusesAVersionThatIsNotTheCurrentOne() {
	stored := c.create(c.draft(c.accounts.Owner, "Zopiclone"))

	err := c.repository.Delete(c.ctx(), c.accounts.Owner, stored.ID, "not-the-version-that-was-read")
	c.Require().ErrorIs(err, domain.ErrVersionMismatch)

	_, err = c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Assert().NoError(err, "the refused delete removed the row anyway")
}

func (c *repositoryContract) TestDeleteIsScopedToTheOwner() {
	stored := c.create(c.draft(c.accounts.Owner, "Xylometazoline"))

	err := c.repository.Delete(c.ctx(), c.accounts.Stranger, stored.ID, stored.Version)
	c.Require().ErrorIs(err, domain.ErrNotFound)

	_, err = c.repository.Get(c.ctx(), c.accounts.Owner, stored.ID)
	c.Assert().NoError(err, "a stranger deleted a row they were told does not exist")
}

func (c *repositoryContract) TestDeleteOfAnIdentityThatNeverExistedIsNotFound() {
	err := c.repository.Delete(c.ctx(), c.accounts.Owner, "nosuchrecord01", "any-version")

	c.Require().ErrorIs(err, domain.ErrNotFound)
}

// TestEveryRefusalIsOneOfTheDomainSentinels. The service maps what it gets from
// here onto a status, and it maps by errors.Is: a repository returning a bare
// driver error for a missing row produces a 500 where the contract says 404.
func (c *repositoryContract) TestEveryRefusalIsOneOfTheDomainSentinels() {
	_, err := c.repository.Get(c.ctx(), c.accounts.Owner, "nosuchrecord01")

	c.Require().Error(err)
	c.Assert().True(
		errors.Is(err, domain.ErrNotFound) ||
			errors.Is(err, domain.ErrVersionMismatch) ||
			errors.Is(err, domain.ErrConflict),
		"a refusal that matches no sentinel reaches the client as an internal error")
}
