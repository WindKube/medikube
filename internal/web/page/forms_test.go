package page_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
	"medikube/internal/testsupport"
	"medikube/internal/testsupport/seed"
	"medikube/internal/web/page"
	"medikube/internal/web/views/directory"
	"medikube/internal/web/views/ids"
)

// routePath is one operation's registered path, read out of the route table
// the way every other page test in this package does (pageRoutes above).
func routePath(t *testing.T, opID string) string {
	t.Helper()

	for _, route := range httproute.Inventory().Routes() {
		if route.OpID == opID {
			return route.Path
		}
	}

	require.FailNowf(t, "no such route", "the route table has no %s", opID)

	return ""
}

// The bug this covers: patients, practitioners and facilities render an "Add
// a person/practitioner/facility" link pointing at `#<form id>`, and until
// now nothing on the page carried that id — the form was never rendered.

func TestThePatientListPageRendersTheCreateForm(t *testing.T) {
	t.Parallel()

	_, _, body := newBrowser(t).get(routePath(t, page.OpPatientListPage))

	assert.Contains(t, body, attribute("id", ids.PatientForm("")),
		"the list page's create link targets #%s, but no form on the page carries that id", ids.PatientForm(""))
}

func TestThePatientDetailPageRendersThePrefilledEditForm(t *testing.T) {
	t.Parallel()

	url := strings.ReplaceAll(routePath(t, page.OpPatientDetailPage), "{patientId}", testsupport.AccountAPatientSelfID)

	_, _, body := newBrowser(t).get(url)

	formID := ids.PatientForm(testsupport.AccountAPatientSelfID)
	assert.Contains(t, body, attribute("id", formID),
		"the detail page's edit link targets #%s, but no form on the page carries that id", formID)
	assert.Contains(t, body, `value="Amara"`, "the edit form is not pre-filled with the patient's own values")
}

func TestThePractitionerListPageRendersTheCreateForm(t *testing.T) {
	t.Parallel()

	_, _, body := newBrowser(t).get(routePath(t, page.OpPractitionerListPage))

	formID := ids.DirectoryForm(directory.PractitionerSegment, "")
	assert.Contains(t, body, attribute("id", formID),
		"the list page's create link targets #%s, but no form on the page carries that id", formID)
}

func TestThePractitionerDetailPageRendersThePrefilledEditForm(t *testing.T) {
	t.Parallel()

	url := strings.ReplaceAll(routePath(t, page.OpPractitionerDetailPage), "{id}", seed.AccountAPractitionerID)

	_, _, body := newBrowser(t).get(url)

	formID := ids.DirectoryForm(directory.PractitionerSegment, seed.AccountAPractitionerID)
	assert.Contains(t, body, attribute("id", formID),
		"the detail page's edit link targets #%s, but no form on the page carries that id", formID)
	assert.Contains(t, body, `value="Dr. Ngozi Adeyemi"`, "the edit form is not pre-filled with the practitioner's own values")
}

func TestTheFacilityListPageRendersTheCreateForm(t *testing.T) {
	t.Parallel()

	_, _, body := newBrowser(t).get(routePath(t, page.OpFacilityListPage))

	formID := ids.DirectoryForm(directory.FacilitySegment, "")
	assert.Contains(t, body, attribute("id", formID),
		"the list page's create link targets #%s, but no form on the page carries that id", formID)
}

func TestTheFacilityDetailPageRendersThePrefilledEditForm(t *testing.T) {
	t.Parallel()

	url := strings.ReplaceAll(routePath(t, page.OpFacilityDetailPage), "{id}", seed.AccountAFacilityPracticeID)

	_, _, body := newBrowser(t).get(url)

	formID := ids.DirectoryForm(directory.FacilitySegment, seed.AccountAFacilityPracticeID)
	assert.Contains(t, body, attribute("id", formID),
		"the detail page's edit link targets #%s, but no form on the page carries that id", formID)
	assert.Contains(t, body, `value="Riverside Family Practice"`, "the edit form is not pre-filled with the facility's own values")
}
