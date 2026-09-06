package overview_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/web/views/overview"
	"medikube/internal/web/views/viewstest"
)

func TestOverviewRendersTheLandmarkAndBothLinks(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, overview.Overview(overview.Props{
		MedicationCount:  3,
		MedicationsLabel: "meds",
		MedicationsHref:  "/meds",
		SettingsHref:     "/settings",
	}), "div")

	tree.One(t, viewstest.Region("Overview"))
	assert.Len(t, tree.All(viewstest.WithAttr("href", "/meds")), 1)
	assert.Len(t, tree.All(viewstest.WithAttr("href", "/settings")), 1)
	assert.Contains(t, tree.Markup, "3 meds")
}

func TestOverviewSaysNothingRecordedWhenEmpty(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, overview.Overview(overview.Props{
		MedicationsLabel: "meds",
	}), "div")

	assert.Contains(t, tree.Markup, "not recorded any meds yet")
}
