package directory

import (
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// Facility is a place where care happens — a practice, a pharmacy, a
// hospital, a lab or an imaging centre — kept in one directory regardless of
// kind (research D-24). It never crosses the wire — a DTO always mediates —
// and it never reaches a template unrendered.
//
// Name and notes are PHI-adjacent or PHI outright: which places a person
// attends, and anything written about why, discloses something about their
// care. That is why MarshalZerologObject below is an allowlist of two
// identifiers rather than a convenience.
type Facility struct {
	ID string

	// The authorization anchor and the cascade parent (FR-037). Server-set:
	// absent from every request DTO.
	OwnerID string

	Kind FacilityKind
	Name string
	// The chain a branch belongs to, e.g. "Boots" (research D-24: branches
	// are separate rows, deliberately with no uniqueness on Name).
	Brand string

	Street     string
	City       string
	Region     string
	PostalCode string
	Country    string

	Phone string
	Fax   string
	Email string

	Website   string
	PortalURL string

	Hours        string
	Open24h      bool
	DriveThrough bool
	Services     string
	Notes        string

	CreatedAt time.Time
	UpdatedAt time.Time

	// The ETag source, derived from UpdatedAt by the store and never a column
	// of its own, mirroring clinical.Medication.Version.
	Version string
}

const (
	maxFacilityName       = 200
	maxFacilityBrand      = 120
	maxFacilityStreet     = 200
	maxFacilityCity       = 120
	maxFacilityRegion     = 120
	maxFacilityPostalCode = 20
	maxFacilityCountry    = 80
	maxFacilityPhone      = 40
	maxFacilityFax        = 40
	maxFacilityHours      = 300
	maxFacilityServices   = 500
	maxFacilityNotes      = 5000
)

// Validate reports every offending field at once, in data-model §1's order.
func (f Facility) Validate() error {
	var invalid domain.ValidationError

	if f.Kind == "" {
		invalid.Add("kind", domain.CodeRequired, "a kind is required")
	} else if !f.Kind.Valid() {
		invalid.Add("kind", domain.CodeInvalidValue, "not one of the kinds MediKube accepts")
	}

	switch name := strings.TrimSpace(f.Name); {
	case name == "":
		invalid.Add("name", domain.CodeRequired, "a name is required")
	case utf8.RuneCountInString(name) > maxFacilityName:
		invalid.Addf("name", domain.CodeTooLong, "the name accepts at most %d characters", maxFacilityName)
	}

	checkFacilityLength(&invalid, "brand", "the brand", f.Brand, maxFacilityBrand)
	checkFacilityLength(&invalid, "street", "the street", f.Street, maxFacilityStreet)
	checkFacilityLength(&invalid, "city", "the city", f.City, maxFacilityCity)
	checkFacilityLength(&invalid, "region", "the region", f.Region, maxFacilityRegion)
	checkFacilityLength(&invalid, "postal_code", "the postal code", f.PostalCode, maxFacilityPostalCode)
	checkFacilityLength(&invalid, "country", "the country", f.Country, maxFacilityCountry)
	checkFacilityLength(&invalid, "phone", "the phone number", f.Phone, maxFacilityPhone)
	checkFacilityLength(&invalid, "fax", "the fax number", f.Fax, maxFacilityFax)
	checkFacilityLength(&invalid, "hours", "the hours", f.Hours, maxFacilityHours)
	checkFacilityLength(&invalid, "services", "the services", f.Services, maxFacilityServices)
	checkFacilityLength(&invalid, "notes", "the notes", f.Notes, maxFacilityNotes)

	if f.Email != "" && !isBareAddress(f.Email) {
		invalid.Add("email", domain.CodeInvalidValue, "that is not an email address")
	}
	if f.Website != "" && !isAbsoluteHTTPURL(f.Website) {
		invalid.Add("website", domain.CodeInvalidValue, "that is not a web address")
	}
	if f.PortalURL != "" && !isAbsoluteHTTPURL(f.PortalURL) {
		invalid.Add("portal_url", domain.CodeInvalidValue, "that is not a web address")
	}

	return invalid.OrNil()
}

func checkFacilityLength(invalid *domain.ValidationError, field, label, value string, limit int) {
	if utf8.RuneCountInString(value) > limit {
		invalid.Addf(field, domain.CodeTooLong, "%s accepts at most %d characters", label, limit)
	}
}

// isBareAddress refuses everything a mailbox may legally be wrapped in, the
// same rule identity.User's email applies.
func isBareAddress(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value
}

// isAbsoluteHTTPURL refuses a relative reference or a non-http(s) scheme —
// neither is a web address a browser can navigate to directly.
//
// Hand-rolled rather than net/url.Parse: internal/architecture's domain-imports
// walk denies net/url under internal/domain (Principle II — "a URL is the
// edge's vocabulary"), so this checks only what Validate needs, a scheme and a
// non-empty host, not full RFC 3986.
func isAbsoluteHTTPURL(value string) bool {
	rest, ok := strings.CutPrefix(value, "https://")
	if !ok {
		rest, ok = strings.CutPrefix(value, "http://")
	}
	if !ok || rest == "" {
		return false
	}

	host := rest
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		host = rest[:i]
	}

	return host != ""
}

// MarshalZerologObject emits the two identifiers and nothing else.
//
// This is what makes FR-046 structural for a facility. Logging one is a
// reasonable thing for a handler or a hook to do, and the only reason it
// cannot leak a name, an address or a note is that this method never had a
// line that could. Adding one here is the whole failure — there is no second
// gate.
func (f Facility) MarshalZerologObject(e *zerolog.Event) {
	e.Str("id", f.ID).Str("owner_id", f.OwnerID)
}
