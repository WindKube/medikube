package tags_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/web/views/tags"
	"medikube/internal/web/views/viewstest"
)

// FR-068: the picker offers matching tags as the user types, each carrying
// its own derived usage count.
func TestPickerOffersEveryOptionWithItsUsageCount(t *testing.T) {
	t.Parallel()

	props := tags.PickerProps{
		ID:     "picker1",
		Label:  "Tags",
		Signal: "tagQuery",
		Options: []tags.TagView{
			{ID: "t1", Name: "cardiology", Color: "#aa3311", UsageCount: 12},
			{ID: "t2", Name: "allergy", Color: "#112233", UsageCount: 0},
		},
	}

	tree := viewstest.Render(t, tags.Picker(props), "div")

	input := tree.One(t, viewstest.WithID("picker1-input"))
	assert.Equal(t, "tagQuery", viewstest.Attr(input, "data-bind"))

	options := tree.All(viewstest.WithAttr("role", "option"))
	require.Len(t, options, 2)

	for _, option := range options {
		id := viewstest.Attr(option, "data-tag-id")
		text := viewstest.Text(option)

		switch id {
		case "t1":
			assert.Contains(t, text, "cardiology")
			assert.Contains(t, text, "12")
		case "t2":
			assert.Contains(t, text, "allergy")
			assert.Contains(t, text, "0")
		default:
			t.Fatalf("unexpected option %q", id)
		}

		assert.Contains(t, viewstest.Attr(option, "data-show"), "tagQuery")
	}
}
