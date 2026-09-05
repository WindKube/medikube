package pb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/obs"
	"medikube/internal/store"
)

// The hook ids, so a second Bind replaces rather than appends. PocketBase's
// hook.Bind appends when no Id is given, and an instance wired twice would
// write two audit rows for one write — which is precisely what the test that
// counts them would then be unable to distinguish from a handler auditing on
// its own.
const (
	auditCreateHookID = "medikube_audit_create"
	auditUpdateHookID = "medikube_audit_update"
	auditDeleteHookID = "medikube_audit_delete"
)

// Trail is the audit writer, declared here by the consumer: this package binds
// the hooks and knows nothing about how a row is stored.
type Trail interface {
	Record(ctx context.Context, event audit.Event) error
}

// RecordAudit is everything the three hooks need.
//
// Actor and Request are functions rather than imports because the actor and the
// correlation id live on the request context put there by internal/web and
// internal/obs, and a platform package that imported the web layer to read them
// would invert the dependency for two accessors.
type RecordAudit struct {
	Trail Trail

	// Kinds is what to audit. A collection absent from it is not audited, which
	// is why this is passed in rather than derived from the schema: audit_events
	// is itself a collection, and a hook that audited every save would record
	// its own writes forever.
	Kinds []kind.Kind

	Actor   func(context.Context) (access.Actor, bool)
	Request func(context.Context) string
}

// BindRecordAudit binds FR-036's three post-commit hooks, one set for every
// kind.
//
// After…Success and not the pre-commit family, and that is the whole decision
// (research D-21): a hook that ran before the commit would write "this
// happened" for a transaction that then rolled back, and a trail that records
// what did not happen is worse than no trail at all.
//
// Handlers write no audit rows for record CRUD. A handler that audits is a
// handler that can forget to, and the six operations are served by one generic
// handler that must not know which kinds are audited.
func BindRecordAudit(app core.App, config RecordAudit) error {
	if app == nil {
		return errors.New("pb: the record audit is bound to no application")
	}

	if config.Trail == nil {
		return errors.New("pb: the record audit is bound with no trail, so every write would go unrecorded")
	}

	if len(config.Kinds) == 0 {
		return errors.New("pb: the record audit is bound to no kinds, so it would record nothing")
	}

	collections := make(map[string]audit.TargetKind, len(config.Kinds))

	for _, k := range config.Kinds {
		target := audit.TargetKind(k.Enum())
		if !target.Valid() {
			return fmt.Errorf("pb: %s has no audit target kind, so its writes could not be recorded", k)
		}

		collections[k.Collection()] = target
	}

	bind(app.OnRecordAfterCreateSuccess(), auditCreateHookID, config, collections, audit.ActionCreate)
	bind(app.OnRecordAfterUpdateSuccess(), auditUpdateHookID, config, collections, audit.ActionUpdate)
	bind(app.OnRecordAfterDeleteSuccess(), auditDeleteHookID, config, collections, audit.ActionDelete)

	return nil
}

// The hook ids of the patients audit, namespaced apart from the kind-based
// ones above: "patients" is not a kind.Kind (research D-05), so it cannot go
// through BindRecordAudit's registry-derived collection list.
func bind(
	on *hook.TaggedHook[*core.RecordEvent],
	id string,
	config RecordAudit,
	collections map[string]audit.TargetKind,
	action audit.Action,
) {
	on.Bind(&hook.Handler[*core.RecordEvent]{
		Id: id,
		Func: func(e *core.RecordEvent) error {
			target, audited := collections[e.Record.Collection().Name]
			if !audited {
				return e.Next()
			}

			if err := config.Trail.Record(e.Context, config.event(e, action, target)); err != nil {
				// Reported, not swallowed. The write has already committed, so
				// there is nothing to undo; what an unrecorded write must not
				// be is silent, and the error travels back out through the
				// save the caller is still waiting on.
				return fmt.Errorf("pb: recording the %s of a %s: %w", action, target, err)
			}

			return e.Next()
		},
	})
}

// event is the row. It carries the record's id and nothing else about the
// record: there is no column here a name, a dose or a note could be written
// into, which is what makes FR-038 structural (data-model §3).
func (c RecordAudit) event(e *core.RecordEvent, action audit.Action, target audit.TargetKind) audit.Event {
	event := audit.Event{
		OccurredAt: time.Now().UTC(),
		ActorKind:  audit.ActorKindSystem,
		Action:     action,
		TargetKind: target,
		TargetID:   e.Record.Id,
		RequestID:  c.requestID(e.Context),
	}

	// store.RecordPatientField is the column every registered kind carries
	// its person in (data-model §6), which is what lets the chart summary's
	// recent activity find this row without a hook per kind. A delete
	// cascading from the patient's own deletion (US6) runs in the same
	// transaction, and by the time this AFTER hook fires the patient row is
	// genuinely gone — the relation field cannot validate against it, so
	// this row is left with no patient exactly as the patient's own delete
	// audit row is (research D-25/FR-029). A medication deleted on its own,
	// patient still very much alive, keeps it.
	if patientID := e.Record.GetString(store.RecordPatientField); patientID != "" {
		if action != audit.ActionDelete || c.patientExists(e, patientID) {
			event.PatientID = patientID
		}
	}

	actor, present := c.actor(e.Context)
	if !present || !actor.Authenticated() {
		return event
	}

	switch {
	case actor.IsSuperuser:
		event.ActorKind = audit.ActorKindSuperuser
	default:
		event.ActorKind = audit.ActorKindUser
		event.ActorID = actor.UserID
	}

	return event
}

// patientExists is only ever asked during a delete, and only when the
// deleted record still names one: a false here means this delete is riding a
// cascade from the patient's own deletion, the one case the relation field
// cannot validate against (see event's comment above).
func (c RecordAudit) patientExists(e *core.RecordEvent, patientID string) bool {
	_, err := e.App.FindRecordById(store.PatientCollection, patientID)

	return err == nil
}

func (c RecordAudit) actor(ctx context.Context) (access.Actor, bool) {
	if c.Actor == nil || ctx == nil {
		return access.Actor{}, false
	}

	return c.Actor(ctx)
}

// requestID is FR-054's correlation handle, and it is required: a row that
// correlates to nothing cannot be joined to the log line for the request that
// produced it, which is the whole use an operator has for the trail.
//
// A write with no request behind it — a migration, a seed, a background job —
// gets a fresh one from the same minter internal/obs uses for a request, so the
// two are the same shape. T231's run id replaces this branch, at which point a
// job's rows and its log lines carry one handle rather than one each.
func (c RecordAudit) requestID(ctx context.Context) string {
	if c.Request != nil && ctx != nil {
		if id := c.Request(ctx); id != "" {
			return id
		}
	}

	_, edge := obs.NewEdge(context.Background(), "")

	return edge.CorrelationID()
}
