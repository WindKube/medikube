package records_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/testsupport/seed"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/records"
	"medikube/internal/web/views/viewstest"
)

// T107, FR-041, US4-4. Mirrors immunization_test.go's shape — List/Row/
// Detail/Form rendering for the one kind whose components all live in
// injury.templ — with the added requirement that the laterality is visible
// wherever an injury is shown: the row, the detail view and the form.

func injuryLinks(id string) records.InjuryLinks {
	list := "/" + kind.Injury.Segment()

	return records.InjuryLinks{
		Detail: list + "/" + id,
		Edit:   list + "/" + id + "/edit",
		Record: "/api/v1/records/" + kind.Injury.Segment() + "/" + id,
	}
}

func injuryView(t *testing.T, entity clinical.Injury) records.InjuryView {
	t.Helper()

	return records.NewInjuryView(entity, injuryLinks(entity.ID))
}

func seededInjury(t *testing.T, id string) clinical.Injury {
	t.Helper()

	for _, injury := range seed.Injuries() {
		if injury.ID == id {
			injury.UpdatedAt = time.Date(2026, time.August, 27, 9, 14, 5, 0, time.UTC)
			injury.Version = "v1"

			return injury
		}
	}

	require.FailNowf(t, "no such seeded row", "%s is not in the fixture", id)

	return clinical.Injury{}
}

func injuryEveryFieldFilledIn(t *testing.T) clinical.Injury {
	t.Helper()

	occurred, err := domain.NewDate(2025, time.August, 20)
	require.NoError(t, err)

	return clinical.Injury{
		ID:             seed.Injuries()[0].ID,
		Name:           "Sprained ankle",
		Type:           clinical.InjuryTypeSprain,
		BodyPart:       "ankle",
		Laterality:     clinical.LateralityRight,
		OccurredOn:     occurred,
		Mechanism:      "fell while running",
		Severity:       clinical.SeverityModerate,
		Status:         clinical.ConditionStatusHealing,
		RecoveryNotes:  "still icing it",
		MedicationIDs:  []string{"med-1"},
		PractitionerID: "prac-1",
		UpdatedAt:      time.Date(2026, time.August, 27, 9, 14, 5, 0, time.UTC),
		Version:        "v7",
	}
}

func TestTheInjuryListRegionIsPresentWhetherOrNotAnythingIsRecorded(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		props records.InjuryListProps
	}{
		{
			name: "an account with records",
			props: records.InjuryListProps{
				Injuries: []records.InjuryView{injuryView(t, injuryEveryFieldFilledIn(t))},
			},
		},
		{name: "an account with none"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, records.InjuryList(testCase.props), "div")

			region := tree.One(t, viewstest.Region("Injuries"))
			assert.Equal(t, ids.RecordList(kind.Injury), viewstest.Attr(region, "id"))
		})
	}
}

func TestTheInjuryRowCarriesTheIDTheStreamPatchesBy(t *testing.T) {
	t.Parallel()

	injury := injuryView(t, seededInjury(t, seed.Injuries()[0].ID))
	tree := viewstest.Render(t, records.InjuryRow(injury), "tbody")

	row := tree.One(t, viewstest.Tag("tr"))

	assert.Equal(t, ids.RecordRow(kind.Injury, injury.ID), viewstest.Attr(row, "id"))
}

// FR-041/US4-4: the row is one of the places an injury is shown, so the
// laterality must be in it.
func TestTheInjuryRowShowsTheLaterality(t *testing.T) {
	t.Parallel()

	injury := injuryView(t, injuryEveryFieldFilledIn(t))
	tree := viewstest.Render(t, records.InjuryRow(injury), "tbody")

	text := viewstest.Text(tree.One(t, viewstest.Tag("tr")))

	assert.Contains(t, text, "Sprained ankle")
	assert.Contains(t, text, "Moderate")
	assert.Contains(t, text, "Healing")
}

func TestTheInjuryDetailLandmarkIsNamedInjury(t *testing.T) {
	t.Parallel()

	injury := injuryView(t, seededInjury(t, seed.Injuries()[0].ID))
	tree := viewstest.Render(t, records.InjuryDetail(records.InjuryDetailProps{Injury: injury}), "div")

	article := tree.One(t, viewstest.Article("Injury"))
	assert.Equal(t, ids.RecordDetail(kind.Injury, injury.ID), viewstest.Attr(article, "id"))
}

