package records

import (
	"strconv"

	"medikube/internal/domain/clinical"
	viewtags "medikube/internal/web/views/tags"
)

const (
	FieldContactName  = "name"
	FieldRelationship = "relationship"
	FieldPhone        = "phone"
	FieldPhoneAlt     = "phone_alt"
	FieldEmail        = "email"
	FieldAddress      = "address"
	FieldIsPrimary    = "is_primary"
	FieldIsActive     = "is_active"
)

const (
	ContactFormLabelCreate = "Record an emergency contact"
	ContactFormLabelEdit   = "Edit emergency contact"
)

var contactRelationshipLabels = map[clinical.ContactRelationship]string{
	clinical.ContactRelationshipSpouse:    "Spouse",
	clinical.ContactRelationshipPartner:   "Partner",
	clinical.ContactRelationshipParent:    "Parent",
	clinical.ContactRelationshipChild:     "Child",
	clinical.ContactRelationshipSibling:   "Sibling",
	clinical.ContactRelationshipFriend:    "Friend",
	clinical.ContactRelationshipGuardian:  "Guardian",
	clinical.ContactRelationshipCaregiver: "Caregiver",
	clinical.ContactRelationshipOther:     "Other",
}

func ContactRelationshipLabel(value clinical.ContactRelationship) string {
	return label(string(value), contactRelationshipLabels[value])
}

func ContactRelationshipOptions(selected clinical.ContactRelationship) []Option {
	published := clinical.ContactRelationships()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: ContactRelationshipLabel(value), Selected: value == selected})
	}

	return options
}

type EmergencyContactLinks struct {
	Detail string
	Edit   string
	Record string
}

type EmergencyContactView struct {
	ID        string
	PatientID string

	Name              string
	Relationship      string
	RelationshipValue string
	Phone             string
	PhoneAlt          string
	Email             string
	Address           string
	IsPrimary         bool
	IsActive          bool
	Notes             string

	Created     Timestamp
	LastChanged Timestamp
	Version     string

	Links EmergencyContactLinks
}

func NewEmergencyContactView(contact clinical.EmergencyContact, links EmergencyContactLinks) EmergencyContactView {
	return EmergencyContactView{
		ID:                contact.ID,
		PatientID:         contact.PatientID,
		Name:              contact.Name,
		Relationship:      ContactRelationshipLabel(contact.Relationship),
		RelationshipValue: string(contact.Relationship),
		Phone:             contact.Phone,
		PhoneAlt:          contact.PhoneAlt,
		Email:             contact.Email,
		Address:           contact.Address,
		IsPrimary:         contact.IsPrimary,
		IsActive:          contact.IsActive,
		Notes:             contact.Notes,
		Created:           NewTimestamp(contact.CreatedAt),
		LastChanged:       NewTimestamp(contact.UpdatedAt),
		Version:           contact.Version,
		Links:             links,
	}
}

func (c EmergencyContactView) RelationshipOptions() []Option {
	return ContactRelationshipOptions(clinical.ContactRelationship(c.RelationshipValue))
}

var contactFieldLabels = map[string]string{
	FieldContactName:  "Name",
	FieldRelationship: "Relationship",
	FieldPhone:        "Phone",
	FieldPhoneAlt:     "Second phone",
	FieldEmail:        "Email",
	FieldAddress:      "Address",
	FieldIsPrimary:    "Primary contact",
	FieldIsActive:     "Active",
	FieldNotes:        "Notes",
	FieldCreated:      "Recorded",
	FieldLastChanged:  "Last changed",
}

func contactFieldLabel(field string) string {
	if l, known := contactFieldLabels[field]; known {
		return l
	}

	return field
}

func (c EmergencyContactView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: FieldRelationship, Value: c.Relationship},
		{Field: FieldPhone, Value: c.Phone},
		{Field: FieldPhoneAlt, Value: c.PhoneAlt},
		{Field: FieldEmail, Value: c.Email},
		{Field: FieldAddress, Value: c.Address, Multiline: true},
		{Field: FieldNotes, Value: c.Notes, Multiline: true},
		{Field: FieldCreated, Value: c.Created.Human, Datetime: c.Created.Machine},
		{Field: FieldLastChanged, Value: c.LastChanged.Human, Datetime: c.LastChanged.Machine},
	}

	entries := make([]DetailEntry, 0, len(candidates))

	for _, entry := range candidates {
		if entry.Value == "" {
			continue
		}

		entry.Label = contactFieldLabel(entry.Field)
		entries = append(entries, entry)
	}

	return entries
}

func (c EmergencyContactView) Value(field string) string {
	switch field {
	case FieldContactName:
		return c.Name
	case FieldRelationship:
		return c.RelationshipValue
	case FieldPhone:
		return c.Phone
	case FieldPhoneAlt:
		return c.PhoneAlt
	case FieldEmail:
		return c.Email
	case FieldAddress:
		return c.Address
	case FieldIsPrimary:
		return strconv.FormatBool(c.IsPrimary)
	case FieldIsActive:
		return strconv.FormatBool(c.IsActive)
	case FieldNotes:
		return c.Notes
	default:
		return ""
	}
}

type EmergencyContactListProps struct {
	Contacts []EmergencyContactView

	CreateHref   string
	PreviousHref string
	NextHref     string
}

type EmergencyContactDetailProps struct {
	Contact EmergencyContactView
}

type EmergencyContactFormProps struct {
	FormID     string
	New        bool
	OnSubmit   string
	CancelHref string

	Contact EmergencyContactView
	Errors  FieldErrors
	Notice  string

	Tags viewtags.FieldProps
}

func (p EmergencyContactFormProps) Label() string {
	if p.New {
		return ContactFormLabelCreate
	}

	return ContactFormLabelEdit
}

func (p EmergencyContactFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}

	return "Save changes"
}
