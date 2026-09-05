package pb_test

import (
	"context"
	"database/sql"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/audit"
	"medikube/internal/platform/pb"
	"medikube/internal/store"
	storeaudit "medikube/internal/store/audit"
)

// T232. An audit row cannot be edited or removed through ANY path: PocketBase's
// own API, the record hooks, or a direct call on the application from Go.
//
// THE COLLECTION IS BUILT BY HAND AND MediKube's MIGRATIONS ARE NOT IMPORTED,
// for the reason hooks_records_test.go states at length: core.AppMigrations is a
// package-level registry, so one import of medikube/internal/store/migrations —
// directly or through medikube/internal/testsupport — would apply MediKube's
// schema to every tests.NewTestApp in this binary and the lockdown suite next
// door would start failing on `users` records it creates bare.
//
// The cost of building it by hand is that this file spells six column names
// internal/store keeps unexported. That cost is paid off by
// TestTheHandBuiltTrailIsTheOneStoresMapperReads, which round-trips a fully
// populated event through store's own mapper and a reload from disk: a
// misspelling here stops being written, reads back as a zero value, and fails.

// The seven columns of data-model §3. Only the first is published by
// internal/store; the other six are spelled here and proved by the round trip.
const (
	trailFieldOccurredAt = store.AuditFieldOccurredAt
	trailFieldActor      = "actor"
	trailFieldActorKind  = "actor_kind"
	trailFieldAction     = "action"
	trailFieldTargetKind = "target_kind"
	trailFieldTargetID   = "target_id"
	trailFieldRequestID  = "request_id"
	trailFieldPatient    = "patient"
)

// The two PocketBase bookkeeping columns the migration adds explicitly, because
// NewBaseCollection does not.
const (
	trailFieldCreated = "created"
	trailFieldUpdated = "updated"
)

// trailPassword is not a fixture credential. There is no fixture here: the
// account is created two lines above the row that points at it.
//
//nolint:gosec // a throwaway password for an account this test creates
const trailPassword = "medikube-immutability-probe"

const (
	trailActorEmail    = "actor@immutability.test"
	trailOtherEmail    = "other@immutability.test"
	trailBystanderMail = "bystander@immutability.test"
)

// trailNeighbourCollection is a collection that is not the trail. It is not a
// kind either: there are no kinds in this file, because the schema is created
// three lines above the record.
const trailNeighbourCollection = "not_the_audit_trail"

// trailHiddenAccounts is where the account table is moved to when a test needs
// the actor lookup to fail rather than to come back empty.
const trailHiddenAccounts = "accounts_the_lookup_cannot_reach"

// trailValues is the migration's enumValues, which this file cannot import.
func trailValues[T ~string](values []T) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}

	return out
}

// trailSchema builds data-model §3's collection on a stock instance.
//
// The actor relation is the shape the whole permitted-update branch turns on:
// not required and NOT cascading, so PocketBase unsets it and re-saves the row
// when the account goes rather than deleting the row with it (research D-22).
func trailSchema(t testing.TB, app core.App) {
	t.Helper()

	accounts, err := app.FindCollectionByNameOrId(store.AccountCollection)
	require.NoError(t, err, "the stock instance has no account collection for the actor relation to point at")

	collection := core.NewBaseCollection(store.AuditCollection)

	collection.Fields.Add(&core.DateField{Name: trailFieldOccurredAt, Required: true})
	collection.Fields.Add(&core.RelationField{
		Name:          trailFieldActor,
		Required:      false,
		CascadeDelete: false,
		MaxSelect:     1,
		CollectionId:  accounts.Id,
	})
	collection.Fields.Add(&core.SelectField{
		Name: trailFieldActorKind, Required: true, MaxSelect: 1, Values: trailValues(audit.ActorKinds()),
	})
	collection.Fields.Add(&core.SelectField{
		Name: trailFieldAction, Required: true, MaxSelect: 1, Values: trailValues(audit.Actions()),
	})
	collection.Fields.Add(&core.SelectField{
		Name: trailFieldTargetKind, Required: true, MaxSelect: 1, Values: trailValues(audit.TargetKinds()),
	})
	collection.Fields.Add(&core.TextField{Name: trailFieldTargetID, Max: audit.MaxTargetID})
	collection.Fields.Add(&core.TextField{Name: trailFieldRequestID, Required: true, Max: audit.MaxRequestID})
	// A bare TextField rather than a RelationField to `patients`: this file
	// deliberately builds no other MediKube collection than the accounts one
	// (see the file header), and the column stores an opaque id either way.
	collection.Fields.Add(&core.TextField{Name: trailFieldPatient, Max: audit.MaxPatientID})
	collection.Fields.Add(&core.AutodateField{Name: trailFieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: trailFieldUpdated, OnCreate: true, OnUpdate: true})

	require.NoError(t, app.Save(collection), "create the audit trail collection")
}

