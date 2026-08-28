// Package migrations defines MediKube's collections as Go code under version
// control, together with the assertions every migration must satisfy.
//
// Schema is never changed by clicking in the admin UI, because a change made
// there exists on exactly one instance and in no review.
//
// It sits on the PocketBase side of the import boundary.
package migrations
