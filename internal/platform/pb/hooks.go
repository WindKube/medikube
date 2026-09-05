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
				// store.MedicationPatient is "patient", the column every
				// clinical collection carries its person in from phase 002
				// onward (data-model §6), and store.AssertMappedFields is what
				// refuses to boot if the schema stops spelling it that way.
				// Reading it through that constant rather than a literal is
				// what stops this hook publishing an empty patient in silence
				// — core.Record's getters are casts that cannot fail, so a
				// misspelling reads back as the empty string and every removal
				// would then be suppressed.
				PatientID: e.Record.GetString(store.MedicationPatient),
			})

			return e.Next()
		},
	})
}

// The hook ids of the sign-in audit, so a second Bind replaces rather than
// appends. An instance wired twice would write two `login` rows for one
// sign-in, which is exactly what a test counting them could then not tell apart
// from a handler auditing on its own.
const (
	auditLoginHookID        = "medikube_audit_login"
	auditLoginFailedHookID  = "medikube_audit_login_failed"
	auditAdminSessionHookID = "medikube_audit_admin_session"
)

// AuthAuditPriority puts the audit row AHEAD of the response writer bound on
// the same hook (internal/web/api's, at -10).
//
// The ordering is the decision, not an accident. PocketBase mints the token
// before it triggers OnRecordAuthRequest, so nothing here can stop a token
// existing — but everything here runs before it is handed over. A sign-in whose
// row cannot be written therefore hands out no cookie and no body, and the
// caller gets a failure rather than a session nothing recorded. Bound after the
// writer, the same failure would arrive with the response already sent and the
// credential already in the browser.
//
// It is a number rather than an import because internal/web/api imports this
// package; hooks_auth_test.go asserts the two agree from the side that can see
// both.
const AuthAuditPriority = -20

// AuthAudit is everything the sign-in hooks need.
//
// Request is a function rather than an import because the correlation id lives
// on the request context internal/obs put it there, and a platform package that
// imported the web layer to read one accessor would invert the dependency.
type AuthAudit struct {
	Trail Trail

	Request func(context.Context) string

	// Now is the clock. It is injected because every row carries a server
	// timestamp and a test that could not pin one would be asserting against
	// time.Now.
	Now func() time.Time
}

// BindAuthAudit binds FR-036's sign-in rows: `login` for a successful sign-in
// through EITHER path, `login_failed` for one refused by PocketBase's own
// route, and `admin_session` when a superuser session begins (FR-040).
//
// OnRecordAuthRequest and not the login handler, and that is research D-14's
// whole point: PocketBase's native POST /api/collections/users/auth-with-password
// stays reachable by design, so there are two paths to a valid session and a
// handler-side audit would leave one of them silently unrecorded. The hook
// covers both with one piece of code, and T205 is the structural proof — it
// fails the moment anybody reimplements authentication inside a handler.
//
// This is the one OnRecord*Request family the forbidigo rule permits, for the
// same reason: the CRUD hooks are bound inside handlers the lockdown disables
// and are therefore dead code, while the auth hooks genuinely fire.
//
// NO CREDENTIAL REACHES A ROW. audit.Event has no field a password, a token or
// an address could be written into, so that is a property of the shape rather
// than a rule each call site has to remember (data-model §3).
func BindAuthAudit(app core.App, config AuthAudit) error {
	if app == nil {
		return errors.New("pb: the sign-in audit is bound to no application")
	}

	if config.Trail == nil {
		return errors.New("pb: the sign-in audit is bound with no trail, so every sign-in would go unrecorded")
	}

	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}

	app.OnRecordAuthRequest(store.AccountCollection).Bind(&hook.Handler[*core.RecordAuthRequestEvent]{
		Id:       auditLoginHookID,
		Priority: AuthAuditPriority,
		Func:     config.recordSignIn(audit.ActionLogin, audit.ActorKindUser),
	})

	// The same hook on the other collection. The tags isolate cleanly, so an
	// ordinary sign-in writes no admin_session row and a superuser's writes no
	// login row — which is the whole of T221a's assertion (FR-040).
	app.OnRecordAuthRequest(core.CollectionNameSuperusers).Bind(&hook.Handler[*core.RecordAuthRequestEvent]{
		Id:       auditAdminSessionHookID,
		Priority: AuthAuditPriority,
		Func:     config.recordSignIn(audit.ActionAdminSession, audit.ActorKindSuperuser),
	})

	app.OnRecordAuthWithPasswordRequest(store.AccountCollection).Bind(&hook.Handler[*core.RecordAuthWithPasswordRequestEvent]{
		Id:       auditLoginFailedHookID,
		Priority: AuthAuditPriority,
		Func:     config.recordRefusal,
	})

	return nil
}

