package records_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/i18n"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/records"
	"medikube/internal/web/views/viewstest"
)

// A submission that breaks every rule the domain has, so the form is asked to
// display every refusal the domain can raise rather than the four somebody
// thought of. FR-027 requires them all in one response, which is only useful if
// the form can render them all.
func everyRuleBroken(t *testing.T) (clinical.Medication, *domain.ValidationError) {
	t.Helper()

	started, err := domain.NewDate(2025, time.June, 2)
	require.NoError(t, err)

	ended, err := domain.NewDate(2025, time.June, 1)
	require.NoError(t, err)

	overLong := strings.Repeat("x", 5001)

	medication := clinical.Medication{
		Name:            "",
		AlternativeName: overLong,
		Type:            "not_a_kind",
		Dosage:          overLong,
		Frequency:       overLong,
		Route:           "not_a_route",
		Indication:      overLong,
		StartedOn:       started,
		EndedOn:         ended,
		Status:          "not_a_state",
		SideEffects:     overLong,
		Notes:           overLong,
	}

	var invalid *domain.ValidationError
	require.ErrorAs(t, medication.Validate(), &invalid)
	require.GreaterOrEqual(t, len(invalid.Fields), 11,
		"the fixture stopped tripping rules and the assertions below have nothing to check")

	return medication, invalid
}

func formProps(medication records.MedicationView, invalid *domain.ValidationError, create bool) records.MedicationFormProps {
	return records.MedicationFormProps{
		FormID:     ids.RecordForm(kind.Medication, medication.ID),
		New:        create,
		OnSubmit:   "@post('" + medication.Links.Record + "')",
		CancelHref: medication.Links.Detail,
		Medication: medication,
		Errors:     records.NewFieldErrors(invalid),
	}
}

// T150 and FR-048. "Adjacent" is a relationship between elements: the message
// is the control's next sibling, and aria-describedby on the control names it.
// A form that renders every message in a block at the top passes a substring
// check and fails the person using a screen reader.
func TestEveryFieldErrorIsAdjacentToItsControlAndNamedByAriaDescribedby(t *testing.T) {
	t.Parallel()

	medication, invalid := everyRuleBroken(t)
	props := formProps(records.NewMedicationView(medication, links(medication.ID)), invalid, true)

	tree := viewstest.Render(t, records.MedicationForm(props), "div")

	for _, refusal := range invalid.Fields {
		t.Run(refusal.Field, func(t *testing.T) {
			controlID := ids.Field(props.FormID, refusal.Field)
			messageID := ids.FieldError(props.FormID, refusal.Field)

			control := tree.One(t, viewstest.WithID(controlID))
			assert.Equal(t, messageID, viewstest.Attr(control, "aria-describedby"),
				"the control does not point at its message")
			assert.Equal(t, "true", viewstest.Attr(control, "aria-invalid"))

			message := tree.One(t, viewstest.WithID(messageID))
			assert.Contains(t, viewstest.Text(message), refusal.Message)

			assert.Same(t, message, viewstest.NextElement(control),
				"the message is not adjacent to the control it concerns (FR-048)")
		})
	}
}

// The other half of aria-describedby: a control that names a message that was
// never rendered is a dangling reference, and assistive technology announces
// nothing at all.
func TestNoControlDescribesAMessageThatWasNotRendered(t *testing.T) {
	t.Parallel()

	cases := map[string]*domain.ValidationError{"a clean form": nil}
	_, invalid := everyRuleBroken(t)
	cases["a refused form"] = invalid

	for name, errs := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			medication := view(t, everyFieldFilledIn(t))
			tree := viewstest.Render(t, records.MedicationForm(formProps(medication, errs, false)), "div")

			for _, control := range tree.All(viewstest.HasAttr("aria-describedby")) {
				described := viewstest.Attr(control, "aria-describedby")
				assert.Lenf(t, tree.All(viewstest.WithID(described)), 1,
					"aria-describedby names %q and nothing has that id", described)
			}
		})
	}
}

// FR-025: every recorded field is editable. The list is the domain's, not a
// list of the fields somebody remembered to add a control for.
func TestEveryRecordedFieldHasAControl(t *testing.T) {
	t.Parallel()

	medication := view(t, everyFieldFilledIn(t))
	props := formProps(medication, nil, false)
	tree := viewstest.Render(t, records.MedicationForm(props), "div")

	fields := records.MedicationFields()
	require.Len(t, fields, 12, "FR-015 records twelve values and the form offers what it records")

	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			control := tree.One(t, viewstest.WithID(ids.Field(props.FormID, field)))
			assert.Contains(t, []string{"input", "select", "textarea"}, control.Data)
			assert.Equal(t, field, viewstest.Attr(control, "name"))

			label := tree.One(t, viewstest.And(
				viewstest.Tag("label"), viewstest.WithAttr("for", ids.Field(props.FormID, field))))
			assert.Equal(t, i18n.T(context.Background(), records.FieldLabel(field)), viewstest.Text(label))
		})
	}
}

