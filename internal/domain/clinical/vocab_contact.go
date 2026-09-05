package clinical

import "slices"

// ContactRelationship is an emergency contact's relation to the patient, with
// a catch-all.
type ContactRelationship string

const (
	ContactRelationshipSpouse    ContactRelationship = "spouse"
	ContactRelationshipPartner   ContactRelationship = "partner"
	ContactRelationshipParent    ContactRelationship = "parent"
	ContactRelationshipChild     ContactRelationship = "child"
	ContactRelationshipSibling   ContactRelationship = "sibling"
	ContactRelationshipFriend    ContactRelationship = "friend"
	ContactRelationshipGuardian  ContactRelationship = "guardian"
	ContactRelationshipCaregiver ContactRelationship = "caregiver"
	ContactRelationshipOther     ContactRelationship = "other"
)

var contactRelationships = []ContactRelationship{
	ContactRelationshipSpouse, ContactRelationshipPartner, ContactRelationshipParent,
	ContactRelationshipChild, ContactRelationshipSibling, ContactRelationshipFriend,
	ContactRelationshipGuardian, ContactRelationshipCaregiver, ContactRelationshipOther,
}

func ContactRelationships() []ContactRelationship { return slices.Clone(contactRelationships) }

func (v ContactRelationship) Valid() bool { return slices.Contains(contactRelationships, v) }
