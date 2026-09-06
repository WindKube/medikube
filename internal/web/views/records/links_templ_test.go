package records_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/web/views/records"
	"medikube/internal/web/views/viewstest"
)

// T142, FR-059. Every related record renders its kind, an identifying
// summary and an openable link — never a bare id.
func TestLinkedRecordsRendersKindSummaryAndAnOpenableLink(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, records.LinkedRecords("links", "Linked records", []records.LinkedRecordItem{
		{Kind: "condition", Summary: "Type 2 diabetes", Href: "/" + kind.Condition.Collection() + "/cnd1"},
	}), "div")

	section := tree.One(t, viewstest.Region("Linked records"))

	text := viewstest.Text(section)
	assert.Contains(t, text, "condition")
	assert.Contains(t, text, "Type 2 diabetes")

	anchor := tree.One(t, viewstest.Tag("a"))
	assert.Equal(t, "/"+kind.Condition.Collection()+"/cnd1", viewstest.Attr(anchor, "href"))
	assert.Equal(t, "Type 2 diabetes", viewstest.Text(anchor))
}

// The anti-vacuity control: an empty set renders an empty state rather than
// nothing at all, so a page that forgot to populate the items still shows a
// landmark a render test — or a person — can tell apart from a bug.
func TestLinkedRecordsRendersAnEmptyStateWithNoItems(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, records.LinkedRecords("links", "Linked records", nil), "div")

	section := tree.One(t, viewstest.Region("Linked records"))
	require.Empty(t, tree.All(viewstest.Tag("a")))
	assert.NotEmpty(t, viewstest.Text(section))
}

// allergyRecordHref and medicationHref build the same shapes the page layer
// does, off the kind table rather than spelled by hand (research D-05).
func allergyRecordHref(id string) string {
	return "/api/v1/records/" + kind.Allergy.Collection() + "/" + id
}

func symptomRecordHref(id string) string {
	return "/api/v1/records/" + kind.Symptom.Collection() + "/" + id
}

func medicationHref(id string) string {
	return "/" + kind.Medication.Collection() + "/" + id
}

// FR-055's editor, empty: no medication linked yet still offers the picker
// that adds one, rather than an editor with nothing to interact with at all.
func TestMedicationLinksEditorRendersAnEmptyStateWithAPicker(t *testing.T) {
	t.Parallel()

	props := records.MedicationLinksEditorProps{
		ID:         "allergy-" + kind.Medication.Collection(),
		Title:      "Medications",
		RecordHref: allergyRecordHref("a1"),
		Options:    []records.MedicationLinkOption{{ID: "med1", Name: "Amoxicillin"}},
		Roles:      []records.MedicationLinkRole{{Field: kind.Medication.Collection(), Items: nil, IDs: nil}},
	}

	tree := viewstest.Render(t, records.MedicationLinksEditor(props), "div")

	section := tree.One(t, viewstest.Region("Medications"))
	require.Empty(t, tree.All(viewstest.Tag("a")), "nothing linked yet, so nothing to open")
	require.NotEmpty(t, tree.All(viewstest.Tag("select")), "the picker itself still renders")
	assert.Contains(t, viewstest.Text(section), "Amoxicillin")
}

// Populated, one role (allergy/condition/injury's shape): every linked
// medication is an openable link with its own Remove button.
func TestMedicationLinksEditorRendersLinkedMedicationsWithRemove(t *testing.T) {
	t.Parallel()

	props := records.MedicationLinksEditorProps{
		ID:         "allergy-" + kind.Medication.Collection(),
		Title:      "Medications",
		RecordHref: allergyRecordHref("a1"),
		Options:    []records.MedicationLinkOption{{ID: "med1", Name: "Amoxicillin"}},
		Roles: []records.MedicationLinkRole{{
			Field: kind.Medication.Collection(),
			Items: []records.LinkedRecordItem{{Kind: "medication", Summary: "Amoxicillin", Href: medicationHref("med1")}},
			IDs:   []string{"med1"},
		}},
	}

	tree := viewstest.Render(t, records.MedicationLinksEditor(props), "div")

	anchor := tree.One(t, viewstest.Tag("a"))
	assert.Equal(t, medicationHref("med1"), viewstest.Attr(anchor, "href"))
	assert.Equal(t, "Amoxicillin", viewstest.Text(anchor))
	assert.NotEmpty(t, tree.All(viewstest.Tag("button")))
}

