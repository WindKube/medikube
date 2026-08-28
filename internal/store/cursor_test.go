package store

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
)

// A secret of the shape PocketBase persists: 50 random characters. Two of them,
// because half of what this file proves is that a cursor minted under one key
// is refused under another.
const (
	testSecret      = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN"
	testOtherSecret = "NMLKJIHGFEDCBAzyxwvutsrqponmlkjihgfedcba9876543210"
)

// The scope is the query a cursor continues: the kind, and whose list it is.
// The kind's spelling comes from the kind table like every other spelling of it
// (research D-05).
var testScope = kind.Medication.Collection() + ":owner-abc123"

var startedThenID = []domain.SortKey{{Field: "started_on", Desc: true}}

func newTestCodec(t *testing.T, secret string) *CursorCodec {
	t.Helper()

	codec, err := NewCursorCodec(secret)
	require.NoError(t, err)

	return codec
}

// T077, research D-25. The round trip is the cheap half; what it has to prove
// is that every part of the keyset boundary survives, including the parts that
// are easy to drop — an empty boundary value (the absent started_on, which
// sorts before every real date), a value carrying the delimiter a naive
// encoding would split on, and a multi-key sort.
func TestACursorRoundTripsThroughTheCodec(t *testing.T) {
	t.Parallel()

	codec := newTestCodec(t, testSecret)

	cases := []struct {
		name   string
		cursor Cursor
	}{
		{
			name:   "the id alone, which is the ordering with no sort term",
			cursor: Cursor{ID: "abcdefghij12345"},
		},
		{
			name: "one descending sort term",
			cursor: Cursor{
				Sort:   startedThenID,
				Values: []string{"2026-03-01"},
				ID:     "abcdefghij12345",
			},
		},
		{
			name: "the absent date, which is the empty string and sorts first",
			cursor: Cursor{
				Sort:   startedThenID,
				Values: []string{""},
				ID:     "abcdefghij12345",
			},
		},
		{
			name: "two sort terms in opposite directions",
			cursor: Cursor{
				Sort: []domain.SortKey{
					{Field: "status", Desc: false},
					{Field: "updated", Desc: true},
				},
				Values: []string{"active", "2026-03-01 09:00:00.000Z"},
				ID:     "abcdefghij12345",
			},
		},
		{
			name: "a boundary value carrying the separators a delimited encoding would split on",
			cursor: Cursor{
				Sort:   []domain.SortKey{{Field: "name"}},
				Values: []string{`a|b:c.d,e"f'g` + "\x00" + "h"},
				ID:     "abcdefghij12345",
			},
		},
		{
			name: "a boundary value outside ASCII",
			cursor: Cursor{
				Sort:   []domain.SortKey{{Field: "name"}},
				Values: []string{"リン酸コデイン"},
				ID:     "abcdefghij12345",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			token, err := codec.Encode(testScope, testCase.cursor)
			require.NoError(t, err)
			require.NotEmpty(t, token)

			decoded, err := codec.Decode(testScope, testCase.cursor.Sort, token)
			require.NoError(t, err)
			assert.Equal(t, testCase.cursor, decoded)
		})
	}
}

// The token is URL-safe and unpadded, because it is carried in a query string,
// and it is not a readable encoding of the boundary: a drug name in a query
// string reaches the browser history, the Referer header and whatever reverse
// proxy the operator put in front of the instance, all of which log the full
// URI. base64 of a plaintext name would be a disclosure with an extra step.
func TestTheCursorTokenIsURLSafeAndDisclosesNothing(t *testing.T) {
	t.Parallel()

	codec := newTestCodec(t, testSecret)

	const drug = "Amoxicillin"

	token, err := codec.Encode(testScope, Cursor{
		Sort:   []domain.SortKey{{Field: "name"}},
		Values: []string{drug},
		ID:     "abcdefghij12345",
	})
	require.NoError(t, err)

	assert.NotContains(t, token, "=", "the token is padded, so it needs escaping in a query string")
	assert.Equal(t, token, strings.TrimSpace(token))
	for _, r := range token {
		require.Truef(t,
			(r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_',
			"%q is not a base64url character", r)
	}

	raw, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), drug,
		"the boundary value is readable in the token; a medication name in a URL is FR-038's disclosure with an extra step")
	assert.NotContains(t, string(raw), testScope)
}

