package directory_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/web/views/directory"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/viewstest"
)

const practitionersRegion = "Practitioners"

// T131, FR-029's rule applied to the directory: the landmark is present
// whether or not anything is recorded, and the empty state renders inside it.
func TestPractitionerListRegionIsPresentPopulatedAndEmpty(t *testing.T) {
	t.Parallel()

	populated := directory.PractitionerListProps{
		Practitioners: []directory.PractitionerView{{ID: "p1", Name: "Dr Amara"}},
		CreateHref:    "/practitioners/new",
	}
	empty := directory.PractitionerListProps{CreateHref: "/practitioners/new"}

	t.Run("populated", func(t *testing.T) {
		t.Parallel()

		tree := viewstest.Render(t, directory.PractitionerList(populated), "div")
		region := tree.One(t, viewstest.Region(practitionersRegion))
		assert.Equal(t, ids.DirectoryList(directory.PractitionerSegment), viewstest.Attr(region, "id"))
		assert.NotEmpty(t, viewstest.Find(region, viewstest.Tag("tr")))
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		tree := viewstest.Render(t, directory.PractitionerList(empty), "div")
		region := tree.One(t, viewstest.Region(practitionersRegion))
		emptyState := tree.One(t, viewstest.WithID(ids.DirectoryEmpty(directory.PractitionerSegment)))

		require.True(t, viewstest.Descends(region, emptyState),
			"the empty state must render inside the landmark, not instead of it")
		assert.NotEmpty(t, viewstest.Text(emptyState))
	})
}
