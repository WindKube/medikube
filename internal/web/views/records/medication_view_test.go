package records_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/testsupport/seed"
	"medikube/internal/web/views/records"
)

// The published vocabularies get a display label each, and the label table is
// the fifth consumer of a value nobody remembers when adding one. A missing
// entry falls through to the stored spelling, which reads as `on_hold` on the
// page and passes every test that only asks whether something was rendered —
// so the assertion is that the label is NOT the stored spelling.
func TestEveryPublishedValueHasALabelThatIsNotItsStoredSpelling(t *testing.T) {
	t.Parallel()

	cases := map[string][]records.Option{
		"kind":  records.MedicationTypeOptions(""),
		"route": records.MedicationRouteOptions(""),
		"state": records.TherapyStatusOptions(""),
	}

	for vocabulary, options := range cases {
		t.Run(vocabulary, func(t *testing.T) {
			t.Parallel()

			require.NotEmpty(t, options)
			seen := map[string]string{}

			for _, option := range options {
				assert.NotEmptyf(t, option.Label, "%s has no label", option.Value)
				assert.NotEqualf(t, option.Value, option.Label,
					"%s renders as its stored spelling, which is what a missing label looks like", option.Value)

				previous, clash := seen[option.Label]
				assert.Falsef(t, clash, "%s and %s both render as %q", previous, option.Value, option.Label)
				seen[option.Label] = option.Value
			}
		})
	}
}

// The options are the form's offer and the domain's Valid() is what accepts
// them. A value offered and refused is a 422 the person cannot act on; a value
// accepted and never offered is a state they cannot reach.
func TestTheOptionsAreExactlyWhatTheDomainPublishes(t *testing.T) {
	t.Parallel()

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
		name    string
		offered []records.Option
		want    []string
	}{
		{name: "kind", offered: records.MedicationTypeOptions(""), want: types},
		{name: "route", offered: records.MedicationRouteOptions(""), want: routes},
		{name: "state", offered: records.TherapyStatusOptions(""), want: statuses},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			values := make([]string, 0, len(testCase.offered))
			for _, option := range testCase.offered {
				values = append(values, option.Value)
			}

			assert.Equal(t, testCase.want, values)
		})
	}
}

func TestExactlyOneOptionIsSelected(t *testing.T) {
	t.Parallel()

	options := records.TherapyStatusOptions(clinical.TherapyStatusOnHold)

	var selected []string
	for _, option := range options {
		if option.Selected {
			selected = append(selected, option.Value)
		}
	}

	assert.Equal(t, []string{string(clinical.TherapyStatusOnHold)}, selected)
}

func TestEveryFormFieldHasALabelOfItsOwn(t *testing.T) {
	t.Parallel()

	fields := records.MedicationFields()
	require.Len(t, fields, 12)

	seen := map[string]string{}
	for _, field := range fields {
		label := records.FieldLabel(field)
		assert.NotEmptyf(t, label, "%s has no label", field)
		assert.NotEqualf(t, field, label, "%s renders as its own column name", field)

		previous, clash := seen[label]
		assert.Falsef(t, clash, "%s and %s are both %q", previous, field, label)
		seen[label] = field
	}
}

// FR-024 as a property of the mapping rather than of the template: a value that
// was never recorded produces no entry at all, so the detail has nothing to
// render an empty placeholder for.
func TestTheDetailEntriesHoldOnlyWhatWasRecorded(t *testing.T) {
	t.Parallel()

	nameOnly := view(t, seeded(t, seed.NameOnlyID))

	fields := make([]string, 0, len(nameOnly.Entries()))
	for _, entry := range nameOnly.Entries() {
		assert.NotEmptyf(t, entry.Value, "%s produced an entry with nothing in it", entry.Field)
		assert.NotEmptyf(t, entry.Label, "%s produced an entry with no label", entry.Field)
		fields = append(fields, entry.Field)
	}

	assert.Equal(t, []string{
		records.FieldStatus, records.FieldCreated, records.FieldLastChanged,
	}, fields, "a row with only a name and a state has only a state, a creation and a change to show")
}

// The order is data-model §2's column order, which is the order validate.go
// checks them in and the order the form offers them — so the person reads the
// same sequence in all three places.
func TestTheDetailEntriesAreInTheRecordedOrder(t *testing.T) {
	t.Parallel()

	complete := view(t, everyFieldFilledIn(t))

	fields := make([]string, 0, len(complete.Entries()))
	for _, entry := range complete.Entries() {
		fields = append(fields, entry.Field)
	}

	assert.Equal(t, []string{
		records.FieldAlternativeName, records.FieldType, records.FieldDosage,
		records.FieldFrequency, records.FieldRoute, records.FieldIndication,
		records.FieldStartedOn, records.FieldEndedOn, records.FieldStatus,
		records.FieldSideEffects, records.FieldNotes,
		records.FieldCreated, records.FieldLastChanged,
	}, fields)
}

// FR-019: a calendar date reads identically to every viewer, so it carries no
// time of day and no zone into the view.
func TestTheViewCarriesCalendarDatesAndMachineReadableInstants(t *testing.T) {
	t.Parallel()

	complete := everyFieldFilledIn(t)
	rendered := view(t, complete)

	assert.Equal(t, complete.StartedOn.String(), rendered.StartedOn)
	assert.Equal(t, complete.EndedOn.String(), rendered.EndedOn)
	assert.Equal(t, "2026-08-27T09:14:05Z", rendered.LastChanged.Machine)
	assert.NotEmpty(t, rendered.LastChanged.Human)
	assert.NotEqual(t, rendered.LastChanged.Machine, rendered.LastChanged.Human,
		"an RFC3339 instant is not what a person reads")

	nameOnly := view(t, seeded(t, seed.NameOnlyID))
	assert.Empty(t, nameOnly.StartedOn, "the absent date renders as nothing, not as a zeroth day")
	assert.Empty(t, nameOnly.EndedOn)
}

// The version is the ETag the edit form and the delete confirmation send back
// as If-Match (FR-026). A view that dropped it would make every save a blind
// one.
func TestTheViewCarriesTheVersion(t *testing.T) {
	t.Parallel()

	complete := everyFieldFilledIn(t)

	assert.Equal(t, complete.Version, view(t, complete).Version)
}

func TestFieldErrorsAreEmptyWhenThereAreNone(t *testing.T) {
	t.Parallel()

	errs := records.NewFieldErrors(nil)

	assert.False(t, errs.Has(records.FieldName))
	assert.Empty(t, errs.Messages(context.Background(), records.FieldName))
	assert.Empty(t, errs.Fields())
}