// recordSignIn writes one row for a session that has just begun.
//
// It writes NOTHING for a token renewal. OnRecordAuthRequest fires for
// auth-refresh as well as for a sign-in, and the two are told apart by
// AuthMethod: a fresh sign-in carries core.MFAMethodPassword and a renewal
// carries the empty string. Bound naively, every browser extending its session
// would write a second sign-in into a two-year medical trail — and the trail
// would then say a person signed in hourly all night (research D-14).
func (c AuthAudit) recordSignIn(action audit.Action, actorKind audit.ActorKind) func(*core.RecordAuthRequestEvent) error {
	return func(e *core.RecordAuthRequestEvent) error {
		if e.AuthMethod != core.MFAMethodPassword {
			return e.Next()
		}

		event := audit.Event{
			OccurredAt: c.Now().UTC(),
			ActorKind:  actorKind,
			Action:     action,
			TargetKind: audit.TargetKindUser,
			TargetID:   e.Record.Id,
			RequestID:  c.requestID(e.Request.Context()),
		}

		// The actor relation points at `users` and a superuser is not in it, so
		// a superuser's id would be a dangling reference the column refuses.
		// actor_kind is what says a superuser did it, which is the same reason
		// it survives an account deletion (research D-22).
		if actorKind == audit.ActorKindUser {
			event.ActorID = e.Record.Id
		}

		if err := c.Trail.Record(e.Request.Context(), event); err != nil {
			// Returned, not swallowed, and returned BEFORE e.Next(): the
			// response writer is bound after this, so a sign-in that cannot be
			// recorded hands out no session.
			return fmt.Errorf("pb: recording a %s: %w", action, err)
		}

		return e.Next()
	}
}

// recordRefusal writes the `login_failed` row for a sign-in PocketBase's own
// route refused.
//
// It covers that route and only that route, which is deliberate rather than a
// gap: OnRecordAuthWithPasswordRequest is triggered from
// apis/record_auth_with_password.go and from nowhere else, so MediKube's own
// /api/v1/auth/login never reaches it. The service writes that path's row from
// its one refusal branch, and between them every refused sign-in leaves exactly
// one row — hooks_auth_test.go counts both paths for exactly that reason.
//
// The row names the ACCOUNT somebody aimed at and never the address they typed.
// target_id is the account id when the address has one and empty when it does
// not: writing the typed string would put a real person's address — possibly a
// stranger's, possibly a typo of one — into a two-year medical audit trail
// (contracts/auth.md).
func (c AuthAudit) recordRefusal(e *core.RecordAuthWithPasswordRequestEvent) error {
	refused := e.Next()
	if refused == nil {
		return nil
	}

	var aimedAt string
	if e.Record != nil {
		aimedAt = e.Record.Id
	}

	// ActorKind is `user` with no id: somebody was at the keyboard, and who it
	// was is exactly what the attempt failed to establish. `system` would claim
	// MediKube refused itself.
	if err := c.Trail.Record(e.Request.Context(), audit.Event{
		OccurredAt: c.Now().UTC(),
		ActorKind:  audit.ActorKindUser,
		Action:     audit.ActionLoginFailed,
		TargetKind: audit.TargetKindUser,
		TargetID:   aimedAt,
		RequestID:  c.requestID(e.Request.Context()),
	}); err != nil {
		// Joined, so the caller still gets the refusal it earned. A 500 where
		// every other failed sign-in is a 400 would be an oracle by itself.
		return errors.Join(refused, fmt.Errorf("pb: recording a %s: %w", audit.ActionLoginFailed, err))
	}

	return refused
}

// requestID is FR-054's correlation handle, and it is required for the same
// reason the record hooks' is: a row that correlates to nothing cannot be
// joined to the log line for the request that produced it.
func (c AuthAudit) requestID(ctx context.Context) string {
	if c.Request != nil && ctx != nil {
		if id := c.Request(ctx); id != "" {
			return id
		}
	}

	_, edge := obs.NewEdge(context.Background(), "")

	return edge.CorrelationID()
}

