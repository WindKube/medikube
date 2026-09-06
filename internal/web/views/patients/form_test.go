package patients_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/domain"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/patients"
	"medikube/internal/web/views/viewstest"
)

// T055: the create form and the edit form are the same component, and each
// renders as a form landmark carrying every field the person submitted.
func TestTheFormRendersBothAsCreateAndAsEdit(t *testing.T) {
	t.Parallel()

	self := selfRecord(t)

	cases := []struct {
		name  string
		props patients.PatientFormProps
		label string
	}{
		{
			name:  "create",
			props: patients.PatientFormProps{FormID: "patient-form", New: true},
			label: "Add a person",
		},
		{
			name:  "edit",
			props: patients.PatientFormProps{FormID: "patient-form", Patient: self},
			label: "Edit person",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, patients.PatientForm(testCase.props), "div")

			form := tree.One(t, viewstest.Form(testCase.label))
			assert.NotEmpty(t, viewstest.Elements(form))
		})
	}
}

// FR-048: a refused field is described by its own error, named by
// aria-describedby.
func TestAFieldRefusalIsDescribedByItsOwnError(t *testing.T) {
	t.Parallel()

	var invalid domain.ValidationError
	invalid.Add(patients.FieldFirstName, domain.CodeRequired, "a first name is required")

	tree := viewstest.Render(t, patients.PatientForm(patients.PatientFormProps{
		FormID: "patient-form", New: true, Errors: patients.NewFieldErrors(&invalid),
	}), "div")

	form := tree.One(t, viewstest.Form("Add a person"))
	control := tree.One(t, viewstest.WithID(ids.Field("patient-form", patients.FieldFirstName)))
	assert.Equal(t, "true", viewstest.Attr(control, "aria-invalid"))
	assert.Equal(t, ids.FieldError("patient-form", patients.FieldFirstName), viewstest.Attr(control, "aria-describedby"))
	assert.NotEmpty(t, viewstest.Elements(form))
}
