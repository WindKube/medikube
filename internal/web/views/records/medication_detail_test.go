package records_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport/seed"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/records"
	"medikube/internal/web/views/viewstest"
)

// The landmark contracts/pages.md P5 publishes.
const medicationArticle = "Medication"

// T151, FR-024. The article is the landmark and it carries the id the detail
// region is patched by.
func TestTheDetailRendersItsLandmark(t *testing.T) {
	t.Parallel()

	for _, id := range []string{seed.NameOnlyID, seed.ScriptedNameID, seed.SingleDayID} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			medication := view(t, seeded(t, id))
			tree := viewstest.Render(t, records.MedicationDetail(records.MedicationDetailProps{
				Medication: medication,
			}), "div")

			article := tree.One(t, viewstest.Article(medicationArticle))
			assert.Equal(t, ids.RecordDetail(kind.Medication, medication.ID), viewstest.Attr(article, "id"))
			assert.NotEmpty(t, viewstest.Elements(article))
		})
	}
}

// FR-024: every value recorded for a medication.
func TestTheDetailShowsEveryRecordedValue(t *testing.T) {
	t.Parallel()

	medication := view(t, everyFieldFilledIn(t))
	tree := viewstest.Render(t, records.MedicationDetail(records.MedicationDetailProps{
		Medication: medication,
	}), "div")

	text := viewstest.Text(tree.One(t, viewstest.Article(medicationArticle)))

	for _, value := range []string{
		medication.Name, medication.AlternativeName, medication.Type, medication.Dosage,
		medication.Frequency, medication.Route, medication.Indication, medication.StartedOn,
		medication.EndedOn, medication.Status, medication.SideEffects, medication.Notes,
	} {
		require.NotEmpty(t, value, "the fixture left a field empty, so this assertion proves nothing")
		assert.Containsf(t, text, value, "FR-024 requires %q to be shown", value)
	}
}

// FR-020: the detail view shows when the medication was last changed.
func TestTheDetailShowsTheLastChangedTime(t *testing.T) {
	t.Parallel()

	medication := view(t, everyFieldFilledIn(t))
	tree := viewstest.Render(t, records.MedicationDetail(records.MedicationDetailProps{
		Medication: medication,
	}), "div")

	article := tree.One(t, viewstest.Article(medicationArticle))

	assert.Contains(t, viewstest.Text(article), records.FieldLabel(records.FieldLastChanged))

	stamps := viewstest.Find(article, viewstest.And(
		viewstest.Tag("time"), viewstest.WithAttr("datetime", medication.LastChanged.Machine)))
	require.Len(t, stamps, 1,
		"the last-changed time must be machine readable: a bare string is not a time (FR-019, FR-020)")
	assert.Equal(t, medication.LastChanged.Human, viewstest.Text(stamps[0]))
}

// T151 and FR-024's actual words: fields that were never filled in are omitted
// "rather than presenting empty placeholders". A <dt> with an empty <dd> is the
// empty placeholder, so the assertion is over the whole description list and
// not over a list of field names somebody remembered to check.
func TestTheDetailNeverRendersALabelWithNothingBesideIt(t *testing.T) {
	t.Parallel()

	for _, id := range []string{seed.NameOnlyID, seed.ScriptedNameID, seed.SingleDayID} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, records.MedicationDetail(records.MedicationDetailProps{
				Medication: view(t, seeded(t, id)),
			}), "div")

			article := tree.One(t, viewstest.Article(medicationArticle))
			labels := viewstest.Find(article, viewstest.Tag("dt"))
			require.NotEmpty(t, labels)

			for _, label := range labels {
				value := viewstest.NextElement(label)
				require.NotNilf(t, value, "%q labels nothing at all", viewstest.Text(label))
				assert.Equal(t, "dd", value.Data)
				assert.NotEmptyf(t, viewstest.Text(value),
					"%q is rendered as an empty placeholder (FR-024)", viewstest.Text(label))
			}
		})
	}
}

// The complement, stated positively: a row with only a name and a state carries
// no label for any of the ten values it does not have.
func TestTheDetailOmitsTheLabelOfEveryFieldThatWasNotFilledIn(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, records.MedicationDetail(records.MedicationDetailProps{
		Medication: view(t, seeded(t, seed.NameOnlyID)),
	}), "div")

	text := viewstest.Text(tree.One(t, viewstest.Article(medicationArticle)))

	absent := []string{
		records.FieldAlternativeName, records.FieldType, records.FieldDosage,
		records.FieldFrequency, records.FieldRoute, records.FieldIndication,
		records.FieldStartedOn, records.FieldEndedOn, records.FieldSideEffects,
		records.FieldNotes,
	}
	require.Len(t, absent, 10)

	for _, field := range absent {
		assert.NotContainsf(t, text, records.FieldLabel(field),
			"%q was never recorded and the detail labels it anyway", field)
	}
}

// FR-028: the confirmation is a rendered element reachable from the detail, not
// a browser dialog the render gate cannot see.
func TestTheDetailCarriesTheDeleteConfirmation(t *testing.T) {
	t.Parallel()

	medication := view(t, everyFieldFilledIn(t))
	tree := viewstest.Render(t, records.MedicationDetail(records.MedicationDetailProps{
		Medication: medication,
	}), "div")

	article := tree.One(t, viewstest.Article(medicationArticle))
	confirm := tree.One(t, viewstest.WithID(ids.RecordConfirm(kind.Medication, medication.ID)))

	assert.True(t, viewstest.Descends(article, confirm))
	assert.Contains(t, viewstest.Text(confirm), medication.Name,
		"FR-028 requires the confirmation to name the medication")
}
