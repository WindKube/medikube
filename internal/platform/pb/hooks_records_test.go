package pb_test

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/platform/pb"
	"medikube/internal/realtime"
	"medikube/internal/store"
)

// T160. The realtime publisher is bound to the After…Success hooks, so a
// rolled-back transaction publishes nothing.
//
// A pre-commit binding is a live view showing a change that did not happen, and
// it passes every test that only ever commits — which is every other test in
// this repository. PocketBase registers the success trigger as a deferred
// function on the transaction and runs it after the transaction is closed with
// the commit's own error (core/db.go, core/db_tx.go), so what follows asserts a
// property of the binding and not of a check inside the hook.
//
// THE COLLECTIONS BELOW ARE BUILT BY HAND AND MediKube's MIGRATIONS ARE NOT
// IMPORTED. That is not a shortcut; it is the only shape this file can have.
// core.AppMigrations is a package-level registry, so a single import of
// medikube/internal/store/migrations — directly, or through
// medikube/internal/testsupport — applies MediKube's schema to every
// tests.NewTestApp in this whole test binary, and the lockdown suite next door
// creates bare `users` records that MediKube's profile columns then refuse.
// stockSchema below is the tripwire for anybody who adds that import later.

var errDeliberateRollback = errors.New("deliberate rollback")

// streamOwnerID is not a fixture id. There is no fixture here: the collection
// is created three lines above the record, so the owner is a value this file
// invents for a schema this file invents.
const streamOwnerID = "mkstreamowner001"

// unpublishedCollection stands in for every collection that is not a registered
// kind — audit_events in the real instance, which is itself a collection and
// which a publisher fanning out every save would broadcast forever.
const unpublishedCollection = "not_a_registered_kind"

// recorder is a pb.Publisher that keeps what it was handed.
type recorder struct {
	mu     sync.Mutex
	events []realtime.Event
}

func (r *recorder) Publish(event realtime.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, event)
}

func (r *recorder) published() []realtime.Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]realtime.Event(nil), r.events...)
}

// stockSchema fails the moment this test binary starts applying MediKube's
// migrations, which is what would break the lockdown suite in ways that read as
// unrelated validation failures.
func stockSchema(t *testing.T, app core.App) {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)

	require.Nilf(t, users.Fields.GetByName("role"),
		"this test binary is applying MediKube's migrations: something under internal/platform/pb now imports "+
			"medikube/internal/store/migrations, directly or through internal/testsupport, and core.AppMigrations is "+
			"a package-level registry. The lockdown suite creates bare users records and MediKube's profile columns "+
			"refuse them.")
}

// streaming builds a throwaway instance with one publishable collection and one
// unpublishable one, and binds the publisher to the first.
func streaming(t *testing.T) (*tests.TestApp, *recorder) {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	stockSchema(t, app)

	for _, name := range []string{kind.Medication.Collection(), unpublishedCollection} {
		collection := core.NewBaseCollection(name)
		collection.Fields.Add(&core.TextField{Name: store.MedicationOwner})
		collection.Fields.Add(&core.TextField{Name: "name"})
		require.NoError(t, app.Save(collection))
	}

	published := &recorder{}

	require.NoError(t, pb.BindRecordStream(app, pb.RecordStream{
		Hub:   published,
		Kinds: []kind.Kind{kind.Medication},
	}))

	return app, published
}

func newRow(t *testing.T, app core.App, collection, name string) *core.Record {
	t.Helper()

	found, err := app.FindCollectionByNameOrId(collection)
	require.NoError(t, err)

	record := core.NewRecord(found)
	record.Set(store.MedicationOwner, streamOwnerID)
	record.Set("name", name)

	return record
}

func TestARolledBackTransactionPublishesNothingAndACommittedOnePublishesOnce(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name      string
		rollback  bool
		wantCount int
	}{
		{name: "rolled back", rollback: true, wantCount: 0},
		{name: "committed", rollback: false, wantCount: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			app, published := streaming(t)

			// The control: the error twin of the hook the publisher is bound
			// to. Without it, "nothing was published" reads exactly the same as
			// "the deferred functions never ran at all", and a publisher bound
			// to nothing whatsoever would pass the rolled-back case.
			var (
				mu        sync.Mutex
				errorful  int
				succeeded int
			)

			app.OnRecordAfterCreateError(kind.Medication.Collection()).
				BindFunc(func(e *core.RecordErrorEvent) error {
					mu.Lock()
					errorful++
					mu.Unlock()

					return e.Next()
				})

			app.OnRecordAfterCreateSuccess(kind.Medication.Collection()).
				BindFunc(func(e *core.RecordEvent) error {
					mu.Lock()
					succeeded++
					mu.Unlock()

					return e.Next()
				})

			var written string

			err := store.RunInTransaction(app, func(txApp core.App) error {
				record := newRow(t, txApp, kind.Medication.Collection(), "Amoxicillin")
				require.NoError(t, txApp.Save(record))

				written = record.Id

				if testCase.rollback {
					return errDeliberateRollback
				}

				return nil
			})

			if testCase.rollback {
				require.ErrorIs(t, err, errDeliberateRollback)
			} else {
				require.NoError(t, err)
			}

			events := published.published()
			require.Lenf(t, events, testCase.wantCount,
				"a %s transaction published %d events: %+v", testCase.name, len(events), events)

			mu.Lock()
			defer mu.Unlock()

			if testCase.rollback {
				assert.Equal(t, 1, errorful,
					"the deferred after-funcs never ran, so 'nothing was published' says nothing about where the publisher is bound")
				assert.Equal(t, 0, succeeded)

				_, findErr := app.FindRecordById(kind.Medication.Collection(), written)
				assert.Error(t, findErr, "the row survived the rollback, so this case is not about a rollback at all")

				return
			}

			assert.Equal(t, 1, succeeded)
			assert.Equal(t, 0, errorful)
			assert.Equal(t, kind.Medication, events[0].Kind)
			assert.Equal(t, written, events[0].RecordID)
			assert.Equal(t, streamOwnerID, events[0].OwnerID)
		})
	}
}