// T077, and the property the whole file exists for: a cursor a client edited is
// refused, not believed. A client that can move the keyset boundary is choosing
// a query the service never offered (research D-25, Principle VII).
func TestATamperedCursorIsRejectedRatherThanTrusted(t *testing.T) {
	t.Parallel()

	codec := newTestCodec(t, testSecret)
	other := newTestCodec(t, testOtherSecret)

	genuine, err := codec.Encode(testScope, Cursor{
		Sort:   startedThenID,
		Values: []string{"2026-03-01"},
		ID:     "abcdefghij12345",
	})
	require.NoError(t, err)

	raw, err := base64.RawURLEncoding.DecodeString(genuine)
	require.NoError(t, err)

	fromAnotherKey, err := other.Encode(testScope, Cursor{
		Sort:   startedThenID,
		Values: []string{"2026-03-01"},
		ID:     "abcdefghij12345",
	})
	require.NoError(t, err)

	fromAnotherScope, err := codec.Encode(kind.Medication.Collection()+":owner-zzz999", Cursor{
		Sort:   startedThenID,
		Values: []string{"2026-03-01"},
		ID:     "abcdefghij12345",
	})
	require.NoError(t, err)

	cases := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"not base64 at all", "not a cursor"},
		{"base64 of nothing", base64.RawURLEncoding.EncodeToString(nil)},
		{"base64 of less than a nonce", base64.RawURLEncoding.EncodeToString([]byte("short"))},
		{"truncated by one byte", base64.RawURLEncoding.EncodeToString(raw[:len(raw)-1])},
		{"one byte appended", base64.RawURLEncoding.EncodeToString(append(append([]byte(nil), raw...), 0x2a))},
		{"the nonce replaced", base64.RawURLEncoding.EncodeToString(append(make([]byte, 12), raw[12:]...))},
		{"minted with another key", fromAnotherKey},
		{"minted for another owner", fromAnotherScope},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decoded, decodeErr := codec.Decode(testScope, startedThenID, testCase.token)
			require.ErrorIs(t, decodeErr, ErrInvalidCursor)
			assert.Equal(t, Cursor{}, decoded, "a rejected cursor must not hand back a usable boundary")
		})
	}

	// Every single-bit edit, not a sample of them: an authenticator that
	// happened to ignore one byte of the token would pass a spot check.
	t.Run("every single-bit edit", func(t *testing.T) {
		t.Parallel()

		for index := range raw {
			for bit := range 8 {
				edited := append([]byte(nil), raw...)
				edited[index] ^= 1 << bit

				_, decodeErr := codec.Decode(testScope, startedThenID,
					base64.RawURLEncoding.EncodeToString(edited))
				require.ErrorIsf(t, decodeErr, ErrInvalidCursor,
					"byte %d bit %d was accepted after being flipped", index, bit)
			}
		}
	})
}

// T077's "the encoding is keyset, never an offset", made mechanical. A cursor
// that carried a row count would repeat or skip a row the moment somebody else
// inserted one, and the failure would be silent — so the type is not allowed to
// grow a member a count could live in.
func TestTheCursorIsAKeysetBoundaryAndHasNowhereToPutAnOffset(t *testing.T) {
	t.Parallel()

	cursorType := reflect.TypeOf(Cursor{})

	names := make([]string, 0, cursorType.NumField())
	for i := range cursorType.NumField() {
		field := cursorType.Field(i)
		names = append(names, field.Name)

		assert.NotContainsf(t, strings.ToLower(field.Name), "offset",
			"%s is an offset by name", field.Name)
		assert.NotContainsf(t, strings.ToLower(field.Name), "page",
			"%s is a page number by name", field.Name)
		require.NotEqualf(t, reflect.Int, field.Type.Kind(),
			"%s is a number, and the only number a cursor could hold is a row count", field.Name)
	}

	assert.Equal(t, []string{"Sort", "Values", "ID"}, names,
		"the cursor grew a member; a keyset boundary is a row, and a row is its sort values and its id")
}

