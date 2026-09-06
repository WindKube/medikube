package page

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	domainsearch "medikube/internal/domain/search"
	"medikube/internal/httproute"
	"medikube/internal/i18n"
	searchsvc "medikube/internal/service/search"
	"medikube/internal/web"
	"medikube/internal/web/api"
	views "medikube/internal/web/views/search"
	"medikube/internal/web/views/shell"
)

// OpSearchPage is contracts/pages.md P: /search, landmark `search`.
const OpSearchPage = "searchPage"

const searchPageTitleID = "page.searchPage.title"

// SearchHandlers is the search page's contribution to the route table.
func SearchHandlers(resolve api.SearchResolve, patients api.PatientResolve) (httproute.Handlers, error) {
	if resolve == nil {
		return nil, api.ErrNoSearch
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	links, err := newSearchLinks()
	if err != nil {
		return nil, err
	}

	pages := &searchPages{resolve: resolve, patients: patients, links: links}

	return httproute.Handlers{
		OpSearchPage: web.WithActor(pages.show),
	}, nil
}

type searchPages struct {
	resolve  api.SearchResolve
	patients api.PatientResolve
	links    searchLinks
}

// show renders /search. Absent `?patient=` and absent `?q=` are each their
// own explicit state (contracts/pages.md §3, §5) — neither falls back to
// anything and neither calls the search service at all.
func (p *searchPages) show(e *core.RequestEvent, actor access.Actor) error {
	if !actor.Authenticated() {
		return fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	web.Localize(e)

	values := e.Request.URL.Query()
	patientID := values.Get(api.ParamPatient)
	term := values.Get(api.ParamQ)

	props := views.Props{FormAction: p.links.searchPage, PatientID: patientID, Query: term}

	if patientID == "" {
		props.NoPatient = true

		return p.render(e, actor, props)
	}

	if term == "" {
		props.NoTerm = true

		return p.render(e, actor, props)
	}

	svc, registered, err := p.resolve()
	if err != nil {
		return err
	}

	query, err := domainsearch.NewQuery(term, patientID,
		splitCSV(values.Get(api.ParamKinds)), splitCSV(values.Get(api.ParamTags)), values.Get(api.ParamMatch), registered)
	if err != nil {
		return err
	}

	limit, err := api.SearchLimit(values.Get(web.ParamLimit))
	if err != nil {
		return err
	}

	cursors, err := api.ParseSearchCursors(values.Get(web.ParamCursor), registered)
	if err != nil {
		return err
	}

	result, err := svc.Search(e.Request.Context(), actor, query, limit, cursors)
	if err != nil {
		return web.OwnerScoped(err)
	}

	props.EmptyReason = result.EmptyReason
	props.Chips = p.chips(e, values)
	props.ClearHref = p.clearHref(e)
	props.Groups = p.groups(e, cursors, result)

	return p.render(e, actor, props)
}

// chips is one removable chip per kind and per tag the caller explicitly
// narrowed to — an unnarrowed search (every registered kind, no tags) offers
// nothing to remove.
func (p *searchPages) chips(e *core.RequestEvent, values url.Values) []views.Chip {
	chips := p.kindChips(e, values)

	return append(chips, p.tagChips(e, values)...)
}

func (p *searchPages) kindChips(e *core.RequestEvent, values url.Values) []views.Chip {
	segments := splitCSV(values.Get(api.ParamKinds))
	if len(segments) == 0 {
		return nil
	}

	ctx := e.Request.Context()

	chips := make([]views.Chip, 0, len(segments))

	for i, segment := range segments {
		k, ok := kind.FromSegment(segment)
		if !ok {
			continue
		}

		remaining := make([]string, 0, len(segments)-1)
		remaining = append(remaining, segments[:i]...)
		remaining = append(remaining, segments[i+1:]...)

		chips = append(chips, views.Chip{Label: kindNoun(ctx, k), Href: withQuery(e, func(q url.Values) {
			if len(remaining) == 0 {
				q.Del(api.ParamKinds)
			} else {
				q.Set(api.ParamKinds, strings.Join(remaining, ","))
			}

			q.Del(web.ParamCursor)
		})})
	}

	return chips
}

// tagChips is kindChips's twin for `?tags=` (T164-T177 follow-up): one
// removable chip per narrowing tag id. It has no tag name to render — this
// page resolves no tag service of its own — so the chip names the id, the
// same way a search result's own item tags do (views.Item.Tags).
func (p *searchPages) tagChips(e *core.RequestEvent, values url.Values) []views.Chip {
	ids := splitCSV(values.Get(api.ParamTags))
	if len(ids) == 0 {
		return nil
	}

	chips := make([]views.Chip, 0, len(ids))

	for i, id := range ids {
		remaining := make([]string, 0, len(ids)-1)
		remaining = append(remaining, ids[:i]...)
		remaining = append(remaining, ids[i+1:]...)

		chips = append(chips, views.Chip{Label: id, Href: withQuery(e, func(q url.Values) {
			if len(remaining) == 0 {
				q.Del(api.ParamTags)
				q.Del(api.ParamMatch)
			} else {
				q.Set(api.ParamTags, strings.Join(remaining, ","))
			}

			q.Del(web.ParamCursor)
		})})
	}

	return chips
}

// clearHref removes every narrowing this page offers (kinds, tags, match,
// cursor) while keeping the person and the term (US8 scenario 2).
func (p *searchPages) clearHref(e *core.RequestEvent) string {
	return withQuery(e, func(q url.Values) {
		q.Del(api.ParamKinds)
		q.Del(api.ParamTags)
		q.Del(api.ParamMatch)
		q.Del(web.ParamCursor)
	})
}

// groups renders each matched kind's page and its own "load more", which
// continues that one kind from the boundary the search just minted while
// leaving every other group's incoming cursor untouched.
func (p *searchPages) groups(e *core.RequestEvent, incoming searchsvc.Cursors, result searchsvc.Result) []views.Group {
	ctx := e.Request.Context()

	groups := make([]views.Group, 0, len(result.Groups))

	for _, group := range result.Groups {
		items := make([]views.Item, 0, len(group.Items))

		for _, hit := range group.Items {
			items = append(items, views.Item{
				Title:      hit.Title,
				OccurredOn: hit.OccurredOn.String(),
				Href:       "/" + hit.Kind.Segment() + "/" + hit.RecordID,
				Tags:       hit.TagIDs,
			})
		}

		var loadMore string

		if group.NextCursor != nil {
			loadMore = withQuery(e, func(q url.Values) {
				q.Set(web.ParamCursor, encodeCursors(incoming, group.Kind, *group.NextCursor))
			})
		}

		groups = append(groups, views.Group{
			Kind: group.Kind.Enum(), Label: kindNoun(ctx, group.Kind), Items: items, LoadMoreHref: loadMore,
		})
	}

	return groups
}

// encodeCursors renders the `?cursor=` value api.ParseSearchCursors reads
// back: every incoming cursor, with one kind's replaced (or added).
func encodeCursors(incoming searchsvc.Cursors, k kind.Kind, token string) string {
	merged := make(searchsvc.Cursors, len(incoming)+1)
	for existingKind, existingToken := range incoming {
		merged[existingKind] = existingToken
	}

	merged[k] = token

	kinds := make([]kind.Kind, 0, len(merged))
	for existingKind := range merged {
		kinds = append(kinds, existingKind)
	}

	// Sorted so the rendered link is byte-stable across requests carrying
	// the same cursors, which is what makes it testable.
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })

	pairs := make([]string, 0, len(merged))
	for _, existingKind := range kinds {
		pairs = append(pairs, existingKind.Enum()+":"+merged[existingKind])
	}

	return strings.Join(pairs, ",")
}

