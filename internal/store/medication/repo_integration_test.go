package medication_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/service/medication"
	"medikube/internal/service/medication/medicationtest"
	"medikube/internal/store"
	pbmedication "medikube/internal/store/medication"

	// The migrations register themselves from their own init and
	// tests.NewTestApp runs core.AppMigrations against the instance. Without
	// this import every case below would run against PocketBase's stock schema,
	// where the collection this package reads does not exist.
	_ "medikube/internal/store/migrations"
)

// The two accounts every case is scoped against. The stranger is written to
// only where a scoping assertion needs somebody to be scoped against.
const (
	ownerEmail    = "owner@example.test"
	strangerEmail = "stranger@example.test"
)

// harness is one instance, one repository and two patients.
//
// A new one per test rather than one shared: the contract's ordering
// assertions are written against a repository holding only what that case put
// in it, and a shared instance would make every one of them depend on the order
// the cases happen to run in.
//
// Patient and not owner (research D-13): the repository narrows a list by the
// patient a medication names, and Get/Update/Delete are unscoped by design —
// the service resolves and authorizes the patient from the row itself, so the
// scoping this package still owns is List's alone.
type harness struct {
	app             *tests.TestApp
	repo            *pbmedication.Repo
	patient         string
	strangerPatient string
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

	repo, err := pbmedication.New(app, codec)
	require.NoError(t, err)

	owner := seedAccount(t, app, ownerEmail)
	stranger := seedAccount(t, app, strangerEmail)

	return harness{
		app:             app,
		repo:            repo,
		patient:         seedPatient(t, app, owner),
		strangerPatient: seedPatient(t, app, stranger),
	}
}

// seedAccount writes one account with every column the profile migration made
// required.
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

// seedPatient writes one minimal patient. A medication cannot be stored
// without a patient that exists: `patient` is a required relation
// (research D-13), so a row pointing at nobody is refused by the schema
// before it is refused by anything asserted here.
func seedPatient(t *testing.T, app core.App, ownerID string) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("patients")
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set("owner", ownerID)
	record.Set("first_name", "Test")
	record.Set("last_name", "Patient")

	require.NoError(t, app.Save(record))

	return record.Id
}

func (h harness) draft(patientID, name string) clinical.Medication {
	return clinical.Medication{PatientID: patientID, Name: name, Status: clinical.TherapyStatusActive}
}

func (h harness) create(t *testing.T, ctx context.Context, entity clinical.Medication) clinical.Medication {
	t.Helper()

	stored, err := h.repo.Create(ctx, entity)
	require.NoError(t, err)

	return stored
}

// seedBulk writes many rows in one transaction, through the same mapper the
// repository writes through. One transaction because a thousand rows is a
// thousand fsyncs otherwise, and the assertions below are about paging rather
// than about how the rows got there.
func (h harness) seedBulk(t *testing.T, rows []clinical.Medication) []string {
	t.Helper()

	ids := make([]string, 0, len(rows))

	require.NoError(t, h.app.RunInTransaction(func(txApp core.App) error {
		collection, err := txApp.FindCollectionByNameOrId(kind.Medication.Collection())
		if err != nil {
			return err
		}

		for _, row := range rows {
			record := core.NewRecord(collection)
			if err := store.MedicationToRecord(record, row); err != nil {
				return err
			}

			if err := txApp.Save(record); err != nil {
				return err
			}

			ids = append(ids, record.Id)
		}

		return nil
	}))

	return ids
}

func pageIDs(page domain.Page[clinical.Medication]) []string {
	found := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		found = append(found, item.ID)
	}

	return found
}