// The hook ids of the patients audit, so a second Bind replaces rather than
// appends (T067).
const (
	patientAuditCreateHookID = "medikube_audit_patient_create"
	patientAuditUpdateHookID = "medikube_audit_patient_update"
	patientAuditDeleteHookID = "medikube_audit_patient_delete"
)

// PatientAudit is everything the three patient hooks need. It is not
// RecordAudit (hooks_records.go): a patient is not a kind.Kind (research
// D-05, the anchor rather than a record kind), so it carries no Kinds
// allowlist and every write it is bound to is a patient row by construction —
// and every row it writes sets PatientID, which RecordAudit's rows never
// carry.
type PatientAudit struct {
	Trail Trail

	Actor   func(context.Context) (access.Actor, bool)
	Request func(context.Context) string
}

// BindPatientAudit binds FR-036's three post-commit hooks for the patients
// collection.
//
// After…Success and not the pre-commit family, for the same reason
// BindRecordAudit is (research D-21): a hook bound before the commit would
// record a write that then rolled back.
//
// It tolerates a nil request context (T067): a patient write reached from a
// background job or a migration still gets a row, with a freshly minted
// correlation id in place of one a request never carried.
func BindPatientAudit(app core.App, config PatientAudit) error {
	if app == nil {
		return errors.New("pb: the patient audit is bound to no application")
	}

	if config.Trail == nil {
		return errors.New("pb: the patient audit is bound with no trail, so every write would go unrecorded")
	}

	bindPatient(app.OnRecordAfterCreateSuccess(), patientAuditCreateHookID, config, audit.ActionCreate)
	bindPatient(app.OnRecordAfterUpdateSuccess(), patientAuditUpdateHookID, config, audit.ActionUpdate)
	bindPatient(app.OnRecordAfterDeleteSuccess(), patientAuditDeleteHookID, config, audit.ActionDelete)

	return nil
}

func bindPatient(
	on *hook.TaggedHook[*core.RecordEvent],
	id string,
	config PatientAudit,
	action audit.Action,
) {
	on.Bind(&hook.Handler[*core.RecordEvent]{
		Id: id,
		Func: func(e *core.RecordEvent) error {
			if e.Record.Collection().Name != store.PatientCollection {
				return e.Next()
			}

			if err := config.Trail.Record(e.Context, config.event(e, action)); err != nil {
				return fmt.Errorf("pb: recording the %s of a patient: %w", action, err)
			}

			return e.Next()
		},
	})
}

// event is the row. TargetID and PatientID are both the patient's own id —
// unlike a clinical record's audit row, a patient row's target IS the
// patient — and there is nowhere on it a name, a birth date or an address
// could be written (FR-038).
func (c PatientAudit) event(e *core.RecordEvent, action audit.Action) audit.Event {
	event := audit.Event{
		OccurredAt: time.Now().UTC(),
		ActorKind:  audit.ActorKindSystem,
		Action:     action,
		TargetKind: audit.TargetKindPatient,
		TargetID:   e.Record.Id,
		RequestID:  c.requestID(e.Context),
	}

	// PatientID is a RelationField (research D-25/FR-029): a delete row would
	// name a patient the write itself just removed, which the relation
	// cannot validate against — this AFTER hook runs once the row is
	// genuinely gone. TargetID already carries the same id as plain text, so
	// nothing about which patient this was is lost (US6-5, SC-009).
	if action != audit.ActionDelete {
		event.PatientID = e.Record.Id
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

func (c PatientAudit) actor(ctx context.Context) (access.Actor, bool) {
	if c.Actor == nil || ctx == nil {
		return access.Actor{}, false
	}

	return c.Actor(ctx)
}

// requestID mirrors RecordAudit's own: a write with no request behind it
// still gets a correlation handle, minted the same way a request's is.
func (c PatientAudit) requestID(ctx context.Context) string {
	if c.Request != nil && ctx != nil {
		if id := c.Request(ctx); id != "" {
			return id
		}
	}

	_, edge := obs.NewEdge(context.Background(), "")

	return edge.CorrelationID()
}
