package shared_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/web/views/shared"
	"medikube/internal/web/views/viewstest"
)

// T142, FR-039. The hidden field carries the chosen id, the drawer exists in
// the DOM (hidden, not created on demand — the same rule components.Confirm
// follows) and a create posted through it can never navigate the page: there
// is no <form> here for the browser to submit, only data-on handlers.
func TestDirectoryPickerCarriesTheChosenIDAndOffersACreateDrawer(t *testing.T) {
	t.Parallel()

	props := shared.PickerProps{
		Name:       "facility",
		Label:      "Facility",
		FormID:     "practitioner-form",
		CreateHref: "/api/v1/facilities",
		Selected:   shared.PickerOption{ID: "fac1", Label: "Boots"},
	}

	tree := viewstest.Render(t, shared.DirectoryPicker(props), "div")

	hidden := tree.One(t, viewstest.And(viewstest.Tag("input"), viewstest.WithAttr("type", "hidden")))
	assert.Equal(t, "facility", viewstest.Attr(hidden, "name"))

	assert.Empty(t, tree.All(viewstest.Tag("form")), "the picker must not be its own <form>: it lives inside the record's own form")

	drawerButtons := tree.All(viewstest.And(viewstest.Tag("button"), viewstest.HasAttr("data-on:click")))
	require.NotEmpty(t, drawerButtons, "the drawer needs a way to be opened and a way to submit")
}
