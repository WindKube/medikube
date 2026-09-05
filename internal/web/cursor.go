package web

import (
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

// The list parameters of contracts/README.md, named once. Filtering is explicit
// named parameters only: PocketBase's filter DSL never reaches the wire, so
// there is no parameter here a caller could put an expression in.
const (
	ParamLimit  = "limit"
	ParamCursor = "cursor"
	ParamSort   = "sort"
	ParamCount  = "count"
	ParamSearch = "q"
)

// The page-size bounds. A ceiling exists because a caller that asks for
// everything is asking the database to read a whole account's records into
// memory on one connection.
const (
	MinLimit     = 1
	MaxLimit     = 100
	DefaultLimit = 25
)

// ListParams is one list request as the edge parsed it, before any kind has
// seen it.
type ListParams struct {
	// Sort is resolved, never raw. The cursor codec binds the ordering into its
	// associated data, so a cursor issued under the kind's default and decoded
	// under an empty list would fail to authenticate — and every request after
	// the first page carries no ?sort= at all.
	Sort   []domain.SortKey
	Limit  int
	Cursor string
	Count  bool
	Search string
}

// ListQuery reads the shared list parameters and resolves the ordering against
// the kind's allowlist.
//
// Every refusal is 422 validation_failed with the parameter's own name, and
// every problem is reported at once (FR-027). A value outside the allowlist is
// never silently ignored, because a silently ignored sort produces a list that
// looks right and is not.
func ListQuery(e *core.RequestEvent, allowed []domain.SortKey) (ListParams, error) {
	query := e.Request.URL.Query()

	var invalid domain.ValidationError

	params := ListParams{
		Limit:  DefaultLimit,
		Cursor: query.Get(ParamCursor),
		Search: query.Get(ParamSearch),
		Sort:   defaultSort(allowed),
	}

	if raw := query.Get(ParamLimit); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < MinLimit || limit > MaxLimit {
			// The submitted value is not in the message. This one is a number,
			// but the rule is the rule: an error string reaches the log.
			invalid.Addf(ParamLimit, domain.CodeInvalidValue, "a page holds between %d and %d entries", MinLimit, MaxLimit)
		} else {
			params.Limit = limit
		}
	}

	if raw := query.Get(ParamSort); raw != "" {
		sort, ok := resolveSort(raw, allowed)
		if !ok {
			invalid.Add(ParamSort, domain.CodeInvalidValue, "the ordering is not one this resource publishes")
		} else {
			params.Sort = sort
		}
	}

	if raw := query.Get(ParamCount); raw != "" {
		count, err := strconv.ParseBool(raw)
		if err != nil {
			invalid.Add(ParamCount, domain.CodeInvalidValue, "the value is true or false")
		} else {
			params.Count = count
		}
	}

	if err := invalid.OrNil(); err != nil {
		return ListParams{}, err
	}

	return params, nil
}

// defaultSort is the first entry of the allowlist, which is the order OpenAPI
// publishes it in and therefore the documented default.
func defaultSort(allowed []domain.SortKey) []domain.SortKey {
	if len(allowed) == 0 {
		return nil
	}

	return []domain.SortKey{allowed[0]}
}

func resolveSort(raw string, allowed []domain.SortKey) ([]domain.SortKey, bool) {
	terms := strings.Split(raw, ",")
	resolved := make([]domain.SortKey, 0, len(terms))

	for _, term := range terms {
		key, err := domain.ParseSortKey(strings.TrimSpace(term))
		if err != nil {
			return nil, false
		}

		found := false

		for _, permitted := range allowed {
			if key == permitted {
				found = true

				break
			}
		}

		if !found {
			return nil, false
		}

		resolved = append(resolved, key)
	}

	return resolved, true
}

// The three sort-key sets research D-29 fixes for phase 002's lists. They live
// here rather than in a service package the way medication.Sorts does: patients,
// practitioners and facilities are not a kind.Kind (research D-05 — the anchor,
// not a record kind), so there is no per-kind service package for this
// foundational phase to put them in ahead of the story that builds one.
//
// Every set ends in `id`: the mandatory tiebreaker, because two rows sharing
// every other sorted column — twins, a father and son with the same name — would
// otherwise make a cursor ambiguous (research D-29, spec Edge Cases).

// PatientsSort is the default sort for the patients list.
func PatientsSort() []domain.SortKey {
	return []domain.SortKey{
		{Field: "last_name"},
		{Field: "first_name"},
		{Field: "id"},
	}
}

// PractitionersSort is the default sort for the practitioners list.
func PractitionersSort() []domain.SortKey {
	return []domain.SortKey{
		{Field: "name"},
		{Field: "id"},
	}
}

// FacilitiesSort is the default sort for the facilities list.
func FacilitiesSort() []domain.SortKey {
	return []domain.SortKey{
		{Field: "kind"},
		{Field: "name"},
		{Field: "id"},
	}
}

// CursorScope is the query a cursor continues, as one unambiguous string.
//
// It is authenticated into the token and never transmitted, so the cursor
// discloses nothing about whose list it is and cannot be replayed against
// somebody else's. The kind is spelled by its enum value and the two halves are
// separated by a byte neither of them can contain, so no two (kind, owner)
// pairs can render to the same scope.
func CursorScope(k kind.Kind, ownerID string) string {
	return "records\x00" + k.Enum() + "\x00" + ownerID
}

// Cursors is the cursor at the HTTP edge: internal/store owns the AEAD, the
// keyset payload and the sort binding, and this wraps it so a handler never
// touches the codec directly and never invents a second scope spelling.
type Cursors struct {
	codec *store.CursorCodec
}

// NewCursors wraps the codec the composition root derived from the collection's
// persisted auth-token secret.
func NewCursors(codec *store.CursorCodec) *Cursors {
	return &Cursors{codec: codec}
}

// Encode mints the token for the next page.
func (c *Cursors) Encode(scope string, cursor store.Cursor) (string, error) {
	return c.codec.Encode(scope, cursor)
}

// Decode returns the boundary a token carries, or store.ErrInvalidCursor —
// which the mapper answers as 400 invalid_cursor.
//
// sort is the RESOLVED ordering the caller is about to page in, because that is
// what the token was sealed with. Handing the raw `?sort=` here would fail to
// authenticate every cursor issued under a kind's default.
func (c *Cursors) Decode(scope string, sort []domain.SortKey, token string) (store.Cursor, error) {
	if token == "" {
		return store.Cursor{}, store.ErrInvalidCursor
	}

	return c.codec.Decode(scope, sort, token)
}
