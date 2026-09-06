package tags_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/web/views/tags"
	"medikube/internal/web/views/viewstest"
)

func fieldOptions() []tags.TagView {
	return []tags.TagView{
		{ID: "t1", Name: "cardiology", Color: "#aa3311", UsageCount: 12},
		{ID: "t2", Name: "allergy", Color: "#112233", UsageCount: 0},
	}
}

// FR-064: an untagged record's field opens with no chips and every option
// offered as a suggestion.
func TestFieldEmptyOffersEveryOptionAndNoChips(t *testing.T) {
	t.Parallel()

	props := tags.FieldProps{FormID: "record-form", Options: fieldOptions()}

	tree := viewstest.Render(t, tags.Field(props), "div")

	container := tree.One(t, viewstest.WithID("record-form-field-tags"))
	assert.Equal(t, "{tags: []}", viewstest.Attr(container, "data-signals"))

	suggestions := tree.All(viewstest.WithAttr("role", "option"))
	require.Len(t, suggestions, 2)

	for _, suggestion := range suggestions {
		assert.Contains(t, viewstest.Attr(suggestion, "data-show"), "!$tags.includes(")
	}
}

// FR-064: a record carrying tags seeds $tags with them, so every already
// applied tag renders as a removable chip.
func TestFieldWithSelectionsSeedsSignalAndChips(t *testing.T) {
	t.Parallel()

	props := tags.FieldProps{FormID: "record-form", Options: fieldOptions(), Selected: []string{"t1"}}

	tree := viewstest.Render(t, tags.Field(props), "div")

	container := tree.One(t, viewstest.WithID("record-form-field-tags"))
	assert.Equal(t, "{tags: [\"t1\"]}", viewstest.Attr(container, "data-signals"))

	removeButtons := tree.All(viewstest.WithAttr("aria-label", "Remove cardiology"))
	require.Len(t, removeButtons, 1)
	assert.Contains(t, viewstest.Attr(removeButtons[0], "data-on:click"), "$tags.filter(")
}

// FR-068: every suggestion narrows against the picker's own query signal,
// distinct from the submitted `tags` array.
func TestFieldSuggestionsNarrowAgainstOwnQuerySignal(t *testing.T) {
	t.Parallel()

	props := tags.FieldProps{FormID: "record-form", Options: fieldOptions()}

	tree := viewstest.Render(t, tags.Field(props), "div")

	input := tree.One(t, viewstest.WithID("record-form-field-tags-input"))
	assert.Equal(t, "_tags_query", viewstest.Attr(input, "data-bind"))

	suggestions := tree.All(viewstest.WithAttr("role", "option"))
	require.Len(t, suggestions, 2)

	for _, suggestion := range suggestions {
		assert.Contains(t, viewstest.Attr(suggestion, "data-show"), "_tags_query")
		assert.Contains(t, viewstest.Attr(suggestion, "data-on:click"), "$_tags_query = ''")
	}
}
