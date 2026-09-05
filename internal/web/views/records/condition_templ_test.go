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

const conditionRegion = "Conditions"

const conditionArticle = "Condition"

func testConditionView() records.ConditionView {
	return records.NewConditionView(clinical.Condition{
		ID: "condition-render-test", PatientID: "patient-render-test",
		Diagnosis: "Type 2 diabetes", Status: clinical.ConditionStatusActive,
	}, records.ConditionLinks{Detail: fmt.Sprintf("/%s/condition-render-test", kind.Condition.Segment())})
}

// T043. Both landmarks contracts/pages.md publishes, and one field each.
func TestTheConditionListRendersItsLandmarkAndAField(t *testing.T) {
	t.Parallel()

	condition := testConditionView()
	tree := viewstest.Render(t, records.ConditionList(records.ConditionListProps{
		Conditions: []records.ConditionView{condition}, CreateHref: fmt.Sprintf("/%s/new", kind.Condition.Segment()),
	}), "div")

	region := tree.One(t, viewstest.Region(conditionRegion))
	assert.Equal(t, ids.RecordList(kind.Condition), viewstest.Attr(region, "id"))
	assert.Contains(t, viewstest.Text(region), condition.Diagnosis)
}

func TestTheConditionDetailRendersItsLandmarkAndAField(t *testing.T) {
	t.Parallel()

	condition := testConditionView()
	tree := viewstest.Render(t, records.ConditionDetail(records.ConditionDetailProps{Condition: condition}), "div")

	article := tree.One(t, viewstest.Article(conditionArticle))
	assert.Equal(t, ids.RecordDetail(kind.Condition, condition.ID), viewstest.Attr(article, "id"))
	assert.Contains(t, viewstest.Text(article), condition.Diagnosis)
}
