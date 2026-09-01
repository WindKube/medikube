package store

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// RunInTransaction runs fn against a transactional handle on the app, rolls
// back if it returns an error, and commits otherwise.
//
// # The txApp is the only handle inside the transaction
//
// PocketBase hands the callback a shallow clone of the app whose database
// builder is the transaction (core/db_tx.go:52-69). A write through the app the
// caller closed over — the receiver, not the argument — goes to the connection
// directly and commits on its own, whatever the transaction then does. There is
// no error, no warning and no symptom until somebody reads the row that should
// not exist. tx_test.go pins that behaviour so a PocketBase change makes it a
// failing test rather than a surprise.
//
// A nested call joins the transaction it is already in rather than opening a
// second one (core/db_tx.go:26-29), so a repository method that manages its own
// transaction is safe to call from inside a service-level one.
//
// # A rolled-back write publishes nothing
//
// This is the property the audit trail and the realtime layer both rest on, and
// it is a consequence of where PocketBase fires its hooks rather than of
// anything MediKube does:
//
//   - A write inside a transaction does not trigger its after-success hook
//     inline. It registers the trigger on the transaction instead
//     (core/db.go:343-363 for create, :427-445 for update, :152-170 for
//     delete), and those registered calls run once, after the transaction has
//     finished, with the transaction's error in hand (core/db_tx.go:37-43).
//   - With an error in hand they take the OnModelAfter*Error branch.
//     OnModelAfterCreateSuccess is never triggered.
//   - Realtime is bound to precisely those success hooks — the record-create
//     broadcast at apis/realtime.go:419, update at :439, delete at :484 — and
//     core/record_model.go:110 forwards the same hook family to
//     OnRecordAfterCreateSuccess, which is where MediKube's own audit and
//     realtime work belongs.
//
// So a rolled-back medication is never broadcast, never audited and never
// counted, without anybody having to remember to undo it.
//
// One asymmetry worth knowing: a delete is *cached* per subscriber inside the
// transaction, at OnModelDelete (apis/realtime.go:460-481), and only broadcast
// on success. On a rollback the cache entry is cleaned by the matching error
// hook (:508-526). Nothing is emitted either way, but an error handler that
// swallowed its event would leak a per-client cache key — which is the reason
// MediKube's own hooks must return e.Next() on the error path too.
func RunInTransaction(app core.App, fn func(txApp core.App) error) error {
	if err := app.RunInTransaction(fn); err != nil {
		return fmt.Errorf("transaction: %w", err)
	}

	return nil
}

// InTransaction reports whether this handle is inside one. It is the guard for
// code that must not open a transaction of its own — or, more usefully, for an
// assertion that it is already in one.
func InTransaction(app core.App) bool {
	return app.TxInfo() != nil
}