// Populated, two roles (symptom's shape, FR-032): each role renders its own
// label and its own linked list, so a medication that both treats and causes
// the same symptom is never ambiguous about which it is.
func TestMedicationLinksEditorRendersTwoRolesSeparately(t *testing.T) {
	t.Parallel()

	treatedByField := "treated_by_" + kind.Medication.Collection()
	causedByField := "caused_by_" + kind.Medication.Collection()

	props := records.MedicationLinksEditorProps{
		ID:         "symptom-" + kind.Medication.Collection(),
		Title:      "Medications",
		RecordHref: symptomRecordHref("s1"),
		Options:    []records.MedicationLinkOption{{ID: "med1", Name: "Ibuprofen"}},
		Roles: []records.MedicationLinkRole{
			{
				Field: treatedByField, Label: "Treats",
				Items: []records.LinkedRecordItem{{Kind: "medication", Summary: "Ibuprofen", Href: medicationHref("med1")}},
				IDs:   []string{"med1"},
			},
			{Field: causedByField, Label: "Causes"},
		},
	}

	tree := viewstest.Render(t, records.MedicationLinksEditor(props), "div")

	section := tree.One(t, viewstest.Region("Medications"))
	text := viewstest.Text(section)
	assert.Contains(t, text, "Treats")
	assert.Contains(t, text, "Causes")
	assert.Contains(t, text, "Ibuprofen")

	selects := tree.All(viewstest.Tag("select"))
	assert.NotEmpty(t, selects, "two roles offer a role picker alongside the medication picker")
}

// A dangling id — one Options no longer names, because the medication moved
// or the caller built Items by hand rather than through medicationLinkRole —
// still renders rather than panicking: the item's Summary is whatever the
// caller supplied, shown as-is.
func TestMedicationLinksEditorRendersAnUnresolvedIDWithoutPanicking(t *testing.T) {
	t.Parallel()

	props := records.MedicationLinksEditorProps{
		ID:         "allergy-" + kind.Medication.Collection(),
		Title:      "Medications",
		RecordHref: allergyRecordHref("a1"),
		Roles: []records.MedicationLinkRole{{
			Field: kind.Medication.Collection(),
			Items: []records.LinkedRecordItem{{Kind: "medication", Summary: "med-gone", Href: medicationHref("med-gone")}},
			IDs:   []string{"med-gone"},
		}},
	}

	require.NotPanics(t, func() {
		viewstest.Render(t, records.MedicationLinksEditor(props), "div")
	})
}

// FR-059's other end: medication's own page renders every back-relation as
// an openable, removable link, with the same empty-state rule LinkedRecords
// follows.
func TestRemovableLinksRendersKindSummaryAndRemove(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, records.RemovableLinks(records.RemovableLinksProps{
		ID:    kind.Medication.Collection() + "-links",
		Title: "Linked records",
		Items: []records.RemovableLink{
			{Kind: "allergy", Summary: "Penicillin allergy", Href: allergyRecordHref("a1"), RemoveOn: "@patch('/x')"},
		},
	}), "div")

	section := tree.One(t, viewstest.Region("Linked records"))
	text := viewstest.Text(section)
	assert.Contains(t, text, "allergy")
	assert.Contains(t, text, "Penicillin allergy")

	anchor := tree.One(t, viewstest.Tag("a"))
	assert.Equal(t, allergyRecordHref("a1"), viewstest.Attr(anchor, "href"))
}

func TestRemovableLinksRendersAnEmptyStateWithNoItems(t *testing.T) {
	t.Parallel()

	tree := viewstest.Render(t, records.RemovableLinks(records.RemovableLinksProps{ID: kind.Medication.Collection() + "-links", Title: "Linked records"}), "div")

	section := tree.One(t, viewstest.Region("Linked records"))
	require.Empty(t, tree.All(viewstest.Tag("a")))
	assert.NotEmpty(t, viewstest.Text(section))
}
