package pb

import (
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"medikube/internal/domain/kind"
	"medikube/internal/realtime"
	"medikube/internal/store"
)

// The hook ids of the realtime publisher, so a second Bind replaces rather than
// appends. PocketBase's hook.Bind appends when no Id is given, and an instance
// wired twice would publish every change twice — which a subscriber cannot tell
// from two changes.
const (
	streamCreateHookID = "medikube_stream_create"
	streamUpdateHookID = "medikube_stream_update"
	streamDeleteHookID = "medikube_stream_delete"
)

// Publisher is the half of realtime.Hub this file uses, declared here by the
// consumer. It takes an event and returns nothing, which is the whole contract:
// publishing happens on the request path inside a post-commit hook, so it must
// not block and it must not fail the write that caused it.
type Publisher interface {
	Publish(event realtime.Event)
}

// RecordStream is everything the publisher needs.
type RecordStream struct {
	Hub Publisher

	// Kinds is what to publish. A collection absent from it is not published,
	// which is why this is passed in rather than derived from the schema:
	// audit_events is a collection too, and a hook that published every save
	// would fan out the audit trail's own writes to every open browser.
	Kinds []kind.Kind
}

// BindRecordStream binds the realtime publisher to the three post-commit record
// hooks.
//
// After…Success and not the pre-commit family, and that is the entire decision
// (contracts/streams.md, research D-21). A pre-commit binding renders a row for
// a transaction that then rolls back, and a live view showing a change that did
// not happen is worse than a live view that lags. PocketBase registers the
// success trigger as a deferred function on the transaction and runs it after
// the transaction is closed with the commit's own error (core/db.go,
// core/db_tx.go), so "rolled back" and "no event" are the same thing by
// construction rather than by a check here.
//
// OnRecord*Request is never used: those hooks are bound inside the built-in CRUD
// handlers that the lockdown disables, so a publisher placed there would be
// silently dead code and every live view would simply never update.
func BindRecordStream(app core.App, config RecordStream) error {
	if app == nil {
		return errors.New("pb: the record stream is bound to no application")
	}

	if config.Hub == nil {
		return errors.New("pb: the record stream is bound with no hub, so no committed change would reach an open view")
	}

	if len(config.Kinds) == 0 {
		return errors.New("pb: the record stream is bound to no kinds, so it would publish nothing")
	}

	collections := make(map[string]kind.Kind, len(config.Kinds))

	for _, k := range config.Kinds {
		if !k.Valid() {
			return fmt.Errorf("pb: %q is not a declared kind, so it has no collection to publish from", k)
		}

		collections[k.Collection()] = k
	}

	publish(app.OnRecordAfterCreateSuccess(), streamCreateHookID, config, collections)
	publish(app.OnRecordAfterUpdateSuccess(), streamUpdateHookID, config, collections)
	publish(app.OnRecordAfterDeleteSuccess(), streamDeleteHookID, config, collections)

	return nil
}

// publish binds one of the three hooks.
//
// It carries no action, and that is deliberate rather than an omission: the
// subscriber re-fetches every id it is told about, and a fetch that comes back
// empty is exactly a row removal. An action on the wire would be a second
// source of truth for something the fetch already answers, and the fetch is the
// one that cannot lie.
func publish(
	on *hook.TaggedHook[*core.RecordEvent],
	id string,
	config RecordStream,
	collections map[string]kind.Kind,
) {
	on.Bind(&hook.Handler[*core.RecordEvent]{
		Id: id,
		Func: func(e *core.RecordEvent) error {
			k, published := collections[e.Record.Collection().Name]
			if !published {
				return e.Next()
			}

			// Three fields, and there is nowhere here a name, a dose or a note
			// could be written. That is what makes contracts/streams.md's
			// "IDs, never bodies" a property of the shape rather than a rule
			// somebody has to remember (FR-038).
			config.Hub.Publish(realtime.Event{
				Kind:     k,
				RecordID: e.Record.Id,
				// store.MedicationOwner is "owner", the column every clinical
				// collection carries its account in (data-model §2), and
				// store.AssertMappedFields is what refuses to boot if the
				// schema stops spelling it that way. Reading it through that
				// constant rather than a literal is what stops this hook
				// publishing an empty owner in silence — core.Record's getters
				// are casts that cannot fail, so a misspelling reads back as
				// the empty string and every removal would then be suppressed.
				OwnerID: e.Record.GetString(store.MedicationOwner),
			})

			return e.Next()
		},
	})
}