// trailApp is one throwaway instance with the trail on it and the guard bound.
func trailApp(t *testing.T) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	trailSchema(t, app)
	require.NoError(t, pb.BindAuditImmutability(app))

	return app
}

func trailAccount(t testing.TB, app core.App, email string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(store.AccountCollection)
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.SetEmail(email)
	record.SetPassword(trailPassword)
	require.NoError(t, app.Save(record), "create the account %s", email)

	return record
}

// trailEvent is one row with every column carrying a distinct, non-zero value,
// so a column that silently stopped being written reads back as a zero value
// that a comparison catches.
func trailEvent(actorID string) audit.Event {
	return audit.Event{
		OccurredAt: time.Date(2024, time.March, 14, 9, 30, 0, 0, time.UTC),
		ActorID:    actorID,
		ActorKind:  audit.ActorKindUser,
		Action:     audit.ActionAccessDenied,
		TargetKind: audit.TargetKindMedication,
		TargetID:   "the-record-somebody-reached-for",
		RequestID:  "the-request-that-reached-for-it",
		PatientID:  "mkpatamara00001",
	}
}

// trailRow appends one row the way production appends one: through
// internal/store's own mapper, so the columns this file names are the columns
// the writer names.
func trailRow(t testing.TB, app core.App, event audit.Event) *core.Record {
	t.Helper()

	collection, err := app.FindCachedCollectionByNameOrId(store.AuditCollection)
	require.NoError(t, err)

	record := core.NewRecord(collection)
	require.NoError(t, store.AuditEventToRecord(record, event))
	require.NoError(t, app.Save(record),
		"appending must still work: the guard refuses edits and removals, never the append the trail is for")

	return record
}

// trailReload reads a row back from disk. From disk and never from the handed
// record: an in-memory record still carries whatever a refused edit set on it,
// so an assertion against it would pass while the row on disk had changed.
func trailReload(t testing.TB, app core.App, id string) audit.Event {
	t.Helper()

	record, err := app.FindRecordById(store.AuditCollection, id)
	require.NoError(t, err, "the row is gone from the trail")

	event, err := store.AuditEventFromRecord(record)
	require.NoError(t, err)

	return event
}

func trailFresh(t testing.TB, app core.App, id string) *core.Record {
	t.Helper()

	record, err := app.FindRecordById(store.AuditCollection, id)
	require.NoError(t, err, "the row is gone from the trail")

	return record
}

// -----------------------------------------------------------------------------
// The schema this file invents is the schema internal/store reads.
// -----------------------------------------------------------------------------

func TestTheHandBuiltTrailIsTheOneStoresMapperReads(t *testing.T) {
	t.Parallel()

	app := trailApp(t)
	actor := trailAccount(t, app, trailActorEmail)

	written := trailEvent(actor.Id)

	// Guard on the guard: every column has to carry something a zero value
	// could not be mistaken for, or a misspelled column name would round-trip
	// as "" == "" and this whole file would be testing a collection nobody
	// writes to.
	populated := reflect.ValueOf(written)
	for i := range populated.NumField() {
		require.Falsef(t, populated.Field(i).IsZero(),
			"trailEvent leaves %s zero, so a column that stopped being written would still round-trip",
			populated.Type().Field(i).Name)
	}

	record := trailRow(t, app, written)

	assert.Equal(t, written, trailReload(t, app, record.Id),
		"a column this file names is not the column internal/store's mapper writes")
}

// -----------------------------------------------------------------------------
// PATH 1 of 3 — a direct call on the application from Go.
// -----------------------------------------------------------------------------

// trailWrite is one way Go code reaches the database with a record in hand.
type trailWrite struct {
	name  string
	apply func(app *tests.TestApp, record *core.Record) error
}

