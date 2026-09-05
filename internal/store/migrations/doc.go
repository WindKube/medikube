// Package migrations defines MediKube's collections as Go code under version
// control, together with the assertions every migration must satisfy.
//
// Schema is never changed by clicking in the admin UI, because a change made
// there exists on exactly one instance and in no review.
//
// It sits on the PocketBase side of the import boundary.
//
// Phase 002 adds six migrations, and the order below is forced by the
// relation graph (research D-15): each RelationField needs its target
// collection to exist before it can be declared, and all six share one
// transaction (core/migrations_runner.go), so a mis-ordering fails the whole
// boot rather than half-migrating.
//
//  1. 1756200100_facilities.go     — no dependency.
//  2. 1756200200_practitioners.go  — practitioners.facility needs facilities.
//  3. 1756200300_patients.go       — patients.primary_practitioner needs
//     practitioners.
//  4. 1756200400_users_active_patient.go — users.active_patient needs
//     patients.
//  5. 1756200500_audit_events_patient.go — audit_events.patient needs
//     patients.
//  6. 1756200600_medications_repoint.go  — medications.patient needs
//     patients, and the backfill needs every self-record patients rows
//     step 3 depends on to already exist (research D-13).
package migrations
