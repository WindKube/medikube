package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"hash"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
)

// CursorKeyInfo is the HKDF context label research D-25 names. It is part of
// the contract with every cursor already in somebody's browser: change it and
// they all stop validating at once, with nothing in the log that reads as a
// cause.
const CursorKeyInfo = "medikube-cursor-v1"

// MinCursorSecretLength is PocketBase's own floor for a token secret
// (core/collection_model_auth_options.go:300-306, 30..255). It is not a
// MediKube number, and it is checked here because the failure it guards is
// silent: Collection.MarshalJSON blanks every token secret, so a collection
// that reached this code through a JSON round trip carries the empty string —
// which HKDF will happily turn into a perfectly usable key that every other
// MediKube instance in the world derives too.
const MinCursorSecretLength = 30

// cursorFormat is the payload version. It is authenticated along with
// everything else, so an old cursor cannot be replayed as a new one after the
// encoding changes; a bump therefore retires every outstanding cursor
// deliberately, which is the point.
const cursorFormat = 1

// ErrInvalidCursor is every way a cursor can fail to be one this instance
// issued for this query: edited, truncated, minted under another key, replayed
// against another owner, or handed back under another ordering. The edge maps
// it to 400 invalid_cursor and audits an access_denied (research D-25, D-20).
//
// There is deliberately only one. Telling a client *which* check failed tells
// it how to get closer.
var ErrInvalidCursor = errors.New("store: the cursor is not one this instance issued for this query")

// authCollection is the auth collection MediKube's accounts live in and whose
// persisted token secret keys the cursor. PocketBase names it; it is not a
// kind.Kind, because an account is not a clinical record.
const authCollection = "users"

// AccountCollection is that name, published, for the account repository in
// internal/store/identity.
//
// It is exported for the same reason AuditCollection is: an account is not a
// record kind, so there is no kind.Collection() to read the name from, and the
// repository would otherwise spell it for itself — which is the drift the kind
// table exists to prevent, one collection to the left of where that table can
// reach.
const AccountCollection = authCollection

// Cursor is one keyset boundary: the last row of a page, expressed as the
// values it sorted by and its id.
//
// It is not an offset and there is nowhere in it to put one. An offset is
// defined against a result set that is changing underneath it, so a row
// inserted above the boundary shifts every later page by one — FR-023's "must
// not show the same entry twice nor skip an entry" is exactly that failure. A
// boundary is a row, and a row does not move when another row is added.
//
// Values are text because every column MediKube pages by is TEXT in SQLite: a
// date is stored in a layout whose lexicographic order is its chronological
// order, and the empty string — the absent date — sorts before every real one,
// in the cursor and in the index alike.
type Cursor struct {
	// The ordering the boundary belongs to, without the id tiebreaker, which
	// is always present and is carried in ID.
	Sort []domain.SortKey

	// One boundary value per sort term, positionally.
	Values []string

	// The tiebreaker. Two rows sharing every sort value are still ordered, and
	// they are ordered by this, which is why every index in data-model §2 ends
	// in id.
	ID string
}

func (c Cursor) IsZero() bool {
	return c.ID == "" && len(c.Sort) == 0 && len(c.Values) == 0
}

// CursorCodec turns a boundary into an opaque token and back.
//
// It is authenticated encryption rather than a signature, and the difference
// matters twice. Integrity is the requirement research D-25 states — a client
// that can move the boundary is choosing a query the service never offered.
// Confidentiality is the requirement D-29 states from the other side: the
// boundary value for FR-022's by-name ordering *is a drug name*, the cursor
// travels in a query string, and a query string reaches the browser history,
// the Referer header and whatever reverse proxy the operator runs — all of
// which log the full URI. A signed-but-readable cursor would put a medication
// name in an access log with one base64 decode in the way.
type CursorCodec struct {
	aead cipher.AEAD
}