// trailSaves is every save PocketBase publishes, including the two that skip
// validation and the two that run inside a transaction. A guard bound to only
// one of them is a guard with three doors left open.
func trailSaves() []trailWrite {
	return []trailWrite{
		{name: "app.Save", apply: func(app *tests.TestApp, r *core.Record) error { return app.Save(r) }},
		{name: "app.SaveWithContext", apply: func(app *tests.TestApp, r *core.Record) error {
			return app.SaveWithContext(context.Background(), r)
		}},
		{name: "app.SaveNoValidate, which skips every validator", apply: func(app *tests.TestApp, r *core.Record) error {
			return app.SaveNoValidate(r)
		}},
		{name: "app.SaveNoValidateWithContext", apply: func(app *tests.TestApp, r *core.Record) error {
			return app.SaveNoValidateWithContext(context.Background(), r)
		}},
		{name: "a save inside a transaction", apply: func(app *tests.TestApp, r *core.Record) error {
			return app.RunInTransaction(func(txApp core.App) error { return txApp.Save(r) })
		}},
	}
}

func trailDeletes() []trailWrite {
	return []trailWrite{
		{name: "app.Delete", apply: func(app *tests.TestApp, r *core.Record) error { return app.Delete(r) }},
		{name: "app.DeleteWithContext", apply: func(app *tests.TestApp, r *core.Record) error {
			return app.DeleteWithContext(context.Background(), r)
		}},
		{name: "a delete inside a transaction", apply: func(app *tests.TestApp, r *core.Record) error {
			return app.RunInTransaction(func(txApp core.App) error { return txApp.Delete(r) })
		}},
	}
}

// trailTampering is one column of the row and one way to change it.
type trailTampering struct {
	field  string
	change func(event *audit.Event, otherActorID string)
}

// trailTamperings covers EVERY field of audit.Event, and
// TestEveryColumnOfTheTrailHasATamperingCase is what keeps that true: a guard
// that refused six columns and let the seventh through would pass a table that
// only tried six.
func trailTamperings() []trailTampering {
	return []trailTampering{
		{field: "OccurredAt", change: func(e *audit.Event, _ string) { e.OccurredAt = e.OccurredAt.Add(-time.Hour) }},
		{field: "ActorID", change: func(e *audit.Event, other string) { e.ActorID = other }},
		{field: "ActorKind", change: func(e *audit.Event, _ string) { e.ActorKind = audit.ActorKindSystem }},
		{field: "Action", change: func(e *audit.Event, _ string) { e.Action = audit.ActionCreate }},
		{field: "TargetKind", change: func(e *audit.Event, _ string) { e.TargetKind = audit.TargetKindAllergy }},
		{field: "TargetID", change: func(e *audit.Event, _ string) { e.TargetID = "a-record-nobody-reached-for" }},
		{field: "RequestID", change: func(e *audit.Event, _ string) { e.RequestID = "a-request-that-never-happened" }},
		{field: "PatientID", change: func(e *audit.Event, _ string) { e.PatientID = "mkptdifferent01" }},
	}
}

func TestEveryColumnOfTheTrailHasATamperingCase(t *testing.T) {
	t.Parallel()

	covered := make(map[string]struct{}, len(trailTamperings()))
	for _, tampering := range trailTamperings() {
		covered[tampering.field] = struct{}{}
	}

	shape := reflect.TypeOf(audit.Event{})
	require.Positive(t, shape.NumField(), "audit.Event has no fields, so every table below ranges over nothing")

	for i := range shape.NumField() {
		name := shape.Field(i).Name
		assert.Containsf(t, covered, name,
			"audit.Event grew %s and no case below tries to edit it: a guard that let that column "+
				"through would pass every test in this file", name)
	}

	assert.Len(t, covered, shape.NumField(), "a tampering case names a column audit.Event does not have")
}

func TestAnAuditRowCannotBeEditedThroughAnyGoPath(t *testing.T) {
	t.Parallel()

	saves := trailSaves()
	tamperings := trailTamperings()

	require.GreaterOrEqual(t, len(saves), 5, "the save table has been narrowed and no longer covers every path")
	require.GreaterOrEqual(t, len(tamperings), 7, "the tampering table has been narrowed")

	for _, save := range saves {
		for _, tampering := range tamperings {
			t.Run(save.name+" changing "+tampering.field, func(t *testing.T) {
				t.Parallel()

				app := trailApp(t)
				actor := trailAccount(t, app, trailActorEmail)
				other := trailAccount(t, app, trailOtherEmail)

				written := trailEvent(actor.Id)
				record := trailRow(t, app, written)

				edited := written
				tampering.change(&edited, other.Id)
				require.NotEqual(t, written, edited, "the tampering case changes nothing, so nothing is being refused")

				stored := trailFresh(t, app, record.Id)
				require.NoError(t, store.AuditEventToRecord(stored, edited))

				err := save.apply(app, stored)

				require.Error(t, err, "the edit went through")
				assert.ErrorIs(t, err, pb.ErrAuditImmutable)
				assert.Equal(t, written, trailReload(t, app, record.Id), "the row on disk changed")
			})
		}
	}
}

