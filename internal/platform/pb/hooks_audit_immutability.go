package pb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"medikube/internal/domain/audit"
	"medikube/internal/store"
)

// The hook ids, so a second Bind replaces rather than appends, and so that a
// later hand can find these by name to reorder or remove them. They are
// exported because the test that proves an unbound guard is a boot failure has
// to be able to unbind exactly these two.
const (
	AuditImmutableUpdateHookID = "medikube_audit_immutable_update"
	AuditImmutableDeleteHookID = "medikube_audit_immutable_delete"
)

// AuditImmutabilityPriority runs the guard ahead of PocketBase's own
// record-level handler, which sits at -99 (core/record_model.go).
//
// Ahead, and not merely somewhere on the chain: the refusal should be the first
// thing that happens to a write nobody is allowed to make, before field
// interceptors have run and before anything downstream has done work on the
// strength of it.
const AuditImmutabilityPriority = -100

var (
	// ErrAuditImmutable is every refusal this file makes. FR-037: an audit
	// entry is not editable and not deletable through the application.
	ErrAuditImmutable = errors.New("pb: the audit trail is append-only")

	// ErrAuditImmutabilityUnbound reports that the guard is not in force. It is
	// a boot failure and not a warning: a trail nobody protects is a trail an
	// operator will nonetheless be told is immutable.
	ErrAuditImmutabilityUnbound = errors.New("pb: the audit immutability guard is not in force")
)

// BindAuditImmutability makes the audit trail append-only (FR-037, data-model §3).
//
// It binds the two RECORD-level hooks and not the OnRecord*Request family, and
// that is the whole reason this closes every path rather than one of them.
// OnRecordUpdate and OnRecordDelete are triggered from core.App's own save and
// delete (core/record_model.go binds them to OnModelUpdate/OnModelDelete), so
// they fire for PocketBase's REST API, for a save made inside another hook, for
// a direct call from Go, for SaveNoValidate, and for any of those inside a
// transaction. The request hooks would have covered the API alone — and the
// lockdown disables the handlers those are bound inside anyway, so a guard
// placed there would have been silently dead code (research D-14). They are
// also the family forbidigo bans outside hooks.go; this file needs none of
// them, so no lint exclusion is widened.
//
// What it does NOT cover is a statement issued straight at the database.
// internal/store/audit's retention purge is exactly that, deliberately: it
// deletes in one bulk statement that takes an age and can never be pointed at
// an id, which is what lets the guard below refuse EVERY record-level removal
// with no exception in it. An exception is a door, and the horizon belongs in
// the purge's own WHERE clause rather than in a check the caller reaches by
// naming itself.
func BindAuditImmutability(app core.App) error {
	if app == nil {
		return errors.New("pb: the audit immutability guard is bound to no application")
	}

	app.OnRecordUpdate(store.AuditCollection).Bind(&hook.Handler[*core.RecordEvent]{
		Id:       AuditImmutableUpdateHookID,
		Priority: AuditImmutabilityPriority,
		Func:     refuseAuditEdit,
	})

	app.OnRecordDelete(store.AuditCollection).Bind(&hook.Handler[*core.RecordEvent]{
		Id:       AuditImmutableDeleteHookID,
		Priority: AuditImmutabilityPriority,
		Func:     refuseAuditRemoval,
	})

	return nil
}

// refuseAuditRemoval refuses every record-level removal, with no age in it and
// no caller it will listen to.
func refuseAuditRemoval(e *core.RecordEvent) error {
	return fmt.Errorf("%w: the row %s cannot be removed; only the retention purge empties the trail, "+
		"and it does so past the horizon in one statement that never reaches this hook",
		ErrAuditImmutable, e.Record.Id)
}

// refuseAuditEdit refuses every update but the ones PocketBase makes itself.
func refuseAuditEdit(e *core.RecordEvent) error {
	cascade, err := cascadeClearedARelation(e)
	if err != nil {
		return err
	}

	if cascade {
		return e.Next()
	}

	return fmt.Errorf("%w: the row %s cannot be edited", ErrAuditImmutable, e.Record.Id)
}

// clearableAuditRelation is one of audit_events' non-cascading relations: a
// column name to find the field by, and the two accessors that read its value
// off an audit.Event without this file knowing the Go field is called ActorID
// or PatientID.
type clearableAuditRelation struct {
	field string
	get   func(audit.Event) string
	clear func(*audit.Event)
}

// auditClearableRelations is both of data-model §3/§5's non-cascading
// relations: `actor`, unset when the account that made the entry is deleted
// (research D-22), and `patient`, unset when the patient the entry concerned is
// deleted (data-model §5). Both re-saves run through app.SaveNoValidate inside
// the deleting transaction (core/record_model.go's deleteRefRecords) and arrive
// here as an ordinary update; a guard that refused every update would make
// either deletion fail outright.
func auditClearableRelations() []clearableAuditRelation {
	return []clearableAuditRelation{
		{
			field: store.AuditFieldActor,
			get:   func(ev audit.Event) string { return ev.ActorID },
			clear: func(ev *audit.Event) { ev.ActorID = "" },
		},
		{
			field: store.AuditFieldPatient,
			get:   func(ev audit.Event) string { return ev.PatientID },
			clear: func(ev *audit.Event) { ev.PatientID = "" },
		},
	}
}

