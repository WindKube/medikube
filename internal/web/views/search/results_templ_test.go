package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/domain/kind"
	"medikube/internal/web/views/search"
	"medikube/internal/web/views/viewstest"
)

// T171, contracts/pages.md P: /search's landmark is a bare <search> element,
// not region[name="…"] like every other page (research: it carries the ARIA
// role natively and has no accessible name of its own).
func searchLandmark() viewstest.Matcher {
	return viewstest.And(viewstest.Tag("search"), viewstest.WithID("search-page"))
}

func TestTheLandmarkIsPresent(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, search.Results(search.Props{FormAction: "/search", NoTerm: true}), "div")

	tree.One(t, searchLandmark())
}

// US8 scenario 2: a person is named but nothing has been typed yet — no
// results, no "nothing matched" claim either.
func TestNoTermStateReadsAsAnInvitationNotAFailure(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, search.Results(search.Props{FormAction: "/search", PatientID: "pat1", NoTerm: true}), "div")

	landmark := tree.One(t, searchLandmark())
	text := viewstest.Text(landmark)

	assert.Contains(t, text, "Type a term")
	assert.NotContains(t, text, "Nothing matched")
}

// The two empty states US8 scenario 2 requires read differently: a person
// with no records at all, versus a term that matched nothing of theirs.
func TestTheTwoEmptyStatesAreVisiblyDistinct(t *testing.T) {
	t.Parallel()

	noRecords := viewstest.Render(t, search.Results(search.Props{
		FormAction: "/search", PatientID: "pat1", EmptyReason: "no_records",
	}), "div")
	noMatches := viewstest.Render(t, search.Results(search.Props{
		FormAction: "/search", PatientID: "pat1", EmptyReason: "no_matches",
	}), "div")

	noRecordsText := viewstest.Text(noRecords.One(t, searchLandmark()))
	noMatchesText := viewstest.Text(noMatches.One(t, searchLandmark()))

	assert.Contains(t, noRecordsText, "Nothing recorded yet")
	assert.Contains(t, noMatchesText, "Nothing matched")
	assert.NotEqual(t, noRecordsText, noMatchesText)
}

// A per-group load-more link is present only when its own group has more, and
// carries that group's own href — not another group's.
func TestPerGroupLoadMoreIsIndependent(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, search.Results(search.Props{
		FormAction: "/search", PatientID: "pat1",
		Groups: []search.Group{
			{Kind: kind.Medication.Enum(), Label: "Medications", Items: []search.Item{{Title: "Warfarin", Href: "/x"}}, LoadMoreHref: "/search?cursor=" + kind.Medication.Enum() + ":tok"},
			{Kind: kind.Allergy.Enum(), Label: "Allergy records", Items: []search.Item{{Title: "Warfarin allergy", Href: "/y"}}},
		},
	}), "div")

	landmark := tree.One(t, searchLandmark())
	links := viewstest.Find(landmark, viewstest.WithAttr("href", "/search?cursor="+kind.Medication.Enum()+":tok"))
	assert.Len(t, links, 1)

	for _, section := range viewstest.Find(landmark, viewstest.Tag("section")) {
		if viewstest.Attr(section, "aria-label") == "Allergy records" {
			assert.NotContains(t, viewstest.Text(section), "Load more", "that group has no more pages and must carry no load-more link")
		}
	}
}

// Removable narrowing chips: each carries the href that drops it, and the
// chips are absent when nothing is narrowing the search.
func TestNarrowingChipsAreRemovable(t *testing.T) {
	t.Parallel()

	withChips := viewstest.Render(t, search.Results(search.Props{
		FormAction: "/search", PatientID: "pat1", NoTerm: false, EmptyReason: "no_matches",
		Chips: []search.Chip{{Label: "Medications only", Href: "/search?patient=pat1&q=warfarin"}},
	}), "div")

	chip := withChips.One(t, viewstest.WithAttr("href", "/search?patient=pat1&q=warfarin"))
	assert.Contains(t, viewstest.Text(chip), "Medications only")

	withoutChips := viewstest.Render(t, search.Results(search.Props{
		FormAction: "/search", PatientID: "pat1", NoTerm: true,
	}), "div")

	assert.Empty(t, withoutChips.All(viewstest.WithAttr("aria-label", "Narrowing")))
}