func TestAnAuditRowCannotBeRemovedThroughAnyGoPath(t *testing.T) {
	t.Parallel()

	deletes := trailDeletes()
	require.GreaterOrEqual(t, len(deletes), 3, "the delete table has been narrowed and no longer covers every path")

	for _, remove := range deletes {
		t.Run(remove.name, func(t *testing.T) {
			t.Parallel()

			app := trailApp(t)
			actor := trailAccount(t, app, trailActorEmail)

			written := trailEvent(actor.Id)
			record := trailRow(t, app, written)

			err := remove.apply(app, trailFresh(t, app, record.Id))

			require.Error(t, err, "the row was removed")
			assert.ErrorIs(t, err, pb.ErrAuditImmutable)
			assert.Equal(t, written, trailReload(t, app, record.Id), "the row is gone or changed")
		})
	}
}

// A row PAST the retention horizon is refused too, and that is the design
// rather than an oversight. internal/store/audit purges with one bulk statement
// that never goes near the record layer, precisely so that the record-level
// guard can refuse EVERY removal with no exception in it — and an exception is
// a door. The horizon is enforced in the purge's own WHERE clause, which
// TestTheRetentionPurgeStillClearsThePastAndKeepsThePresent exercises.
func TestEvenARowLongPastAnyRetentionHorizonIsRefusedRecordByRecord(t *testing.T) {
	t.Parallel()

	app := trailApp(t)
	actor := trailAccount(t, app, trailActorEmail)

	ancient := trailEvent(actor.Id)
	ancient.OccurredAt = time.Now().UTC().AddDate(-40, 0, 0).Truncate(time.Millisecond)

	record := trailRow(t, app, ancient)

	err := app.Delete(trailFresh(t, app, record.Id))

	require.Error(t, err)
	assert.ErrorIs(t, err, pb.ErrAuditImmutable)
	assert.Equal(t, ancient, trailReload(t, app, record.Id))
}

// -----------------------------------------------------------------------------
// PATH 2 of 3 — the record hooks themselves.
// -----------------------------------------------------------------------------

// The guard is ON the hooks and not inside a repository method, which is the
// difference between one checkpoint and a convention. A second caller reaching
// the same collection through its own code cannot route around a hook.
func TestTheGuardSitsOnTheRecordHooksThemselves(t *testing.T) {
	t.Parallel()

	app := trailApp(t)
	actor := trailAccount(t, app, trailActorEmail)
	record := trailRow(t, app, trailEvent(actor.Id))

	for _, testCase := range []struct {
		name    string
		trigger func(record *core.Record) error
	}{
		{name: "OnRecordUpdate", trigger: func(r *core.Record) error {
			return app.OnRecordUpdate().Trigger(trailEventFor(app, r), func(*core.RecordEvent) error { return nil })
		}},
		{name: "OnRecordDelete", trigger: func(r *core.Record) error {
			return app.OnRecordDelete().Trigger(trailEventFor(app, r), func(*core.RecordEvent) error { return nil })
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.trigger(trailFresh(t, app, record.Id))

			require.Error(t, err, "the hook chain ran to the end, so nothing on it refuses")
			assert.ErrorIs(t, err, pb.ErrAuditImmutable)
		})
	}
}

func trailEventFor(app core.App, record *core.Record) *core.RecordEvent {
	event := new(core.RecordEvent)
	event.App = app
	event.Context = context.Background()
	event.Record = record
	event.Type = core.ModelEventTypeUpdate

	return event
}

