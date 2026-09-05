package shared_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/web/views/shared"
	"medikube/internal/web/views/viewstest"
)

// FR-008: the two empty states read differently, and only one of them offers
// to clear a narrowing.
func TestEmptyStateNothingRecordedOffersNoClear(t *testing.T) {
	t.Parallel()

	props := shared.NothingRecorded("row-empty", "widgets")
	tree := viewstest.Render(t, shared.EmptyState(props), "div")

	root := tree.One(t, viewstest.Tag("div"))
	assert.Equal(t, "row-empty", viewstest.Attr(root, "id"))
	assert.Empty(t, tree.All(viewstest.Tag("button")), "nothing recorded has no narrowing to clear")
}

func TestEmptyStateNothingMatchesOffersToClear(t *testing.T) {
	t.Parallel()

	props := shared.NothingMatches("row-empty", "widgets", "$rowSearch = ''")
	tree := viewstest.Render(t, shared.EmptyState(props), "div")

	clear := tree.One(t, viewstest.And(viewstest.Tag("button"), viewstest.HasAttr("data-on:click")))
	assert.Equal(t, "$rowSearch = ''", viewstest.Attr(clear, "data-on:click"))
}

// FR-006: the confirmation names the subject and states the reference count,
// including the zero case.
func TestDeleteConfirmStatesWhatIsDestroyedAndHowManyRecordsReferToIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		count int
		want  string
	}{
		{name: "none", count: 0, want: "No other record refers to it"},
		{name: "one", count: 1, want: "One other record refers to it"},
		{name: "several", count: 3, want: "3 other records refer to it"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			props := shared.DeleteConfirmProps{
				ID:             "medication-confirm-abc",
				Signal:         "confirmDelete",
				Title:          "Delete medication",
				Subject:        "Amoxicillin",
				ReferenceCount: test.count,
				ConfirmLabel:   "Delete",
				ConfirmOn:      "@delete('/api/v1/records/medication/abc')",
				CancelLabel:    "Cancel",
			}

			tree := viewstest.Render(t, shared.DeleteConfirm(props), "section")

			section := tree.One(t, viewstest.Tag("section"))
			assert.Equal(t, "medication-confirm-abc", viewstest.Attr(section, "id"))
			assert.Equal(t, "$confirmDelete", viewstest.Attr(section, "data-show"))
			assert.Contains(t, viewstest.Text(section), "Amoxicillin")
			assert.Contains(t, viewstest.Text(section), test.want)

			confirmButton := tree.One(t, viewstest.And(viewstest.Tag("button"), viewstest.WithAttr("data-on:click", props.ConfirmOn)))
			require.NotNil(t, confirmButton)
		})
	}
}

// FR-026, FR-078: every row states the basis it was selected on, and can
// carry more than one.
func TestBasisRendersEveryLabel(t *testing.T) {
	t.Parallel()

	props := shared.BasisProps{ID: "condition-basis-abc", Labels: []string{"Active", "Scheduled"}}
	tree := viewstest.Render(t, shared.Basis(props), "ul")

	items := tree.All(viewstest.Tag("li"))
	require.Len(t, items, 2)
	list := tree.One(t, viewstest.Tag("ul"))
	assert.Contains(t, viewstest.Text(list), "Active")
	assert.Contains(t, viewstest.Text(list), "Scheduled")
}

// FR-071, FR-073: the narrowing in force is visible and each criterion is
// individually removable.
func TestCriteriaEachChipClearsItselfAlone(t *testing.T) {
	t.Parallel()

	props := shared.CriteriaProps{
		ID: "search-criteria",
		Chips: []shared.CriteriaChip{
			{ID: "search-criteria-0", Label: "kind: medication", ClearOn: "@get('/api/v1/search?kind=')"},
			{ID: "search-criteria-1", Label: "tag: chronic", ClearOn: "@get('/api/v1/search?tag=')"},
		},
	}

	tree := viewstest.Render(t, shared.Criteria(props), "ul")

	list := tree.One(t, viewstest.Tag("ul"))
	assert.Equal(t, "search-criteria", viewstest.Attr(list, "id"))

	items := tree.All(viewstest.Tag("li"))
	require.Len(t, items, 2)

	for i, chip := range props.Chips {
		button := tree.One(t, viewstest.And(viewstest.Tag("button"), viewstest.WithAttr("data-on:click", chip.ClearOn)))
		assert.Equal(t, chip.ClearOn, viewstest.Attr(button, "data-on:click"), "chip %d", i)
	}
}
