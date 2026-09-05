package facility

import (
	"context"

	"medikube/internal/domain/access"
)

// defaultAuthorizer is FR-037 in its entirety: a facility is the account's
// own, so the only question is whether the actor is a real, authenticated
// account and not the break-glass superuser credential (FR-040's audited
// session, not a MediKube role). There is no record to resolve here — that is
// Repository.Owner's job — so this never fails and never reaches the store.
type defaultAuthorizer struct{}

// NewAuthorizer builds the default Authorizer, for the composition root to
// wire in. The type behind it is unexported: there is nothing to configure and
// nothing a caller should hold beyond the interface.
func NewAuthorizer() Authorizer { return defaultAuthorizer{} }

func (defaultAuthorizer) Actor(_ context.Context, actor access.Actor) (access.Grant, error) {
	if actor.Authenticated() && !actor.IsSuperuser {
		return access.Grant{Level: access.PermOwn}, nil
	}

	return access.Grant{}, nil
}
