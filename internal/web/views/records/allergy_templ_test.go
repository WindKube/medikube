package records_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/records"
	"medikube/internal/web/views/viewstest"
)

const allergyRegion = "Allergies"

const allergyArticle = "Allergy"

func testAllergyView() records.AllergyView {
	return records.NewAllergyView(clinical.Allergy{
		ID: "allergy-render-test", PatientID: "patient-render-test",
		Allergen: "Penicillin", Severity: clinical.SeverityMild, Status: clinical.ConditionStatusActive,
	}, records.AllergyLinks{Detail: fmt.Sprintf("/%s/allergy-render-test", kind.Allergy.Segment())})
}

// T043. Both landmarks contracts/pages.md publishes, and one field each,
// mirroring medication's own landmark tests at the smallest size that still
// proves the wiring.
func TestTheAllergyListRendersItsLandmarkAndAField(t *testing.T) {
	t.Parallel()

	allergy := testAllergyView()
	tree := viewstest.Render(t, records.AllergyList(records.AllergyListProps{
		Allergies: []records.AllergyView{allergy}, CreateHref: fmt.Sprintf("/%s/new", kind.Allergy.Segment()),
	}), "div")

	region := tree.One(t, viewstest.Region(allergyRegion))
	assert.Equal(t, ids.RecordList(kind.Allergy), viewstest.Attr(region, "id"))
	assert.Contains(t, viewstest.Text(region), allergy.Allergen)
}

func TestTheAllergyDetailRendersItsLandmarkAndAField(t *testing.T) {
	t.Parallel()

	allergy := testAllergyView()
	tree := viewstest.Render(t, records.AllergyDetail(records.AllergyDetailProps{Allergy: allergy}), "div")

	article := tree.One(t, viewstest.Article(allergyArticle))
	assert.Equal(t, ids.RecordDetail(kind.Allergy, allergy.ID), viewstest.Attr(article, "id"))
	assert.Contains(t, viewstest.Text(article), allergy.Allergen)
}
