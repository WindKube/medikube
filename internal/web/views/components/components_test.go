package components_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/web/views/components"
	"medikube/internal/web/views/viewstest"
)

// FR-029. The empty state is a page's explanation of why it is empty plus the
// action that fills it, and it is rendered inside its landmark by whoever calls
// it — this asserts the piece, and medication_list_test.go asserts the place.
func TestTheEmptyStateExplainsAndOffersAnAction(t *testing.T) {
	t.Parallel()

	props := components.EmptyStateProps{
		ID:          "medication-empty",
		Title:       "Nothing recorded yet",
		Body:        "Record the first one and it will appear here.",
		ActionHref:  "/x/new",
		ActionLabel: "Record a medication",
	}

	tree := viewstest.Render(t, components.EmptyState(props), "div")

	state := tree.One(t, viewstest.WithID(props.ID))
	assert.Contains(t, viewstest.Text(state), props.Title)
	assert.Contains(t, viewstest.Text(state), props.Body)

	action := tree.One(t, viewstest.Tag("a"))
	assert.Equal(t, props.ActionHref, viewstest.Attr(action, "href"))
	assert.Equal(t, props.ActionLabel, viewstest.Text(action))
}

// An empty state with nowhere to go is a dead end. It is still rendered,
// because the explanation is the point and a page that offers no action is a
// legitimate caller — but the link is not rendered empty.
func TestTheEmptyStateOmitsTheActionWhenThereIsNone(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, components.EmptyState(components.EmptyStateProps{
		ID: "x", Title: "t", Body: "b",
	}), "div")

	assert.Empty(t, tree.All(viewstest.Tag("a")))
}

func TestTheFieldErrorRendersNothingWhenThereIsNothingToSay(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, components.FieldError("x-error-name", nil), "div")

	assert.Empty(t, tree.Markup,
		"an empty message element would still be announced by aria-describedby")
}

// Every refusal for a field arrives in one element, because aria-describedby
// names one id and a second message rendered outside it is a message nobody
// hears.
func TestTheFieldErrorCarriesEveryMessageForItsField(t *testing.T) {
	t.Parallel()

	messages := []string{"a name is required", "the name accepts at most 200 characters"}
	tree := viewstest.Render(t, components.FieldError("x-error-name", messages), "div")

	element := tree.One(t, viewstest.WithID("x-error-name"))
	for _, message := range messages {
		assert.Contains(t, viewstest.Text(element), message)
	}
	assert.Equal(t, "alert", viewstest.Attr(element, "role"),
		"FR-047 requires the failure to be announced, not only shown")
}

// The pager is rendered whether or not there is anywhere to go: Datastar
// patches by id, and an element that does not exist cannot be patched.
func TestThePaginationIsAlwaysAnElementAndOnlySometimesALink(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		props components.PaginationProps
		links int
	}{
		{name: "no pages either way", props: components.PaginationProps{ID: "p"}, links: 0},
		{name: "a next page", props: components.PaginationProps{ID: "p", NextHref: "/x?cursor=a"}, links: 1},
		{
			name:  "both ways",
			props: components.PaginationProps{ID: "p", PreviousHref: "/x", NextHref: "/x?cursor=a"},
			links: 2,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, components.Pagination(testCase.props), "div")

			pager := tree.One(t, viewstest.WithID(testCase.props.ID))
			require.Equal(t, "nav", pager.Data)
			assert.NotEmpty(t, viewstest.Attr(pager, "aria-label"),
				"a second navigation landmark with no name is indistinguishable from the primary one")
			assert.Len(t, viewstest.Find(pager, viewstest.Tag("a")), testCase.links)
		})
	}
}
