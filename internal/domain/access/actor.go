package access

import "medikube/internal/domain/identity"

// Actor is who is asking. It is built once per request from the authentication
// record and is the only thing derived from the token (shared design §6.6);
// every service method takes it explicitly rather than reaching into a context,
// so authorization is a visible part of the signature.
//
// It deliberately carries no email address and no display name. Both are
// PHI-adjacent, both are redacted where identity.User is logged, and an actor
// travels into every audit write and every service call — which is exactly the
// path a copy of them would leak along.
type Actor struct {
	// The account id, and the anchor every ownership decision is made against.
	// Empty means nobody: an unauthenticated request still has an actor so that
	// its refusal can be audited and correlated.
	UserID string

	// MediKube's application tier, not a PocketBase superuser (data-model §1).
	Role identity.Role

	// A PocketBase superuser, which is the break-glass credential and not a
	// MediKube role at all. Every session it opens is audited (FR-040).
	IsSuperuser bool

	// The correlation id of the request or of the background run, carried here
	// because the audit row a refusal produces requires one.
	RequestID string
}

// Anonymous is the actor of a request that carried no usable token. It is a
// value rather than a nil pointer so that no call site has to nil-check before
// authorizing, and every path through the checkpoint ends in the same refusal.
func Anonymous(requestID string) Actor {
	return Actor{RequestID: requestID}
}

// Authenticated reads the account id and not the superuser flag: the flag is
// only ever true beside an id, and a check that trusted it alone would promote
// a request that carries no account at all.
func (a Actor) Authenticated() bool {
	return a.UserID != ""
}