// withQuery copies the current request's URL, applies one mutation to its
// query and returns the resulting path plus query — nextPageHref's own
// pattern (medications.go), generalised to more than one parameter at a
// time.
func withQuery(e *core.RequestEvent, mutate func(url.Values)) string {
	next := *e.Request.URL
	q := next.Query()
	mutate(q)
	next.RawQuery = q.Encode()

	return next.RequestURI()
}

func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}

	return strings.Split(raw, ",")
}

func (p *searchPages) render(e *core.RequestEvent, actor access.Actor, props views.Props) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	web.Localize(e)

	return RenderPage(e, http.StatusOK, i18n.T(e.Request.Context(), searchPageTitleID),
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.URL.Path), Switcher: switcher}, views.Results(props))
}

type searchLinks struct {
	searchPage      string
	settingsPage    string
	medicationsPage string
}

func newSearchLinks() (searchLinks, error) {
	paths, err := routePaths(map[string]string{
		OpSearchPage:         "",
		OpSettingsPage:       "",
		OpMedicationListPage: "",
	})
	if err != nil {
		return searchLinks{}, err
	}

	return searchLinks{
		searchPage:      paths[OpSearchPage],
		settingsPage:    paths[OpSettingsPage],
		medicationsPage: paths[OpMedicationListPage],
	}, nil
}

func (l searchLinks) nav(current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: medicationListTitle, Href: l.medicationsPage, Current: strings.HasPrefix(current, l.medicationsPage)},
		{Label: settingsTitle, Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
