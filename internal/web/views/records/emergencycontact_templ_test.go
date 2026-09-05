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

const emergencyContactRegion = "Emergency contacts"

const emergencyContactArticle = "Emergency contact"

func testEmergencyContactView() records.EmergencyContactView {
	return records.NewEmergencyContactView(clinical.EmergencyContact{
		ID: "contact-render-test", PatientID: "patient-render-test",
		Name: "Ngozi Okonkwo", Relationship: clinical.ContactRelationshipSpouse, Phone: "+1-555-0100",
	}, records.EmergencyContactLinks{Detail: fmt.Sprintf("/%s/contact-render-test", kind.EmergencyContact.Segment())})
}

// T043. Both landmarks contracts/pages.md publishes, and one field each.
func TestTheEmergencyContactListRendersItsLandmarkAndAField(t *testing.T) {
	t.Parallel()

	contact := testEmergencyContactView()
	tree := viewstest.Render(t, records.EmergencyContactList(records.EmergencyContactListProps{
		Contacts: []records.EmergencyContactView{contact}, CreateHref: fmt.Sprintf("/%s/new", kind.EmergencyContact.Segment()),
	}), "div")

	region := tree.One(t, viewstest.Region(emergencyContactRegion))
	assert.Equal(t, ids.RecordList(kind.EmergencyContact), viewstest.Attr(region, "id"))
	assert.Contains(t, viewstest.Text(region), contact.Name)
}

func TestTheEmergencyContactDetailRendersItsLandmarkAndAField(t *testing.T) {
	t.Parallel()

	contact := testEmergencyContactView()
	tree := viewstest.Render(t, records.EmergencyContactDetail(records.EmergencyContactDetailProps{Contact: contact}), "div")

	article := tree.One(t, viewstest.Article(emergencyContactArticle))
	assert.Equal(t, ids.RecordDetail(kind.EmergencyContact, contact.ID), viewstest.Attr(article, "id"))
	assert.Contains(t, viewstest.Text(article), contact.Name)
}
