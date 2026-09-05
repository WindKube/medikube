package records_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/records"
	"medikube/internal/web/views/viewstest"
)

// T066 for US2's three kinds, at the size medication's own exhaustive detail
// suite (medication_detail_test.go) does not need repeating three times: the
// landmark and one field per view, the way every other kind's detail already
// proves its own landmark and its own fields (FR-024).
func TestEncounterProcedureTreatmentDetailRenderTheirLandmarkAndOneField(t *testing.T) {
	t.Parallel()

	t.Run(kind.Encounter.Segment(), func(t *testing.T) {
		t.Parallel()

		view := records.NewEncounterView(clinical.Encounter{
			ID: "mkenc00000000001", Reason: "Twisted ankle",
		}, records.EncounterLinks{})

		tree := viewstest.Render(t, records.EncounterDetail(records.EncounterDetailProps{Encounter: view}), "div")

		article := tree.One(t, viewstest.Article("Encounter"))
		assert.Equal(t, ids.RecordDetail(kind.Encounter, view.ID), viewstest.Attr(article, "id"))
		assert.Contains(t, viewstest.Text(article), "Twisted ankle")
	})

	t.Run(kind.Procedure.Segment(), func(t *testing.T) {
		t.Parallel()

		view := records.NewProcedureView(clinical.Procedure{
			ID: "mkproc0000000001", Name: "Skin biopsy",
		}, records.ProcedureLinks{})

		tree := viewstest.Render(t, records.ProcedureDetail(records.ProcedureDetailProps{Procedure: view}), "div")

		article := tree.One(t, viewstest.Article("Procedure"))
		assert.Equal(t, ids.RecordDetail(kind.Procedure, view.ID), viewstest.Attr(article, "id"))
		assert.Contains(t, viewstest.Text(article), "Skin biopsy")
	})

	t.Run(kind.Treatment.Segment(), func(t *testing.T) {
		t.Parallel()

		view := records.NewTreatmentView(clinical.Treatment{
			ID: "mktrt00000000001", Name: "Physical therapy",
		}, records.TreatmentLinks{})

		tree := viewstest.Render(t, records.TreatmentDetail(records.TreatmentDetailProps{Treatment: view}), "div")

		article := tree.One(t, viewstest.Article("Treatment"))
		assert.Equal(t, ids.RecordDetail(kind.Treatment, view.ID), viewstest.Attr(article, "id"))
		assert.Contains(t, viewstest.Text(article), "Physical therapy")
	})
}
