// Package store holds what every repository adapter shares: the transaction
// helper, opaque signed keyset cursors, record-to-domain mapping, and query
// translation.
//
// PocketBase's filter DSL is written here and never leaves.
//
// It sits on the PocketBase side of the import boundary.
package store
