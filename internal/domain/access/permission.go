package access

import "slices"

// Permission is what a caller may do with a patient's data. It is a property of
// the route and never a client-supplied parameter: upstream's
// `required_permission` query argument let the caller name the check it wanted
// to pass, on 41 operations, defaulting to view even on writes.
type Permission int

// The ladder, ascending, so that a held level satisfying a needed one is a
// comparison. It starts at one: a zero Permission is a forgotten argument or an
// unset struct field, and one that read as "may view" would be a silent grant.
const (
	PermView Permission = iota + 1
	PermEdit
	PermOwn
)

var permissions = []Permission{PermView, PermEdit, PermOwn}

// Permissions is the published ladder, cloned so that a caller ranging over it
// cannot reorder the ladder for everybody. The authorization suite is
// table-driven over exactly this.
func Permissions() []Permission { return slices.Clone(permissions) }

func (p Permission) Valid() bool { return slices.Contains(permissions, p) }

// String is what a log line and a refusal reason carry. An unpublished value
// spells "unknown" rather than its integer, because the number would read as a
// level somebody could grant.
func (p Permission) String() string {
	switch p {
	case PermView:
		return "view"
	case PermEdit:
		return "edit"
	case PermOwn:
		return "own"
	default:
		return "unknown"
	}
}

// Satisfies fails closed on both sides. A held level outside the ladder is not
// a level at all, and a needed level outside it is a caller that forgot to say
// what it needed — neither is an authorization.
func (p Permission) Satisfies(need Permission) bool {
	if !p.Valid() || !need.Valid() {
		return false
	}

	return p >= need
}

// Grant is what the authorization checkpoint resolved. It is returned rather
// than being re-derived by the caller: the service asks once, at the top of the
// method, and everything downstream reads the answer.
//
// One field today because the anchor in this phase is the medication's owner.
// Phase 002 adds the patient it resolved against and phase 005 the share it
// came through — both additive, and neither reachable from here.
type Grant struct {
	Level Permission
}

func (g Grant) Allows(need Permission) bool { return g.Level.Satisfies(need) }
