package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The account schema publishes ONE narrowable column and it stays one.
//
// There is no account list in MediKube and no phase in this spec adds one:
// every account operation reaches the caller's own account and no other
// (FR-032). A second column here would be the query surface for a listing
// nobody asked for, over a table of the people who have medical records on this
// instance — and it would arrive with nothing objecting, because the builder
// answers whatever the schema declares.
func TestTheAccountSchemaPublishesTheAddressAndNothingElse(t *testing.T) {
	t.Parallel()

	schema := AccountSchema()

	assert.Equal(t, []string{fieldID, userFieldEmail}, schema.Columns())
	assert.Equal(t, authCollection, schema.Collection())

	email, declared := schema.Column(userFieldEmail)
	require.True(t, declared)
	assert.True(t, email.FilterOnly,
		"the address is orderable, so it can become a keyset boundary — and a boundary travels in a query string, through the browser history, the Referer header and every proxy log")
	assert.False(t, email.Searchable,
		"the address may join a disjunction, which is the one term shape that can make another term optional")
}

// The one that would otherwise rot silently, and it is the one FR-003 rests on.
//
// SameAddress folds the supplied address in Go and compares it against the
// column's LOWER(email) in SQLite. The two have to agree byte for byte: SQLite
// folds ASCII and nothing else, strings.ToLower folds the whole of Unicode, and
// a disagreement means the query stops matching idx_users_email_lower — so a
// second account for one address would be created by MediKube and refused by
// the database, as a 500, at the storage layer.
func TestTheAddressFoldsInGoExactlyAsSQLiteFoldsIt(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	schema := AccountSchema()

	email, declared := schema.Column(userFieldEmail)
	require.True(t, declared)

	// Addresses picked so a case-folding disagreement shows: a Turkish dotted
	// capital I folds to two code points in Go and not at all in SQLite, and an
	// address may legally carry either.
	for _, address := range []string{
		"amara@example.test",
		"AMARA.OKONKWO@Example.Test",
		"İstanbul@example.test",
		"boris+TAG@example.test",
	} {
		record := seedUser(t, app, address)

		var fromSQLite string

		require.NoError(t, app.DB().
			Select(email.Expr).
			From(authCollection).
			Where(dbxID(record.Id)).
			Row(&fromSQLite))

		assert.Equalf(t, fromSQLite, email.Value(record),
			"%s: the Go fold and the SQL expression disagree, so a lookup stops matching idx_users_email_lower",
			address)
	}
}

// SameAddress finds the account whatever case it is asked in, and finds no
// other. Run against the database rather than against the rendered SQL,
// because what is being asserted is that the comparison the index performs and
// the comparison this builds are the same comparison.
func TestSameAddressResolvesOneAccountInAnyLetterCase(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	// STORED IN MIXED CASE, deliberately. An account keeps the address its
	// owner typed, so the fold has to happen on BOTH sides: a stored address
	// that is already lower-case would be found by a query that folded only the
	// value it was given, and the column's own LOWER() would never be exercised.
	wanted := seedUser(t, app, "Amara.Okonkwo@Example.Test")
	seedUser(t, app, "amara.okonkwo2@example.test")
	seedUser(t, app, "boris@example.test")

	cases := []struct {
		name  string
		asked string
		found bool
	}{
		{name: "as registered", asked: "Amara.Okonkwo@Example.Test", found: true},
		{name: "all lower case", asked: "amara.okonkwo@example.test", found: true},
		{name: "shouted", asked: "AMARA.OKONKWO@EXAMPLE.TEST", found: true},
		{name: "capitalised differently", asked: "aMARA.oKONKWO@eXAMPLE.tEST", found: true},
		{name: "an address nobody has", asked: "nobody@example.test"},
		{name: "a prefix of a registered address", asked: "amara.okonkwo@example.tes"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built, err := AccountSchema().Build(Query{
				Conditions: []Condition{SameAddress(testCase.asked)},
				Limit:      2,
			})
			require.NoError(t, err)

			var ids []struct {
				ID string `db:"id"`
			}

			require.NoError(t, built.Apply(app.RecordQuery(authCollection)).All(&ids))

			if !testCase.found {
				assert.Empty(t, ids, "the address resolved to an account nobody registered")

				return
			}

			require.Len(t, ids, 1, "one address resolved to more than one account")
			assert.Equal(t, wanted.Id, ids[0].ID)
		})
	}
}
