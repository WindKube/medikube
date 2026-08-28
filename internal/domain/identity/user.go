package identity

import (
	"time"

	"github.com/rs/zerolog"
)

// User is one account. It never crosses the wire — a DTO always mediates — and
// it never reaches a template unrendered, so it carries no tags, no JSON names
// and no display strings.
//
// It holds no password and no token, deliberately. PocketBase owns the credential
// and the tokenKey that a password change rotates (research D-13, D-16); a copy
// of either on this struct would be a second place for a secret to leak from
// and a second place for it to go stale.
type User struct {
	ID string

	// The sign-in identity, and the one field FR-011 does not let a person
	// change about themselves. Addresses differing only in case are the same
	// account, which the LOWER(email) unique index — not this type — enforces
	// (FR-003).
	Email string

	// PocketBase's `verified`, set by confirming a message sent to the address
	// and never by a request DTO (FR-075).
	EmailConfirmed bool

	// The display name. PHI-adjacent: it names a person who has a medical record
	// on this instance, which on a self-hosted server is a disclosure by itself.
	Name string

	Role Role

	UnitSystem UnitSystem
	Locale     string
	DateFormat DateFormat
	Theme      Theme

	// The zero value is an account in good standing. A non-zero instant refuses
	// sign-in, is set only by an operator, and is absent from every request DTO
	// (data-model §1, FR-012).
	DisabledAt time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsDisabled is data-model §1's "non-null ⇒ sign-in refused". A method rather
// than a stored flag, because a flag alongside the instant is a second truth to
// keep in step with the first.
func (u User) IsDisabled() bool { return !u.DisabledAt.IsZero() }

// MarshalZerologObject emits the record id and nothing else.
//
// This is what makes FR-038 structural for an account. Logging a user is a
// reasonable thing for a handler or a hook to do, and the only reason it cannot
// leak an address or a name is that this method never had a line that could.
// Adding one here is the whole failure — there is no second gate.
//
// The id is opaque: PocketBase's fifteen random characters identify the record
// without describing the person, so an operator can follow one account through
// the log stream and learn nothing about who it belongs to.
func (u User) MarshalZerologObject(e *zerolog.Event) {
	e.Str("user_id", u.ID)
}