// The boundary has to be the row, so two different rows produce two different
// cursors — including two rows sharing a sort value, which is the case the id
// tiebreaker exists for and the one an unstable index would page wrong.
func TestTwoDifferentRowsProduceTwoDifferentCursors(t *testing.T) {
	t.Parallel()

	codec := newTestCodec(t, testSecret)

	first, err := codec.Encode(testScope, Cursor{
		Sort: startedThenID, Values: []string{"2026-03-01"}, ID: "aaaaaaaaaa11111",
	})
	require.NoError(t, err)

	sameDayOtherRow, err := codec.Encode(testScope, Cursor{
		Sort: startedThenID, Values: []string{"2026-03-01"}, ID: "bbbbbbbbbb22222",
	})
	require.NoError(t, err)

	otherDay, err := codec.Encode(testScope, Cursor{
		Sort: startedThenID, Values: []string{"2026-03-02"}, ID: "aaaaaaaaaa11111",
	})
	require.NoError(t, err)

	assert.NotEqual(t, first, sameDayOtherRow)
	assert.NotEqual(t, first, otherDay)

	// And the same row twice is still two different tokens, because the nonce
	// is fresh: a cursor is not a stable identifier somebody can index on.
	repeat, err := codec.Encode(testScope, Cursor{
		Sort: startedThenID, Values: []string{"2026-03-01"}, ID: "aaaaaaaaaa11111",
	})
	require.NoError(t, err)
	assert.NotEqual(t, first, repeat)
}

// A cursor the store would not have issued is refused at the point it is
// issued, rather than becoming a query with a missing boundary.
func TestEncodeRefusesACursorThatIsNotAKeysetBoundary(t *testing.T) {
	t.Parallel()

	codec := newTestCodec(t, testSecret)

	cases := []struct {
		name   string
		cursor Cursor
	}{
		{"no id, so no tiebreaker", Cursor{Sort: startedThenID, Values: []string{"2026-03-01"}}},
		{"a sort term with no boundary value", Cursor{Sort: startedThenID, ID: "abcdefghij12345"}},
		{"a boundary value with no sort term", Cursor{Values: []string{"2026-03-01"}, ID: "abcdefghij12345"}},
		{"an unnamed sort term", Cursor{Sort: []domain.SortKey{{}}, Values: []string{"x"}, ID: "abcdefghij12345"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := codec.Encode(testScope, testCase.cursor)
			require.ErrorIs(t, err, ErrInvalidCursor)
		})
	}
}

// T077, CT-3. SC-007 requires a list left open for an hour to still work, and a
// deploy inside that hour is ordinary. A per-process random key would break
// every open page on every restart with no error anybody could act on, so the
// key comes from something the database already persists.
func TestACursorIssuedBeforeARestartStillValidatesAfterwards(t *testing.T) {
	t.Parallel()

	dataDir := tempDataDir(t)
	app := newBaseApp(t, dataDir)

	secret, err := CursorSecret(app, "")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(secret), MinCursorSecretLength)

	before := newTestCodec(t, secret)

	boundary := Cursor{Sort: startedThenID, Values: []string{"2026-03-01"}, ID: "abcdefghij12345"}

	token, err := before.Encode(testScope, boundary)
	require.NoError(t, err)

	// The restart. A second BaseApp over the same directory, which reads the
	// collection back off disk rather than out of the first app's memory.
	require.NoError(t, app.ResetBootstrapState())

	restarted := newBaseApp(t, dataDir)

	afterSecret, err := CursorSecret(restarted, "")
	require.NoError(t, err)
	require.Equal(t, secret, afterSecret, "the key material did not survive the restart")

	decoded, err := newTestCodec(t, afterSecret).Decode(testScope, startedThenID, token)
	require.NoError(t, err)
	assert.Equal(t, boundary, decoded)
}

