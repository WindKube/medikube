package page

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/i18n"
	svctimeline "medikube/internal/service/timeline"
	"medikube/internal/web"
	"medikube/internal/web/api"
	"medikube/internal/web/views/shared"
	"medikube/internal/web/views/shell"
	viewtimeline "medikube/internal/web/views/timeline"
)

// OpTimelinePage is contracts/pages.md §3's /timeline.
const OpTimelinePage = "timelinePage"

const timelineTitle = "Timeline"

// TimelineResolve resolves the timeline service, the same lazy pattern
// api.Resolve uses: the store it reads through needs a cursor codec keyed
// from a secret migrations have only just created.
type TimelineResolve func() (*svctimeline.Service, error)

// ErrNoTimeline is a build whose /timeline was wired with nothing to resolve.
var ErrNoTimeline = fmt.Errorf("page: /timeline was wired with no way to resolve the timeline service")

// TimelineHandlers is /timeline's contribution to the route table.
func TimelineHandlers(resolve TimelineResolve, patients api.PatientResolve) (httproute.Handlers, error) {
	if resolve == nil {
		return nil, ErrNoTimeline
	}

	if patients == nil {
		return nil, api.ErrNoPatients
	}

	links, err := newTimelineLinks()
	if err != nil {
		return nil, err
	}

	pages := &timelinePages{resolve: resolve, patients: patients, links: links}

	return httproute.Handlers{
		OpTimelinePage: web.WithActor(pages.list),
	}, nil
}

type timelinePages struct {
	resolve  TimelineResolve
	patients api.PatientResolve
	links    timelineLinks
}

// timelineParams are US9's own narrowing parameters, none of which
// contracts/records-clinical.md's kind-scoped list publishes: they narrow
// the cross-kind view itself (FR-077).
const (
	paramKind = "kind"
	paramTags = "tags"
	paramFrom = "from"
	paramTo   = "to"
)

func (p *timelinePages) list(e *core.RequestEvent, actor access.Actor) error {
	if !actor.Authenticated() {
		return fmt.Errorf("page: the page needs a session: %w", domain.ErrForbidden)
	}

	query := e.Request.URL.Query()
	patientID := query.Get(api.ParamPatient)

	if patientID == "" {
		return p.render(e, actor, viewtimeline.Props{ChoosePerson: true})
	}

	service, err := p.resolve()
	if err != nil {
		return err
	}

	kinds, err := parseKinds(query.Get(paramKind))
	if err != nil {
		return err
	}

	tags := splitCSV(query.Get(paramTags))
	from, to := query.Get(paramFrom), query.Get(paramTo)

	params, err := web.ListQuery(e, nil)
	if err != nil {
		return err
	}

	listing, err := service.List(e.Request.Context(), actor, svctimeline.Query{
		PatientID: patientID,
		Kinds:     kinds,
		Tags:      tags,
		From:      from,
		To:        to,
		Limit:     params.Limit,
		Cursor:    params.Cursor,
	})
	if err != nil {
		return web.OwnerScoped(err)
	}

	props := viewtimeline.Props{
		Criteria: criteriaOf(e, kinds, tags, from, to),
		NextHref: nextTimelineHref(e, listing),
	}

	if len(listing.Items) == 0 {
		empty := shared.NothingRecorded("timeline-empty", "records")
		if len(props.Criteria.Chips) > 0 {
			empty = shared.NothingMatches("timeline-empty", "records", clearTimelineNarrowing(e))
		}

		props.Empty = &empty
	} else {
		props.Groups = groupEntries(listing.Items)
	}

	return p.render(e, actor, props)
}

func (p *timelinePages) render(e *core.RequestEvent, actor access.Actor, props viewtimeline.Props) error {
	switcher, err := patientSwitcherProps(e.Request.Context(), actor, p.patients)
	if err != nil {
		return err
	}

	return RenderPage(e, http.StatusOK, timelineTitle,
		NavState{SignedIn: true, Nav: p.links.nav(e.Request.Context(), e.Request.URL.Path), Switcher: switcher},
		viewtimeline.Timeline(props))
}

func groupEntries(items []svctimeline.Entry) []viewtimeline.Group {
	groups := make([]viewtimeline.Group, 0)
	byLabel := make(map[string]int, len(items))

	undatedLabel := "Date not recorded"

	for _, item := range items {
		label := undatedLabel
		if item.OccurredOn != nil {
			label = *item.OccurredOn
		}

		entry := viewtimeline.EntryView{
			ID:    "timeline-entry-" + item.ID,
			Kind:  strings.ReplaceAll(item.Kind.Segment(), "-", " "),
			Title: item.Title,
			Date:  label,
			Href:  "/" + item.Kind.Segment() + "/" + item.ID,
		}

		if index, exists := byLabel[label]; exists {
			groups[index].Entries = append(groups[index].Entries, entry)

			continue
		}

		byLabel[label] = len(groups)
		groups = append(groups, viewtimeline.Group{Label: label, Entries: []viewtimeline.EntryView{entry}})
	}

	return groups
}

