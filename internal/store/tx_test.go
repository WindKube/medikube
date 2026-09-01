package store

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

var errDeliberate = errors.New("the operation this test makes fail")

func countMedications(t *testing.T, app core.App) int {
	t.Helper()

	total, err := app.CountRecords(kind.Medication.Collection())
	require.NoError(t, err)

	return int(total)
}

// T081, first half. A transaction that fails leaves nothing behind, including
// the rows written before the failure — which is the case a per-statement
// rollback would get wrong and a caller would never notice, because the
// operation it asked for did fail and the half-written row is somebody else's
// problem later.
func TestAFailingOperationInsideATransactionRollsBackCompletely(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "rollback@example.test")

	require.Zero(t, countMedications(t, app))

	err := RunInTransaction(app, func(txApp core.App) error {
		collection, findErr := txApp.FindCollectionByNameOrId(kind.Medication.Collection())
		if findErr != nil {
			return findErr
		}

		for _, name := range []string{"Amoxicillin", "Ibuprofen", "Metformin"} {
			record := core.NewRecord(collection)
			if mapErr := MedicationToRecord(record, clinical.Medication{
				OwnerID: owner.Id,
				Name:    name,
				Status:  clinical.TherapyStatusActive,
			}); mapErr != nil {
				return mapErr
			}

			if saveErr := txApp.Save(record); saveErr != nil {
				return saveErr
			}
		}

		// Three rows are in the transaction at this point, and they have to
		// leave with it.
		if got := countMedications(t, txApp); got != 3 {
			return errors.New("the writes did not reach the transaction at all")
		}

		return errDeliberate
	})

	require.ErrorIs(t, err, errDeliberate, "the caller's error has to survive; it is what the caller reports")
	assert.Zero(t, countMedications(t, app), "a row written before the failure outlived the transaction")
}

func TestASucceedingTransactionCommitsEverythingInIt(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "commit@example.test")

	require.NoError(t, RunInTransaction(app, func(txApp core.App) error {
		collection, findErr := txApp.FindCollectionByNameOrId(kind.Medication.Collection())
		require.NoError(t, findErr)

		for _, name := range []string{"Amoxicillin", "Ibuprofen"} {
			record := core.NewRecord(collection)
			require.NoError(t, MedicationToRecord(record, clinical.Medication{
				OwnerID: owner.Id,
				Name:    name,
				Status:  clinical.TherapyStatusActive,
			}))
			require.NoError(t, txApp.Save(record))
		}

		return nil
	}))

	assert.Equal(t, 2, countMedications(t, app))
}

