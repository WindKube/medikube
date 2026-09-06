package api

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	domainsearch "medikube/internal/domain/search"
	"medikube/internal/httproute"
	searchsvc "medikube/internal/service/search"
	"medikube/internal/web"
)

// OpSearch is contracts/search.md's one operation.
const OpSearch = "search"

// The query parameters this operation reads. `q` deliberately has no
// exported symbol beyond ParamQ — it is never logged and never echoed back
// whole, only as ParamQPresent in the response (FR-075, research D-12).
const (
	ParamQ     = "q"
	ParamKinds = "kinds"
	ParamMatch = "match"
	MatchAny   = "any"
)

// ErrNoSearch is a build whose search read side was never resolved.
var ErrNoSearch = errors.New("api: the search operation was wired without a way to resolve the search service")

// SearchResolve is the read side's own resolver, mirroring Resolve for the
// same reason: the registry is not complete — and its registered kinds are
// not known — until migrations have run (Resolve's own doc explains why).
type SearchResolve func() (*searchsvc.Service, []kind.Kind, error)

// SearchHandlers is the search operation's contribution to the route table.
func SearchHandlers(resolve SearchResolve) (httproute.Handlers, error) {
	if resolve == nil {
		return nil, ErrNoSearch
	}

	handlers := &searchHandlers{resolve: resolve}

	return httproute.Handlers{
		OpSearch: web.WithActor(handlers.search),
	}, nil
}

type searchHandlers struct {
	resolve SearchResolve
}

func (h *searchHandlers) search(e *core.RequestEvent, actor access.Actor) error {
	svc, registered, err := h.resolve()
	if err != nil {
		return err
	}

	patientID, err := requiredPatient(e)
	if err != nil {
		return err
	}

	values := e.Request.URL.Query()

	query, err := domainsearch.NewQuery(values.Get(ParamQ), patientID, commaList(values.Get(ParamKinds)), registered)
	if err != nil {
		return err
	}

	limit, err := SearchLimit(values.Get(web.ParamLimit))
	if err != nil {
		return err
	}

	cursors, err := ParseSearchCursors(values.Get(web.ParamCursor), registered)
	if err != nil {
		return err
	}

	result, err := svc.Search(e.Request.Context(), actor, query, limit, cursors)
	if err != nil {
		return web.OwnerScoped(err)
	}

	return web.WriteJSON(e, http.StatusOK, searchResponse(query, result))
}

// SearchLimit reads `?limit=`, the shared 1..100 bound (research D-25), for a
// caller that needs it outside the operation itself (the search page).
func SearchLimit(raw string) (int, error) {
	if raw == "" {
		return web.DefaultLimit, nil
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n < web.MinLimit || n > web.MaxLimit {
		var invalid domain.ValidationError
		invalid.Addf(web.ParamLimit, domain.CodeInvalidValue, "a page holds between %d and %d entries", web.MinLimit, web.MaxLimit)

		return 0, invalid.OrNil()
	}

	return n, nil
}

// ParseSearchCursors reads `?cursor=`: a csv of `kind:token` pairs, one per
// group (contracts/search.md §1) — `kind` here is the enum spelling every
// group's own "kind" member carries, which is what a client copies back
// unchanged from the page it is continuing. A malformed pair, or one naming
// a kind this instance does not serve search over, is 400 bad_request
// (contracts/search.md §3) and never echoes what it could not parse. Exported
// for the search page handler, which parses `?cursor=` the same way.
func ParseSearchCursors(raw string, registered []kind.Kind) (searchsvc.Cursors, error) {
	if raw == "" {
		return nil, nil
	}

	cursors := make(searchsvc.Cursors, len(registered))

	for _, pair := range strings.Split(raw, ",") {
		name, token, found := strings.Cut(pair, ":")
		if !found || name == "" || token == "" {
			return nil, malformedCursor()
		}

		k, ok := kind.FromEnum(name)
		if !ok || !slices.Contains(registered, k) {
			return nil, malformedCursor()
		}

		cursors[k] = token
	}

	return cursors, nil
}

func malformedCursor() error {
	return fmt.Errorf("%w: the cursor is not a well-formed kind:token list", domain.ErrBadRequest)
}

// searchResponse builds contracts/search.md §2's envelope. criteria echoes
// q_present rather than q, and never the term itself (FR-075).
func searchResponse(query domainsearch.Query, result searchsvc.Result) SearchResponse {
	groups := make([]SearchGroup, 0, len(result.Groups))

	for _, group := range result.Groups {
		groups = append(groups, SearchGroup{
			Kind:       group.Kind.Enum(),
			Items:      searchItems(group.Items),
			NextCursor: group.NextCursor,
			HasMore:    group.HasMore,
		})
	}

	kinds := make([]string, 0, len(query.Kinds))
	for _, k := range query.Kinds {
		kinds = append(kinds, k.Enum())
	}

	return SearchResponse{
		Groups: groups,
		Criteria: SearchCriteria{
			QPresent: query.Term != "",
			Kinds:    kinds,
			Tags:     []string{},
			Match:    MatchAny,
		},
		EmptyReason: emptyReasonPtr(result.EmptyReason),
	}
}

func searchItems(hits []searchsvc.Hit) []SearchItem {
	items := make([]SearchItem, 0, len(hits))

	for _, hit := range hits {
		items = append(items, SearchItem{
			ID:         hit.RecordID,
			Kind:       hit.Kind.Enum(),
			Title:      hit.Title,
			Snippet:    nil,
			OccurredOn: wireDate(hit.OccurredOn),
			Tags:       nonNil(hit.TagIDs),
		})
	}

	return items
}

func emptyReasonPtr(reason string) *string {
	if reason == "" {
		return nil
	}

	return &reason
}

// SearchItem is one matched row (contracts/search.md §2). snippet is always
// null in this phase: MediKube does not highlight matches or claim relevance
// ranking (FR-073), and the field exists only so phase 004 can populate it
// without a wire change.
type SearchItem struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Title      string   `json:"title"`
	Snippet    *string  `json:"snippet"`
	OccurredOn *string  `json:"occurred_on"`
	Tags       []string `json:"tags"`
}

// SearchGroup is one kind's page of a grouped search result.
type SearchGroup struct {
	Kind       string       `json:"kind"`
	Items      []SearchItem `json:"items"`
	NextCursor *string      `json:"next_cursor"`
	HasMore    bool         `json:"has_more"`
}

// SearchCriteria echoes the narrowing in force, for the page's removable
// chips. It carries q_present, never q (FR-075, research D-12).
type SearchCriteria struct {
	QPresent bool     `json:"q_present"`
	Kinds    []string `json:"kinds"`
	Tags     []string `json:"tags"`
	Match    string   `json:"match"`
}

// SearchResponse is contracts/search.md §2's whole envelope.
type SearchResponse struct {
	Groups      []SearchGroup  `json:"groups"`
	Criteria    SearchCriteria `json:"criteria"`
	EmptyReason *string        `json:"empty_reason"`
}
