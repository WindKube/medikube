// Package pb owns the embedded PocketBase instance: how it is constructed,
// which settings it boots with, the middleware it serves behind, the hooks it
// binds, and the boot assertions it refuses to start without.
//
// It sits on the PocketBase side of the import boundary — this is the package
// that boundary exists to contain.
package pb
