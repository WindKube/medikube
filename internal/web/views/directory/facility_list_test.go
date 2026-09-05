package directory_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/web/views/directory"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/viewstest"
)

const facilitiesRegion = "Facilities"

// T131, the mirror of the practitioner test.
func TestFacilityListRegionIsPresentPopulatedAndEmpty(t *testing.T) {
	t.Parallel()

	populated := directory.FacilityListProps{
		Facilities: []directory.FacilityView{{ID: "f1", Name: "Boots", Kind: "Pharmacy"}},
		CreateHref: "/facilities/new",
	}
	empty := directory.FacilityListProps{CreateHref: "/facilities/new"}

	t.Run("populated", func(t *testing.T) {
		t.Parallel()

		tree := viewstest.Render(t, directory.FacilityList(populated), "div")
		region := tree.One(t, viewstest.Region(facilitiesRegion))
		assert.Equal(t, ids.DirectoryList(directory.FacilitySegment), viewstest.Attr(region, "id"))
		assert.NotEmpty(t, viewstest.Find(region, viewstest.Tag("tr")))
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		tree := viewstest.Render(t, directory.FacilityList(empty), "div")
		region := tree.One(t, viewstest.Region(facilitiesRegion))
		emptyState := tree.One(t, viewstest.WithID(ids.DirectoryEmpty(directory.FacilitySegment)))

		require.True(t, viewstest.Descends(region, emptyState),
			"the empty state must render inside the landmark, not instead of it")
		assert.NotEmpty(t, viewstest.Text(emptyState))
	})
}
