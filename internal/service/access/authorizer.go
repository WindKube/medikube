package access

import (
	"context"
	"errors"
	"fmt"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	domainaudit "medikube/internal/domain/audit"
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

// PatientOwners resolves the account a patient belongs to. Declared
// separately from Owners: a patient is not a kind.Kind — it is the anchor
// kinds are authorized against in this phase, not one of them — so it carries
// no kind argument and needs no kind.Valid() gate (phase 002 research D-05).
type PatientOwners interface {
	PatientOwner(ctx context.Context, patientID string) (string, error)
}

// Auditor is the one row Patient's every refusal produces (FR-045, research
// D-28). It is declared here, by the consumer, so this package still owns its
// own seams (Principle II) rather than importing internal/service/audit's
// Writer type; the composition root wires the real writer's Record method
// straight into it.
type Auditor interface {
	Record(ctx context.Context, event domainaudit.Event) error
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
// written against these two methods. Phase 002 adds a third: Patient anchors
// on the person rather than on a kind, and it is optional wiring precisely
// because nothing before phase 002 has a patient to anchor on.
type Authorizer struct {
	owners        Owners
	patientOwners PatientOwners
	auditor       Auditor
}

// Option configures the parts of the checkpoint only phase 002 onward needs,
// so every caller built before patients existed is unaffected by their
// addition.
type Option func(*Authorizer)

// WithPatients wires the patient anchor. Patient refuses every call with a
// construction-time error until this is supplied — a checkpoint that could not
// audit a refusal must not silently grant instead.
func WithPatients(patientOwners PatientOwners, auditor Auditor) Option {
	return func(a *Authorizer) {
		a.patientOwners = patientOwners
		a.auditor = auditor
	}
}

func New(owners Owners, options ...Option) (*Authorizer, error) {
	if owners == nil {
		return nil, errors.New("access: the checkpoint is wired with no way to resolve a record's owner, so it would grant or refuse everything alike")
	}

	authorizer := &Authorizer{owners: owners}

	for _, option := range options {
		option(authorizer)
	}

	return authorizer, nil
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

// Patient is phase 002's anchor (research D-05). Unlike Kind and Record it
// answers with an error on every refusal rather than a Grant of zero value:
// domain.ErrUnauthenticated for a caller with no session and domain.ErrNotFound
// for everyone else who is not the owner — never domain.ErrForbidden, because
// FR-042 makes a patient's existence itself PHI and a 403 would confirm it.
//
// Every refusal writes exactly one audit row (FR-045, research D-28) before it
// is returned, so the row exists whether or not the caller's own request ever
// completes.
func (a *Authorizer) Patient(
	ctx context.Context,
	actor access.Actor,
	patientID string,
	_ access.Permission,
) (access.Grant, error) {
	if a.patientOwners == nil || a.auditor == nil {
		return access.Grant{}, errors.New("access: the checkpoint has no patient anchor wired; construct it with WithPatients")
	}

	if !actor.Authenticated() {
		a.denyPatient(ctx, actor, patientID)

		return access.Grant{}, domain.ErrUnauthenticated
	}

	owner, err := a.patientOwners.PatientOwner(ctx, patientID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			a.denyPatient(ctx, actor, patientID)

			return access.Grant{}, domain.ErrNotFound
		}

		// A checkpoint that could not answer has refused nobody: reporting the
		// failure rather than a refusal is what stops a database outage
		// reading, and being audited, as "that person is not yours".
		return access.Grant{}, fmt.Errorf("access: resolving the owner of a patient: %w", err)
	}

	if actor.IsSuperuser || owner != actor.UserID {
		a.denyPatient(ctx, actor, patientID)

		return access.Grant{}, domain.ErrNotFound
	}

	return access.Grant{Level: access.PermOwn}, nil
}

// denyPatient writes the one row FR-045 requires. Its own failure is not
// reported to the caller: a refusal the trail failed to record is still a
// refusal, and turning an audit outage into a 500 on every denied request
// would make the audit trail load-bearing for availability rather than for
// compliance.
func (a *Authorizer) denyPatient(ctx context.Context, actor access.Actor, patientID string) {
	_ = a.auditor.Record(ctx, domainaudit.Event{
		OccurredAt: time.Now().UTC(),
		ActorID:    actor.UserID,
		ActorKind:  domainaudit.ActorKindUser,
		Action:     domainaudit.ActionAccessDenied,
		TargetKind: domainaudit.TargetKindPatient,
		TargetID:   patientID,
		RequestID:  actor.RequestID,
		PatientID:  patientID,
	})
}
