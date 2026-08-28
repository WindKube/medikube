// Package testsupport builds what MediKube's integration tests share: a new
// test application per call rather than one passed around, the seeded fixture
// identifiers, and the ownership matrix every authorized endpoint is run
// through.
//
// It sits on the PocketBase side of the import boundary.
package testsupport