// cascadeClearedARelation reports whether this update is one of the two
// legitimate writes to a stored audit row: PocketBase emptying a relation
// because the record it pointed at has just been deleted.
//
// The branch is bounded by a fact about the world rather than by a flag the
// caller sets: the referenced record is already gone, because it is deleted
// before its references are unset. Without that clause this is a door — set the
// relation to empty and the row stops saying who did it or which patient it
// concerned, which is the single most valuable edit anyone could make to an
// audit trail. Exactly one relation may clear per update: two clearing at once
// is not a cascade PocketBase produces and this refuses it like any other edit.
func cascadeClearedARelation(e *core.RecordEvent) (bool, error) {
	// Both sides are read through internal/store's own mapper rather than
	// column by column, so "nothing else changed" covers every column the
	// writer writes and keeps covering them when data-model §3 grows one.
	now, err := store.AuditEventFromRecord(e.Record)
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrAuditImmutable, err)
	}

	was, err := store.AuditEventFromRecord(e.Record.Original())
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrAuditImmutable, err)
	}

	var (
		clearedField string
		clearedID    string
		clearedCount int
	)

	for _, relation := range auditClearableRelations() {
		if relation.get(was) == "" || relation.get(now) != "" {
			continue
		}

		clearedCount++
		clearedField = relation.field
		clearedID = relation.get(was)
	}

	if clearedCount != 1 {
		return false, nil
	}

	expect := was
	for _, relation := range auditClearableRelations() {
		if relation.field == clearedField {
			relation.clear(&expect)
		}
	}

	if now != expect {
		return false, nil
	}

	return theReferencedRecordIsGone(e, clearedField, clearedID)
}

func theReferencedRecordIsGone(e *core.RecordEvent, field, recordID string) (bool, error) {
	relation, err := namedRelation(e.Record.Collection(), field)
	if err != nil {
		return false, err
	}

	_, err = e.App.FindRecordById(relation.CollectionId, recordID)

	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, sql.ErrNoRows):
		return true, nil
	default:
		// Fail closed. A lookup that could not answer is not an answer, and a
		// guard that treated a database error as "the record is gone" would
		// hand the one permitted edit to anybody who could make a query fail.
		return false, fmt.Errorf(
			"%w: whether %s %s still exists could not be established, so the edit is refused: %w",
			ErrAuditImmutable, field, recordID, err)
	}
}

// namedRelation finds one relation field by its column name, which is the
// shape the branch above depends on: a non-cascading reference this guard
// already knows how to identify by data-model §3/§5's own field name.
func namedRelation(collection *core.Collection, name string) (*core.RelationField, error) {
	field := collection.Fields.GetByName(name)
	if field == nil {
		return nil, fmt.Errorf("%w: %s has no %s field", ErrAuditImmutable, collection.Name, name)
	}

	relation, isRelation := field.(*core.RelationField)
	if !isRelation {
		return nil, fmt.Errorf("%w: %s.%s is not a relation", ErrAuditImmutable, collection.Name, name)
	}

	return relation, nil
}

// AssertAuditImmutabilityBound fails when the guard is not in force.
//
// It proves it by EXERCISING the hooks rather than by counting what is bound to
// them. Counting answers a different question: a handler bound under the right
// id but tagged to the wrong collection, or shadowed by one bound later, is
// still one handler and would satisfy every arithmetic check while refusing
// nothing. This triggers both hooks with a synthetic event carrying a record
// from the trail and requires each to refuse it.
//
// It writes nothing. The event is never handed to a save, and the terminal
// function does no work — so a boot check for an append-only trail does not
// append to it.
func AssertAuditImmutabilityBound(app core.App) error {
	if app == nil {
		return fmt.Errorf("%w: there is no application for it to be bound to", ErrAuditImmutabilityUnbound)
	}

	collection, err := app.FindCachedCollectionByNameOrId(store.AuditCollection)
	if err != nil {
		return fmt.Errorf("%w: %s is not in the schema: %w", ErrAuditImmutabilityUnbound, store.AuditCollection, err)
	}

	return errors.Join(
		probeAuditGuard(app, collection, "an edit", func(e *core.RecordEvent) error {
			return app.OnRecordUpdate().Trigger(e, func(*core.RecordEvent) error { return nil })
		}),
		probeAuditGuard(app, collection, "a removal", func(e *core.RecordEvent) error {
			return app.OnRecordDelete().Trigger(e, func(*core.RecordEvent) error { return nil })
		}),
	)
}

func probeAuditGuard(
	app core.App,
	collection *core.Collection,
	what string,
	trigger func(*core.RecordEvent) error,
) error {
	event := new(core.RecordEvent)
	event.App = app
	event.Context = context.Background()
	event.Record = core.NewRecord(collection)
	event.Type = core.ModelEventTypeUpdate

	err := trigger(event)

	switch {
	case errors.Is(err, ErrAuditImmutable):
		return nil
	case err != nil:
		return fmt.Errorf("%w: %s of an audit row was refused by something that is not the guard: %w",
			ErrAuditImmutabilityUnbound, what, err)
	default:
		return fmt.Errorf("%w: %s of an audit row ran to the end of the hook chain unrefused",
			ErrAuditImmutabilityUnbound, what)
	}
}