// traverse pages a list to its end and returns every id in the order it was
// served, calling disturb after each page so a case can change the collection
// underneath the traversal.
func (h harness) traverse(
	t *testing.T,
	ctx context.Context,
	query medication.Query,
	disturb func(page int, got domain.Page[clinical.Medication]),
) ([]string, [][]string) {
	t.Helper()

	var (
		seen  []string
		pages [][]string
	)

	for page := 1; ; page++ {
		require.LessOrEqual(t, page, 200, "the traversal is not terminating")

		got, err := h.repo.List(ctx, h.patient, query)
		require.NoError(t, err)

		if query.Limit > 0 {
			require.LessOrEqual(t, len(got.Items), query.Limit, "a page longer than the limit was served")
		}

		seen = append(seen, pageIDs(got)...)
		pages = append(pages, pageIDs(got))

		if disturb != nil {
			disturb(page, got)
		}

		if got.NextCursor == nil {
			return seen, pages
		}

		require.NotEmpty(t, *got.NextCursor, "an empty cursor is not a cursor")
		query.Cursor = *got.NextCursor
	}
}

func duplicates(ids []string) []string {
	seen := make(map[string]int, len(ids))
	for _, id := range ids {
		seen[id]++
	}

	var repeated []string

	for id, count := range seen {
		if count > 1 {
			repeated = append(repeated, id)
		}
	}

	slices.Sort(repeated)

	return repeated
}

// T140. The PocketBase repository passes the same suite the in-memory fake
// passes, which is the whole of Principle II: two implementations, one
// contract. A property asserted against either alone — patient scoping, the
// version check, where an absent date sorts — is a property of that
// implementation and not of the seam.
func TestThePocketBaseRepositoryPassesTheSameContractTheFakeDoes(t *testing.T) {
	t.Parallel()

	medicationtest.RunRepositoryContract(t, func(t *testing.T) (medication.Repository, medicationtest.Accounts) {
		t.Helper()

		h := newHarness(t)

		return h.repo, medicationtest.Accounts{Patient: h.patient, StrangerPatient: h.strangerPatient}
	})
}

// FR-023 at the scale SC-002 names, and the case an offset pager fails while
// passing every other test in this file: a row is inserted into the part of the
// list the traversal has already gone past, and a row that was already served
// is deleted out from under it.
//
// An OFFSET pager is defined against a result set that is changing underneath
// it, so the insert pushes every later page along by one and the reader
// silently never sees one entry. A keyset boundary is a row, and a row does not
// move when another row is added above it.
func TestKeysetPagingOverAThousandRowsNeitherRepeatsNorSkipsWhileTheListChanges(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	const (
		rows      = 1000
		pageSize  = 100
		nameCycle = 250
	)

	drafts := make([]clinical.Medication, 0, rows)

	for i := range rows {
		row := h.draft(h.patient, fmt.Sprintf("Medicine %03d", i%nameCycle))

		// Every seventh row has no start date, so the traversal crosses from
		// the dated rows into the undated tail and back out of it under both
		// directions. A run that never crosses that boundary passes with a
		// comparator that gets it wrong.
		if i%7 != 0 {
			started, err := domain.NewDate(2026, time.January, 1+i%28)
			require.NoError(t, err)

			row.StartedOn = started
		}

		drafts = append(drafts, row)
	}

	seeded := h.seedBulk(t, drafts)
	require.Len(t, seeded, rows)

	var (
		insertedID string
		deletedID  string
	)

	query := medication.Query{Sort: []domain.SortKey{{Field: medication.FieldStartedOn, Desc: true}}, Limit: pageSize}

	seen, pages := h.traverse(t, ctx, query, func(page int, got domain.Page[clinical.Medication]) {
		if page != 2 {
			return
		}

		// Sorts first under this ordering and is therefore behind the boundary
		// the second page ended on. A keyset never serves it; an offset pager
		// shifts every later page by one and drops an entry off the end.
		future, err := domain.NewDate(2099, time.January, 1)
		require.NoError(t, err)

		ahead := h.draft(h.patient, "Zafirlukast")
		ahead.StartedOn = future
		insertedID = h.create(t, ctx, ahead).ID

		// And one that was already served disappears, which is the other half
		// of contracts/records.md's "inserts and deletes rows between pages".
		first := got.Items[0]
		require.NoError(t, h.repo.Delete(ctx, first.ID, first.Version))
		deletedID = first.ID
	})

	require.NotEmpty(t, insertedID)
	require.NotEmpty(t, deletedID)

	assert.Empty(t, duplicates(seen), "the traversal served a row twice")

	for _, id := range seeded {
		if id == deletedID {
			continue
		}

		assert.Containsf(t, seen, id, "the traversal skipped %s, which existed for the whole of it", id)
	}

	assert.NotContains(t, seen, insertedID,
		"a row inserted behind the boundary was served, so the boundary moved when the list changed")

	require.Greater(t, len(pages), 1, "one page proves nothing about paging")
}

