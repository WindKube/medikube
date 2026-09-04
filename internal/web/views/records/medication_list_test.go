package records_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport/seed"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/records"
	"medikube/internal/web/views/viewstest"
)

// The landmark contracts/pages.md P4 publishes. It is a Playwright selector, so
// the spelling is pinned here as a literal rather than read back from the
// component: a test that asks the component what it renders and then asserts it
// renders that proves nothing.
const medicationRegion = "Medications"

func listProps(t *testing.T, medications ...records.MedicationView) records.MedicationListProps {
	t.Helper()

	return records.MedicationListProps{
		Medications: medications,
		CreateHref:  "/" + kind.Medication.Segment() + "/new",
	}
}

// T149, FR-029, contracts/pages.md. The empty state renders INSIDE the
// landmark, never instead of it — an account with nothing recorded must still
// answer region[name="Medications"], because phase 003 depends on that holding
// and the smoke gate navigates to account C on every run.
func TestTheRegionIsPresentWhetherOrNotAnythingIsRecorded(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		props func(*testing.T) records.MedicationListProps
		rows  int
	}{
		{
			name: "an account with records",
			props: func(t *testing.T) records.MedicationListProps {
				return listProps(t,
					view(t, seeded(t, seed.NameOnlyID)),
					view(t, everyFieldFilledIn(t)))
			},
			rows: 2,
		},
		{
			name:  "an account with none",
			props: func(t *testing.T) records.MedicationListProps { return listProps(t) },
			rows:  0,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, records.MedicationList(testCase.props(t)), "div")

			region := tree.One(t, viewstest.Region(medicationRegion))
			assert.Equal(t, ids.RecordList(kind.Medication), viewstest.Attr(region, "id"))
			assert.NotEmpty(t, viewstest.Elements(region),
				"contracts/pages.md assertion 3: the landmark is present AND non-empty")

			// The header row is a <tr> too, so the count is over the rows that
			// carry a record's state and not over every row in the table.
			assert.Len(t, viewstest.Find(region, viewstest.And(
				viewstest.Tag("tr"), viewstest.HasAttr("data-status"))), testCase.rows)
		})
	}
}

// FR-029: the empty state offers the action to record the first one, and it is
// inside the region rather than a bare paragraph where the region should be.
func TestTheEmptyStateSitsInsideTheRegionAndOffersTheCreateAction(t *testing.T) {
	t.Parallel()

	props := listProps(t)
	tree := viewstest.Render(t, records.MedicationList(props), "div")

	region := tree.One(t, viewstest.Region(medicationRegion))
	empty := tree.One(t, viewstest.WithID(ids.RecordEmpty(kind.Medication)))

	require.True(t, viewstest.Descends(region, empty),
		"the empty state renders instead of the landmark rather than inside it")
	assert.NotEmpty(t, viewstest.Text(empty), "the empty state explains nothing")

	create := viewstest.Find(region, viewstest.And(
		viewstest.Tag("a"), viewstest.WithAttr("href", props.CreateHref)))
	assert.NotEmpty(t, create, "FR-029 requires the empty state to offer the action to record the first one")
	assert.True(t, viewstest.Descends(empty, create[len(create)-1]),
		"the create action is somewhere in the region but not in the empty state")
}

// The same action is reachable when the list is populated, because FR-029 asks
// for the same page structure in both states.
func TestTheCreateActionIsInTheRegionWhenThereAreRecords(t *testing.T) {
	t.Parallel()

	props := listProps(t, view(t, everyFieldFilledIn(t)))
	tree := viewstest.Render(t, records.MedicationList(props), "div")

	region := tree.One(t, viewstest.Region(medicationRegion))

	assert.NotEmpty(t, viewstest.Find(region, viewstest.And(
		viewstest.Tag("a"), viewstest.WithAttr("href", props.CreateHref))))
	assert.Empty(t, tree.All(viewstest.WithID(ids.RecordEmpty(kind.Medication))),
		"the empty state renders over a populated list")
}

// Every row is patchable by the id the stream will address, and they sit in the
// one container a newly created record is patched into.
func TestEveryRowIsInsideThePatchContainerUnderItsOwnID(t *testing.T) {
	t.Parallel()

	medications := []records.MedicationView{
		view(t, seeded(t, seed.NameOnlyID)),
		view(t, seeded(t, seed.ScriptedNameID)),
		view(t, everyFieldFilledIn(t)),
	}

	tree := viewstest.Render(t, records.MedicationList(listProps(t, medications...)), "div")
	container := tree.One(t, viewstest.WithID(ids.RecordRows(kind.Medication)))

	for _, medication := range medications {
		rowID := ids.RecordRow(kind.Medication, medication.ID)
		row := tree.One(t, viewstest.WithID(rowID))
		assert.Truef(t, viewstest.Descends(container, row), "%s is not inside the patch container", rowID)
	}
}

// The pager is rendered even with nowhere to go, for the reason #error-banner
// and #toast are: Datastar patches by id and an element that does not exist
// cannot be patched.
func TestThePagerIsRenderedEvenWithNoNextPage(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, records.MedicationList(listProps(t)), "div")

	assert.Len(t, tree.All(viewstest.WithID(ids.RecordPager(kind.Medication))), 1)
}

// T254, FR-048. A future full-region patch replaces this whole landmark, and
// the heading is where focus has to land when it does: tabindex="-1" makes it
// programmatically focusable and autofocus is what the browser's own
// connected-element algorithm honours, whether the element arrived by parsing
// or by a later patch — no inline script, which the CSP bans, and no signal
// machinery the free Datastar bundle does not have.
func TestTheHeadingCarriesTheFocusMechanism(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, records.MedicationList(listProps(t)), "div")
	heading := tree.One(t, viewstest.WithID(ids.RecordListHeading(kind.Medication)))

	require.Equal(t, "h1", heading.Data)
	assert.Equal(t, "-1", viewstest.Attr(heading, "tabindex"))
	assert.True(t, viewstest.HasAttr("autofocus")(heading))
}