// NewCursorCodec derives the key from the secret by HKDF-SHA256 and returns a
// codec bound to it. The secret is never stored on the codec.
func NewCursorCodec(secret string) (*CursorCodec, error) {
	if len(secret) < MinCursorSecretLength {
		// The value is not in the message. An error string reaches the log and
		// Sentry, and this one is key material (constitution VII).
		return nil, fmt.Errorf("store: the cursor key material is %d characters, and at least %d are required",
			len(secret), MinCursorSecretLength)
	}

	key, err := hkdf.Key(sha256.New, []byte(secret), nil, CursorKeyInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("store: deriving the cursor key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("store: the cursor key is not a usable AES key: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("store: building the cursor cipher: %w", err)
	}

	return &CursorCodec{aead: aead}, nil
}

// CursorSecret is the key material, and CT-3 in one function: the operator's
// MEDIKUBE_CURSOR_KEY when they set one, and otherwise PocketBase's persisted
// per-collection auth-token secret.
//
// Deriving it from something the database already holds is what makes SC-007
// work across a deploy — a per-process random key breaks every open page on
// every restart — without adding a secret the operator has to generate before
// the instance will start (SC-008).
//
// The hazard, which is real and accepted: PocketBase rotates that secret
// whenever the collection's AuthRule *value* changes
// (core/collection_model.go:862-866), and every outstanding cursor dies with
// it. That is tolerable only because the same rotation has already invalidated
// every issued auth token, so everybody holding a page has been signed out
// anyway. Re-saving the same rule value does not rotate — the comparison is on
// the dereferenced strings, not on pointer identity — so an ordinary migration
// re-run is not a mass logout.
func CursorSecret(app core.App, override string) (string, error) {
	if override != "" {
		return override, nil
	}

	collection, err := app.FindCachedCollectionByNameOrId(authCollection)
	if err != nil {
		return "", fmt.Errorf("store: reading the %s collection for the cursor key: %w", authCollection, err)
	}

	return cursorSecretFrom(collection)
}

func cursorSecretFrom(collection *core.Collection) (string, error) {
	secret := collection.AuthToken.Secret
	if len(secret) < MinCursorSecretLength {
		return "", fmt.Errorf(
			"store: %s carries a %d-character auth-token secret; a collection read through JSON has none, and an empty one would key every instance identically",
			collection.Name, len(secret))
	}

	return secret, nil
}

// Encode writes the boundary as a URL-safe opaque token.
//
// scope is the query the cursor continues — for an owner-scoped list, the kind
// and the owner. It is authenticated but never transmitted, so the token
// discloses nothing about whose list it is and cannot be replayed against
// somebody else's.
func (c *CursorCodec) Encode(scope string, cursor Cursor) (string, error) {
	if err := cursor.validate(); err != nil {
		return "", err
	}

	plaintext, err := marshalCursor(cursor)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("store: drawing a cursor nonce: %w", err)
	}

	sealed := c.aead.Seal(nonce, nonce, plaintext, cursorContext(scope, cursor.Sort))

	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decode returns the boundary a token carries, or ErrInvalidCursor.
//
// sort is the ordering the caller is about to page in. It goes into the same
// associated data the token was sealed with, so a cursor handed back under a
// different ordering fails to authenticate rather than being compared against
// something afterwards — there is no check here for a later refactor to drop.
func (c *CursorCodec) Decode(scope string, sort []domain.SortKey, token string) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w", ErrInvalidCursor)
	}

	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return Cursor{}, fmt.Errorf("%w", ErrInvalidCursor)
	}

	plaintext, err := c.aead.Open(nil, raw[:nonceSize], raw[nonceSize:], cursorContext(scope, sort))
	if err != nil {
		return Cursor{}, fmt.Errorf("%w", ErrInvalidCursor)
	}

	cursor, err := unmarshalCursor(plaintext)
	if err != nil {
		return Cursor{}, err
	}

	// Belt and braces. The ordering is already authenticated, so this can only
	// fire if the associated data stopped covering it — which is exactly the
	// refactor worth failing loudly on.
	if !sameSort(cursor.Sort, sort) {
		return Cursor{}, fmt.Errorf("%w", ErrInvalidCursor)
	}

	return cursor, nil
}