// The traversal has to survive the crossing from the dated rows into the
// undated ones, in both directions. contracts/records.md puts the absent date
// last under both, so ascending is the direction where the answer is not
// SQLite's own: the empty string sorts before every real date.
func TestPagingCrossesTheAbsentDateBoundaryInBothDirections(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	const (
		dated   = 32
		undated = 8
		limit   = 7
	)

	drafts := make([]clinical.Medication, 0, dated+undated)

	for i := range dated {
		row := h.draft(h.patient, fmt.Sprintf("Dated %02d", i))

		started, err := domain.NewDate(2026, time.January+time.Month(i/28), 1+i%28)
		require.NoError(t, err)

		row.StartedOn = started
		drafts = append(drafts, row)
	}

	for i := range undated {
		drafts = append(drafts, h.draft(h.patient, fmt.Sprintf("Undated %02d", i)))
	}

	seeded := h.seedBulk(t, drafts)
	undatedIDs := seeded[dated:]

	for _, descending := range []bool{false, true} {
		t.Run(domain.SortKey{Field: medication.FieldStartedOn, Desc: descending}.String(), func(t *testing.T) {
			query := medication.Query{
				Sort:  []domain.SortKey{{Field: medication.FieldStartedOn, Desc: descending}},
				Limit: limit,
			}

			seen, pages := h.traverse(t, ctx, query, nil)

			require.Len(t, seen, dated+undated)
			assert.Empty(t, duplicates(seen), "the traversal served a row twice")

			tail := seen[len(seen)-undated:]
			assert.ElementsMatch(t, undatedIDs, tail,
				"the rows with no start date are not the last ones in the sequence")

			// A page holding both kinds proves the comparison crossed the
			// boundary inside one query; a page holding only undated rows
			// proves a boundary minted from an undated row was handed back and
			// understood. Either one alone leaves the other untested.
			var mixed, allUndated int

			for _, page := range pages {
				undatedOnPage := 0

				for _, id := range page {
					if slices.Contains(undatedIDs, id) {
						undatedOnPage++
					}
				}

				switch {
				case undatedOnPage == len(page):
					allUndated++
				case undatedOnPage > 0:
					mixed++
				}
			}

			assert.Positive(t, mixed, "no page held both a dated and an undated row, so the crossing was never exercised")
			assert.Positive(t, allUndated, "no page was served from a boundary minted on an undated row")
		})
	}
}

// FR-022's three orderings, each traversed twice. The sequence has to be the
// same both times, and it is only the same because every index ends in the
// identity: two rows sharing a sort value have no other order, and a database
// left to choose returns them differently on two runs — which makes the page
// after them repeat one and skip the other.
func TestEveryPublishedOrderingPagesTheSameSequenceTwice(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	const (
		rows  = 40
		limit = 7
	)

	drafts := make([]clinical.Medication, 0, rows)

	for i := range rows {
		// Four rows share every name and four share every start date, so the
		// tiebreaker decides rather than merely being present.
		row := h.draft(h.patient, fmt.Sprintf("Medicine %02d", i%10))

		if i%5 != 0 {
			started, err := domain.NewDate(2026, time.March, 1+i%10)
			require.NoError(t, err)

			row.StartedOn = started
		}

		drafts = append(drafts, row)
	}

	seeded := h.seedBulk(t, drafts)

	for _, ordering := range medication.Sorts() {
		t.Run(ordering.String(), func(t *testing.T) {
			query := medication.Query{Sort: []domain.SortKey{ordering}, Limit: limit}

			first, _ := h.traverse(t, ctx, query, nil)
			second, _ := h.traverse(t, ctx, query, nil)

			require.Len(t, first, rows)
			assert.Empty(t, duplicates(first), "the traversal served a row twice")
			assert.ElementsMatch(t, seeded, first, "the traversal did not serve every row exactly once")
			assert.Equal(t, first, second, "two traversals of the same ordering served the rows in different orders")
		})
	}
}

