package api_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/store"
	"medikube/internal/testsupport"
)

// T194, FR-012. THE privilege-escalation test.
//
// It is driven from the COLLECTION'S OWN SCHEMA rather than from a list
// somebody wrote out, and that is the entire value of it: a column added to the
// account collection in a later phase is refused by default and fails this test
// the day it is added, instead of quietly becoming writable because nobody
// thought to add it to a deny list. A deny list is a list of the attacks
// somebody already imagined.

// theFiveWritableMembers is FR-011's enumeration and the ONLY members
// updateMe accepts. Every other column of the account collection — including
// ones that do not exist yet — is refused.
var theFiveWritableMembers = map[string]string{
	"name":        quoted("Amara O."),
	"unit_system": quoted(string(domainidentity.UnitSystemImperial)),
	"locale":      quoted("en-GB"),
	"date_format": quoted(string(domainidentity.DateFormatDMY)),
	"theme":       quoted(string(domainidentity.ThemeDark)),
}

func TestUpdateMeAcceptsExactlyTheFiveMembersFRelevenEnumerates(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	collection, err := instance.instance.App.FindCollectionByNameOrId(store.AccountCollection)
	require.NoError(t, err)

	columns := collection.Fields.FieldNames()
	require.NotEmpty(t, columns, "the account collection has no columns, so this test asserts nothing")

	// Every writable member is a real column, so the allowlist cannot drift
	// into naming something the account does not have.
	for member := range theFiveWritableMembers {
		assert.Containsf(t, columns, member, "%s is accepted by updateMe and is not a column", member)
	}

	for _, column := range columns {
		t.Run(column, func(t *testing.T) {
			t.Parallel()

			one := newRig(t)
			signedIn := one.as(testsupport.AccountAEmail)
			before := snapshot(t, one)

			value, writable := theFiveWritableMembers[column]
			if !writable {
				value = quoted("whatever a caller would like this to be")
			}

			answer := signedIn.do(http.MethodPatch, meURL, body(column, value), nil)

			if writable {
				require.Equalf(t, http.StatusOK, answer.Status,
					"%s is one of FR-011's five and was refused: %s", column, answer.Body)

				return
			}

			require.Equalf(t, http.StatusUnprocessableEntity, answer.Status,
				"%s is not one of FR-011's five and was accepted: %s", column, answer.Body)
			assert.Equal(t, [][2]string{{column, domain.CodeUnknownField}}, answer.envelope(t).fieldCodes())

			assertUnchangedSince(t, one, before)
		})
	}
}

// The spellings a schema walk cannot produce: a member that is not a column at
// all, a null, a nested object, a case-mismatched name and an array. Each of
// them is a shape somebody would reach for after the plain string was refused.
func TestNoSpellingOfARoleReachesTheAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		document string
		member   string
	}{
		{name: "a role", document: body("role", quoted("admin")), member: "role"},
		{name: "a null role", document: body("role", "null"), member: "role"},
		{name: "a nested role", document: body("role", `{"tier":"admin"}`), member: "role"},
		{name: "an array of roles", document: body("role", `["admin"]`), member: "role"},
		{name: "a capitalised role", document: body("Role", quoted("admin")), member: "Role"},
		{name: "an account status", document: body("status", quoted("active")), member: "status"},
		{name: "a disabled instant", document: body("disabled_at", quoted("")), member: "disabled_at"},
		{name: "a confirmed address", document: body("verified", "true"), member: "verified"},
		{name: "a new address", document: body("email", quoted("elsewhere@example.test")), member: "email"},
		{name: "a password", document: body("password", quoted("chosen-without-the-old-one")), member: "password"},
		{name: "an id", document: body("id", quoted(testsupport.AccountBID)), member: "id"},
		{
			name:     "a role smuggled beside a member that is allowed",
			document: body("theme", quoted("dark"), "role", quoted("admin")),
			member:   "role",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			instance := newRig(t)
			before := snapshot(t, instance)

			answer := instance.as(testsupport.AccountAEmail).do(http.MethodPatch, meURL, test.document, nil)

			require.Equalf(t, http.StatusUnprocessableEntity, answer.Status,
				"%s was not refused: %s", test.name, answer.Body)
			assert.Equal(t, [][2]string{{test.member, domain.CodeUnknownField}}, answer.envelope(t).fieldCodes())

			assertUnchangedSince(t, instance, before)
		})
	}
}

// snapshot is the account as the database holds it, read before the request
// under test.
func snapshot(t *testing.T, instance *rig) domainidentity.User {
	t.Helper()

	stored, err := store.UserFromRecord(instance.stored(t, testsupport.AccountAEmail))
	require.NoError(t, err)

	return stored
}

// assertUnchangedSince is the other half of the requirement: a refused body
// must not have changed ANYTHING — not the member it was refused for, and not
// the allowed member sitting beside it.
//
// It compares against a snapshot rather than against a list of expected values,
// so a column added later is covered without anybody adding a line: whatever it
// held before the request is what it must hold after one.
func assertUnchangedSince(t *testing.T, instance *rig, before domainidentity.User) {
	t.Helper()

	after := snapshot(t, instance)

	// Compared member by member so a failure names the one that moved. The
	// stored instants are excluded deliberately: a save that changed nothing
	// would still move `updated`, and a test that could not tell those apart
	// would be asserting that no write happened rather than that no VALUE
	// changed.
	assert.Equal(t, before.Role, after.Role, "the stored role moved")
	assert.Equal(t, before.Email, after.Email, "the stored address moved")
	assert.Equal(t, before.EmailConfirmed, after.EmailConfirmed, "the stored confirmation moved")
	assert.Equal(t, before.Name, after.Name, "a member beside a refused one was applied")
	assert.Equal(t, before.Theme, after.Theme, "a member beside a refused one was applied")
	assert.Equal(t, before.UnitSystem, after.UnitSystem)
	assert.Equal(t, before.Locale, after.Locale)
	assert.Equal(t, before.DateFormat, after.DateFormat)
	assert.Equal(t, before.DisabledAt, after.DisabledAt, "the account was taken out of service by a request")
	assert.Equal(t, before.ID, after.ID)
}

// A promotion attempted through the domain's own vocabulary: `admin` IS a role
// MediKube publishes, so the refusal cannot be "that is not a role".
func TestTheRoleVocabularyIsPublishedAndStillUnreachable(t *testing.T) {
	t.Parallel()

	require.True(t, slices.Contains(domainidentity.Roles(), domainidentity.RoleAdmin),
		"admin is not a published role, so this test is not attempting a real promotion")

	instance := newRig(t)
	before := snapshot(t, instance)

	answer := instance.as(testsupport.AccountAEmail).
		do(http.MethodPatch, meURL, body("role", quoted(string(domainidentity.RoleAdmin))), nil)

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)

	// And the account is still exactly what it was — asserted against the
	// database, because a 422 that had already written the row would look
	// identical from the outside.
	assertUnchangedSince(t, instance, before)
	assert.Equal(t, string(domainidentity.RoleUser),
		instance.as(testsupport.AccountAEmail).get(meURL).me(t).Role)
}
