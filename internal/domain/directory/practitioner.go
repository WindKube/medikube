package directory

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// Practitioner is a clinician the account keeps a directory entry for
// (data-model §2). It never crosses the wire — a DTO always mediates — and
// it never reaches a template unrendered.
//
// Name, phone, email, website and notes are PHI-adjacent: they name a real
// person the account holder consults, and notes may say why. That is why
// MarshalZerologObject below is an allowlist of two identifiers rather than a
// convenience.
type Practitioner struct {
	ID string

	// The authorization anchor and the cascade parent (FR-037). Server-set:
	// absent from every request DTO.
	OwnerID string

	Name string

	// The empty string is "unset", never NULL (research D-25) — the
	// uniqueness index this entity relies on depends on that.
	Specialty Specialty

	// Absent from every request DTO in the same sense OwnerID is: it is set
	// by choosing an existing facility, not typed. Empty means none.
	FacilityID string

	Phone   string
	Email   string
	Website string
	Notes   string

	CreatedAt time.Time
	UpdatedAt time.Time

	// The ETag source, derived from UpdatedAt by the store and never a column
	// of its own, mirroring clinical.Medication.Version.
	Version string
}

const (
	maxPractitionerName  = 200
	maxPractitionerPhone = 40
	maxPractitionerNotes = 5000
)

// Validate reports every offending field at once, in data-model §2's order.
func (p Practitioner) Validate() error {
	var invalid domain.ValidationError

	switch name := strings.TrimSpace(p.Name); {
	case name == "":
		invalid.Add("name", domain.CodeRequired, "a name is required")
	case utf8.RuneCountInString(name) > maxPractitionerName:
		invalid.Addf("name", domain.CodeTooLong, "the name accepts at most %d characters", maxPractitionerName)
	}

	if p.Specialty != "" && !p.Specialty.Valid() {
		invalid.Add("specialty", domain.CodeInvalidValue, "not one of the specialties MediKube accepts")
	}

	if utf8.RuneCountInString(p.Phone) > maxPractitionerPhone {
		invalid.Addf("phone", domain.CodeTooLong, "the phone number accepts at most %d characters", maxPractitionerPhone)
	}

	if utf8.RuneCountInString(p.Notes) > maxPractitionerNotes {
		invalid.Addf("notes", domain.CodeTooLong, "the notes accept at most %d characters", maxPractitionerNotes)
	}

	if p.Email != "" && !isBareAddress(p.Email) {
		invalid.Add("email", domain.CodeInvalidValue, "that is not an email address")
	}

	if p.Website != "" && !isAbsoluteHTTPURL(p.Website) {
		invalid.Add("website", domain.CodeInvalidValue, "that is not a web address")
	}

	return invalid.OrNil()
}

// MarshalZerologObject emits the two identifiers and nothing else.
//
// This is what makes FR-046 structural for a practitioner. Logging one is a
// reasonable thing for a handler or a hook to do, and the only reason it
// cannot leak a name, a phone number or a note is that this method never had
// a line that could. Adding one here is the whole failure — there is no
// second gate.
func (p Practitioner) MarshalZerologObject(e *zerolog.Event) {
	e.Str("id", p.ID).Str("owner_id", p.OwnerID)
}