// T081, second half, at the mechanism.
//
// PocketBase does not fire the after-success hooks inline when a write happens
// inside a transaction. core/db.go:343-363 defers them onto txInfo.OnComplete,
// and core/db_tx.go:37-43 runs those deferred calls once the transaction has
// finished, handing them the transaction's error. With an error they take the
// *Error* branch; OnModelAfterCreateSuccess is never triggered.
//
// That matters because realtime hangs off exactly those hooks:
// apis/realtime.go:419 binds the record-create broadcast to
// OnModelAfterCreateSuccess, and core/record_model.go:110 forwards the same
// hook to OnRecordAfterCreateSuccess. No success hook, no broadcast.
//
// The error counter is asserted too, so the zeroes are a decision rather than
// an absence: if the deferred calls had not run at all, this would read the
// same as a suppressed broadcast.
func TestAFailingTransactionFiresNoAfterSuccessHookAndAFailingOneFiresTheError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		fail         bool
		wantSuccess  int
		wantErrorful int
	}{
		{name: "rolled back", fail: true, wantSuccess: 0, wantErrorful: 1},
		{name: "committed", fail: false, wantSuccess: 1, wantErrorful: 0},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			app := newTestApp(t)
			owner := seedUser(t, app, "hooks@example.test")

			var (
				mu             sync.Mutex
				modelSuccess   int
				recordSuccess  int
				modelErrorful  int
				recordErrorful int
			)

			app.OnModelAfterCreateSuccess(kind.Medication.Collection()).
				BindFunc(func(e *core.ModelEvent) error {
					mu.Lock()
					modelSuccess++
					mu.Unlock()

					return e.Next()
				})
			app.OnRecordAfterCreateSuccess(kind.Medication.Collection()).
				BindFunc(func(e *core.RecordEvent) error {
					mu.Lock()
					recordSuccess++
					mu.Unlock()

					return e.Next()
				})
			app.OnModelAfterCreateError(kind.Medication.Collection()).
				BindFunc(func(e *core.ModelErrorEvent) error {
					mu.Lock()
					modelErrorful++
					mu.Unlock()

					return e.Next()
				})
			app.OnRecordAfterCreateError(kind.Medication.Collection()).
				BindFunc(func(e *core.RecordErrorEvent) error {
					mu.Lock()
					recordErrorful++
					mu.Unlock()

					return e.Next()
				})

			err := RunInTransaction(app, func(txApp core.App) error {
				collection, findErr := txApp.FindCollectionByNameOrId(kind.Medication.Collection())
				require.NoError(t, findErr)

				record := core.NewRecord(collection)
				require.NoError(t, MedicationToRecord(record, clinical.Medication{
					OwnerID: owner.Id,
					Name:    "Amoxicillin",
					Status:  clinical.TherapyStatusActive,
				}))
				require.NoError(t, txApp.Save(record))

				if testCase.fail {
					return errDeliberate
				}

				return nil
			})

			if testCase.fail {
				require.ErrorIs(t, err, errDeliberate)
			} else {
				require.NoError(t, err)
			}

			mu.Lock()
			defer mu.Unlock()

			assert.Equal(t, testCase.wantSuccess, modelSuccess,
				"OnModelAfterCreateSuccess is what apis/realtime.go:419 broadcasts from")
			assert.Equal(t, testCase.wantSuccess, recordSuccess)
			assert.Equal(t, testCase.wantErrorful, modelErrorful,
				"the deferred after-funcs did not run at all, so the zero above proves nothing")
			assert.Equal(t, testCase.wantErrorful, recordErrorful)
		})
	}
}

// T081, second half, on the wire.
//
// The hook assertion above is the mechanism; this is the observable. A real
// subscriber is registered with the broker and the record-create broadcast is
// bound the way apis.NewRouter binds it in production, so a rollback that
// somehow reached the broadcast would be seen here rather than reasoned about.
//
// The positive control is not optional: realtimeBroadcastRecord returns
// immediately when there are no subscribers (apis/realtime.go:600-603), so a
// subscriber that never receives anything is exactly what a broken test looks
// like.
func TestAFailingTransactionSendsNothingToARealtimeSubscriber(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "realtime@example.test")

	// This is what binds the nine realtime handlers, including the
	// record-create broadcast (apis/base.go:49 -> apis/realtime.go:40).
	_, err := apis.NewRouter(app)
	require.NoError(t, err)

	superusers, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	require.NoError(t, err)

	superuser := core.NewRecord(superusers)
	superuser.SetEmail("superuser@example.test")
	superuser.SetPassword("correct-horse-battery-staple")
	require.NoError(t, app.Save(superuser))

	// The collection's API rules are all nil under the lockdown, so only a
	// superuser subscription can see anything at all — which is what makes a
	// silent access refusal distinguishable from a suppressed broadcast.
	client := subscriptions.NewDefaultClient()
	client.Subscribe(kind.Medication.Collection() + "/*")
	client.Set(apis.RealtimeClientAuthKey, superuser)

	app.SubscriptionsBroker().Register(client)
	t.Cleanup(func() { app.SubscriptionsBroker().Unregister(client.Id()) })

	var (
		mu       sync.Mutex
		received []subscriptions.Message
	)

	drained := make(chan struct{})

	go func() {
		defer close(drained)

		for message := range client.Channel() {
			mu.Lock()
			received = append(received, message)
			mu.Unlock()
		}
	}()

	delivered := func() int {
		mu.Lock()
		defer mu.Unlock()

		return len(received)
	}

	create := func(txApp core.App, name string) error {
		collection, findErr := txApp.FindCollectionByNameOrId(kind.Medication.Collection())
		if findErr != nil {
			return findErr
		}

		record := core.NewRecord(collection)
		if mapErr := MedicationToRecord(record, clinical.Medication{
			OwnerID: owner.Id,
			Name:    name,
			Status:  clinical.TherapyStatusActive,
		}); mapErr != nil {
			return mapErr
		}

		return txApp.Save(record)
	}

	rollbackErr := RunInTransaction(app, func(txApp core.App) error {
		if createErr := create(txApp, "Amoxicillin"); createErr != nil {
			return createErr
		}

		return errDeliberate
	})
	require.ErrorIs(t, rollbackErr, errDeliberate)

	// Delivery is fire-and-forget (apis/realtime.go:760-763), so "nothing was
	// sent" needs a window rather than an immediate read.
	require.Never(t, func() bool { return delivered() > 0 }, 500*time.Millisecond, 25*time.Millisecond,
		"a rolled-back write reached a realtime subscriber")

	// The control. The same write, committed, does arrive — so the silence
	// above is the rollback and not a subscription that was never live.
	require.NoError(t, RunInTransaction(app, func(txApp core.App) error {
		return create(txApp, "Ibuprofen")
	}))

	require.Eventually(t, func() bool { return delivered() == 1 }, 5*time.Second, 10*time.Millisecond,
		"the committed write never reached the subscriber, so this test could not have detected the rolled-back one either")

	mu.Lock()
	assert.Contains(t, string(received[0].Data), `"action":"create"`)
	assert.NotContains(t, string(received[0].Data), "Amoxicillin", "the rolled-back row was broadcast late")
	mu.Unlock()

	app.SubscriptionsBroker().Unregister(client.Id())
	<-drained
}

