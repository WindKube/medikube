package access

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/identity"
)

const requestID = "01K3Q8Z0000000000000000000"

// The actor is built from the auth record at the HTTP edge and is the only
// thing derived from the token (shared design §6.6). It is asserted field for
// field because the tempting additions — the email address, the display name —
// are the two PHI-adjacent fields on the account, and an actor is carried into
// every service call and every audit write.
func TestAnActorCarriesOnlyWhatAuthorizationNeeds(t *testing.T) {
	t.Parallel()

	type field struct {
		name string
		typ  string
	}

	want := []field{
		{name: "UserID", typ: "string"},
		{name: "Role", typ: "identity.Role"},
		{name: "IsSuperuser", typ: "bool"},
		{name: "RequestID", typ: "string"},
	}

	got := make([]field, 0, len(want))
	for _, declared := range reflect.VisibleFields(reflect.TypeFor[Actor]()) {
		got = append(got, field{name: declared.Name, typ: declared.Type.String()})
	}

	assert.Equal(t, want, got, "an actor grew a field; anything beyond these four is derived, not carried")
}

func TestWhetherAnActorIsAuthenticated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		actor Actor
		want  bool
	}{
		{
			name:  "the zero actor is nobody",
			actor: Actor{},
			want:  false,
		},
		{
			name:  "an anonymous request is nobody, but it correlates",
			actor: Anonymous(requestID),
			want:  false,
		},
		{
			name:  "an account id is what makes an actor somebody",
			actor: Actor{UserID: "usr0000000001", Role: identity.RoleUser, RequestID: requestID},
			want:  true,
		},
		{
			// Fail closed: the superuser flag is read off the auth record, and
			// a request that carries no account is not promoted by it.
			name:  "a superuser flag without an account is still nobody",
			actor: Actor{IsSuperuser: true, RequestID: requestID},
			want:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, test.actor.Authenticated())
		})
	}
}

// A refusal is audited, and an audit row's request id is required — so the
// anonymous actor has to carry one or the refusal it causes could not be
// written (data-model §3).
func TestAnonymousCarriesTheCorrelationIDAndNothingElse(t *testing.T) {
	t.Parallel()

	actor := Anonymous(requestID)

	assert.Equal(t, Actor{RequestID: requestID}, actor)
	assert.False(t, actor.Authenticated())
}

func TestThePermissionVocabulary(t *testing.T) {
	t.Parallel()

	t.Run("the zero value is not a permission", func(t *testing.T) {
		t.Parallel()

		// A struct field or a forgotten argument defaults to zero, and a zero
		// that read as "may view" would be a silent grant.
		var unset Permission

		assert.False(t, unset.Valid())
		assert.Equal(t, "unknown", unset.String())
	})

	t.Run("the three published levels ascend", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, []Permission{PermView, PermEdit, PermOwn}, Permissions())
		assert.Less(t, PermView, PermEdit, "Satisfies compares them, so the order is load-bearing")
		assert.Less(t, PermEdit, PermOwn)
	})

	t.Run("the accessor clones", func(t *testing.T) {
		t.Parallel()

		got := Permissions()
		require.NotEmpty(t, got)
		got[0] = PermOwn

		assert.Equal(t, []Permission{PermView, PermEdit, PermOwn}, Permissions())
	})

	spellings := map[Permission]string{
		PermView: "view",
		PermEdit: "edit",
		PermOwn:  "own",
	}

	for permission, spelling := range spellings {
		t.Run("spells "+spelling, func(t *testing.T) {
			t.Parallel()

			assert.True(t, permission.Valid())
			assert.Equal(t, spelling, permission.String())
		})
	}

	t.Run("a value beyond the vocabulary is refused", func(t *testing.T) {
		t.Parallel()

		beyond := PermOwn + 1

		assert.False(t, beyond.Valid())
		assert.Equal(t, "unknown", beyond.String())
	})
}

// The whole matrix, both directions, including the invalid rows — because the
// interesting failure is not "view cannot edit", it is a zero value or a stray
// integer authorizing something. A Grant is asserted from the same table as the
// permission it carries, so the two cannot drift.
func TestWhatALevelSatisfiesAndWhatAGrantAllows(t *testing.T) {
	t.Parallel()

	// The two invalid rows spell the same word, so the subtest name carries the
	// number as well and a failure names the value that authorized.
	label := func(p Permission) string { return p.String() + "(" + strconv.Itoa(int(p)) + ")" }

	levels := []Permission{0, PermView, PermEdit, PermOwn, PermOwn + 1}

	for _, held := range levels {
		for _, need := range levels {
			want := held.Valid() && need.Valid() && held >= need

			t.Run(label(held)+" needing "+label(need), func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, want, held.Satisfies(need))
				assert.Equal(t, want, Grant{Level: held}.Allows(need),
					"a grant allows exactly what its level satisfies")
			})
		}
	}
}

func TestTheZeroGrantAllowsNothing(t *testing.T) {
	t.Parallel()

	// The authorizer returns a zero Grant beside every error, and a caller that
	// checked the grant instead of the error must still be refused.
	var refused Grant

	for _, need := range Permissions() {
		assert.False(t, refused.Allows(need), "the zero grant allowed %s", need)
	}
}

// FR-032's shape: authorization is a property of the route and the data, and
// what the caller may do arrives back from the checkpoint rather than being
// asserted by the caller.
func TestAGrantCarriesOnlyTheResolvedLevel(t *testing.T) {
	t.Parallel()

	fields := reflect.VisibleFields(reflect.TypeFor[Grant]())
	require.Len(t, fields, 1)

	assert.Equal(t, "Level", fields[0].Name)
	assert.Equal(t, "access.Permission", fields[0].Type.String())
}
