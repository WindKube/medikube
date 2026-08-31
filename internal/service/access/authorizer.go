package access

import (
	"context"
	"errors"
	"fmt"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
)

// Owners resolves the account a record belongs to. It is declared here, by the
// consumer (Principle II), and implemented by internal/store — so this package
// decides and knows nothing about a database.
//
// A record that does not exist must be reported as domain.ErrNotFound and NOT
// as a refusal. That distinction is the whole of research D-20: a checkpoint
// that refused every unknown identifier would write an access_denied row for
// every typo, and a genuine miss would become indistinguishable from a denial
// in the one place that can still tell them apart.
type Owners interface {
	Owner(ctx context.Context, k kind.Kind, recordID string) (string, error)
}

// Authorizer is THE authorization checkpoint. There is one, every service
// reaches it, and nothing else decides who may see what.
//
// In this phase the anchor is the record's owner and the ladder has one rung
// that matters: the owner holds everything over their own records and nobody
// else holds anything. Phase 002 anchors on the patient and phase 005 on the
// share; both are changes to what this resolves, not to who asks it.
//
// This is the minimal implementation US1 needs to be reachable at all. T239
// extends it — it does not replace the seam, because every caller is already
// written against these two methods.
type Authorizer struct {
	owners Owners
}

func New(owners Owners) (*Authorizer, error) {
	if owners == nil {
		return nil, errors.New("access: the checkpoint is wired with no way to resolve a record's owner, so it would grant or refuse everything alike")
	}

	return &Authorizer{owners: owners}, nil
}

// Kind is the checkpoint for the kind itself: may this actor reach these
// records at all, and at what level. A list and a create are authorized against
// it, neither of which names an existing record.
//
// A PocketBase superuser is granted nothing here. It is the break-glass
// credential and not a MediKube role (data-model §1, FR-040); the admin UI is
// where it reads data, audited, and MediKube's own routes are not a second
// unaudited way in.
func (a *Authorizer) Kind(
	_ context.Context,
	actor access.Actor,
	k kind.Kind,
	_ access.Permission,
) (access.Grant, error) {
	if !reachable(actor, k) {
		return access.Grant{}, nil
	}

	return access.Grant{Level: access.PermOwn}, nil
}

// Record is the checkpoint for one addressed record.
//
// It grants for a record that is not there. The repository reports the miss,
// and that is what keeps a genuine not-found out of the audit trail while every
// real refusal is in it.
func (a *Authorizer) Record(
	ctx context.Context,
	actor access.Actor,
	k kind.Kind,
	recordID string,
	_ access.Permission,
) (access.Grant, error) {
	if !reachable(actor, k) {
		return access.Grant{}, nil
	}

	owner, err := a.owners.Owner(ctx, k, recordID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return access.Grant{Level: access.PermOwn}, nil
		}

		// A checkpoint that could not answer has refused nobody. Reporting the
		// failure rather than a refusal is what stops a database outage
		// reading as "that record is not yours".
		return access.Grant{}, fmt.Errorf("access: resolving the owner of a %s: %w", k, err)
	}

	if owner != actor.UserID {
		return access.Grant{}, nil
	}

	return access.Grant{Level: access.PermOwn}, nil
}

func reachable(actor access.Actor, k kind.Kind) bool {
	return actor.Authenticated() && !actor.IsSuperuser && k.Valid()
}
