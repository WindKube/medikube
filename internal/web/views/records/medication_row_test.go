package records_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/i18n"
	"medikube/internal/testsupport/seed"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/records"
	"medikube/internal/web/views/viewstest"
)

// T148, FR-021. The row is the element contracts/streams.md patches by id, so
// the id it renders and the id the stream targets have to be the same call.
func TestTheRowCarriesTheIDTheStreamPatchesBy(t *testing.T) {
	t.Parallel()

	medication := view(t, seeded(t, seed.NameOnlyID))
	tree := viewstest.Render(t, records.MedicationRow(medication), "tbody")

	row := tree.One(t, viewstest.Tag("tr"))

	assert.Equal(t, ids.RecordRow(kind.Medication, medication.ID), viewstest.Attr(row, "id"),
		"the stream patches #%s and would find nothing", ids.RecordRow(kind.Medication, medication.ID))
}

// FR-021: enough of each entry to identify it without opening it.
func TestTheRowShowsWhatIdentifiesTheMedication(t *testing.T) {
	t.Parallel()

	medication := view(t, everyFieldFilledIn(t))
	tree := viewstest.Render(t, records.MedicationRow(medication), "tbody")

	row := tree.One(t, viewstest.Tag("tr"))
	text := viewstest.Text(row)

	for _, value := range []string{
		medication.Name, medication.Dosage, medication.Frequency,
		medication.Status, medication.StartedOn,
	} {
		resolved := i18n.T(context.Background(), value)
		assert.Containsf(t, text, resolved, "FR-021 requires the row to show %q", resolved)
	}

	link := tree.One(t, viewstest.Tag("a"))
	assert.Equal(t, medication.Links.Detail, viewstest.Attr(link, "href"))
}

// FR-024, and the failure mode T148 names: a label rendered beside a value that
// was never recorded. The row's labels are for a stacked narrow viewport, so
// they are real elements and a label without its value is a real defect.
func TestARowNeverRendersTheLabelOfAValueThatWasNotRecorded(t *testing.T) {
	t.Parallel()

	optional := []string{
		i18n.T(context.Background(), records.FieldLabel(records.FieldDosage)),
		i18n.T(context.Background(), records.FieldLabel(records.FieldFrequency)),
		i18n.T(context.Background(), records.FieldLabel(records.FieldStartedOn)),
	}
	require.Len(t, optional, 3)

	cases := []struct {
		name       string
		medication func(*testing.T) records.MedicationView
		labelled   bool
	}{
		{
			name:       "every field recorded",
			medication: func(t *testing.T) records.MedicationView { return view(t, everyFieldFilledIn(t)) },
			labelled:   true,
		},
		{
			name:       "a name and a state and nothing else",
			medication: func(t *testing.T) records.MedicationView { return view(t, seeded(t, seed.NameOnlyID)) },
			labelled:   false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, records.MedicationRow(testCase.medication(t)), "tbody")
			text := viewstest.Text(tree.One(t, viewstest.Tag("tr")))

			for _, label := range optional {
				if testCase.labelled {
					assert.Containsf(t, text, label, "%q is recorded and its label is missing", label)
					continue
				}
				assert.NotContainsf(t, text, label,
					"%q was never recorded and the row labels it anyway (FR-024)", label)
			}
		})
	}
}

// The seeded row exists to catch exactly this: a name that is right-to-left
// text mixed with a tag, an ampersand and a quote. templ's escaping is
// load-bearing because 'unsafe-eval' is in the CSP (internal/web/views/doc.go).
func TestAMedicationNameThatLooksLikeMarkupRendersAsText(t *testing.T) {
	t.Parallel()

	medication := view(t, seeded(t, seed.ScriptedNameID))
	tree := viewstest.Render(t, records.MedicationRow(medication), "tbody")

	row := tree.One(t, viewstest.Tag("tr"))

	assert.Equal(t, medication.Name, strings.TrimSpace(viewstest.Text(tree.One(t, viewstest.Tag("a")))),
		"the name must survive intact as text")
	assert.Empty(t, viewstest.Find(row, viewstest.Tag("b")),
		"the name's <b> was rendered as an element")
	assert.Empty(t, viewstest.Find(row, viewstest.Tag("script")))
}

// The stream patches one row at a time, so a row that is not a single element
// cannot be the target of an outer-mode patch.
func TestTheRowIsOneElement(t *testing.T) {
	t.Parallel()

	medication := view(t, everyFieldFilledIn(t))
	tree := viewstest.Render(t, records.MedicationRow(medication), "tbody")

	assert.Equal(t, 1, tree.Count(viewstest.Tag("tr")))
}
