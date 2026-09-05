package clinical

import (
	"net/mail"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// EmergencyContact is who to call, in what capacity, and how (data-model
// §4.12, US1). It carries no primary date — FR-051's sort is
// active/primary/name/id, none of them a calendar day.
type EmergencyContact struct {
	ID        string
	PatientID string

	Name         string
	Relationship ContactRelationship
	Phone        string
	PhoneAlt     string
	Email        string
	Address      string
	IsPrimary    bool
	IsActive     bool

	Notes string

	// Tags is data-model §0.8's universal field: any number of the owning
	// account's tags, applied with replace-set semantics (FR-064).
	Tags []string

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string

	// DisplacedID is transient and never stored (research D-16): the id of the
	// contact this write demoted from primary, or empty when nothing was
	// displaced. The repository sets it on the entity a create or an update
	// returns, so the codec can report it without a second read.
	DisplacedID string
}

func (c EmergencyContact) MarshalZerologObject(e *zerolog.Event) {
	e.Str("emergency_contact_id", c.ID).Str("patient_id", c.PatientID)
}

const (
	minContactName = 2
	maxContactName = 100
	maxPhone       = 40
	maxPhoneAlt    = 40
	maxAddress     = 500
)

// Validate is FR-050: name, relationship and phone required; the rest
// optional.
func (c EmergencyContact) Validate() error {
	var invalid domain.ValidationError

	name := strings.TrimSpace(c.Name)
	switch {
	case name == "":
		invalid.Add("name", domain.CodeRequired, "a name is required")
	case len([]rune(name)) < minContactName:
		invalid.Addf("name", domain.CodeTooShort, "the name needs at least %d characters", minContactName)
	default:
		checkLength(&invalid, "name", "the name", name, maxContactName)
	}

	switch {
	case c.Relationship == "":
		invalid.Add("relationship", domain.CodeRequired, "a relationship is required")
	case !c.Relationship.Valid():
		invalid.Add("relationship", domain.CodeInvalidValue, "not one of the relationships MediKube accepts")
	}

	if phone := strings.TrimSpace(c.Phone); phone == "" {
		invalid.Add("phone", domain.CodeRequired, "a phone number is required")
	} else {
		checkLength(&invalid, "phone", "the phone number", phone, maxPhone)
	}

	checkLength(&invalid, "phone_alt", "the second phone number", c.PhoneAlt, maxPhoneAlt)
	checkLength(&invalid, "address", "the address", c.Address, maxAddress)
	checkLength(&invalid, "notes", "the notes", c.Notes, maxNotes)

	if c.Email != "" && !isBareEmail(c.Email) {
		invalid.Add("email", domain.CodeInvalidValue, "that is not an email address")
	}

	return invalid.OrNil()
}

// isBareEmail refuses everything a mailbox may legally be wrapped in — the
// same rule internal/domain/directory applies to a facility's own email.
func isBareEmail(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value
}
