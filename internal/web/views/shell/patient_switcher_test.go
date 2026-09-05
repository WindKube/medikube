package shell_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/web/views/shell"
	"medikube/internal/web/views/viewstest"
)

// T096/FR-014. The switcher is role=combobox named "Active patient", shows
// the person in view by name and photograph, offers a switch to any of them,
// and every option carries name AND date of birth so twins and same-named
// relatives are distinguishable.
func TestPatientSwitcherNamesAndOffersEveryReachablePerson(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, shell.PatientSwitcher(shell.PatientSwitcherProps{
		Href: "/api/v1/me/active-patient",
		Options: []shell.PatientOption{
			{ID: "pat1", Name: "Amara Okonkwo", BirthDate: "1988-04-12", PhotoURL: "/api/v1/patients/pat1/photo", Active: true},
			{ID: "pat2", Name: "Amara Okonkwo", BirthDate: "2015-09-03"},
		},
	}), "div")

	combobox := tree.One(t, viewstest.WithAttr("role", "combobox"))
	assert.Equal(t, "Active patient", viewstest.Attr(combobox, "aria-label"))

	assert.Equal(t, 1, tree.Count(viewstest.Tag("img")), "the person in view must be shown by photograph")
	assert.Contains(t, tree.Markup, "Amara Okonkwo (1988-04-12)", "the active option must carry name AND date of birth")
	assert.Contains(t, tree.Markup, "Amara Okonkwo (2015-09-03)", "a same-named relative must be distinguishable by birth date")

	options := tree.All(viewstest.Tag("option"))
	require.Len(t, options, 2, "every reachable patient must be offered")
}

// TestPatientSwitcherWithNobodyInViewShowsNoPhotograph is the null-pointer
// side of FR-017: nobody chosen yet, so nothing is shown as the person in
// view, but the switcher still lists whoever is reachable.
func TestPatientSwitcherWithNobodyInViewShowsNoPhotograph(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, shell.PatientSwitcher(shell.PatientSwitcherProps{
		Href:    "/api/v1/me/active-patient",
		Options: []shell.PatientOption{{ID: "pat1", Name: "Amara Okonkwo", BirthDate: "1988-04-12"}},
	}), "div")

	assert.Zero(t, tree.Count(viewstest.Tag("img")), "nobody is in view, so no photograph is shown")
	require.Len(t, tree.All(viewstest.Tag("option")), 1)
}