// T150's second clause. The vocabularies are published by internal/domain and
// pinned here against the domain's own accessors: a form offering a value the
// domain refuses produces a 422 the person cannot act on, and a form missing
// one hides a state they are entitled to.
func TestTheEnumFieldsOfferExactlyThePublishedValues(t *testing.T) {
	t.Parallel()

	medication := view(t, everyFieldFilledIn(t))
	props := formProps(medication, nil, false)
	tree := viewstest.Render(t, records.MedicationForm(props), "div")

	types := make([]string, 0, len(clinical.MedicationTypes()))
	for _, value := range clinical.MedicationTypes() {
		types = append(types, string(value))
	}

	routes := make([]string, 0, len(clinical.MedicationRoutes()))
	for _, value := range clinical.MedicationRoutes() {
		routes = append(routes, string(value))
	}

	statuses := make([]string, 0, len(clinical.TherapyStatuses()))
	for _, value := range clinical.TherapyStatuses() {
		statuses = append(statuses, string(value))
	}

	cases := []struct {
		field    string
		want     []string
		optional bool
	}{
		{field: records.FieldType, want: types, optional: true},
		{field: records.FieldRoute, want: routes, optional: true},
		{field: records.FieldStatus, want: statuses, optional: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.field, func(t *testing.T) {
			control := tree.One(t, viewstest.WithID(ids.Field(props.FormID, testCase.field)))
			require.Equal(t, "select", control.Data)

			var offered []string
			for _, option := range viewstest.Find(control, viewstest.Tag("option")) {
				offered = append(offered, viewstest.Attr(option, "value"))
			}

			want := testCase.want
			if testCase.optional {
				// The empty option is "not recorded", which is a state the
				// domain distinguishes from an unpublished value and the form
				// therefore has to offer.
				want = append([]string{""}, want...)
			}

			assert.Equal(t, want, offered)
		})
	}
}

// FR-027: the interface preserves what the person typed. A form that clears
// itself on a refusal makes the person retype twelve fields to fix one.
func TestTheFormPreservesWhatWasSubmitted(t *testing.T) {
	t.Parallel()

	medication := view(t, everyFieldFilledIn(t))
	props := formProps(medication, nil, false)
	tree := viewstest.Render(t, records.MedicationForm(props), "div")

	text := map[string]string{
		records.FieldName:            medication.Name,
		records.FieldAlternativeName: medication.AlternativeName,
		records.FieldDosage:          medication.Dosage,
		records.FieldFrequency:       medication.Frequency,
		records.FieldIndication:      medication.Indication,
		records.FieldStartedOn:       medication.StartedOn,
		records.FieldEndedOn:         medication.EndedOn,
	}

	for field, want := range text {
		control := tree.One(t, viewstest.WithID(ids.Field(props.FormID, field)))
		assert.Equalf(t, want, viewstest.Attr(control, "value"), "%s lost what was typed", field)
	}

	for field, want := range map[string]string{
		records.FieldSideEffects: medication.SideEffects,
		records.FieldNotes:       medication.Notes,
	} {
		control := tree.One(t, viewstest.WithID(ids.Field(props.FormID, field)))
		assert.Equalf(t, want, viewstest.Text(control), "%s lost what was typed", field)
	}

	for field, want := range map[string]string{
		records.FieldType:   medication.TypeValue,
		records.FieldRoute:  medication.RouteValue,
		records.FieldStatus: medication.StatusValue,
	} {
		control := tree.One(t, viewstest.WithID(ids.Field(props.FormID, field)))
		selected := viewstest.Find(control, viewstest.HasAttr("selected"))
		require.Lenf(t, selected, 1, "%s has no selected option", field)
		assert.Equal(t, want, viewstest.Attr(selected[0], "value"))
	}
}

// The create form and the edit form are the same component, so the only thing
// that can differ is what the props say. Both are named, because a form with no
// accessible name is a form assistive technology announces as "form".
func TestTheCreateAndTheEditFormAreBothNamed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		props records.MedicationFormProps
		label string
	}{
		{
			name:  "create",
			props: formProps(records.MedicationView{}, nil, true),
			label: records.FormLabelCreate,
		},
		{
			name:  "edit",
			props: formProps(view(t, everyFieldFilledIn(t)), nil, false),
			label: records.FormLabelEdit,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, records.MedicationForm(testCase.props), "div")

			form := tree.One(t, viewstest.Form(i18n.T(context.Background(), testCase.label)))
			assert.Equal(t, testCase.props.FormID, viewstest.Attr(form, "id"))
			assert.Equal(t, testCase.props.OnSubmit, viewstest.Attr(form, "data-on:submit__prevent"))
		})
	}
}

// The refusal messages come from the domain and reach the page unchanged. They
// are written never to carry the value that broke the rule, because that value
// is patient data (constitution VII) — so the form may not add it back.
func TestTheFormNeverEchoesTheValueThatWasRefused(t *testing.T) {
	t.Parallel()

	medication, invalid := everyRuleBroken(t)
	props := formProps(records.NewMedicationView(medication, links(medication.ID)), invalid, true)
	tree := viewstest.Render(t, records.MedicationForm(props), "div")

	for _, refusal := range invalid.Fields {
		message := tree.One(t, viewstest.WithID(ids.FieldError(props.FormID, refusal.Field)))
		assert.NotContains(t, viewstest.Text(message), strings.Repeat("x", 40))
	}
}