// FR-041/US4-4: the detail view is the second place an injury is shown, so
// the laterality's label must appear among its entries.
func TestTheInjuryDetailShowsTheLaterality(t *testing.T) {
	t.Parallel()

	injury := injuryView(t, injuryEveryFieldFilledIn(t))
	tree := viewstest.Render(t, records.InjuryDetail(records.InjuryDetailProps{Injury: injury}), "div")

	text := viewstest.Text(tree.One(t, viewstest.Article("Injury")))

	assert.Contains(t, text, records.InjuryFieldLabel(records.InjuryFieldLaterality))
	assert.Contains(t, text, "Right")
}

// FR-024, mirroring medication's own: a value never recorded produces no
// entry at all.
func TestTheInjuryDetailEntriesHoldOnlyWhatWasRecorded(t *testing.T) {
	t.Parallel()

	sparse := clinical.Injury{
		ID:         "inj-1",
		Name:       "Sprained ankle",
		OccurredOn: mustInjuryDate(t, 2025, time.August, 20),
	}

	entries := injuryView(t, sparse).Entries()

	fields := make([]string, 0, len(entries))
	for _, entry := range entries {
		assert.NotEmpty(t, entry.Value)
		assert.NotEmpty(t, entry.Label)
		fields = append(fields, entry.Field)
	}

	assert.Equal(t, []string{records.InjuryFieldOccurredOn}, fields)
}

func TestTheInjuryFormRendersEveryFieldAndAdjacentErrors(t *testing.T) {
	t.Parallel()

	injury := injuryView(t, injuryEveryFieldFilledIn(t))

	var invalid domain.ValidationError
	invalid.Add(records.InjuryFieldName, domain.CodeRequired, "a name is required")

	props := records.InjuryFormProps{
		FormID:     ids.RecordForm(kind.Injury, injury.ID),
		New:        false,
		OnSubmit:   "@patch('" + injury.Links.Record + "')",
		CancelHref: injury.Links.Detail,
		Injury:     injury,
		Errors:     records.NewFieldErrors(&invalid),
	}

	tree := viewstest.Render(t, records.InjuryForm(props), "div")

	form := tree.One(t, viewstest.Tag("form"))
	assert.Equal(t, records.InjuryFormLabelEdit, viewstest.Attr(form, "aria-label"))

	control := tree.One(t, viewstest.WithAttr("name", records.InjuryFieldName))
	described := viewstest.Attr(control, "aria-describedby")
	require.NotEmpty(t, described)

	message := tree.One(t, viewstest.WithAttr("id", described))
	assert.Contains(t, viewstest.Text(message), "a name is required")
}

// FR-041/US4-4: the form is the third place an injury is shown, so it must
// offer the laterality as a control, with the injury's own value selected.
func TestTheInjuryFormOffersTheLateralityControl(t *testing.T) {
	t.Parallel()

	injury := injuryView(t, injuryEveryFieldFilledIn(t))

	props := records.InjuryFormProps{
		FormID:   ids.RecordForm(kind.Injury, injury.ID),
		New:      false,
		OnSubmit: "@patch('" + injury.Links.Record + "')",
		Injury:   injury,
	}

	tree := viewstest.Render(t, records.InjuryForm(props), "div")

	control := tree.One(t, viewstest.WithID(ids.Field(props.FormID, records.InjuryFieldLaterality)))
	assert.Equal(t, "select", control.Data)

	selected := viewstest.Find(control, viewstest.HasAttr("selected"))
	require.Len(t, selected, 1, "laterality has no selected option")
	assert.Equal(t, string(clinical.LateralityRight), viewstest.Attr(selected[0], "value"))
}

func TestEveryPublishedInjuryVocabularyHasALabelThatIsNotItsStoredSpelling(t *testing.T) {
	t.Parallel()

	cases := map[string][]records.Option{
		"type":       records.InjuryTypeOptions(""),
		"laterality": records.LateralityOptions(""),
		"severity":   records.SeverityOptions(""),
		"status":     records.ConditionStatusOptions(""),
	}

	for vocabulary, options := range cases {
		t.Run(vocabulary, func(t *testing.T) {
			t.Parallel()

			require.NotEmpty(t, options)

			for _, option := range options {
				assert.NotEmptyf(t, option.Label, "%s has no label", option.Value)
				assert.NotEqualf(t, option.Value, option.Label, "%s renders as its stored spelling", option.Value)
			}
		})
	}
}

func mustInjuryDate(t *testing.T, year int, month time.Month, day int) domain.Date {
	t.Helper()

	value, err := domain.NewDate(year, month, day)
	require.NoError(t, err)

	return value
}
