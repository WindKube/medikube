// Package seed writes MediKube's deterministic demo data: the three accounts of
// data-model §6, their medications and the superuser credential.
//
// It is a leaf on purpose. The committed fixture generator and `medikube seed`
// both write the same rows, and there is exactly one place those rows are
// written — a second seeder is how the fixture and the demo instance drift into
// two different applications. Keeping the package free of testing scaffolding
// is what lets the CLI call it without linking pocketbase/tests into the
// shipped binary.
//
// It sits on the PocketBase side of the import boundary.
package seed