// A nested call inside the callback joins the transaction it is already in
// rather than opening a second one (core/db_tx.go:26-29), so an inner failure
// takes the outer writes with it. This is what makes a repository method safe
// to call from inside a service-level transaction.
func TestANestedTransactionJoinsTheOuterOne(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "nested@example.test")

	err := RunInTransaction(app, func(outer core.App) error {
		collection, findErr := outer.FindCollectionByNameOrId(kind.Medication.Collection())
		require.NoError(t, findErr)

		record := core.NewRecord(collection)
		require.NoError(t, MedicationToRecord(record, clinical.Medication{
			OwnerID: owner.Id,
			Name:    "Amoxicillin",
			Status:  clinical.TherapyStatusActive,
		}))
		require.NoError(t, outer.Save(record))

		require.True(t, InTransaction(outer))

		return RunInTransaction(outer, func(inner core.App) error {
			require.True(t, InTransaction(inner))

			return errDeliberate
		})
	})

	require.ErrorIs(t, err, errDeliberate)
	assert.Zero(t, countMedications(t, app), "the outer write survived an inner failure")
	assert.False(t, InTransaction(app))
}

// The foot-gun, written down as a test because it is invisible at the call
// site: the callback's txApp is the only handle inside the transaction, and the
// app the caller closed over is outside it — it cannot see the transaction's
// writes, and a write of its own would not be part of them.
//
// This is why RunInTransaction's callback takes its app as an argument and why
// every repository method does the same rather than holding one.
//
// The demonstration is a read rather than a write on purpose: two writers on
// one SQLite database is a lock contention, and the point here is the isolation
// boundary, not what happens when you fight it.
func TestTheOuterAppCannotSeeInsideTheTransaction(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner := seedUser(t, app, "escape@example.test")

	err := RunInTransaction(app, func(txApp core.App) error {
		collection, findErr := txApp.FindCollectionByNameOrId(kind.Medication.Collection())
		require.NoError(t, findErr)

		record := core.NewRecord(collection)
		require.NoError(t, MedicationToRecord(record, clinical.Medication{
			OwnerID: owner.Id,
			Name:    "Amoxicillin",
			Status:  clinical.TherapyStatusActive,
		}))
		require.NoError(t, txApp.Save(record))

		assert.Equal(t, 1, countMedications(t, txApp), "the write did not reach the transaction")
		assert.Zero(t, countMedications(t, app),
			"the app the caller closed over can see uncommitted rows; it is not outside the transaction after all")

		return errDeliberate
	})

	require.ErrorIs(t, err, errDeliberate)
	assert.Zero(t, countMedications(t, app))
}

func TestRunInTransactionWrapsWhatTheCallbackReturns(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	err := RunInTransaction(app, func(core.App) error { return errDeliberate })

	require.ErrorIs(t, err, errDeliberate)
	assert.NotEqual(t, errDeliberate, err, "the error is not wrapped, so nothing says where it came from")
	assert.Contains(t, err.Error(), "transaction")
}
