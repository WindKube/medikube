package person

import (
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// Patient is one person an account keeps records for — themselves, a child, a
// parent (data-model §3). It never crosses the wire — a DTO always mediates —
// and it never reaches a template unrendered, so it carries no tags, no JSON
// names and no display strings.
//
// Every field but the identifiers, the flags and the two relations is PHI: a
// name, a birth date and an address on their own identify a person and their
// medical record. That is why MarshalZerologObject below is an allowlist of
// two identifiers rather than a convenience.
type Patient struct {
	ID string

	// The authorization anchor and the cascade parent. Server-set: absent from
	// every request DTO, so a request cannot nominate its own owner (FR-002).
	OwnerID string

	FirstName string
	LastName  string

	// The zero value is "not recorded" (research D-09, D-20) — the only rows
	// permitted to carry it are server-provisioned self-records.
	BirthDate domain.Date

	Sex                 Sex
	BloodType           BloodType
	RelationshipToOwner RelationshipToOwner

	// Canonical SI, always (FR-007, research D-21). Zero means "not set" —
	// the valid ranges (30..272, 0.5..450) both exclude zero.
	HeightCM float64
	WeightKG float64

	Address string

	// Absent from every request DTO. FR-040.
	PrimaryPractitionerID string

	// Server-set (FR-004). At most one true row per owner, enforced by a
	// partial unique index at the storage layer.
	IsSelfRecord bool

	CreatedAt time.Time
	UpdatedAt time.Time

	// The ETag source, derived from UpdatedAt by the store and never a column
	// of its own, mirroring clinical.Medication.Version.
	Version string
}

// MarshalZerologObject emits the two identifiers and nothing else.
//
// This is what makes FR-046 structural. Logging a patient is a reasonable
// thing for a handler or a hook to do, and the only reason it cannot leak a
// name, a birth date or an address is that this method never had a line that
// could. Adding one here is the whole failure — there is no second gate.
func (p Patient) MarshalZerologObject(e *zerolog.Event) {
	e.Str("id", p.ID).Str("owner_id", p.OwnerID)
}
