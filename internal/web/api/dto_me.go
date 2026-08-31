package api

import (
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/service/identity"
)

// Me is the signed-in account as contracts/account.md publishes it.
//
// Role and EmailConfirmed are here and are READ-ONLY: a person may see their
// own tier and whether their address is confirmed, and may change neither. The
// enforcement is not on this type — it is that MePatch has no member for
// either, so there is no shape a request could carry them in (FR-012, FR-075).
type Me struct {
	ID             string   `json:"id"`
	Email          string   `json:"email"`
	EmailConfirmed bool     `json:"email_confirmed"`
	Name           string   `json:"name"`
	Role           string   `json:"role"`
	UnitSystem     string   `json:"unit_system"`
	Locale         string   `json:"locale"`
	DateFormat     string   `json:"date_format"`
	Theme          string   `json:"theme"`
	CreatedAt      string   `json:"created_at"`
	Counts         MeCounts `json:"counts"`
}

// MeCounts is what the danger zone reads so the deletion confirmation can state
// what will be destroyed rather than asking the person to take it on trust
// (FR-013).
//
// It counts the account's own records and there is no shape here for anybody
// else's: the count is resolved through the same authorization checkpoint every
// record read goes through, with the actor as the only input.
//
// A map keyed by the kind's PATH SEGMENT rather than a member per kind. Two
// reasons, and the second is the one that decided it: phases 002 through 006
// add five more kinds, each of which would otherwise be a member here and a
// member in every mirror of this shape; and a struct tag cannot call
// kind.Kind.Segment(), so a member per kind would spell each plural a second
// time — which is exactly the drift research D-05 exists to prevent and
// internal/architecture/kind_literals_test.go refuses.
//
// The wire shape is unchanged: {"medications": 12}.
type MeCounts map[string]int

// MePatch is FR-011's five changeable things and nothing else.
//
// No Email, no Role, no DisabledAt, no Verified. FR-012 is enforced by the
// SHAPE — a body carrying any of them is 422 `unknown_field` from the decoder,
// before any handler has an opinion — and me_privilege_test.go asserts that
// against EVERY column the users collection has, so a column added later is
// refused by default rather than accepted by omission.
//
// Plain pointers rather than web.Optional: none of the five may be cleared.
// Each is a required column with a published vocabulary or a required value, so
// "absent" and "null" mean the same thing here — leave it alone — and a type
// that could tell them apart would be publishing a third state no operation
// offers.
type MePatch struct {
	Name       *string `json:"name,omitempty"`
	UnitSystem *string `json:"unit_system,omitempty"`
	Locale     *string `json:"locale,omitempty"`
	DateFormat *string `json:"date_format,omitempty"`
	Theme      *string `json:"theme,omitempty"`
}

// ChangePasswordRequest is FR-009's proof plus the replacement.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// DeleteAccountRequest is FR-013's two proofs: the password again, and the
// phrase typed out.
type DeleteAccountRequest struct {
	Password     string `json:"password"`
	Confirmation string `json:"confirmation"`
}

// NewMe renders one account.
//
// It copies member by member rather than embedding identity.User, and that is
// the point: the domain entity carries DisabledAt and UpdatedAt, and an
// embedded struct would publish whatever the entity grows next without anybody
// deciding to.
func NewMe(user domainidentity.User, counts MeCounts) Me {
	// An empty map rather than a nil one: encoding/json writes nil as `null`,
	// and a client reading counts["medications"] off null is a crash where the
	// honest answer is "none".
	if counts == nil {
		counts = MeCounts{}
	}

	return Me{
		ID:             user.ID,
		Email:          user.Email,
		EmailConfirmed: user.EmailConfirmed,
		Name:           user.Name,
		Role:           string(user.Role),
		UnitSystem:     string(user.UnitSystem),
		Locale:         user.Locale,
		DateFormat:     string(user.DateFormat),
		Theme:          string(user.Theme),
		CreatedAt:      wireInstant(user.CreatedAt),
		Counts:         counts,
	}
}

// Profile turns the patch into the change the service accepts.
//
// The values are carried across untranslated and unchecked. Whether `theme` is
// one of the three published words is the domain's question, asked by
// identity.User.Validate over the account with the patch applied, so a value
// this layer refused would be a second vocabulary — and the two would drift the
// first time one of them gained a member.
func (p MePatch) Profile() identity.Profile {
	return identity.Profile{
		Name:       p.Name,
		UnitSystem: convert[domainidentity.UnitSystem](p.UnitSystem),
		Locale:     p.Locale,
		DateFormat: convert[domainidentity.DateFormat](p.DateFormat),
		Theme:      convert[domainidentity.Theme](p.Theme),
	}
}
