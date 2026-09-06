package records_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/web/views/records"
	"medikube/internal/web/views/viewstest"
)

// T142, FR-059. Every related record renders its kind, an identifying
// summary and an openable link — never a bare id.
func TestLinkedRecordsRendersKindSummaryAndAnOpenableLink(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, records.LinkedRecords("links", "Linked records", []records.LinkedRecordItem{
		{Kind: "condition", Summary: "Type 2 diabetes", Href: "/" + kind.Condition.Collection() + "/cnd1"},
	}), "div")

	section := tree.One(t, viewstest.Region("Linked records"))

	text := viewstest.Text(section)
	assert.Contains(t, text, "condition")
	assert.Contains(t, text, "Type 2 diabetes")

	anchor := tree.One(t, viewstest.Tag("a"))
	assert.Equal(t, "/"+kind.Condition.Collection()+"/cnd1", viewstest.Attr(anchor, "href"))
	assert.Equal(t, "Type 2 diabetes", viewstest.Text(anchor))
}

// The anti-vacuity control: an empty set renders an empty state rather than
// nothing at all, so a page that forgot to populate the items still shows a
// landmark a render test — or a person — can tell apart from a bug.
func TestLinkedRecordsRendersAnEmptyStateWithNoItems(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, records.LinkedRecords("links", "Linked records", nil), "div")

	section := tree.One(t, viewstest.Region("Linked records"))
	require.Empty(t, tree.All(viewstest.Tag("a")))
	assert.NotEmpty(t, viewstest.Text(section))
}
