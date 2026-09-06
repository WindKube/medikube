package timeline_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/web/views/shared"
	"medikube/internal/web/views/timeline"
	"medikube/internal/web/views/viewstest"
)

const timelineRegion = "Timeline"

func TestChoosePersonStateRendersInsteadOfAnyGroup(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, timeline.Timeline(timeline.Props{ChoosePerson: true}), "div")

	region := tree.One(t, viewstest.Region(timelineRegion))
	assert.Contains(t, viewstest.Text(region), "Choose a person")
	assert.Empty(t, viewstest.Find(region, viewstest.WithID("timeline-groups")))
}

func TestGroupsRenderEachEntrysKindTitleAndDateAndTheUndatedGroupIsLabelled(t *testing.T) {
	t.Parallel()

	props := timeline.Props{
		Groups: []timeline.Group{
			{Label: "2025-08-20", Entries: []timeline.EntryView{
				{ID: "row-1", Kind: "injury", Title: "Sprained ankle", Href: "/" + kind.Injury.Segment() + "/mkinjamara00001"},
			}},
			{Label: "Date not recorded", Entries: []timeline.EntryView{
				{ID: "row-2", Kind: "condition", Title: "Chronic asthma", Href: "/" + kind.Condition.Segment() + "/x"},
			}},
		},
	}

	tree := viewstest.Render(t, timeline.Timeline(props), "div")
	region := tree.One(t, viewstest.Region(timelineRegion))

	dated := tree.One(t, viewstest.WithID("row-1"))
	require.True(t, viewstest.Descends(region, dated))
	assert.Contains(t, viewstest.Text(dated), "injury")
	assert.Contains(t, viewstest.Text(dated), "Sprained ankle")

	undated := tree.One(t, viewstest.WithID("row-2"))
	require.True(t, viewstest.Descends(region, undated))

	assert.Contains(t, viewstest.Text(region), "Date not recorded")
}

// FR-071/FR-073: a narrowing in force is visible and each one removable, not
// only inferable from which rows showed up.
func TestNarrowingChipsRenderRemovable(t *testing.T) {
	t.Parallel()

	props := timeline.Props{
		Criteria: shared.CriteriaProps{
			ID: "timeline-criteria",
			Chips: []shared.CriteriaChip{
				{ID: "chip-kind", Label: "Injury", ClearOn: "@get('/timeline?patient=p1')"},
			},
		},
		Empty: &shared.EmptyStateProps{ID: "timeline-empty", TitleID: "empty.nothing_matches"},
	}

	tree := viewstest.Render(t, timeline.Timeline(props), "div")

	chip := tree.One(t, viewstest.WithID("chip-kind"))
	assert.Contains(t, viewstest.Text(chip), "Injury")

	buttons := viewstest.Find(chip, viewstest.Tag("button"))
	require.Len(t, buttons, 1)
	assert.Equal(t, "@get('/timeline?patient=p1')", viewstest.Attr(buttons[0], "data-on:click"))
}

func TestEmptyStateRendersInsteadOfGroupsWhenSet(t *testing.T) {
	t.Parallel()

	props := timeline.Props{Empty: &shared.EmptyStateProps{ID: "timeline-empty", TitleID: "empty.nothing_matches"}}

	tree := viewstest.Render(t, timeline.Timeline(props), "div")
	region := tree.One(t, viewstest.Region(timelineRegion))

	empty := tree.One(t, viewstest.WithID("timeline-empty"))
	require.True(t, viewstest.Descends(region, empty))
	assert.Empty(t, viewstest.Find(region, viewstest.WithID("timeline-groups")))
}