// The `q` term is an OR across two columns and the patient scope is a separate
// conjunct outside it. This is what happens if the scope ever stops being one:
// dropped, moved inside the disjunction, or made conditional on the search
// being absent, one patient's medications are served to another and every
// other assertion in this file still passes.
//
// It does not catch a disjunction merely *widened* to include the patient —
// that leaves the scope in place as its own term, and it is refused at build
// time anyway, which internal/store's TestATermMaySpanOnlyTheColumnsDeclaredSearchable
// is the assertion for. The two together are the guarantee: the shape cannot be
// built, and if the scope goes missing by any other route this fails.
func TestASearchNeverReachesPastThePatient(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	mine := h.create(t, ctx, h.draft(h.patient, "Salbutamol"))

	theirs := h.draft(h.strangerPatient, "Salbutamol")
	theirsStored, err := h.repo.Create(ctx, theirs)
	require.NoError(t, err)

	page, err := h.repo.List(ctx, h.patient, medication.Query{Search: "salbuta"})
	require.NoError(t, err)

	assert.Equal(t, []string{mine.ID}, pageIDs(page))
	assert.NotContains(t, pageIDs(page), theirsStored.ID,
		"the search reached past the patient scope into another patient's records")

	// The same term against the patient's own id, which is the value the scope
	// compares against and the one an OR that swallowed the patient predicate
	// would match on.
	byPatientID, err := h.repo.List(ctx, h.strangerPatient, medication.Query{Search: h.patient})
	require.NoError(t, err)

	assert.Empty(t, pageIDs(byPatientID), "a patient id searched as text matched rows")
}

// A cursor is sealed against the query it continues, and the patient is part
// of that query. A boundary minted for one patient and replayed against
// another is not a boundary this instance issued.
func TestACursorMintedForAnotherPatientIsRefused(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	for i := range 3 {
		h.create(t, ctx, h.draft(h.patient, fmt.Sprintf("Medicine %d", i)))
		h.create(t, ctx, h.draft(h.strangerPatient, fmt.Sprintf("Medicine %d", i)))
	}

	query := medication.Query{Sort: []domain.SortKey{{Field: medication.FieldName}}, Limit: 2}

	page, err := h.repo.List(ctx, h.patient, query)
	require.NoError(t, err)
	require.NotNil(t, page.NextCursor, "the fixture is too small to produce a boundary")

	replayed := query
	replayed.Cursor = *page.NextCursor

	_, err = h.repo.List(ctx, h.strangerPatient, replayed)
	require.Error(t, err, "one patient's boundary continued another patient's list")
	assert.ErrorIs(t, err, store.ErrInvalidCursor)
}

// A wildcard a person typed is part of what they typed. Without the escape,
// searching for a dose written with a per-cent sign returns the whole list and
// looks like a search that simply matched a lot.
func TestTheWildcardsInASearchTermAreLiteral(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	literal := h.create(t, ctx, h.draft(h.patient, "Hydrocortisone 1% cream"))
	h.create(t, ctx, h.draft(h.patient, "Paracetamol"))

	page, err := h.repo.List(ctx, h.patient, medication.Query{Search: "1%"})
	require.NoError(t, err)

	assert.Equal(t, []string{literal.ID}, pageIDs(page))
}

// A column outside the published surface is refused rather than dropped. A
// dropped sort term is a different list, served without a word about it.
func TestAnOrderingTheResourceDoesNotPublishIsRefused(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	cases := []struct {
		name  string
		query medication.Query
	}{
		{"a column nothing declares", medication.Query{Sort: []domain.SortKey{{Field: "secret"}}}},
		{
			// Free text a person wrote about their own health, and — this is
			// the point — a value that would become a keyset boundary and
			// travel in a query string.
			name:  "a free-text clinical column",
			query: medication.Query{Sort: []domain.SortKey{{Field: "notes"}}},
		},
		{
			name:  "a column the search narrows by but nothing orders by",
			query: medication.Query{Sort: []domain.SortKey{{Field: "alternative_name"}}},
		},
		{"a page larger than the published maximum", medication.Query{Limit: store.MaxLimit + 1}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := h.repo.List(t.Context(), h.patient, testCase.query)
			assert.Error(t, err)
		})
	}
}

