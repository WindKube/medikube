package components_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/web/views/components"
	"medikube/internal/web/views/viewstest"
)

// The landmark contracts/pages.md publishes for the destructive-action
// confirmation. It is a Playwright selector and a literal on purpose.
const confirmRegion = "Confirm delete"

func confirmProps() components.ConfirmProps {
	return components.ConfirmProps{
		ID:           "confirm-under-test",
		Signal:       "confirming_delete",
		Title:        "Delete this medication?",
		Subject:      `مسكن <b>alpha</b> & "strong"`,
		Consequence:  "This is permanent. There is no undo and no recycle bin.",
		ConfirmLabel: "Delete permanently",
		ConfirmOn:    "@delete('/api/v1/records/x/y', {headers: {'If-Match': $etag}})",
		CancelLabel:  "Keep it",
	}
}

// T153, FR-028, contracts/pages.md. The confirmation is a rendered element with
// its own landmark. A window.confirm is invisible to the render gate and to the
// smoke run, so a page that used one would pass every test in this repository
// while deleting a person's record on a single click.
func TestTheDeleteConfirmationIsARenderedLandmark(t *testing.T) {
	t.Parallel()

	props := confirmProps()
	tree := viewstest.Render(t, components.Confirm(props), "div")

	region := tree.One(t, viewstest.Region(confirmRegion))
	assert.Equal(t, props.ID, viewstest.Attr(region, "id"))
	assert.NotEmpty(t, viewstest.Elements(region))
}

// FR-028: the confirmation names the medication and says the deletion cannot be
// undone, before the request is made.
func TestTheConfirmationNamesTheSubjectAndItsConsequence(t *testing.T) {
	t.Parallel()

	props := confirmProps()
	tree := viewstest.Render(t, components.Confirm(props), "div")

	region := tree.One(t, viewstest.Region(confirmRegion))
	text := viewstest.Text(region)

	assert.Contains(t, text, props.Subject, "FR-028 requires the confirmation to name the medication")
	assert.Contains(t, text, props.Consequence)
	assert.Contains(t, text, props.Title)
}

// The seeded name is right-to-left text mixed with a tag, an ampersand and a
// quote. It is in the fixture so that an unescaped template renders an element
// somewhere and is caught here rather than in a browser.
func TestASubjectThatLooksLikeMarkupRendersAsText(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, components.Confirm(confirmProps()), "div")
	region := tree.One(t, viewstest.Region(confirmRegion))

	assert.Empty(t, viewstest.Find(region, viewstest.Tag("b")))
	assert.Empty(t, viewstest.Find(region, viewstest.Tag("script")))
}

// Both actions are real controls: the confirmation is only useful if the person
// can also decline it, and both have to be reachable by keyboard (FR-048).
func TestTheConfirmationOffersBothAnswers(t *testing.T) {
	t.Parallel()

	props := confirmProps()
	tree := viewstest.Render(t, components.Confirm(props), "div")
	region := tree.One(t, viewstest.Region(confirmRegion))

	buttons := viewstest.Find(region, viewstest.Tag("button"))
	require.Len(t, buttons, 2)

	labels := []string{viewstest.Text(buttons[0]), viewstest.Text(buttons[1])}
	assert.Contains(t, labels, props.ConfirmLabel)
	assert.Contains(t, labels, props.CancelLabel)

	for _, button := range buttons {
		assert.Equal(t, "button", viewstest.Attr(button, "type"),
			"a button inside a form defaults to submit and would save instead of confirming")
	}

	confirmed := viewstest.Find(region, viewstest.WithAttr("data-on:click", props.ConfirmOn))
	assert.Len(t, confirmed, 1)
}

// The confirmation is rendered with the page and revealed by a signal, so it
// exists in the DOM for the render gate to see whether or not it is on screen.
func TestTheConfirmationIsRevealedBySignalRatherThanCreatedOnDemand(t *testing.T) {
	t.Parallel()

	props := confirmProps()
	tree := viewstest.Render(t, components.Confirm(props), "div")

	region := tree.One(t, viewstest.Region(confirmRegion))
	assert.Equal(t, "$"+props.Signal, viewstest.Attr(region, "data-show"))
}