// The guard is tagged to one collection. A guard that froze the whole instance
// would break every write in the application and would pass every assertion
// above, because every assertion above only ever asks whether something was
// refused.
func TestTheGuardTouchesNoCollectionButTheTrail(t *testing.T) {
	t.Parallel()

	app := trailApp(t)

	// A neighbour to the trail, built the same way: a guard that keyed on
	// "looks like an audit row" rather than on the collection would refuse this
	// one too.
	neighbour := core.NewBaseCollection(trailNeighbourCollection)
	neighbour.Fields.Add(&core.TextField{Name: trailFieldTargetID})
	require.NoError(t, app.Save(neighbour))

	collections, err := app.FindAllCollections()
	require.NoError(t, err)

	var exercised int

	for _, collection := range collections {
		if collection.Name == store.AuditCollection || collection.IsView() || collection.System {
			continue
		}

		record := core.NewRecord(collection)
		if collection.IsAuth() {
			record.SetEmail(collection.Name + "@immutability.test")
			record.SetPassword(trailPassword)
		} else {
			record.Set(trailFieldTargetID, "a value the guard has no business refusing")
		}

		require.NoErrorf(t, app.Save(record), "create a %s record", collection.Name)
		require.NoErrorf(t, app.Save(record), "the guard refuses an UPDATE on %s", collection.Name)
		require.NoErrorf(t, app.Delete(record), "the guard refuses a DELETE on %s", collection.Name)

		exercised++
	}

	require.GreaterOrEqualf(t, exercised, 2,
		"only %d collections outside the trail were tried, so this proves nothing about the guard's reach", exercised)
}

// -----------------------------------------------------------------------------
// The one update the guard permits, and the reason it is not a door.
// -----------------------------------------------------------------------------

// FR-014 and research D-22. `audit_events.actor` deliberately does not cascade,
// so PocketBase unsets it and re-saves the row when the account is deleted —
// through app.SaveNoValidate, which is an UPDATE on the trail. A guard that
// refused every update would make deleting an account fail outright, in
// production, on the first person who asked to be forgotten.
func TestDeletingAnAccountEmptiesItsActorAndKeepsEveryOtherColumn(t *testing.T) {
	t.Parallel()

	app := trailApp(t)
	actor := trailAccount(t, app, trailActorEmail)

	written := trailEvent(actor.Id)
	record := trailRow(t, app, written)

	require.NoError(t, app.Delete(actor),
		"the guard refuses PocketBase's own cascade, so no account can ever be deleted while it has a trail")

	survived := trailReload(t, app, record.Id)

	expected := written
	expected.ActorID = ""

	assert.Equal(t, expected, survived,
		"the cascade changed more than the actor, or the row did not survive its actor")
	assert.NotEmpty(t, survived.ActorKind,
		"actor_kind is what still says a person did it once the actor is gone (research D-22)")
}

// The exception above is bounded by a fact about the world rather than by a
// flag: the account it pointed at is no longer there. Without that clause the
// permitted branch is a door anybody can walk through — set the actor to empty
// and the row no longer says who did it, which is the single most valuable
// edit an attacker could make to an audit trail.
func TestClearingTheActorByHandIsRefusedWhileTheAccountStillExists(t *testing.T) {
	t.Parallel()

	app := trailApp(t)
	actor := trailAccount(t, app, trailActorEmail)

	written := trailEvent(actor.Id)
	record := trailRow(t, app, written)

	cleared := written
	cleared.ActorID = ""

	stored := trailFresh(t, app, record.Id)
	require.NoError(t, store.AuditEventToRecord(stored, cleared))

	err := app.SaveNoValidate(stored)

	require.Error(t, err, "the actor was erased from the row by hand")
	assert.ErrorIs(t, err, pb.ErrAuditImmutable)
	assert.Equal(t, written, trailReload(t, app, record.Id))
}

// And once the actor is already empty there is nothing left for the exception
// to permit, so a save that changes nothing is refused as well: the permitted
// shape is "a non-empty actor becoming empty", never "an actor that is empty".
func TestASaveThatChangesNothingIsStillRefused(t *testing.T) {
	t.Parallel()

	app := trailApp(t)

	systemRow := trailEvent("")
	systemRow.ActorKind = audit.ActorKindSystem

	record := trailRow(t, app, systemRow)
	stored := trailFresh(t, app, record.Id)

	err := app.SaveNoValidate(stored)

	require.Error(t, err)
	assert.ErrorIs(t, err, pb.ErrAuditImmutable)
}

// A second account's deletion must not be an excuse to rewrite a row that
// belonged to somebody else. The permitted branch keys on the actor the ROW
// carried, so a row whose actor is still a live account stays refused however
// many other accounts have gone.
func TestAnotherAccountsDeletionUnlocksNothing(t *testing.T) {
	t.Parallel()

	app := trailApp(t)
	actor := trailAccount(t, app, trailActorEmail)
	bystander := trailAccount(t, app, trailBystanderMail)

	written := trailEvent(actor.Id)
	record := trailRow(t, app, written)

	require.NoError(t, app.Delete(bystander))

	cleared := written
	cleared.ActorID = ""

	stored := trailFresh(t, app, record.Id)
	require.NoError(t, store.AuditEventToRecord(stored, cleared))

	err := app.SaveNoValidate(stored)

	require.Error(t, err)
	assert.ErrorIs(t, err, pb.ErrAuditImmutable)
	assert.Equal(t, written, trailReload(t, app, record.Id))
}