// The service publishes the ordering vocabulary and this package answers it.
// They are two lists of the same words, in two packages, with nothing but this
// between a rename on one side and a list that silently stops sorting.
func TestEveryPublishedSortFieldIsAColumnTheSchemaDeclares(t *testing.T) {
	t.Parallel()

	schema := store.MedicationSchema()

	for _, ordering := range medication.Sorts() {
		t.Run(ordering.String(), func(t *testing.T) {
			column, declared := schema.Column(ordering.Field)

			require.Truef(t, declared, "%s is published as an ordering and is not a column this schema knows", ordering.Field)
			assert.Falsef(t, column.FilterOnly, "%s is published as an ordering and is declared unorderable", ordering.Field)
		})
	}
}

// Every method takes a context and every method has to mean it. A repository
// that accepts the context and then writes through the plain Save is the shape
// this catches: it passes every other test in this file, because every other
// test hands it a context nobody cancelled.
//
// The failing mode is not academic. A list is read on a connection the client
// has already closed, and — the half that matters — a create, an update and a
// delete are committed for a request that was abandoned before it was sent.
func TestACancelledContextStopsEveryOperation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	stored := h.create(t, t.Context(), h.draft(h.patient, "Amoxicillin"))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	cases := []struct {
		name string
		call func() error
	}{
		{"list", func() error {
			_, err := h.repo.List(ctx, h.patient, medication.Query{})

			return err
		}},
		{"get", func() error {
			_, err := h.repo.Get(ctx, stored.ID)

			return err
		}},
		{"create", func() error {
			_, err := h.repo.Create(ctx, h.draft(h.patient, "Bisoprolol"))

			return err
		}},
		{"update", func() error {
			changed := stored
			changed.Dosage = "500 mg"
			_, err := h.repo.Update(ctx, changed, stored.Version)

			return err
		}},
		{"delete", func() error {
			return h.repo.Delete(ctx, stored.ID, stored.Version)
		}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Error(t, testCase.call(), "the cancelled context was not honoured")
		})
	}

	// And nothing was written for any of it. The row is untouched and it is
	// still the only one.
	page, err := h.repo.List(t.Context(), h.patient, medication.Query{})
	require.NoError(t, err)

	assert.Equal(t, []string{stored.ID}, pageIDs(page))

	read, err := h.repo.Get(t.Context(), stored.ID)
	require.NoError(t, err)
	assert.Empty(t, read.Dosage, "a write on a cancelled context reached the database")
}

// The list scoping with no checkpoint in the path.
//
// The service authorizes above this and the checkpoint is not wired into this
// file at all, so this is what a repository sees when the checkpoint has
// granted unconditionally — which is exactly the state the tree was in while
// internal/service/access had no tests: a stranger's list was scoped here and
// only here, and nothing said so.
//
// Get, Update and Delete are unscoped by design at this layer (research D-13):
// the repository resolves nothing about who may reach a row by id, and the
// service is what fetches, authorizes against the row's own patient, and only
// then proceeds. That is covered by internal/service/medication's own tests
// and by internal/service/access's, not here.
func TestListScopesNarrowlyByPatientWithNoCheckpointInThePath(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	h.create(t, ctx, h.draft(h.patient, "Amoxicillin"))

	page, err := h.repo.List(ctx, h.strangerPatient, medication.Query{})
	require.NoError(t, err)
	assert.Empty(t, pageIDs(page), "a stranger patient's list carried another patient's rows")

	mine, err := h.repo.List(ctx, h.patient, medication.Query{})
	require.NoError(t, err)
	assert.Len(t, mine.Items, 1, "the patient's own list did not carry their own row")
}