// All three writes publish, and each carries the record's own owner. The owner
// is what the subscriber's removal path is scoped by, so a publisher that left
// it empty would suppress every row removal in silence.
func TestEveryCommittedWritePublishesItsKindIDAndOwner(t *testing.T) {
	t.Parallel()

	app, published := streaming(t)

	record := newRow(t, app, kind.Medication.Collection(), "Amoxicillin")
	require.NoError(t, app.Save(record))

	record.Set("name", "Ibuprofen")
	require.NoError(t, app.Save(record))

	require.NoError(t, app.Delete(record))

	events := published.published()
	require.Lenf(t, events, 3, "a create, a change and a deletion did not produce three events: %+v", events)

	for index, event := range events {
		assert.Equalf(t, kind.Medication, event.Kind, "event %d", index)
		assert.Equalf(t, record.Id, event.RecordID, "event %d", index)
		assert.Equalf(t, streamOwnerID, event.OwnerID,
			"event %d carries no owner, so every removal for it would be suppressed", index)
	}
}

// contracts/streams.md's rule that makes per-subscriber authorization possible
// at all: the hub carries identifiers and never record bodies. It is asserted
// on the type, because a field is how a body would arrive — and it would arrive
// working, with nothing to notice it by until somebody read a medication name
// off another account's socket.
func TestTheEventCarriesIdentifiersAndNothingElse(t *testing.T) {
	t.Parallel()

	fields := reflect.VisibleFields(reflect.TypeFor[realtime.Event]())
	names := make([]string, 0, len(fields))

	for _, field := range fields {
		names = append(names, field.Name)
	}

	assert.ElementsMatch(t, []string{"Kind", "RecordID", "OwnerID"}, names,
		"realtime.Event grew a field: the hub publishes ids, never bodies, and a body here would have to be "+
			"authorised at publish time by the one participant that does not know who is listening")
}

// A collection nobody registered is not published. audit_events is itself a
// collection, so a publisher that fanned out every save would put the audit
// trail's own writes on every open browser.
func TestAWriteToAnUnregisteredCollectionPublishesNothing(t *testing.T) {
	t.Parallel()

	app, published := streaming(t)

	require.NoError(t, app.Save(newRow(t, app, unpublishedCollection, "Amoxicillin")))

	assert.Empty(t, published.published(), "a write to a collection nobody registered reached the stream")
}

// The hook ids are what make a second Bind replace rather than append. Without
// them an instance wired twice publishes every change twice, and a subscriber
// cannot tell two publishes of one change from two changes.
func TestBindingThePublisherTwicePublishesOnce(t *testing.T) {
	t.Parallel()

	app, published := streaming(t)

	require.NoError(t, pb.BindRecordStream(app, pb.RecordStream{
		Hub:   published,
		Kinds: []kind.Kind{kind.Medication},
	}))

	require.NoError(t, app.Save(newRow(t, app, kind.Medication.Collection(), "Amoxicillin")))

	assert.Len(t, published.published(), 1, "the publisher was bound twice and every change is now published twice")
}

// A misassembled publisher is refused at wiring time rather than discovered as
// a live view that never updates.
func TestThePublisherRefusesAWiringThatCouldNotPublish(t *testing.T) {
	t.Parallel()

	app, _ := streaming(t)

	for name, config := range map[string]pb.RecordStream{
		"no hub":             {Kinds: []kind.Kind{kind.Medication}},
		"no kinds":           {Hub: &recorder{}},
		"an undeclared kind": {Hub: &recorder{}, Kinds: []kind.Kind{"not_a_kind"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Error(t, pb.BindRecordStream(app, config))
		})
	}

	assert.Error(t, pb.BindRecordStream(nil, pb.RecordStream{Hub: &recorder{}, Kinds: []kind.Kind{kind.Medication}}))
}

// The real hub, end to end at this layer. The recorder above proves what the
// hook hands over; this proves the thing it hands it to is the one a stream
// reads, and that Publish does not block the write that caused it.
func TestACommittedWriteReachesARealSubscriber(t *testing.T) {
	t.Parallel()

	app, _ := streaming(t)

	hub := realtime.New()
	t.Cleanup(hub.Shutdown)

	require.NoError(t, pb.BindRecordStream(app, pb.RecordStream{Hub: hub, Kinds: []kind.Kind{kind.Medication}}))

	events := hub.Subscribe(t.Context())

	record := newRow(t, app, kind.Medication.Collection(), "Amoxicillin")
	require.NoError(t, app.Save(record))

	select {
	case event := <-events:
		assert.Equal(t, record.Id, event.RecordID)
		assert.Equal(t, kind.Medication, event.Kind)
		assert.Equal(t, streamOwnerID, event.OwnerID)
	case <-time.After(5 * time.Second):
		t.Fatal("a committed write never reached the subscriber")
	}
}