// trailForgetAccountBehindTheCascade removes an account with one statement
// straight at the database, so the row it is named on keeps a dangling actor
// that PocketBase's cascade never got to clear.
//
// It is the only way to reach the two clauses below at all: after a real
// cascade every row that named the account already has an empty actor, so
// "the account is gone AND the row still names it" is a state the cascade
// closes behind itself. Reaching it by hand is what proves the clauses are
// load-bearing rather than decoration.
func trailForgetAccountBehindTheCascade(t testing.TB, app core.App, accountID string) {
	t.Helper()

	_, err := app.NonconcurrentDB().
		Delete(store.AccountCollection, dbx.HashExp{"id": accountID}).
		Execute()
	require.NoError(t, err)

	_, err = app.FindRecordById(store.AccountCollection, accountID)
	require.Error(t, err, "the account is still there, so nothing below is testing what it says it is")
}

// Clearing the actor is permitted only once the account is gone — and even
// then, ONLY the actor. A write that empties the actor and changes something
// else in the same save is the shape that turns the cascade's exception into a
// general-purpose edit.
func TestOnceTheAccountIsGoneOnlyTheActorMayBeCleared(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name      string
		alsoEdit  func(event *audit.Event)
		permitted bool
	}{
		{name: "the actor and nothing else", alsoEdit: func(*audit.Event) {}, permitted: true},
		{name: "the actor and the action", alsoEdit: func(e *audit.Event) { e.Action = audit.ActionCreate }},
		{name: "the actor and the target", alsoEdit: func(e *audit.Event) { e.TargetID = "somewhere-else" }},
		{name: "the actor and the time", alsoEdit: func(e *audit.Event) { e.OccurredAt = e.OccurredAt.Add(time.Hour) }},
		{name: "the actor and the request", alsoEdit: func(e *audit.Event) { e.RequestID = "a-different-request" }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			app := trailApp(t)
			actor := trailAccount(t, app, trailActorEmail)

			written := trailEvent(actor.Id)
			record := trailRow(t, app, written)

			trailForgetAccountBehindTheCascade(t, app, actor.Id)

			edited := written
			edited.ActorID = ""
			testCase.alsoEdit(&edited)

			stored := trailFresh(t, app, record.Id)
			require.NoError(t, store.AuditEventToRecord(stored, edited))

			err := app.SaveNoValidate(stored)

			if testCase.permitted {
				require.NoError(t, err,
					"the shape PocketBase's own cascade makes is refused, so no account can be deleted")
				assert.Equal(t, edited, trailReload(t, app, record.Id))

				return
			}

			require.Error(t, err, "an edit rode in on the back of the cascade's exception")
			assert.ErrorIs(t, err, pb.ErrAuditImmutable)
			assert.Equal(t, written, trailReload(t, app, record.Id))
		})
	}
}

// The clause that decides the exception is a lookup, and a lookup that cannot
// answer is not an answer. A guard that read a database failure as "the account
// is gone" would hand the one permitted edit to anybody who could break a
// query — which is the fail-open this repository has already found once.
func TestALookupThatCannotAnswerRefusesTheEdit(t *testing.T) {
	t.Parallel()

	app := trailApp(t)
	actor := trailAccount(t, app, trailActorEmail)

	written := trailEvent(actor.Id)
	record := trailRow(t, app, written)

	// The account's table is taken out from under the lookup while the
	// collection stays in the cache, so the query is issued and fails rather
	// than being skipped.
	_, err := app.NonconcurrentDB().
		NewQuery("ALTER TABLE {{" + store.AccountCollection + "}} RENAME TO {{" + trailHiddenAccounts + "}}").
		Execute()
	require.NoError(t, err)

	cleared := written
	cleared.ActorID = ""

	stored := trailFresh(t, app, record.Id)
	require.NoError(t, store.AuditEventToRecord(stored, cleared))

	err = app.SaveNoValidate(stored)

	require.Error(t, err, "the edit went through on a lookup that never answered")
	assert.ErrorIs(t, err, pb.ErrAuditImmutable)
	assert.NotErrorIs(t, err, sql.ErrNoRows,
		"a failed lookup was read as an empty one, which is the fail-open this case exists for")
}

// -----------------------------------------------------------------------------
// PATH 3 of 3 — PocketBase's own API.
// -----------------------------------------------------------------------------

