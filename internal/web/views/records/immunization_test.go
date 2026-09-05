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

func immunizationLinks(id string) records.ImmunizationLinks {
	list := "/" + kind.Immunization.Segment()

	return records.ImmunizationLinks{
		Detail: list + "/" + id,
		Edit:   list + "/" + id + "/edit",
		Record: "/api/v1/records/" + kind.Immunization.Segment() + "/" + id,
	}
}

func immunizationView(t *testing.T, entity clinical.Immunization) records.ImmunizationView {
	t.Helper()

	return records.NewImmunizationView(entity, immunizationLinks(entity.ID))
}

func seededImmunization(t *testing.T, id string) clinical.Immunization {
	t.Helper()

	for _, immunization := range seed.Immunizations() {
		if immunization.ID == id {
			immunization.UpdatedAt = time.Date(2026, time.August, 27, 9, 14, 5, 0, time.UTC)
			immunization.Version = "v1"

			return immunization
		}
	}

	require.FailNowf(t, "no such seeded row", "%s is not in the fixture", id)

	return clinical.Immunization{}
}

func immunizationEveryFieldFilledIn(t *testing.T) clinical.Immunization {
	t.Helper()

	administered, err := domain.NewDate(2025, time.March, 4)
	require.NoError(t, err)

	expires, err := domain.NewDate(2035, time.March, 4)
	require.NoError(t, err)

	dose := 2

	return clinical.Immunization{
		ID:             seed.ImmunizationSampleID,
		VaccineName:    "Influenza",
		TradeName:      "Fluarix",
		AdministeredOn: administered,
		DoseNumber:     &dose,
		LotNumber:      "AB1234",
		Manufacturer:   "GSK",
		Site:           clinical.ImmunizationSiteLeftArm,
		Route:          clinical.ImmunizationRouteIntramuscular,
		ExpiresOn:      expires,
		UpdatedAt:      time.Date(2026, time.August, 27, 9, 14, 5, 0, time.UTC),
		Version:        "v7",
	}
}

func TestTheImmunizationListRegionIsPresentWhetherOrNotAnythingIsRecorded(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		props records.ImmunizationListProps
	}{
		{
			name: "an account with records",
			props: records.ImmunizationListProps{
				Immunizations: []records.ImmunizationView{immunizationView(t, immunizationEveryFieldFilledIn(t))},
			},
		},
		{name: "an account with none"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tree := viewstest.Render(t, records.ImmunizationList(testCase.props), "div")

			region := tree.One(t, viewstest.Region("Vaccinations"))
			assert.Equal(t, ids.RecordList(kind.Immunization), viewstest.Attr(region, "id"))
		})
	}
}

func TestTheImmunizationRowCarriesTheIDTheStreamPatchesBy(t *testing.T) {
	t.Parallel()

	immunization := immunizationView(t, seededImmunization(t, seed.ImmunizationSampleID))
	tree := viewstest.Render(t, records.ImmunizationRow(immunization), "tbody")

	row := tree.One(t, viewstest.Tag("tr"))

	assert.Equal(t, ids.RecordRow(kind.Immunization, immunization.ID), viewstest.Attr(row, "id"))
}

func TestTheImmunizationRowShowsWhatIdentifiesIt(t *testing.T) {
	t.Parallel()

	immunization := immunizationView(t, immunizationEveryFieldFilledIn(t))
	tree := viewstest.Render(t, records.ImmunizationRow(immunization), "tbody")

	text := viewstest.Text(tree.One(t, viewstest.Tag("tr")))

	assert.Contains(t, text, "Influenza")
	assert.Contains(t, text, "2")
	assert.Contains(t, text, "2025-03-04")
}

func TestTheImmunizationDetailLandmarkIsNamedVaccination(t *testing.T) {
	t.Parallel()

	immunization := immunizationView(t, seededImmunization(t, seed.ImmunizationSampleID))
	tree := viewstest.Render(t, records.ImmunizationDetail(records.ImmunizationDetailProps{Immunization: immunization}), "div")

	article := tree.One(t, viewstest.Article("Vaccination"))
	assert.Equal(t, ids.RecordDetail(kind.Immunization, immunization.ID), viewstest.Attr(article, "id"))
}

// FR-024, mirroring medication's own: a value never recorded produces no
// entry at all.
func TestTheImmunizationDetailEntriesHoldOnlyWhatWasRecorded(t *testing.T) {
	t.Parallel()

	sparse := clinical.Immunization{
		ID:             "imm-1",
		VaccineName:    "Tetanus",
		AdministeredOn: mustDate(t, 2023, time.June, 1),
	}

	entries := immunizationView(t, sparse).Entries()

	fields := make([]string, 0, len(entries))
	for _, entry := range entries {
		assert.NotEmpty(t, entry.Value)
		assert.NotEmpty(t, entry.Label)
		fields = append(fields, entry.Field)
	}

	assert.Equal(t, []string{records.ImmunizationFieldAdministeredOn}, fields)
}

func TestTheImmunizationFormRendersEveryFieldAndAdjacentErrors(t *testing.T) {
	t.Parallel()

	immunization := immunizationView(t, immunizationEveryFieldFilledIn(t))

	var invalid domain.ValidationError
	invalid.Add(records.ImmunizationFieldVaccineName, domain.CodeRequired, "a vaccine name is required")

	props := records.ImmunizationFormProps{
		FormID:       ids.RecordForm(kind.Immunization, immunization.ID),
		New:          false,
		OnSubmit:     "@patch('" + immunization.Links.Record + "')",
		CancelHref:   immunization.Links.Detail,
		Immunization: immunization,
		Errors:       records.NewFieldErrors(&invalid),
	}

	tree := viewstest.Render(t, records.ImmunizationForm(props), "div")

	form := tree.One(t, viewstest.Tag("form"))
	assert.Equal(t, records.ImmunizationFormLabelEdit, viewstest.Attr(form, "aria-label"))

	control := tree.One(t, viewstest.WithAttr("name", records.ImmunizationFieldVaccineName))
	described := viewstest.Attr(control, "aria-describedby")
	require.NotEmpty(t, described)

	message := tree.One(t, viewstest.WithAttr("id", described))
	assert.Contains(t, viewstest.Text(message), "a vaccine name is required")
}

func TestEveryPublishedImmunizationVocabularyHasALabelThatIsNotItsStoredSpelling(t *testing.T) {
	t.Parallel()

	cases := map[string][]records.Option{
		"site":  records.ImmunizationSiteOptions(""),
		"route": records.ImmunizationRouteOptions(""),
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

func mustDate(t *testing.T, year int, month time.Month, day int) domain.Date {
	t.Helper()

	value, err := domain.NewDate(year, month, day)
	require.NoError(t, err)

	return value
}
