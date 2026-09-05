package records_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/records"
	"medikube/internal/web/views/viewstest"
)

func TestFamilyMemberRowRendersTheDeterministicIDs(t *testing.T) {
	t.Parallel()

	view := records.NewFamilyMemberView(clinical.FamilyMember{
		ID: "fam1", Name: "Nadia Okonkwo", Relationship: clinical.FamilyRelationshipGrandmother,
	}, records.FamilyMemberLinks{Detail: "/" + kind.FamilyMember.Segment() + "/fam1"})

	tree := viewstest.Render(t, records.FamilyMemberRow(view), "tbody")
	row := tree.One(t, viewstest.WithID(ids.RecordRow(kind.FamilyMember, "fam1")))

	assert.Contains(t, viewstest.Text(row), "Nadia Okonkwo")
	assert.Contains(t, viewstest.Text(row), "Grandmother")
}

func TestFamilyMemberRowShowsDeceasedOnlyWhenRecorded(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		isDeceased bool
	}{
		{name: "living relative shows no deceased marker", isDeceased: false},
		{name: "deceased relative shows the marker", isDeceased: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			view := records.NewFamilyMemberView(clinical.FamilyMember{
				ID: "fam1", Name: "Nadia", Relationship: clinical.FamilyRelationshipGrandmother,
				IsDeceased: testCase.isDeceased,
			}, records.FamilyMemberLinks{})

			tree := viewstest.Render(t, records.FamilyMemberRow(view), "tbody")
			text := viewstest.Text(tree.One(t, viewstest.Tag("tr")))

			if testCase.isDeceased {
				assert.Contains(t, text, "Deceased")
			} else {
				assert.NotContains(t, text, "Deceased")
			}
		})
	}
}

func TestFamilyMemberDetailRendersAbsentFieldsAsAbsent(t *testing.T) {
	t.Parallel()

	// A record carrying only the required fields must show every optional
	// field as absent rather than as a dash or a zero (spec Edge Cases).
	view := records.NewFamilyMemberView(clinical.FamilyMember{
		ID: "fam1", Name: "Nadia Okonkwo", Relationship: clinical.FamilyRelationshipGrandmother,
	}, records.FamilyMemberLinks{Record: "/api/v1/records/family-history/fam1"})

	tree := viewstest.Render(t, records.FamilyMemberDetail(records.FamilyMemberDetailProps{FamilyMember: view}), "article")
	article := tree.One(t, viewstest.WithID(ids.RecordDetail(kind.FamilyMember, "fam1")))
	text := viewstest.Text(article)

	assert.Contains(t, text, "Nadia Okonkwo")
	assert.NotContains(t, text, "Year of death")
}

func TestFamilyMemberDetailRendersItsConditions(t *testing.T) {
	t.Parallel()

	age := 62

	view := records.NewFamilyMemberView(clinical.FamilyMember{
		ID: "fam1", Name: "Nadia Okonkwo", Relationship: clinical.FamilyRelationshipGrandmother,
		Conditions: []clinical.FamilyCondition{
			{Name: "Breast cancer", DiagnosedAge: &age, Severity: clinical.SeveritySevere, Status: clinical.ConditionStatusResolved},
		},
	}, records.FamilyMemberLinks{Record: "/api/v1/records/family-history/fam1"})

	tree := viewstest.Render(t, records.FamilyMemberDetail(records.FamilyMemberDetailProps{FamilyMember: view}), "article")
	text := viewstest.Text(tree.One(t, viewstest.Tag("article")))

	assert.Contains(t, text, "Breast cancer")
	assert.Contains(t, text, "62")

	require.NotEmpty(t, view.Conditions)
}
