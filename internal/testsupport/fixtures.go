package testsupport

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

// The identifiers of the seeded fixture, so that no test anywhere in the
// repository contains a literal id (data-model §6).
//
// They are written out here rather than aliased to the seeder's own constants,
// and that is the whole point: these are what the tests of five phases are
// pinned to, the seeder is what the fixture is built from, and fixtures_test.go
// is the gate that fails the moment the two stop agreeing. An alias could not
// disagree, and a seed that silently moved an id would take every ownership
// test with it.
const (
	AccountAID = "mkacctamara0001"
	AccountBID = "mkacctboris0001"
	AccountCID = "mkacctchidi0001"

	// SuperuserID is the admin-UI credential. It is a PocketBase superuser and
	// not an account with identity.RoleAdmin — the two are different things and
	// FR-040 turns on them staying different.
	SuperuserID = "mksuperadmin001"
)

const (
	AccountAEmail = "amara@example.test"
	AccountBEmail = "boris@example.test"
	AccountCEmail = "chidi@example.test"

	SuperuserEmail = "admin@example.test"
)

const (
	AccountAName = "Amara Okonkwo"
	AccountBName = "Boris Novak"
	AccountCName = "Chidi Eze"
)

// Password is what every seeded account signs in with. It is published in
// quickstart.md on purpose — a demo credential nobody can look up is a demo
// nobody can run — and the command that writes it refuses to run in production.
//
//nolint:gosec // a published demo credential is the point of this constant
const Password = "medikube-dev-password"

// The four rows data-model §6 calls out by hand. A test that needs the
// partial-data case or the escaping case asks for it by name; one that picks a
// row out of the list by index is asserting the seed's ordering by accident.
const (
	// NameOnlyMedicationID carries a name and a state and nothing else.
	NameOnlyMedicationID = "mkmedamara00003"
	// ScriptedMedicationID mixes right-to-left text with characters that look
	// like markup, so an unescaped template renders an element.
	ScriptedMedicationID = "mkmedamara00004"
	// SingleDayMedicationID starts and ends on the same day.
	SingleDayMedicationID = "mkmedamara00005"
	// FutureStartMedicationID starts in 2099.
	FutureStartMedicationID = "mkmedamara00006"
)

// The counts data-model §6 fixes. Account C's zero is load-bearing: it is what
// the empty-state smoke case navigates to, so the empty branch is exercised on
// every run rather than asserted once (research D-39).
const (
	AccountAMedicationCount = 12
	AccountBMedicationCount = 3
	AccountCMedicationCount = 0
)

// AuthToken mints a PocketBase auth token for a seeded account, which is what
// an HTTP test presents as the caller.
//
// It reads the record from the app under test rather than from a second
// instance, because the token is signed with the collection's own secret and a
// token minted against another clone would be rejected in a way that looks like
// an authorization bug.
func AuthToken(t testing.TB, app core.App, collection, email string) string {
	t.Helper()

	record, err := app.FindAuthRecordByEmail(collection, email)
	require.NoError(t, err, "no %s record for %s: is the fixture seeded?", collection, email)

	token, err := record.NewAuthToken()
	require.NoError(t, err)

	return token
}

// UserToken is AuthToken against the account collection, which is what every
// ordinary caller in this phase is.
func UserToken(t testing.TB, app core.App, email string) string {
	t.Helper()

	return AuthToken(t, app, usersCollection, email)
}
