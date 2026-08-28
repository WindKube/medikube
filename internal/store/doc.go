// Package store holds what every repository adapter shares: the transaction
// helper, opaque authenticated keyset cursors, record-to-domain mapping, and
// query translation.
//
// Queries are typed here and nowhere else. A resource declares the columns it
// publishes and this package turns a Query into bound SQL — PocketBase's filter
// DSL, which interpolates values into a filter *string* before parsing it, is
// never written by MediKube at all. filter_test.go's source walk is the gate.
//
// It sits on the PocketBase side of the import boundary.
package store