// The superuser row is the one with teeth. An ordinary caller is refused twice
// over before the guard is reached — the lockdown answers 404 and the nil
// collection rules would refuse anyway — so a test that only drove an ordinary
// caller would pass with no guard bound at all. A superuser bypasses both by
// design, and the admin UI drives exactly these routes.
func TestNoApiCallerCanEditOrRemoveAnAuditRow(t *testing.T) {
	t.Parallel()

	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		for _, caller := range []string{"anonymous", "an ordinary account", "a superuser"} {
			t.Run(method+" as "+caller, func(t *testing.T) {
				t.Parallel()

				h := newHarness(t, func(app *tests.TestApp) {
					trailSchema(t, app)
					require.NoError(t, pb.BindAuditImmutability(app))
					bindMediKubeServe(app)
				})

				actor := trailAccount(t, h.app, trailActorEmail)
				written := trailEvent(actor.Id)
				record := trailRow(t, h.app, written)

				var token string
				switch caller {
				case "an ordinary account":
					token = h.userToken(t)
				case "a superuser":
					token = h.superuserToken(t)
				}

				res := h.do(t, method,
					"/api/collections/"+store.AuditCollection+"/records/"+record.Id, token,
					`{"`+trailFieldTargetID+`":"tampered"}`)

				assert.GreaterOrEqual(t, res.Status, http.StatusBadRequest,
					"the API accepted an %s on an audit row", method)
				assert.Equal(t, written, trailReload(t, h.app, record.Id), "the row on disk changed")

				if caller == "a superuser" {
					// 400 and not 404: a superuser reaches past the lockdown and
					// past the collection rules, so this status is the proof
					// that the request got all the way to the guard rather than
					// being turned away by something in front of it.
					assert.Equal(t, http.StatusBadRequest, res.Status,
						"a superuser was refused by something other than the immutability guard, "+
							"so this row proves nothing about it")
				}
			})
		}
	}
}

// -----------------------------------------------------------------------------
// The retention purge, which is the only thing that may empty the trail.
// -----------------------------------------------------------------------------

// internal/store/audit purges with one bulk statement that takes an age and can
// never be pointed at an id, deliberately going around the record layer so the
// guard above needs no exception. This asserts the two halves fit: the purge
// still clears the past with the guard bound, and the same row is refused when
// it is deleted record by record.
func TestTheRetentionPurgeStillClearsThePastAndKeepsThePresent(t *testing.T) {
	t.Parallel()

	app := trailApp(t)
	actor := trailAccount(t, app, trailActorEmail)

	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -730)

	past := make([]string, 0, 2)
	for _, age := range []time.Duration{time.Hour, 365 * 24 * time.Hour} {
		event := trailEvent(actor.Id)
		event.OccurredAt = cutoff.Add(-age).Truncate(time.Millisecond)
		past = append(past, trailRow(t, app, event).Id)
	}

	present := make([]string, 0, 2)
	for _, age := range []time.Duration{0, -time.Hour} {
		event := trailEvent(actor.Id)
		event.OccurredAt = cutoff.Add(-age).Truncate(time.Millisecond)
		present = append(present, trailRow(t, app, event).Id)
	}

	require.Len(t, past, 2)
	require.Len(t, present, 2)

	// The same row the purge is about to take is refused when a caller asks for
	// it one record at a time. The guard has no horizon in it and needs none.
	refused := app.Delete(trailFresh(t, app, past[0]))
	require.Error(t, refused)
	assert.ErrorIs(t, refused, pb.ErrAuditImmutable)

	repo, err := storeaudit.New(app)
	require.NoError(t, err)

	removed, err := repo.DeleteBefore(t.Context(), cutoff)
	require.NoError(t, err, "the guard is in the retention purge's way")
	assert.Equal(t, len(past), removed)

	for _, id := range past {
		_, err := app.FindRecordById(store.AuditCollection, id)
		assert.Errorf(t, err, "a row past the horizon survived the purge")
	}

	for _, id := range present {
		assert.NotPanicsf(t, func() { trailReload(t, app, id) }, "a row inside the horizon was purged")
	}
}

// -----------------------------------------------------------------------------
// The guard has to be bound, and an unbound guard has to be a boot failure.
// -----------------------------------------------------------------------------