func parseKinds(raw string) ([]kind.Kind, error) {
	segments := splitCSV(raw)
	if len(segments) == 0 {
		return nil, nil
	}

	var invalid domain.ValidationError

	kinds := make([]kind.Kind, 0, len(segments))

	for _, segment := range segments {
		k, declared := kind.FromSegment(segment)
		if !declared {
			invalid.Add(paramKind, domain.CodeInvalidValue, "the kind is not one this instance serves")

			continue
		}

		kinds = append(kinds, k)
	}

	if err := invalid.OrNil(); err != nil {
		return nil, err
	}

	return kinds, nil
}

// criteriaOf is FR-071/FR-073's removable chips: one per narrowing in force,
// each an expression that reloads the page with that one parameter gone.
func criteriaOf(e *core.RequestEvent, kinds []kind.Kind, tags []string, from, to string) shared.CriteriaProps {
	chips := make([]shared.CriteriaChip, 0, len(kinds)+len(tags)+2)

	segments := make([]string, 0, len(kinds))
	for _, k := range kinds {
		segments = append(segments, k.Segment())
	}

	for i, k := range kinds {
		chips = append(chips, shared.CriteriaChip{
			ID:      "timeline-criteria-kind-" + k.Enum(),
			Label:   strings.ReplaceAll(k.Segment(), "-", " "),
			ClearOn: clearParamExpression(e, paramKind, segments, i),
		})
	}

	for i, tag := range tags {
		chips = append(chips, shared.CriteriaChip{
			ID:      "timeline-criteria-tag-" + tag,
			Label:   "Tag: " + tag,
			ClearOn: clearParamExpression(e, paramTags, tags, i),
		})
	}

	if from != "" {
		chips = append(chips, shared.CriteriaChip{
			ID: "timeline-criteria-from", Label: "From " + from, ClearOn: clearOneParam(e, paramFrom),
		})
	}

	if to != "" {
		chips = append(chips, shared.CriteriaChip{
			ID: "timeline-criteria-to", Label: "To " + to, ClearOn: clearOneParam(e, paramTo),
		})
	}

	return shared.CriteriaProps{ID: "timeline-criteria", Chips: chips}
}

// clearParamExpression removes one value out of a csv parameter's list,
// leaving the rest of the narrowing in force.
func clearParamExpression(e *core.RequestEvent, name string, values []string, remove int) string {
	remaining := make([]string, 0, len(values)-1)
	for i, v := range values {
		if i != remove {
			remaining = append(remaining, v)
		}
	}

	next := *e.Request.URL
	query := next.Query()

	if len(remaining) == 0 {
		query.Del(name)
	} else {
		query.Set(name, strings.Join(remaining, ","))
	}

	next.RawQuery = query.Encode()

	return "@get(" + quote(next.RequestURI()) + ")"
}

func clearOneParam(e *core.RequestEvent, name string) string {
	next := *e.Request.URL
	query := next.Query()
	query.Del(name)
	next.RawQuery = query.Encode()

	return "@get(" + quote(next.RequestURI()) + ")"
}

func clearTimelineNarrowing(e *core.RequestEvent) string {
	next := *e.Request.URL
	query := next.Query()

	for _, name := range []string{paramKind, paramTags, paramFrom, paramTo} {
		query.Del(name)
	}

	next.RawQuery = query.Encode()

	return "@get(" + quote(next.RequestURI()) + ")"
}

func nextTimelineHref(e *core.RequestEvent, listing domain.Page[svctimeline.Entry]) string {
	if listing.NextCursor == nil {
		return ""
	}

	next := *e.Request.URL
	query := next.Query()
	query.Set(web.ParamCursor, *listing.NextCursor)
	next.RawQuery = query.Encode()

	return next.RequestURI()
}

type timelineLinks struct {
	listPage        string
	settingsPage    string
	medicationsPage string
}

func newTimelineLinks() (timelineLinks, error) {
	paths, err := routePaths(map[string]string{
		OpTimelinePage:       "",
		OpSettingsPage:       "",
		OpMedicationListPage: "",
	})
	if err != nil {
		return timelineLinks{}, err
	}

	return timelineLinks{
		listPage:        paths[OpTimelinePage],
		settingsPage:    paths[OpSettingsPage],
		medicationsPage: paths[OpMedicationListPage],
	}, nil
}

// nav's timeline label is the one phrase this foundation phase routes
// through the catalogue (i18n.T(ctx, "nav.timeline")); the other two labels
// are unchanged consts until US1's extraction (T020) reaches them.
func (l timelineLinks) nav(ctx context.Context, current string) []shell.NavLink {
	return []shell.NavLink{
		{Label: medicationListTitle, Href: l.medicationsPage, Current: strings.HasPrefix(current, l.medicationsPage)},
		{Label: i18n.T(ctx, "nav.timeline"), Href: l.listPage, Current: strings.HasPrefix(current, l.listPage)},
		{Label: settingsTitle, Href: l.settingsPage, Current: current == l.settingsPage},
	}
}