func (c Cursor) validate() error {
	if c.ID == "" {
		return fmt.Errorf("%w: a keyset boundary is a row, and a row needs its id", ErrInvalidCursor)
	}

	if len(c.Values) != len(c.Sort) {
		return fmt.Errorf("%w: %d sort terms and %d boundary values", ErrInvalidCursor, len(c.Sort), len(c.Values))
	}

	for _, key := range c.Sort {
		if key.Field == "" {
			return fmt.Errorf("%w: an unnamed sort term has no boundary to compare against", ErrInvalidCursor)
		}
	}

	return nil
}

// cursorContext is the associated data: the format version, the scope and the
// ordering, hashed into a fixed-size block.
//
// Hashing rather than concatenating, because the whole job of this value is to
// be unambiguous. Spelled out and joined, the two-term ordering `-a, b` and the
// one-term ordering `-ab` are the same string, and a cursor would cross between
// two different sequences without being refused. A SHA-256 block is a fixed
// thirty-two bytes, so a sequence of them can only collide if the sequences
// match term for term.
func cursorContext(scope string, sortKeys []domain.SortKey) []byte {
	context := sha256.New()

	context.Write([]byte{cursorFormat})
	writeHashed(context, scope)

	for _, key := range sortKeys {
		context.Write([]byte{boolByte(key.Desc)})
		writeHashed(context, key.Field)
	}

	return context.Sum(nil)
}

func writeHashed(into hash.Hash, value string) {
	sum := sha256.Sum256([]byte(value))
	into.Write(sum[:])
}

// cursorPayload is what the token carries. The member names are short because
// they are repeated in every token and nobody reads them: the whole value is
// inside the AEAD.
type cursorPayload struct {
	Version int             `json:"v"`
	Sort    []cursorSortKey `json:"s"`
	Values  []string        `json:"b"`
	ID      string          `json:"i"`
}

type cursorSortKey struct {
	Field string `json:"f"`
	Desc  bool   `json:"d"`
}

func marshalCursor(cursor Cursor) ([]byte, error) {
	payload := cursorPayload{
		Version: cursorFormat,
		Sort:    make([]cursorSortKey, 0, len(cursor.Sort)),
		Values:  cursor.Values,
		ID:      cursor.ID,
	}

	for _, key := range cursor.Sort {
		payload.Sort = append(payload.Sort, cursorSortKey{Field: key.Field, Desc: key.Desc})
	}

	encoded, err := jsonv2.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: the boundary cannot be encoded", ErrInvalidCursor)
	}

	return encoded, nil
}

func unmarshalCursor(plaintext []byte) (Cursor, error) {
	var payload cursorPayload

	// Strict on the way in even though the bytes are already authenticated: a
	// payload this build does not fully understand is one it must not act on
	// half of. Duplicate members are refused by encoding/json/v2 as it is.
	if err := jsonv2.Unmarshal(plaintext, &payload, jsonv2.RejectUnknownMembers(true)); err != nil {
		return Cursor{}, fmt.Errorf("%w", ErrInvalidCursor)
	}

	if payload.Version != cursorFormat {
		return Cursor{}, fmt.Errorf("%w", ErrInvalidCursor)
	}

	cursor := Cursor{ID: payload.ID}

	// encoding/json/v2 reads an absent or empty list back as an empty slice
	// rather than a nil one, and a boundary with no sort terms holds neither.
	if len(payload.Sort) > 0 {
		cursor.Sort = make([]domain.SortKey, 0, len(payload.Sort))
		for _, key := range payload.Sort {
			cursor.Sort = append(cursor.Sort, domain.SortKey{Field: key.Field, Desc: key.Desc})
		}
	}

	if len(payload.Values) > 0 {
		cursor.Values = payload.Values
	}

	if err := cursor.validate(); err != nil {
		return Cursor{}, fmt.Errorf("%w", ErrInvalidCursor)
	}

	return cursor, nil
}

func boolByte(value bool) byte {
	if value {
		return 1
	}

	return 0
}

func sameSort(left, right []domain.SortKey) bool {
	if len(left) != len(right) {
		return false
	}

	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}

	return true
}