// A guard nobody binds is the defect this repository has already shipped once.
// AssertAuditImmutabilityBound is the composition root's proof, and it proves it
// by EXERCISING the hooks rather than by counting them: a binding that had
// drifted onto the wrong collection tag would still be one handler.
func TestTheBootAssertionAnswersForAGuardThatIsActuallyBound(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		bind    func(app *tests.TestApp)
		refuses bool
	}{
		{
			name:    "bound",
			bind:    func(app *tests.TestApp) { require.NoError(t, pb.BindAuditImmutability(app)) },
			refuses: false,
		},
		{
			name:    "never bound",
			bind:    func(*tests.TestApp) {},
			refuses: true,
		},
		{
			name: "bound and then unbound by a later hand",
			bind: func(app *tests.TestApp) {
				require.NoError(t, pb.BindAuditImmutability(app))
				app.OnRecordUpdate().Unbind(pb.AuditImmutableUpdateHookID)
			},
			refuses: true,
		},
		{
			name: "the delete half unbound",
			bind: func(app *tests.TestApp) {
				require.NoError(t, pb.BindAuditImmutability(app))
				app.OnRecordDelete().Unbind(pb.AuditImmutableDeleteHookID)
			},
			refuses: true,
		},
		{
			name: "bound to the wrong collection",
			bind: func(app *tests.TestApp) {
				app.OnRecordUpdate(store.AccountCollection).Bind(&hook.Handler[*core.RecordEvent]{
					Id:   pb.AuditImmutableUpdateHookID,
					Func: func(*core.RecordEvent) error { return pb.ErrAuditImmutable },
				})
				app.OnRecordDelete(store.AccountCollection).Bind(&hook.Handler[*core.RecordEvent]{
					Id:   pb.AuditImmutableDeleteHookID,
					Func: func(*core.RecordEvent) error { return pb.ErrAuditImmutable },
				})
			},
			refuses: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			app, err := tests.NewTestApp(t.TempDir())
			require.NoError(t, err)
			t.Cleanup(app.Cleanup)

			trailSchema(t, app)
			testCase.bind(app)

			err = pb.AssertAuditImmutabilityBound(app)

			if testCase.refuses {
				require.Error(t, err, "the boot assertion passed with the guard not in force")
				assert.ErrorIs(t, err, pb.ErrAuditImmutabilityUnbound)

				return
			}

			assert.NoError(t, err)
		})
	}
}

// The boot assertion writes nothing. A check that appended a probe row to prove
// the trail is immutable would put a row in it that describes nothing.
func TestTheBootAssertionLeavesTheTrailEmpty(t *testing.T) {
	t.Parallel()

	app := trailApp(t)

	before, err := app.CountRecords(store.AuditCollection)
	require.NoError(t, err)
	require.NoError(t, pb.AssertAuditImmutabilityBound(app))

	after, err := app.CountRecords(store.AuditCollection)
	require.NoError(t, err)

	assert.Equal(t, before, after, "the boot assertion wrote to the trail")
}

func TestTheGuardRefusesToBeBoundToNothing(t *testing.T) {
	t.Parallel()

	assert.Error(t, pb.BindAuditImmutability(nil))
	assert.ErrorIs(t, pb.AssertAuditImmutabilityBound(nil), pb.ErrAuditImmutabilityUnbound)
}

// Binding twice replaces rather than appends, so an instance wired twice
// refuses once. Two identical refusals would be indistinguishable from one in
// every assertion above, which is exactly why it is asserted here instead.
func TestBindingTheGuardTwiceLeavesOneOfEach(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	trailSchema(t, app)

	require.NoError(t, pb.BindAuditImmutability(app))
	updates, deletes := app.OnRecordUpdate().Length(), app.OnRecordDelete().Length()

	require.NoError(t, pb.BindAuditImmutability(app))

	assert.Equal(t, updates, app.OnRecordUpdate().Length())
	assert.Equal(t, deletes, app.OnRecordDelete().Length())
	assert.NoError(t, pb.AssertAuditImmutabilityBound(app))
}

// The guard refuses edits and removals. It does not refuse the append the trail
// exists for, and a guard bound to OnRecordCreate by mistake would silence the
// whole audit trail while passing every refusal assertion in this file.
func TestTheGuardRefusesEditsAndNeverTheAppend(t *testing.T) {
	t.Parallel()

	app := trailApp(t)
	actor := trailAccount(t, app, trailActorEmail)

	for i := range 3 {
		event := trailEvent(actor.Id)
		event.OccurredAt = event.OccurredAt.Add(time.Duration(i) * time.Minute)
		trailRow(t, app, event)
	}

	count, err := app.CountRecords(store.AuditCollection)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}
