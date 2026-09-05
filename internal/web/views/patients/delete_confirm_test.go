package patients_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/web/views/patients"
	"medikube/internal/web/views/viewstest"
)

const confirmDeleteArticle = "Confirm delete"

// T149, FR-048, US6-1. The confirmation names the person and states how many
// records will be destroyed, before anything is actually removed.
func TestDeleteConfirmNamesThePersonAndStatesTheRecordCount(t *testing.T) {
	t.Parallel()

	t.Run("several records", func(t *testing.T) {
		t.Parallel()

		props := patients.DeleteConfirmProps{
			PatientID: "pat0000000001", Name: "Chiamaka Okonkwo", TotalRecords: 3,
			Version: `"abc"`, DeleteHref: "/api/v1/patients/pat0000000001",
		}

		tree := viewstest.Render(t, patients.DeleteConfirm(props), "div")
		region := tree.One(t, viewstest.Region(confirmDeleteArticle))
		text := viewstest.Text(region)

		assert.Contains(t, text, "Chiamaka Okonkwo")
		assert.Contains(t, text, "all 3")
		assert.Contains(t, text, "no recycle bin and no undo")
	})

	t.Run("exactly one record", func(t *testing.T) {
		t.Parallel()

		props := patients.DeleteConfirmProps{
			PatientID: "pat0000000002", Name: "Bo Adeyemi", TotalRecords: 1,
			Version: `"abc"`, DeleteHref: "/api/v1/patients/pat0000000002",
		}

		tree := viewstest.Render(t, patients.DeleteConfirm(props), "div")
		text := viewstest.Text(tree.One(t, viewstest.Region(confirmDeleteArticle)))

		assert.Contains(t, text, "Bo Adeyemi")
		assert.Contains(t, text, "the one record")
	})

	t.Run("nothing recorded yet", func(t *testing.T) {
		t.Parallel()

		props := patients.DeleteConfirmProps{
			PatientID: "pat0000000003", Name: "Emeka Okonkwo", TotalRecords: 0,
			Version: `"abc"`, DeleteHref: "/api/v1/patients/pat0000000003",
		}

		tree := viewstest.Render(t, patients.DeleteConfirm(props), "div")
		text := viewstest.Text(tree.One(t, viewstest.Region(confirmDeleteArticle)))

		assert.Contains(t, text, "Emeka Okonkwo")
		assert.Contains(t, text, "This is permanent")
	})
}