// CT-3's documented hazard, asserted rather than described. PocketBase rotates
// a collection's auth-token secret whenever its AuthRule *value* changes
// (core/collection_model.go:862-866), which invalidates every outstanding
// cursor. That is acceptable only because the same rotation has already
// invalidated every session, so nobody is holding a page they could still use.
func TestChangingTheAuthRuleRotatesTheKeyAndWithItEveryOutstandingCursor(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	secret, err := CursorSecret(app, "")
	require.NoError(t, err)

	token, err := newTestCodec(t, secret).Encode(testScope, Cursor{
		Sort: startedThenID, Values: []string{"2026-03-01"}, ID: "abcdefghij12345",
	})
	require.NoError(t, err)

	users, err := app.FindCollectionByNameOrId(authCollection)
	require.NoError(t, err)

	// A no-op re-save first: the rotation is keyed on the rule's value, not on
	// pointer identity, so writing the same string back must not rotate.
	users.AuthRule = types.Pointer("")
	require.NoError(t, app.Save(users))

	unrotated, err := CursorSecret(app, "")
	require.NoError(t, err)
	require.Equal(t, secret, unrotated, "a no-op rule re-save rotated the secret")

	_, err = newTestCodec(t, unrotated).Decode(testScope, startedThenID, token)
	require.NoError(t, err)

	// Now a real change.
	users, err = app.FindCollectionByNameOrId(authCollection)
	require.NoError(t, err)
	users.AuthRule = types.Pointer("verified = true")
	require.NoError(t, app.Save(users))

	rotated, err := CursorSecret(app, "")
	require.NoError(t, err)
	require.NotEqual(t, secret, rotated, "PocketBase no longer rotates on an auth rule change; cursor.go's comment is stale")

	_, err = newTestCodec(t, rotated).Decode(testScope, startedThenID, token)
	assert.ErrorIs(t, err, ErrInvalidCursor)
}

// The override exists so an operator who does not want a MediKube security
// property riding on a PocketBase field can take it back (CT-3's fallback).
func TestTheKeyComesFromThePersistedSecretUnlessTheOperatorOverridesIt(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	users, err := app.FindCollectionByNameOrId(authCollection)
	require.NoError(t, err)
	require.Len(t, users.AuthToken.Secret, 50, "PocketBase's persisted auth-token secret is 50 characters")

	derived, err := CursorSecret(app, "")
	require.NoError(t, err)
	assert.Equal(t, users.AuthToken.Secret, derived)

	override := strings.Repeat("k", MinCursorSecretLength)
	configured, err := CursorSecret(app, override)
	require.NoError(t, err)
	assert.Equal(t, override, configured)
}

// The failure this guards is silent by construction: Collection.MarshalJSON
// blanks every token secret (core/collection_model.go:558-571), so a collection
// that reached the caller through a JSON round trip carries an empty one — and
// an empty secret still derives a perfectly usable key, just a public one.
func TestAnUnusableSecretIsRefusedRatherThanDerivedFrom(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	cases := []struct {
		name     string
		override string
	}{
		{"empty after a JSON round trip", ""},
		{"one distinctive character", "Q"},
		{"one short of the floor", strings.Repeat("Q", MinCursorSecretLength-1)},
	}

	users, err := app.FindCollectionByNameOrId(authCollection)
	require.NoError(t, err)

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, codecErr := NewCursorCodec(testCase.override)
			require.Error(t, codecErr)

			if testCase.override != "" {
				assert.NotContains(t, codecErr.Error(), testCase.override,
					"the secret is in the error message, and an error message reaches the log and Sentry")
			}
			assert.Contains(t, codecErr.Error(), "characters",
				"the refusal has to say what is wrong without saying what the value was")
		})
	}

	// The same refusal reached through the collection, which is the path a
	// blanked secret actually arrives by.
	blanked := *users
	blanked.AuthToken.Secret = ""

	_, err = cursorSecretFrom(&blanked)
	require.Error(t, err)
}

func TestTheKeyDerivationIsPinned(t *testing.T) {
	t.Parallel()

	// The label is part of the contract: change it and every cursor issued by
	// every running instance stops validating, with no error the operator can
	// read as a cause (research D-25 names this exact string).
	assert.Equal(t, "medikube-cursor-v1", CursorKeyInfo)

	// PocketBase validates its own token secrets at 30..255, so the floor is
	// not a MediKube invention and cannot be tightened past what the source
	// supplies.
	assert.Equal(t, 30, MinCursorSecretLength)

	// Two secrets, two keys. It reads as tautological until somebody derives
	// the key from a constant and passes the secret to nothing.
	first := newTestCodec(t, testSecret)
	second := newTestCodec(t, testOtherSecret)

	token, err := first.Encode(testScope, Cursor{ID: "abcdefghij12345"})
	require.NoError(t, err)

	_, err = second.Decode(testScope, nil, token)
	assert.ErrorIs(t, err, ErrInvalidCursor)
}
