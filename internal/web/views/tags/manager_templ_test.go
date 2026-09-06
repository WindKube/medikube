package tags_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/tags"
	"medikube/internal/web/views/viewstest"
)

const tagsRegion = "Tags"

func TestManagerRegionIsPresentPopulatedAndEmpty(t *testing.T) {
	t.Parallel()

	populated := tags.ManagerProps{
		Tags: []tags.TagView{
			{ID: "t1", Name: "cardiology", Color: "#aa3311", UsageCount: 3, Links: tags.Links{Record: "/api/v1/tags/t1"}},
		},
		CreateHref: "/api/v1/tags",
	}
	empty := tags.ManagerProps{CreateHref: "/api/v1/tags"}

	t.Run("populated", func(t *testing.T) {
		t.Parallel()

		tree := viewstest.Render(t, tags.Manager(populated), "div")
		region := tree.One(t, viewstest.Region(tagsRegion))
		assert.Equal(t, ids.DirectoryList(tags.Segment), viewstest.Attr(region, "id"))
		assert.NotEmpty(t, viewstest.Find(region, viewstest.Tag("li")))
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		tree := viewstest.Render(t, tags.Manager(empty), "div")
		region := tree.One(t, viewstest.Region(tagsRegion))
		emptyState := tree.One(t, viewstest.WithID(ids.DirectoryEmpty(tags.Segment)))

		require.True(t, viewstest.Descends(region, emptyState),
			"the empty state must render inside the landmark, not instead of it")
	})
}

// FR-066, US7-4: the delete confirmation states how many records carry the
// tag before it may be confirmed — the count is in the confirmation's own
// text, not only inferable from elsewhere on the page.
func TestDeleteConfirmationStatesTheUsageCountBeforeConfirming(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		usage int
		want  string
	}{
		{name: "none", usage: 0, want: "0 records carry this tag"},
		{name: "one", usage: 1, want: "1 record carries this tag"},
		{name: "many", usage: 37, want: "37 records carry this tag"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			view := tags.TagView{ID: "t1", Name: "cardiology", UsageCount: test.usage, Links: tags.Links{Record: "/api/v1/tags/t1"}}
			tree := viewstest.Render(t, tags.Row(view), "ul")

			confirm := tree.One(t, viewstest.WithID(ids.DirectoryConfirm(tags.Segment, view.ID)))
			assert.Contains(t, viewstest.Text(confirm), test.want)

			var found bool
			for _, button := range viewstest.Find(confirm, viewstest.Tag("button")) {
				if strings.Contains(viewstest.Attr(button, "data-on:click"), view.Links.Record) {
					found = true
				}
			}
			assert.True(t, found, "the confirm button must target this tag's own record")
		})
	}
}
